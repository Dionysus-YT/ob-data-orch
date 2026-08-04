package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"time"

	"ob-data-orch/internal/agentconnectiontest"
	"ob-data-orch/internal/agentenvironment"
	"ob-data-orch/internal/agentlocalpreflight"
	"ob-data-orch/internal/agenttelemetry"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/config"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/featuregate"
	"ob-data-orch/internal/identifier"
)

func main() {
	if err := run(); err != nil {
		slog.Error("agent stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return runWithContext(ctx, os.Args[1:], os.Stdin, os.Stdout, os.LookupEnv)
}

func runWithContext(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer, lookupEnv func(string) (string, bool)) error {
	flags := flag.NewFlagSet("agent", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	showVersion := flags.Bool("version", false, "print version and exit")
	checkRuntime := flags.Bool("check-runtime", false, "verify local Java and OBDUMPER runtime")
	register := flags.Bool("register", false, "register with a one-time code from standard input")
	enroll := flags.Bool("enroll", false, "store protected enrollment state from standard input")
	nodeID := flags.String("node-id", "", "execution node identifier for enrollment")
	enrollmentID := flags.String("enrollment-id", "", "one-time enrollment identifier")
	once := flags.Bool("once", false, "send at most one enrollment attempt, heartbeat, and G2 connection test")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("agent command arguments are invalid")
	}
	if *showVersion {
		info := buildinfo.Current()
		_, _ = fmt.Fprintf(stdout, "ob-data-orch agent %s (%s, %s)\n", info.Version, info.Commit, info.BuildTime)
		return nil
	}
	if *register && *enroll {
		return errors.New("agent register and enroll modes are mutually exclusive")
	}
	if !*enroll && !*register && (*nodeID != "" || *enrollmentID != "") {
		return errors.New("agent enrollment arguments require -enroll")
	}
	if *register && (*nodeID != "" || *enrollmentID != "") {
		return errors.New("agent register mode does not accept enrollment identifiers")
	}
	var err error
	var stateDirectory string
	var registrationControlPlane config.AgentControlPlane
	if *register {
		registrationControlPlane, err = loadSimpleRegistrationControlPlane(lookupEnv)
		stateDirectory = registrationControlPlane.StateDirectory
	} else {
		stateDirectory, err = loadAgentStateDirectory(lookupEnv)
	}
	if err != nil {
		return errors.New("agent identity configuration is invalid")
	}
	stateStore, err := agentwire.OpenStateStore(stateDirectory)
	if err != nil {
		return errors.New("agent identity configuration is invalid")
	}
	operatingSystem, architecture, err := agentPlatform()
	if err != nil {
		return err
	}
	if *checkRuntime {
		runtimeConfig, platform, runtimeErr := agentRuntimeFromState(stateStore, lookupEnv, operatingSystem, architecture)
		if runtimeErr != nil {
			return errors.New("agent runtime configuration is unavailable")
		}
		if err := (agentlocalpreflight.ToolRuntimeValidator{JavaPath: runtimeConfig.JavaPath, ToolHome: runtimeConfig.ToolHome, Environment: runtimeConfig.Environment, TargetPlatform: platform}).Validate(ctx); err != nil {
			return errors.New("agent runtime verification failed")
		}
		_, _ = fmt.Fprintln(stdout, "agent runtime verified: Java, JDBC connector, and OBDUMPER launcher are ready")
		return nil
	}
	realExecutionEnabled, realExecutionConfigErr := featuregate.LoadRealExecutionEnabled(lookupEnv)
	if realExecutionConfigErr != nil {
		return realExecutionConfigErr
	}
	if realExecutionEnabled && (operatingSystem != "WINDOWS" || architecture != "AMD64") {
		return errors.New("real execution is currently limited to Windows AMD64 local integration")
	}
	jdbcConnectionTestEnabled, jdbcConfigErr := config.LoadAgentJDBCConnectionTestEnabled(lookupEnv)
	if jdbcConfigErr != nil {
		return errors.New("agent JDBC connection test configuration is invalid")
	}
	exportPreflightEnabled, preflightConfigErr := config.LoadAgentExportPreflightEnabled(lookupEnv)
	if preflightConfigErr != nil {
		return errors.New("agent export preflight configuration is invalid")
	}
	if *enroll {
		if err := prepareEnrollment(stateStore, *nodeID, *enrollmentID, stdin, lookupEnv); err != nil {
			return err
		}
	}
	if *register {
		_, _ = fmt.Fprintln(stdout, "请粘贴一次性 Agent 注册码后按 Enter。")
		if err := prepareSimpleRegistration(stateStore, registrationControlPlane, stdin); err != nil {
			return err
		}
	}
	interval, err := config.LoadAgentHeartbeatInterval(lookupEnv)
	if err != nil {
		return errors.New("agent heartbeat configuration is invalid")
	}
	bootID, err := newBootID()
	if err != nil {
		return errors.New("agent boot identity is unavailable")
	}
	logger := slog.New(slog.NewJSONHandler(stdout, nil))
	slog.SetDefault(logger)
	logger.Info("agent started",
		"stage", map[bool]string{true: "G3_LOCAL_MVP", false: "G2"}[realExecutionEnabled],
		"networkEnabled", true,
		"realExecutionEnabled", realExecutionEnabled,
		"agentJDBCConnectionTestEnabled", jdbcConnectionTestEnabled,
		"agentExportPreflightEnabled", exportPreflightEnabled,
	)
	connectionTestWorker := &agentconnectiontest.Worker{
		Protocol: stateStore,
		Clock:    time.Now,
		BootID:   bootID,
	}
	if jdbcConnectionTestEnabled {
		connectionTestWorker.JDBCRunner = configuredJDBCConnectionTestRunner{
			stateStore: stateStore, lookupEnv: lookupEnv, operatingSystem: operatingSystem, architecture: architecture, bootID: bootID,
		}
	}
	environmentCheckWorker := &agentenvironment.Worker{
		Protocol: stateStore, Runtime: environmentRuntimeValidator(stateStore, lookupEnv, operatingSystem, architecture), Clock: time.Now, BootID: bootID,
	}
	var protocolWork connectionTestRunner = connectionTestWorker
	if exportPreflightEnabled {
		protocolWork = connectionAndPrecheckRunner{
			connectionTests: connectionTestWorker,
			prechecks: configuredExportPreflightRunner{
				stateStore: stateStore, lookupEnv: lookupEnv, operatingSystem: operatingSystem, architecture: architecture, bootID: bootID,
			},
		}
	}
	if realExecutionEnabled {
		if !exportPreflightEnabled {
			return errors.New("real execution requires the fixed export preflight worker")
		}
		protocolWork = connectionAndPrecheckRunner{
			connectionTests: connectionTestWorker,
			prechecks: configuredExportPreflightRunner{
				stateStore: stateStore, lookupEnv: lookupEnv, operatingSystem: operatingSystem, architecture: architecture, bootID: bootID,
			},
			executions: &asynchronousExportExecutionRunner{delegate: configuredExportExecutionRunner{
				stateStore: stateStore, lookupEnv: lookupEnv, operatingSystem: operatingSystem, architecture: architecture, bootID: bootID,
			}, logger: logger},
		}
	}
	return runHeartbeatLoop(ctx, stateStore, protocolWork, bootID, operatingSystem, architecture, interval, *once, logger, environmentCheckWorker)
}

// environmentRuntimeValidator 只从首次关联后加密保存的本机配置构造固定运行时核验器。
// 配置缺失、平台漂移或本机复核失败都返回不可用，绝不回退到环境变量中的工具或 Java 路径。
func environmentRuntimeValidator(stateStore *agentwire.StateStore, lookupEnv func(string) (string, bool), operatingSystem, architecture string) agentlocalpreflight.RuntimeValidator {
	return agentlocalpreflight.RuntimeValidatorFunc(func(ctx context.Context) error {
		runtimeConfig, platform, err := agentRuntimeFromState(stateStore, lookupEnv, operatingSystem, architecture)
		if err != nil {
			return agentlocalpreflight.ErrToolRuntimeUnavailable
		}
		if err := (agentlocalpreflight.ToolRuntimeValidator{JavaPath: runtimeConfig.JavaPath, ToolHome: runtimeConfig.ToolHome, Environment: runtimeConfig.Environment, TargetPlatform: platform}).Validate(ctx); err != nil {
			return err
		}
		if _, err := agenttelemetry.NewCollector(stateStore).Sample(); err != nil {
			return agentlocalpreflight.ErrToolRuntimeInvalid
		}
		return nil
	})
}

// agentRuntimeFromState 仅使用首次关联写入加密状态的路径配置，并验证当前 Agent 平台与节点声明一致。
func agentRuntimeFromState(stateStore *agentwire.StateStore, lookupEnv func(string) (string, bool), operatingSystem, architecture string) (config.AgentRuntime, commandgen.Platform, error) {
	if stateStore == nil {
		return config.AgentRuntime{}, "", errors.New("agent runtime state is unavailable")
	}
	configuration, err := stateStore.RuntimeConfiguration()
	if err != nil {
		return config.AgentRuntime{}, "", err
	}
	platform, ok := localCommandPlatform(operatingSystem, architecture)
	if !ok || configuration.Platform != string(platform) {
		return config.AgentRuntime{}, "", errors.New("agent runtime platform does not match")
	}
	runtimeConfig, err := config.LoadAgentRuntimeFromLocalConfiguration(configuration.JavaPath, configuration.ToolHome, lookupEnv)
	if err != nil {
		return config.AgentRuntime{}, "", err
	}
	return runtimeConfig, platform, nil
}

func localCommandPlatform(operatingSystem, architecture string) (commandgen.Platform, bool) {
	switch operatingSystem + "/" + architecture {
	case "WINDOWS/AMD64":
		return commandgen.PlatformWindowsAMD64, true
	case "LINUX/AMD64":
		return commandgen.PlatformLinuxAMD64, true
	case "LINUX/ARM64":
		return commandgen.PlatformLinuxARM64, true
	default:
		return "", false
	}
}

func prepareEnrollment(stateStore *agentwire.StateStore, nodeID, enrollmentID string, stdin io.Reader, lookupEnv func(string) (string, bool)) error {
	controlPlane, err := config.LoadAgentControlPlane(lookupEnv)
	if err != nil {
		return errors.New("agent enrollment configuration is invalid")
	}
	if nodeID == "" || enrollmentID == "" {
		return errors.New("agent enrollment identifiers are required")
	}
	material, err := readEnrollmentMaterial(stdin)
	if err != nil {
		return errors.New("agent enrollment material is invalid")
	}
	defer credential.Zero(material)
	if err := stateStore.PrepareEnrollment(agentwire.EnrollmentConfig{
		ControlPlaneURL:    controlPlane.URL,
		CAFile:             controlPlane.CAFile,
		EnrollmentID:       enrollmentID,
		NodeID:             nodeID,
		EnrollmentMaterial: material,
	}); err != nil {
		return errors.New("agent enrollment state cannot be prepared")
	}
	return nil
}

func readEnrollmentMaterial(reader io.Reader) ([]byte, error) {
	if reader == nil {
		return nil, errors.New("enrollment input is unavailable")
	}
	content, err := io.ReadAll(io.LimitReader(reader, 4097))
	if err != nil || len(content) > 4096 {
		credential.Zero(content)
		return nil, errors.New("enrollment input is invalid")
	}
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) < 32 || len(trimmed) > 4096 {
		credential.Zero(content)
		return nil, errors.New("enrollment input is invalid")
	}
	result := append([]byte(nil), trimmed...)
	credential.Zero(content)
	return result, nil
}

// heartbeatProtocol 收窄主循环可调用的机器身份能力，便于验证心跳与 Worker 的严格先后顺序。
type heartbeatProtocol interface {
	EnsureEnrollment(context.Context) error
	SendHeartbeat(context.Context, agentwire.Heartbeat) (agentwire.HeartbeatResult, error)
}

// connectionTestRunner 只允许主循环触发一条固定连接测试，不暴露任意任务或任意命令能力。
type connectionTestRunner interface {
	RunNext(context.Context) (agentconnectiontest.Outcome, bool, error)
}

// environmentCheckRunner 只允许主循环在心跳明确返回检查标识后运行固定本机核验。
type environmentCheckRunner interface {
	Run(context.Context, string, int64) (bool, error)
}

const connectionTestPollInterval = 2 * time.Second

// runHeartbeatLoop 先在已确认心跳后立即领取一次固定连接测试，随后交给独立短周期领取。
// Worker 失败不会让长期心跳退出，但单次模式必须返回错误，便于安装和受控诊断发现协议问题。
func runHeartbeatLoop(ctx context.Context, heartbeat heartbeatProtocol, connectionTests connectionTestRunner, bootID, operatingSystem, architecture string, interval time.Duration, once bool, logger *slog.Logger, environmentChecks ...environmentCheckRunner) error {
	return runAgentProtocolLoop(ctx, heartbeat, connectionTests, bootID, operatingSystem, architecture, interval, connectionTestPollInterval, once, logger, environmentChecks...)
}

// runAgentProtocolLoop 将低频机器心跳与高响应的固定连接测试领取分开。
// 连接测试只会在至少一次心跳被控制面确认后领取，不能因此放宽 Agent 身份、固定能力或单条串行执行约束。
func runAgentProtocolLoop(ctx context.Context, heartbeat heartbeatProtocol, connectionTests connectionTestRunner, bootID, operatingSystem, architecture string, heartbeatInterval, connectionPollInterval time.Duration, once bool, logger *slog.Logger, environmentChecks ...environmentCheckRunner) error {
	if heartbeatInterval <= 0 || connectionPollInterval <= 0 {
		return errors.New("agent loop interval is invalid")
	}
	var telemetry *agenttelemetry.Collector
	if configuration, ok := heartbeat.(agenttelemetry.RuntimeConfigurationReader); ok {
		telemetry = agenttelemetry.NewCollector(configuration)
	}
	heartbeatAttempt := func() error {
		if heartbeat == nil || connectionTests == nil || logger == nil {
			return errors.New("agent loop configuration is invalid")
		}
		if err := heartbeat.EnsureEnrollment(ctx); err != nil {
			return err
		}
		now := time.Now().UTC()
		facts := agentwire.EnvironmentFacts{
			OperatingSystem: operatingSystem,
			Architecture:    architecture,
			AgentVersion:    buildinfo.Current().Version,
			ObservedAt:      now,
			CapacityTotal:   1,
			CapacityUsed:    0,
		}
		if telemetry != nil {
			if snapshot, telemetryErr := telemetry.Sample(); telemetryErr == nil {
				facts.CPUUsagePercent = snapshot.CPUUsagePercent
				facts.MemoryUsagePercent = snapshot.MemoryUsagePercent
				facts.RuntimeConfigurationDigest = snapshot.RuntimeConfigurationDigest
				facts.DataRootUsages = snapshot.DataRootUsages
			} else {
				logger.Warn("agent telemetry sampling failed")
			}
		}
		heartbeatResult, err := heartbeat.SendHeartbeat(ctx, agentwire.Heartbeat{
			BootID: bootID,
			SentAt: now,
			Facts:  facts,
		})
		if err != nil {
			return err
		}
		if len(environmentChecks) == 1 && environmentChecks[0] != nil {
			if _, err := environmentChecks[0].Run(ctx, heartbeatResult.EnvironmentCheckID, heartbeatResult.FactsRevision); err != nil {
				if once || errors.Is(err, agentenvironment.ErrInvalidConfiguration) {
					return err
				}
				logger.Warn("agent environment check worker attempt failed", "error", err)
			}
		}
		return nil
	}
	connectionTestAttempt := func() error {
		if _, _, err := connectionTests.RunNext(ctx); err != nil {
			if once || errors.Is(err, agentconnectiontest.ErrInvalidConfiguration) {
				return err
			}
			logger.Warn("agent connection test worker attempt failed", "error", err)
		}
		return nil
	}
	heartbeatConfirmed := false
	if err := heartbeatAttempt(); err != nil {
		if once || !errors.Is(err, agentwire.ErrControlPlaneUnavailable) {
			return err
		}
		logger.Warn("agent control plane is temporarily unavailable")
	} else {
		heartbeatConfirmed = true
		if err := connectionTestAttempt(); err != nil {
			return err
		}
	}
	if once {
		return nil
	}
	heartbeatTicker := time.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()
	connectionTestTicker := time.NewTicker(connectionPollInterval)
	defer connectionTestTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeatTicker.C:
			if err := heartbeatAttempt(); err != nil {
				if !errors.Is(err, agentwire.ErrControlPlaneUnavailable) {
					return err
				}
				logger.Warn("agent control plane is temporarily unavailable")
			} else {
				heartbeatConfirmed = true
			}
		case <-connectionTestTicker.C:
			if !heartbeatConfirmed {
				continue
			}
			if err := connectionTestAttempt(); err != nil {
				return err
			}
		}
	}
}

func newBootID() (string, error) {
	return identifier.NewUUIDV4()
}

func agentPlatform() (string, string, error) {
	var operatingSystem string
	switch runtime.GOOS {
	case "windows":
		operatingSystem = "WINDOWS"
	case "linux":
		operatingSystem = "LINUX"
	default:
		return "", "", errors.New("agent platform is not supported")
	}
	switch runtime.GOARCH {
	case "amd64":
		return operatingSystem, "AMD64", nil
	case "arm64":
		return operatingSystem, "ARM64", nil
	default:
		return "", "", errors.New("agent platform is not supported")
	}
}
