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

// ConnectionRunner 是 ALL 范围的数据库级可达性探测边界。
// 生产默认实现只允许 jdbcprobe.TestConnectionInWorkspace，不携带对象输入。
type ConnectionRunner func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.Request) (jdbcprobe.Result, error)

// PrecheckProbe 实现固定 DATABASE_CONNECTIVITY 与 OBJECT_ACCESS 检查。
// 同一实例只能为一个冻结请求解析一次槽位；SPECIFIED 范围逐对象运行冻结单对象探针，
// ALL 范围只运行数据库级连接探测，对象结论由可达性投影，逐对象枚举由工具运行时完成。
type PrecheckProbe struct {
	WorkspaceRoot string
	Runtime       jdbcprobe.Runtime
	Resolver      SecretResolver
	Run           PreflightRunner
	RunConnection ConnectionRunner

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
	// objects 是逗号连接的冻结对象清单；ALL 范围为空。
	objects     string
	contentKind string
}

func newJdbcRequestIdentity(request agentpreflight.Request) jdbcRequestIdentity {
	return jdbcRequestIdentity{
		precheckID:        request.PrecheckID,
		binding:           request.Binding,
		compatibilityMode: jdbcprobe.CompatibilityMode(request.CompatibilityMode),
		database:          request.Database,
		objects:           strings.Join(request.Objects, ","),
		contentKind:       request.ContentKind,
	}
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
	identity := newJdbcRequestIdentity(request)
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
	identity := newJdbcRequestIdentity(request)
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
	jdbcConnection := jdbcprobe.Request{
		Host:     connection.Host,
		Port:     connection.Port,
		Username: connection.Username,
		Password: connection.Password,
	}
	// ALL 范围：只用冻结连接探针完成数据库级可达性检查；对象结论由可达性投影，
	// 逐对象枚举由工具运行时完成，预检查不代替运行时事实。
	if len(request.Objects) == 0 {
		runConnection := p.RunConnection
		if runConnection == nil {
			runConnection = jdbcprobe.TestConnectionInWorkspace
		}
		if _, runErr := runConnection(ctx, workspace, p.Runtime, jdbcConnection); runErr != nil {
			if errors.Is(runErr, jdbcprobe.ErrConnectionFailed) {
				return failedDatabaseResult(), unavailableObjectResult()
			}
			return unavailableDatabaseResult(), unavailableObjectResult()
		}
		return connectedDatabaseResult(), agentpreflight.Result{Check: agentpreflight.CheckObjectAccess, Status: agentpreflight.StatusPassed, EvidenceCode: EvidenceObjectAccessible}
	}
	run := p.Run
	if run == nil {
		run = jdbcprobe.TestPreflightInWorkspace
	}
	// SPECIFIED 范围：对每个冻结对象运行一次固定单对象探针；任一对象不可达即整体失败。
	for _, object := range request.Objects {
		result, runErr := run(ctx, workspace, p.Runtime, jdbcprobe.PreflightRequest{
			Connection:        jdbcConnection,
			CompatibilityMode: jdbcprobe.CompatibilityMode(request.CompatibilityMode),
			Database:          request.Database,
			Table:             object,
		})
		if runErr != nil {
			if errors.Is(runErr, jdbcprobe.ErrConnectionFailed) {
				return failedDatabaseResult(), unavailableObjectResult()
			}
			return unavailableDatabaseResult(), unavailableObjectResult()
		}
		switch result.ObjectAccess {
		case jdbcprobe.ObjectAccessible:
			continue
		case jdbcprobe.ObjectNotAccessible:
			return connectedDatabaseResult(), agentpreflight.Result{Check: agentpreflight.CheckObjectAccess, Status: agentpreflight.StatusFailed, EvidenceCode: EvidenceObjectNotAccessible}
		default:
			return connectedDatabaseResult(), unavailableObjectResult()
		}
	}
	return connectedDatabaseResult(), agentpreflight.Result{Check: agentpreflight.CheckObjectAccess, Status: agentpreflight.StatusPassed, EvidenceCode: EvidenceObjectAccessible}
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

func unavailableObjectResult() agentpreflight.Result {
	return agentpreflight.Result{Check: agentpreflight.CheckObjectAccess, Status: agentpreflight.StatusUnknown, EvidenceCode: EvidenceObjectUnavailable}
}
