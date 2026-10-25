package agentjdbc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"sync"
	"time"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/directloadfiles"
	"ob-data-orch/internal/jdbcprobe"
	"ob-data-orch/internal/nodefacts"
)

// DirectLoadRunner 仅替换旁路固定只读 Java 入口，不接受命令或任意 SQL。
type DirectLoadRunner func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.DirectLoadRequest) (jdbcprobe.DirectLoadResult, error)

// DirectLoadRuntimeObserver 只返回固定本机工具声明和配置安全投影，不运行官方工具。
// 结果中的数据库、权限与表事实字段不被消费，不能替代 JDBC 观测。
type DirectLoadRuntimeObserver func(context.Context, agentpreflight.Request) (jdbcprobe.DirectLoadResult, error)

// DirectLoadProbe 独立处理本租约旁路检查；未装配的零值不解析秘密或连接网络。
// 缓存只保留固定安全结论，并绑定完整配置、租约、平台、根目录与输入清单。
type DirectLoadProbe struct {
	WorkspaceRoot string
	Runtime       jdbcprobe.Runtime
	Resolver      SecretResolver
	Run           DirectLoadRunner
	// DialContext 只接收业务连接 Host 加手工 RPCPort 派生的 TCP 目标。
	DialContext              func(context.Context, string, string) (net.Conn, error)
	ObserveRuntime           DirectLoadRuntimeObserver
	mu                       sync.Mutex
	bound                    string
	completed                bool
	results                  map[agentpreflight.CheckID]agentpreflight.Result
	configurationFingerprint string
}

var directLoadProbeChecks = [...]agentpreflight.CheckID{agentpreflight.CheckDatabaseConnectivity, "RPC_CONNECTIVITY", "DIRECT_LOAD_VERSION", "WRITE_ACCESS", "DIRECT_LOAD_STRUCTURE", "DIRECT_LOAD_CONFIG"}

// Probe 只允许 DATABASE_CONNECTIVITY 启动一次短时槽位与固定探针；后续检查不得重复解析。
func (p *DirectLoadProbe) Probe(ctx context.Context, check agentpreflight.CheckID, r agentpreflight.Request) (agentpreflight.Result, error) {
	unknown := directLoadUnknown(check)
	if p == nil || ctx == nil || ctx.Err() != nil || !validDirectLoadProbeRequest(r) {
		return unknown, nil
	}
	found := false
	for _, c := range directLoadProbeChecks {
		found = found || c == check
	}
	if !found {
		return unknown, nil
	}
	// 绑定包括租约代数和主体，避免同一预检查重领时沿用前租约的安全缓存。
	raw, err := json.Marshal(r)
	if err != nil {
		return unknown, nil
	}
	hash := sha256.Sum256(raw)
	key := hex.EncodeToString(hash[:])
	p.mu.Lock()
	if p.bound != "" {
		result, exists := p.results[check]
		valid := p.bound == key && p.completed && exists
		p.mu.Unlock()
		if valid {
			return result, nil
		}
		return unknown, nil
	}
	if check != agentpreflight.CheckDatabaseConnectivity {
		p.mu.Unlock()
		return unknown, nil
	}
	p.bound = key
	p.mu.Unlock()
	results, fingerprint := p.inspectDirectLoad(ctx, r)
	p.mu.Lock()
	p.results = results
	p.configurationFingerprint = fingerprint
	p.completed = true
	result := results[check]
	p.mu.Unlock()
	return result, nil
}

// DirectLoadConfigFingerprint 只返回同绑定已完成观测的安全投影；未启动、取消或跨租约读取均为空。
func (p *DirectLoadProbe) DirectLoadConfigFingerprint(ctx context.Context, r agentpreflight.Request) string {
	if p == nil || ctx == nil || ctx.Err() != nil || !validDirectLoadProbeRequest(r) {
		return ""
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(raw)
	key := hex.EncodeToString(hash[:])
	p.mu.Lock()
	defer p.mu.Unlock()
	if ctx.Err() != nil || !p.completed || p.bound != key || p.results["DIRECT_LOAD_CONFIG"].Status != agentpreflight.StatusPassed {
		return ""
	}
	return p.configurationFingerprint
}

func validDirectLoadProbeRequest(r agentpreflight.Request) bool {
	return r.DirectLoad != nil && r.NormalImport == nil && r.CheckKind == "DIRECT_LOAD_PREFLIGHT" && string(r.Capability) == r.CheckKind && !r.ActiveExecution && r.ContentKind == "DATA_ONLY" && r.OutputPath == "" && len(r.Objects) == 0 &&
		agentpreflight.ValidDirectLoadContext(r.DirectLoad, r.CheckKind, r.CompatibilityMode, r.TargetPlatform, r.AllowedRoots) && r.DirectLoad.NodeID == r.NodeID && r.Binding.NodeID == r.NodeID && r.Database == r.DirectLoad.Database &&
		r.PrecheckID != "" && r.PrecheckID == r.Binding.PrecheckID && r.AgentID != "" && r.LeaseID != "" && r.LeaseEpoch > 0 && r.Binding.DraftRevision > 0 && r.Binding.CredentialRevision > 0 && r.Binding.NodeFactsVersion > 0 && r.Binding.ConfigFingerprint != "" &&
		r.InputManifest != nil && directloadfiles.Valid(*r.InputManifest, *r.DirectLoad)
}

func directLoadUnknown(check agentpreflight.CheckID) agentpreflight.Result {
	return directLoadOutcome(check, agentpreflight.StatusUnknown)
}
func directLoadOutcome(check agentpreflight.CheckID, status agentpreflight.Status) agentpreflight.Result {
	code := "DIRECT_LOAD_UNAVAILABLE"
	if status == agentpreflight.StatusPassed {
		code = "DIRECT_LOAD_VERIFIED"
	}
	if status == agentpreflight.StatusFailed {
		code = "DIRECT_LOAD_REJECTED"
	}
	return agentpreflight.Result{Check: check, Status: status, EvidenceCode: code}
}

func (p *DirectLoadProbe) inspectDirectLoad(ctx context.Context, r agentpreflight.Request) (results map[agentpreflight.CheckID]agentpreflight.Result, fingerprint string) {
	results = map[agentpreflight.CheckID]agentpreflight.Result{}
	for _, check := range directLoadProbeChecks {
		results[check] = directLoadUnknown(check)
	}
	defer func() {
		if ctx.Err() != nil {
			fingerprint = ""
			for check := range results {
				results[check] = directLoadUnknown(check)
			}
		}
	}()
	// 本机固定读取不需要秘密，也不依赖数据库成功；Java 输出无权提供或覆盖本机事实。
	local := jdbcprobe.DirectLoadResult{}
	if p.ObserveRuntime != nil && ctx.Err() == nil {
		if value, e := p.ObserveRuntime(ctx, r); e == nil {
			local = value
		}
	}
	results["DIRECT_LOAD_CONFIG"] = directLoadConfigResult(local)
	if results["DIRECT_LOAD_CONFIG"].Status == agentpreflight.StatusPassed {
		fingerprint = local.DirectLoadConfig.Fingerprint
	}
	if p.Resolver == nil {
		return results, fingerprint
	}
	workspace, err := credential.CreateWorkspace(p.WorkspaceRoot, r.PrecheckID)
	if err != nil {
		return results, fingerprint
	}
	defer func() {
		if workspace.Cleanup() != nil || ctx.Err() != nil {
			fingerprint = ""
			for check := range results {
				results[check] = directLoadUnknown(check)
			}
		}
	}()
	connection, err := p.Resolver.ResolveDatabaseConnection(ctx, r.Binding)
	defer credential.Zero(connection.Username)
	defer credential.Zero(connection.Password)
	defer credential.Zero(connection.SysUsername)
	defer credential.Zero(connection.SysPassword)
	if err != nil || len(connection.SysUsername) != 0 || len(connection.SysPassword) != 0 || ctx.Err() != nil {
		return results, fingerprint
	}
	path, digest, err := jdbcprobe.Install(workspace)
	if err != nil {
		return results, fingerprint
	}
	runtime := p.Runtime
	runtime.ProbePath = path
	runtime.ProbeSHA256 = digest
	run := p.Run
	if run == nil {
		run = func(ctx context.Context, _ credential.Workspace, runtime jdbcprobe.Runtime, input jdbcprobe.DirectLoadRequest) (jdbcprobe.DirectLoadResult, error) {
			return jdbcprobe.TestDirectLoad(ctx, runtime, input)
		}
	}
	c := *r.DirectLoad
	input := jdbcprobe.DirectLoadRequest{Connection: jdbcprobe.Request{Host: connection.Host, Port: connection.Port, Username: connection.Username, Password: connection.Password}, CompatibilityMode: jdbcprobe.CompatibilityMode(r.CompatibilityMode), Database: c.Database, Table: c.Table, Scenario: c.Scenario, TruncateTable: c.TruncateTable}
	observed, err := run(ctx, workspace, runtime, input)
	if err != nil {
		if errors.Is(err, jdbcprobe.ErrConnectionFailed) {
			results[agentpreflight.CheckDatabaseConnectivity] = directLoadOutcome(agentpreflight.CheckDatabaseConnectivity, agentpreflight.StatusFailed)
		}
		return results, fingerprint
	}
	project := func(check agentpreflight.CheckID, access jdbcprobe.ObjectAccess) {
		status := agentpreflight.StatusUnknown
		if access == jdbcprobe.ObjectAccessible {
			status = agentpreflight.StatusPassed
		}
		if access == jdbcprobe.ObjectNotAccessible {
			status = agentpreflight.StatusFailed
		}
		results[check] = directLoadOutcome(check, status)
	}
	project(agentpreflight.CheckDatabaseConnectivity, observed.DatabaseAccess)
	if observed.DatabaseAccess != jdbcprobe.ObjectAccessible {
		return results, fingerprint
	}
	tool := local.ToolVersion
	if local.ObservedAt.IsZero() || local.ObservedAt.After(time.Now().Add(time.Minute)) {
		tool = ""
	}
	if tool == "4.3.5" {
		tool = directload.ToolVersion
	}
	versionStatus := agentpreflight.StatusPassed
	for _, issue := range directload.ValidateVersions(c.Scenario, tool, observed.ObserverVersion, observed.ODPVersion, r.CompatibilityMode) {
		if issue.Code == "VERSION_UNSUPPORTED" || issue.Code == "UNSUPPORTED_SCENARIO" {
			versionStatus = agentpreflight.StatusFailed
			break
		}
		versionStatus = agentpreflight.StatusUnknown
	}
	results["DIRECT_LOAD_VERSION"] = directLoadOutcome("DIRECT_LOAD_VERSION", versionStatus)
	project("WRITE_ACCESS", observed.WriteAccess)
	results["DIRECT_LOAD_STRUCTURE"] = directLoadStructureResult(c, observed.TableFacts)
	if ctx.Err() == nil {
		results["RPC_CONNECTIVITY"] = p.directLoadTCP(ctx, connection.Host, c.RPCPort)
	}
	return results, fingerprint
}

// directLoadStructureResult 仅投影固定领域原因，不回传表名、列名、SQL 或数据库错误原文。
func directLoadStructureResult(c directload.Config, facts directload.TableFacts) agentpreflight.Result {
	result := directLoadOutcome("DIRECT_LOAD_STRUCTURE", agentpreflight.StatusPassed)
	for _, issue := range directload.AssessTable(c, facts).Issues {
		if issue.Code == "TABLE_FACT_UNAVAILABLE" {
			result = agentpreflight.Result{Check: "DIRECT_LOAD_STRUCTURE", Status: agentpreflight.StatusUnknown, EvidenceCode: "DIRECT_LOAD_TABLE_FACT_UNAVAILABLE"}
			continue
		}
		code := "DIRECT_LOAD_REJECTED"
		switch issue.Code {
		case "TABLE_NOT_FOUND":
			code = "DIRECT_LOAD_TABLE_NOT_FOUND"
		case "STRUCTURE_UNSUPPORTED":
			code = map[string]string{"bit": "DIRECT_LOAD_BIT_BLOCKED", "binary": "DIRECT_LOAD_BINARY_BLOCKED", "virtualGenerated": "DIRECT_LOAD_VIRTUAL_BLOCKED"}[issue.Field]
		case "INCREMENTAL_REPLACE_UNSUPPORTED":
			code = "DIRECT_LOAD_REPLACE_LOB_BLOCKED"
			if facts.HasIndex != nil && *facts.HasIndex {
				code = "DIRECT_LOAD_REPLACE_INDEX_BLOCKED"
			}
		}
		if code == "" {
			code = "DIRECT_LOAD_REJECTED"
		}
		return agentpreflight.Result{Check: "DIRECT_LOAD_STRUCTURE", Status: agentpreflight.StatusFailed, EvidenceCode: code}
	}
	return result
}

func directLoadConfigResult(r jdbcprobe.DirectLoadResult) agentpreflight.Result {
	unknown := directLoadUnknown("DIRECT_LOAD_CONFIG")
	c := r.DirectLoadConfig
	if c == nil || r.ObservedAt.IsZero() || r.ObservedAt.After(time.Now().Add(time.Minute)) || c.Status != "READ" || c.HasUnrecognizedKeys || c.Fingerprint != nodefacts.DirectLoadFingerprint(*c) {
		return unknown
	}
	for _, value := range []*int64{c.TaskTimeoutMS, c.HeartbeatTimeoutMS, c.HeartbeatIntervalMS} {
		if value == nil || *value <= 0 || *value > 1<<53-1 {
			return unknown
		}
	}
	return directLoadOutcome("DIRECT_LOAD_CONFIG", agentpreflight.StatusPassed)
}

func (p *DirectLoadProbe) directLoadTCP(ctx context.Context, host string, port int) agentpreflight.Result {
	check := agentpreflight.CheckID("RPC_CONNECTIVITY")
	unknown := directLoadUnknown(check)
	if host == "" || port < 1 || port > 65535 {
		return unknown
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	dial := p.DialContext
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	connection, err := dial(bounded, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if connection != nil {
		defer connection.Close()
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return unknown
	}
	if err != nil {
		return directLoadOutcome(check, agentpreflight.StatusFailed)
	}
	if connection == nil {
		return unknown
	}
	// 这里只证明 TCP 建连，不做 RPC 握手，也不证明认证、导入或服务端提交。
	return directLoadOutcome(check, agentpreflight.StatusPassed)
}
