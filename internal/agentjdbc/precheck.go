// Package agentjdbc 将固定 JDBC 连接探针适配为 Agent 的数据库与对象预检查。
// 它只处理短时秘密槽位和受控子进程边界；唯一允许的 SQL 是探针内部生成的固定零行读取，不实现自由对象权限诊断或任何 OBDUMPER 执行。
package agentjdbc

import (
	"context"
	"errors"
	"strings"
	"sync"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/jdbcprobe"
)

const (
	EvidenceDatabaseConnected   = "DATABASE_CONNECTED"
	EvidenceDatabaseFailed      = "DATABASE_CONNECTION_FAILED"
	EvidenceDatabaseUnavailable = "DATABASE_CONNECTION_UNAVAILABLE"
	EvidenceObjectAccessible    = "OBJECT_ACCESSIBLE"
	EvidenceObjectNotAccessible = "OBJECT_NOT_ACCESSIBLE"
	EvidenceObjectUnavailable   = "OBJECT_ACCESS_UNAVAILABLE"
)

var ErrUnsupportedCheck = errors.New("JDBC 预检查不支持该检查项")

// Connection 是控制面在有效预检查租约内短时解析的数据库连接输入。
// 它不能被写入日志、任务快照、工作区文件、命令行参数或环境变量。
type Connection struct {
	Host     string
	Port     int
	Username []byte
	Password []byte
}

// SecretResolver 只解析与当前冻结预检查绑定匹配的数据库连接槽位。
// 其正式实现必须校验租约、节点、Agent、凭据 revision 与配置指纹，且不得缓存明文。
type SecretResolver interface {
	ResolveDatabaseConnection(context.Context, agentstate.PrecheckBinding) (Connection, error)
}

// PreflightRunner 为一个 JDBC 进程内的固定连接和对象检查保留窄的可替换边界。
// 生产默认实现只允许 jdbcprobe.TestPreflightInWorkspace，测试替身不得改变外部协议语义。
type PreflightRunner func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.PreflightRequest) (jdbcprobe.PreflightResult, error)

// PrecheckProbe 实现固定 DATABASE_CONNECTIVITY 与 OBJECT_ACCESS 检查。
// 同一实例只能为一个冻结请求解析一次槽位并启动一次探针；对象结论来自同一已建立连接的 JDBC 元数据与固定零行读取。
type PrecheckProbe struct {
	WorkspaceRoot string
	Runtime       jdbcprobe.Runtime
	Resolver      SecretResolver
	Run           PreflightRunner

	mu    sync.Mutex
	state jdbcState
}

type jdbcState struct {
	request   jdbcRequestIdentity
	running   bool
	completed bool
	database  agentpreflight.Result
	object    agentpreflight.Result
}

type jdbcRequestIdentity struct {
	precheckID        string
	binding           agentstate.PrecheckBinding
	compatibilityMode jdbcprobe.CompatibilityMode
	database          string
	table             string
}

// Probe 执行短时 JDBC 连接、固定对象元数据和零行读取验证，并始终以安全状态码投影失败。
// 返回值不含连接地址、用户名、密码、JDBC URL、Java 异常或数据库对象信息。
func (p *PrecheckProbe) Probe(ctx context.Context, check agentpreflight.CheckID, request agentpreflight.Request) (agentpreflight.Result, error) {
	switch check {
	case agentpreflight.CheckDatabaseConnectivity:
		return p.databaseResult(ctx, request), nil
	case agentpreflight.CheckObjectAccess:
		return p.objectResult(request), nil
	default:
		return agentpreflight.Result{}, ErrUnsupportedCheck
	}
}

func (p *PrecheckProbe) databaseResult(ctx context.Context, request agentpreflight.Request) agentpreflight.Result {
	if p == nil || p.Resolver == nil || strings.TrimSpace(p.WorkspaceRoot) == "" {
		return unavailableDatabaseResult()
	}
	identity := jdbcRequestIdentity{precheckID: request.PrecheckID, binding: request.Binding, compatibilityMode: jdbcprobe.CompatibilityMode(request.CompatibilityMode), database: request.Database, table: request.Table}
	p.mu.Lock()
	if p.state.completed {
		if p.state.request != identity {
			p.mu.Unlock()
			return unavailableDatabaseResult()
		}
		result := p.state.database
		p.mu.Unlock()
		return result
	}
	if p.state.running {
		p.mu.Unlock()
		return unavailableDatabaseResult()
	}
	p.state.request = identity
	p.state.running = true
	p.mu.Unlock()

	database, object := p.runPreflight(ctx, request)
	p.mu.Lock()
	p.state.running = false
	p.state.completed = true
	p.state.database = database
	p.state.object = object
	p.mu.Unlock()
	return database
}

func (p *PrecheckProbe) objectResult(request agentpreflight.Request) agentpreflight.Result {
	if p == nil {
		return unavailableObjectResult()
	}
	identity := jdbcRequestIdentity{precheckID: request.PrecheckID, binding: request.Binding, compatibilityMode: jdbcprobe.CompatibilityMode(request.CompatibilityMode), database: request.Database, table: request.Table}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.state.completed || p.state.request != identity {
		return unavailableObjectResult()
	}
	return p.state.object
}

func (p *PrecheckProbe) runPreflight(ctx context.Context, request agentpreflight.Request) (databaseResult agentpreflight.Result, objectResult agentpreflight.Result) {
	workspace, err := credential.CreateWorkspace(p.WorkspaceRoot, request.PrecheckID)
	if err != nil {
		return unavailableDatabaseResult(), unavailableObjectResult()
	}
	defer func() {
		if cleanupErr := workspace.Cleanup(); cleanupErr != nil {
			databaseResult = unavailableDatabaseResult()
			objectResult = unavailableObjectResult()
		}
	}()

	connection, resolveErr := p.Resolver.ResolveDatabaseConnection(ctx, request.Binding)
	defer credential.Zero(connection.Username)
	defer credential.Zero(connection.Password)
	if resolveErr != nil {
		return unavailableDatabaseResult(), unavailableObjectResult()
	}
	run := p.Run
	if run == nil {
		run = jdbcprobe.TestPreflightInWorkspace
	}
	result, runErr := run(ctx, workspace, p.Runtime, jdbcprobe.PreflightRequest{
		Connection: jdbcprobe.Request{
			Host:     connection.Host,
			Port:     connection.Port,
			Username: connection.Username,
			Password: connection.Password,
		},
		CompatibilityMode: jdbcprobe.CompatibilityMode(request.CompatibilityMode),
		Database:          request.Database,
		Table:             request.Table,
	})
	if runErr != nil {
		if errors.Is(runErr, jdbcprobe.ErrConnectionFailed) {
			return failedDatabaseResult(), unavailableObjectResult()
		}
		return unavailableDatabaseResult(), unavailableObjectResult()
	}
	return connectedDatabaseResult(), objectAccessResult(result.ObjectAccess)
}

func connectedDatabaseResult() agentpreflight.Result {
	return agentpreflight.Result{Check: agentpreflight.CheckDatabaseConnectivity, Status: agentpreflight.StatusPassed, EvidenceCode: EvidenceDatabaseConnected}
}

func failedDatabaseResult() agentpreflight.Result {
	return agentpreflight.Result{Check: agentpreflight.CheckDatabaseConnectivity, Status: agentpreflight.StatusFailed, EvidenceCode: EvidenceDatabaseFailed}
}

func unavailableDatabaseResult() agentpreflight.Result {
	return agentpreflight.Result{Check: agentpreflight.CheckDatabaseConnectivity, Status: agentpreflight.StatusUnknown, EvidenceCode: EvidenceDatabaseUnavailable}
}

func objectAccessResult(access jdbcprobe.ObjectAccess) agentpreflight.Result {
	switch access {
	case jdbcprobe.ObjectAccessible:
		return agentpreflight.Result{Check: agentpreflight.CheckObjectAccess, Status: agentpreflight.StatusPassed, EvidenceCode: EvidenceObjectAccessible}
	case jdbcprobe.ObjectNotAccessible:
		return agentpreflight.Result{Check: agentpreflight.CheckObjectAccess, Status: agentpreflight.StatusFailed, EvidenceCode: EvidenceObjectNotAccessible}
	default:
		return unavailableObjectResult()
	}
}

func unavailableObjectResult() agentpreflight.Result {
	return agentpreflight.Result{Check: agentpreflight.CheckObjectAccess, Status: agentpreflight.StatusUnknown, EvidenceCode: EvidenceObjectUnavailable}
}
