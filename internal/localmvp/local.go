// Package localmvp 仅组装显式启动的本机演示控制面。
// 它只能绑定回环地址，不能作为生产身份、授权或 Agent 实现使用。
package localmvp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/controlplane"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/logstream"
	"ob-data-orch/internal/store"
)

const CSRFToken = "local-mvp-csrf-v1"

const localSubjectID = "local-mvp-owner"

// PrepareStore 初始化仅供回环本机演示身份使用的最小 SQLite 身份投影。
// 它不创建生产用户、角色、对象范围或任何可在回环环境外使用的认证材料。
func PrepareStore(ctx context.Context, database *store.Store) error {
	now := time.Now().UTC()
	if err := database.EnsureAuthSubject(ctx, store.AuthSubject{
		SubjectID:       localSubjectID,
		ExternalSubject: localSubjectID,
		DisplayName:     "本机 MVP 管理员",
		AccountStatus:   "ACTIVE",
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		return err
	}
	_, err := database.RevokeLocalSyntheticDataSourceTestFacts(ctx, now)
	return err
}

// Dependencies 为本机 MVP 返回受限身份、SQLite 存储和固定导出命令生成器依赖。
// 参数元数据无法加载时必须阻止启动，避免导出草稿在缺少确定性命令规则时继续处理。
// 两个布尔开关均必须由本机启动入口显式传入；真实执行仍只能在回环 TLS MVP 与受认证 Agent 组合中启用。
func Dependencies(database *store.Store, keyring *credential.Keyring, agentJDBCConnectionTestEnabled, realExecutionEnabled bool) (controlplane.Dependencies, error) {
	generator, err := commandgen.NewDefault()
	if err != nil {
		return controlplane.Dependencies{}, fmt.Errorf("加载本机 MVP 导出命令生成器: %w", err)
	}
	// 泛化能力生成器加载失败同样阻止启动，避免 EX-I2 能力在缺少确定性命令规则时继续处理。
	generalizedGenerator, err := commandgen.NewGeneralized()
	if err != nil {
		return controlplane.Dependencies{}, fmt.Errorf("加载本机 MVP 泛化导出命令生成器: %w", err)
	}
	persistentLogs, err := logstream.NewPersistentStore("var/local-mvp-logs", database)
	if err != nil {
		return controlplane.Dependencies{}, fmt.Errorf("初始化本机 MVP 日志段存储: %w", err)
	}
	if err := persistentLogs.Recover(context.Background()); err != nil {
		return controlplane.Dependencies{}, fmt.Errorf("恢复本机 MVP 已登记日志段: %w", err)
	}
	return controlplane.Dependencies{Identity: localIdentity{}, Authorizer: localAuthorizer{}, Roles: localAuthorizer{}, DataSources: database, Creator: database, StateChanger: database, Deleter: database, ConnectionTests: database, Updater: database, CredentialRefs: database, Drafts: database, Prechecks: database, Tasks: database, Executions: database, Templates: database, LogLedger: logstream.NewBatchLedger(), PersistentLogs: persistentLogs, Nodes: localExecutionNodeReader{database: database}, NodeManagement: database, NodeEnvironment: database, NodeDeleter: database, NodeCandidates: database, AgentProtocol: database, AgentEnvironmentChecks: database, AgentPrechecks: database, PrecheckSecrets: database, AgentConnectionTests: database, ConnectionTestSecrets: database, Generator: generator, GeneralizedGenerator: generalizedGenerator, PrecheckTTL: 2 * time.Minute, AgentJDBCConnectionTestEnabled: agentJDBCConnectionTestEnabled, RealExecutionEnabled: realExecutionEnabled, EnrollmentTTL: 15 * time.Minute, HeartbeatTTL: 2 * time.Minute, ConnectionTestTTL: 2 * time.Minute, Encryptor: keyring, Decryptor: keyring, CSRF: localCSRF{}, CredentialKeyID: "local-mvp-root-v1"}, nil
}

// localExecutionNodeReader 仅将已持久化的受认证 Agent 事实转换为命令生成所需投影。
// 节点声明、禁用状态或无 Agent 心跳都不能伪造成可生成导出草稿的节点事实。
type localExecutionNodeReader struct {
	database *store.Store
}

func (reader localExecutionNodeReader) GetExecutionNodeFact(ctx context.Context, nodeID string) (controlplane.ExecutionNodeFact, error) {
	node, err := reader.database.GetExecutionNode(ctx, nodeID)
	if err != nil {
		return controlplane.ExecutionNodeFact{}, err
	}
	if node.ManagementState != "ENABLED" || node.Agent == nil || node.Agent.FactsRevision < 1 {
		return controlplane.ExecutionNodeFact{}, errors.New("execution node has no eligible agent facts")
	}
	platform, err := exportPlatform(node.Platform)
	if err != nil {
		return controlplane.ExecutionNodeFact{}, err
	}
	return controlplane.ExecutionNodeFact{
		NodeID:        node.NodeID,
		Platform:      platform,
		FactsVersion:  fmt.Sprintf("node-facts-rev-%d", node.Agent.FactsRevision),
		FactsRevision: node.Agent.FactsRevision,
	}, nil
}

// exportPlatform 将已校验的节点平台映射到命令生成器支持的固定目标集合。
func exportPlatform(value string) (commandgen.Platform, error) {
	switch value {
	case "WINDOWS_AMD64":
		return commandgen.PlatformWindowsAMD64, nil
	case "LINUX_AMD64":
		return commandgen.PlatformLinuxAMD64, nil
	case "LINUX_ARM64":
		return commandgen.PlatformLinuxARM64, nil
	default:
		return "", fmt.Errorf("unsupported execution node platform")
	}
}

type localIdentity struct{}

func (localIdentity) AuthenticateBrowser(request *http.Request) (identity.Principal, error) {
	if !loopback(request.RemoteAddr) {
		return identity.Principal{}, identity.ErrUnauthenticated
	}
	return identity.Principal{Type: identity.BrowserPrincipal, ID: localSubjectID}, nil
}
func (localIdentity) AuthenticateAgent(*http.Request) (identity.Principal, error) {
	return identity.Principal{}, identity.ErrUnauthenticated
}

type localAuthorizer struct{}

func (localAuthorizer) Authorize(_ context.Context, principal identity.Principal, scope identity.Scope, objectID string) error {
	if principal.Type != identity.BrowserPrincipal || principal.ID != localSubjectID || objectID == "" || (scope != identity.ScopeDataSourceRead && scope != identity.ScopeDataSourceWrite && scope != identity.ScopeNodeUse && scope != identity.ScopeNodeManage) {
		return identity.ErrDenied
	}
	return nil
}
func (localAuthorizer) AuthorizeRole(_ context.Context, principal identity.Principal, role identity.Role) error {
	if principal.Type != identity.BrowserPrincipal || principal.ID != localSubjectID || (role != identity.RoleDataSourceAdmin && role != identity.RoleNodeAdmin) {
		return identity.ErrDenied
	}
	return nil
}

type localCSRF struct{}

func (localCSRF) ValidateCSRF(request *http.Request) error {
	if request.Header.Get("X-CSRF-Token") != CSRFToken {
		return errors.New("local CSRF token rejected")
	}
	return nil
}

func loopback(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	return net.ParseIP(host).IsLoopback()
}
