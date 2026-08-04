package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/config"
	"ob-data-orch/internal/controlplane"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/featuregate"
	"ob-data-orch/internal/localmvp"
	"ob-data-orch/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("control plane stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	showVersion := flag.Bool("version", false, "print version and exit")
	localMVP := flag.Bool("local-mvp", false, "run the loopback-only local TLS MVP")
	flag.Parse()
	if *showVersion {
		info := buildinfo.Current()
		fmt.Printf("ob-data-orch control-plane %s (%s, %s)\n", info.Version, info.Commit, info.BuildTime)
		return nil
	}

	realExecutionEnabled, err := featuregate.LoadRealExecutionEnabled(os.LookupEnv)
	if err != nil {
		return err
	}
	if realExecutionEnabled && !*localMVP {
		return errors.New("real execution is currently limited to the local MVP Windows integration")
	}
	appConfig, err := config.LoadControlPlane(os.LookupEnv)
	if err != nil {
		return err
	}

	var handler http.Handler = controlplane.NewHandler(buildinfo.Current())
	var database *store.Store
	agentJDBCConnectionTestEnabled := false
	if *localMVP {
		if err := validateLocalMVPConfiguration(appConfig); err != nil {
			return err
		}
		agentJDBCConnectionTestEnabled, err = config.LoadAgentJDBCConnectionTestEnabled(os.LookupEnv)
		if err != nil {
			return errors.New("local MVP JDBC connection test configuration is invalid")
		}
		if err := os.MkdirAll("var", 0o700); err != nil {
			return err
		}
		database, err = store.Open(context.Background(), "var/local-mvp.db")
		if err != nil {
			return err
		}
		defer database.Close()
		if err := localmvp.PrepareStore(context.Background(), database); err != nil {
			return fmt.Errorf("初始化本机 MVP 身份投影: %w", err)
		}
		key, keyErr := credential.LoadOrCreateRootKey("var/local-mvp-root-key.json", "local-mvp-root-v1")
		if keyErr != nil {
			return keyErr
		}
		defer credential.Zero(key)
		keyring, keyErr := credential.NewKeyring(map[string][]byte{"local-mvp-root-v1": key})
		if keyErr != nil {
			return keyErr
		}
		dependencies, dependencyErr := localmvp.Dependencies(database, keyring, agentJDBCConnectionTestEnabled, realExecutionEnabled)
		if dependencyErr != nil {
			return dependencyErr
		}
		defer func() {
			shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := dependencies.PersistentLogs.Shutdown(shutdownContext); err != nil {
				slog.Error("seal persistent logs on shutdown failed", "error", err)
			}
		}()
		handler = controlplane.NewHandlerWithDependencies(buildinfo.Current(), dependencies)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	server := &http.Server{
		Addr:              appConfig.ListenAddress,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	transport, serve := controlPlaneServeFunction(server, appConfig)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("control plane listening",
			"address", appConfig.ListenAddress,
			"transport", transport,
			"stage", map[bool]string{true: "G3_LOCAL_MVP", false: "G2"}[realExecutionEnabled],
			"realExecutionEnabled", realExecutionEnabled,
			"agentJDBCConnectionTestEnabled", agentJDBCConnectionTestEnabled,
		)
		serverErrors <- serve()
	}()

	select {
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownContext)
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// validateLocalMVPConfiguration 将本机 MVP 限制为回环 TLS 联调入口，避免测试身份和机器凭据暴露到非本机网络。
func validateLocalMVPConfiguration(appConfig config.ControlPlane) error {
	host, _, err := net.SplitHostPort(appConfig.ListenAddress)
	if err != nil {
		return errors.New("local MVP must use a valid loopback listen address")
	}
	address := net.ParseIP(host)
	if address == nil || !address.IsLoopback() {
		return errors.New("local MVP must bind a loopback address")
	}
	if appConfig.TLSCertFile == "" || appConfig.TLSKeyFile == "" {
		return errors.New("local MVP requires TLS certificate and key files")
	}
	return nil
}

// controlPlaneServeFunction 根据已验证的 TLS 配置选择固定监听方式，不在日志中暴露证书或私钥路径。
func controlPlaneServeFunction(server *http.Server, appConfig config.ControlPlane) (string, func() error) {
	if appConfig.TLSCertFile == "" {
		return "http", server.ListenAndServe
	}
	return "https", func() error {
		return server.ListenAndServeTLS(appConfig.TLSCertFile, appConfig.TLSKeyFile)
	}
}
