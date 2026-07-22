// Package agentjdbc 将固定 JDBC 连接探针适配为 Agent 的数据库连通性预检查。
// 它只处理短时秘密槽位和受控子进程边界，不实现 SQL、对象权限诊断或任何 OBDUMPER 执行。
package agentjdbc

import (
	"context"
	"errors"
	"strings"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/jdbcprobe"
)

const (
	EvidenceDatabaseConnected   = "DATABASE_CONNECTED"
	EvidenceDatabaseFailed      = "DATABASE_CONNECTION_FAILED"
	EvidenceDatabaseUnavailable = "DATABASE_CONNECTION_UNAVAILABLE"
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

// ConnectionRunner 为固定 JDBC 连接探针保留一个窄的可替换边界。
// 生产默认实现只允许 jdbcprobe.TestConnectionInWorkspace，测试替身不得改变外部协议语义。
type ConnectionRunner func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.Request) (jdbcprobe.Result, error)

// PrecheckProbe 实现固定 DATABASE_CONNECTIVITY 检查。
// 它只可由已校验的 Agent 预检查调用，且每次检查使用独立私有工作区释放探针资产。
type PrecheckProbe struct {
	WorkspaceRoot string
	Runtime       jdbcprobe.Runtime
	Resolver      SecretResolver
	Run           ConnectionRunner
}

// Probe 执行短时的基础 JDBC 连接验证，并且始终以安全状态码投影失败。
// 返回值不含连接地址、用户名、密码、JDBC URL、Java 异常或数据库对象信息。
func (p PrecheckProbe) Probe(ctx context.Context, check agentpreflight.CheckID, request agentpreflight.Request) (result agentpreflight.Result, err error) {
	if check != agentpreflight.CheckDatabaseConnectivity {
		return agentpreflight.Result{}, ErrUnsupportedCheck
	}
	if p.Resolver == nil || strings.TrimSpace(p.WorkspaceRoot) == "" {
		return unavailableResult(check), nil
	}
	connection, err := p.Resolver.ResolveDatabaseConnection(ctx, request.Binding)
	if err != nil {
		return unavailableResult(check), nil
	}
	defer credential.Zero(connection.Username)
	defer credential.Zero(connection.Password)
	workspace, err := credential.CreateWorkspace(p.WorkspaceRoot, request.PrecheckID)
	if err != nil {
		return unavailableResult(check), nil
	}
	defer func() {
		if cleanupErr := workspace.Cleanup(); cleanupErr != nil {
			result = unavailableResult(check)
			err = nil
		}
	}()
	run := p.Run
	if run == nil {
		run = jdbcprobe.TestConnectionInWorkspace
	}
	_, err = run(ctx, workspace, p.Runtime, jdbcprobe.Request{
		Host:     connection.Host,
		Port:     connection.Port,
		Username: connection.Username,
		Password: connection.Password,
	})
	if err != nil {
		if errors.Is(err, jdbcprobe.ErrConnectionFailed) {
			return agentpreflight.Result{Check: check, Status: agentpreflight.StatusFailed, EvidenceCode: EvidenceDatabaseFailed}, nil
		}
		return unavailableResult(check), nil
	}
	return agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: EvidenceDatabaseConnected}, nil
}

func unavailableResult(check agentpreflight.CheckID) agentpreflight.Result {
	return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: EvidenceDatabaseUnavailable}
}
