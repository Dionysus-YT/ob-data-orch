package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/nodefacts"
	"ob-data-orch/internal/store"
)

type nodeObservationTaskStore struct {
	recordingTaskStore
	global bool
	failed bool
}

func Test控制面严格心跳接收诊断字段但拒绝新增任意输出(t *testing.T) {
	now, started := time.Now().UTC(), time.Now().UTC().Add(-time.Minute)
	protocol := &recordingAgentProtocol{identity: store.AgentIdentity{AgentID: "synthetic-agent", NodeID: "synthetic-node", ProtocolVersion: "agent-v1"}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{AgentProtocol: protocol})
	request := agentHeartbeatRequest{ProtocolVersion: "agent-v1", AgentID: "synthetic-agent", NodeID: "synthetic-node", BootID: "synthetic-boot", RequestID: "synthetic-request", SentAt: now.Format(time.RFC3339Nano), PayloadType: "HEARTBEAT", Payload: agentHeartbeatPayload{
		Host:            &nodefacts.HostInventory{Hostname: "synthetic-host", Addresses: []string{"192.0.2.99"}, LogicalCPUs: 8},
		OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic", ObservedAt: now.Format(time.RFC3339Nano), CapacityTotal: 1, StartedAt: &started,
		RuntimeVersions: &nodefacts.RuntimeVersions{Java: &nodefacts.InstalledVersion{Version: "1.8.0_491", Source: "JAVA_RELEASE"}}, ExecutionObservation: &nodefacts.ExecutionObservation{Active: []nodefacts.ActiveExecution{}}, LogQueue: &nodefacts.LogQueue{LastUploadResult: "UNKNOWN"},
	}}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	call := func(body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/agent/v1/heartbeats", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer synthetic-machine-credential-0123456789")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		return response
	}
	response := call(body)
	if response.Code != http.StatusOK || protocol.heartbeat.Facts.StartedAt == nil || protocol.heartbeat.Facts.RuntimeVersions == nil || protocol.heartbeat.Facts.LogQueue == nil || protocol.heartbeat.Facts.ExecutionObservation == nil || protocol.heartbeat.Facts.Host == nil || protocol.heartbeat.Facts.Host.LogicalCPUs != 8 {
		t.Fatal("HTTP 到存储端口丢失诊断字段")
	}
	invalid := bytes.Replace(body, []byte(`"version":"1.8.0_491"`), []byte(`"version":"1.8.0_491","rawOutput":"synthetic-output"`), 1)
	if response := call(invalid); response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "synthetic-output") {
		t.Fatal("严格诊断 DTO 接收或传播原始输出")
	}
}

func (s *nodeObservationTaskStore) ListAuthorizedNodeExecutionBindings(_ context.Context, node, agent, subject string, global bool) ([]store.NodeExecutionBinding, error) {
	s.global = global
	if s.failed {
		return nil, errors.New("synthetic read failure")
	}
	bindings := []store.NodeExecutionBinding{{TaskID: "readable-task", ExecutionID: "readable-execution", LeaseID: "readable-lease", LeaseEpoch: 1}, {TaskID: "stale-task", ExecutionID: "stale-execution", LeaseID: "current-lease", LeaseEpoch: 2}}
	if global {
		bindings = append(bindings, store.NodeExecutionBinding{TaskID: "private-task", ExecutionID: "private-execution", LeaseID: "private-lease", LeaseEpoch: 1})
	}
	return bindings, nil
}

func (s *nodeObservationTaskStore) ListNodeTaskSummaries(context.Context, string, bool) (map[string]store.NodeTaskSummary, error) {
	return map[string]store.NodeTaskSummary{"node-allowed": {Running: 2, Waiting: 3, Reconciling: 1}}, nil
}

type failedNodeDiagnosticBudgetReader struct{ recordingNodeManagementStore }

func (s *failedNodeDiagnosticBudgetReader) ListNodeBudgetUsages(context.Context) (map[string]store.NodeBudgetUsage, error) {
	return nil, errors.New("synthetic budget read failure")
}

func Test详情诊断记录完整且执行标识必须再次授权和绑定(t *testing.T) {
	now, started := time.Now().UTC(), time.Now().UTC().Add(-time.Minute)
	node := store.ExecutionNode{NodeID: "node-allowed", DisplayName: "合成节点", Platform: "WINDOWS_AMD64", ManagementState: "DISABLED", Revision: 1, CreatedAt: now, UpdatedAt: now, ToolHome: `C:\synthetic-tools`, JavaPath: `C:\synthetic-java\bin\java.exe`, AllowedRoots: []string{`C:\synthetic-output`}, EnvironmentCheck: store.ExecutionNodeEnvironmentCheck{Status: "FAILED", Code: "TOOL_RUNTIME_INVALID", FactsRevision: 3, RequestedAt: &started, CompletedAt: &now}}
	node.Agent = &store.ExecutionNodeAgent{AgentID: "synthetic-agent", ProtocolVersion: "agent-v1", FactsRevision: 3, BootID: "synthetic-boot", LastHeartbeatAt: &now, EnvironmentFacts: store.AgentEnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic", ObservedAt: now, StartedAt: &started,
		Host:            &nodefacts.HostInventory{Hostname: "synthetic-host", Addresses: []string{"192.0.2.99", "192.0.2.100", "2001:db8::1234"}, LogicalCPUs: 8},
		RuntimeVersions: &nodefacts.RuntimeVersions{OBDumper: &nodefacts.InstalledVersion{Version: "4.3.5-RELEASE", Source: "JAR_MANIFEST"}}, LogQueue: &nodefacts.LogQueue{LastUploadResult: "UNKNOWN"},
		ExecutionObservation: &nodefacts.ExecutionObservation{Active: []nodefacts.ActiveExecution{{ExecutionID: "readable-execution", LeaseID: "readable-lease", LeaseEpoch: 1, LastEventSeq: 3}, {ExecutionID: "private-execution", LeaseID: "private-lease", LeaseEpoch: 1}, {ExecutionID: "stale-execution", LeaseID: "old-lease", LeaseEpoch: 1}}},
	}}
	node.Agent.EnvironmentFacts.RuntimeConfigurationDigest = store.ExecutionNodeRuntimeConfigurationDigest(node)
	for _, tc := range []struct{ role, read, global bool }{{false, false, false}, {true, false, false}, {false, true, false}, {true, true, true}} {
		auth := scopeTestAuthorizer{globalRole: tc.role, globalRead: tc.read}
		tasks := &nodeObservationTaskStore{}
		for index := range 6 {
			tasks.listItems = append(tasks.listItems, store.TaskListItem{TaskID: fmt.Sprintf("readable-task-%d", index), TaskType: "OBDUMPER_EXPORT", State: "WAITING_SCHEDULE", Database: "synthetic-private-database", Table: "synthetic-private-table", SubmittedAt: now})
		}
		server := &Server{heartbeatTTL: time.Minute, tasks: tasks, roles: auth, authorizer: auth}
		response := server.newAuthorizedExecutionNodeDetailResponse(context.Background(), identity.Principal{Type: identity.BrowserPrincipal, ID: "synthetic-reader"}, node)
		if response.EnvironmentCheck.RequestedAt == nil || response.EnvironmentCheck.CompletedAt == nil || response.EnvironmentCheck.FactsRevision != 3 || response.AgentDiagnostics == nil || response.AgentDiagnostics.RuntimeVersions == nil || response.AgentDiagnostics.FactsRevision != 3 {
			t.Fatal("已有检查与新采集未贯通详情")
		}
		if response.ActiveExecutions == nil || tasks.global != tc.global || len(response.ActiveExecutions.Items) != 1+boolInt(tc.global) {
			t.Fatal("活动执行扩大或丢失授权范围")
		}
		if response.TaskSummary == nil || response.TaskSummary.Running != 2 || (response.TaskSummary.Scope == "ALL") != tc.global {
			t.Fatal("详情没有复用完整的授权统计")
		}
		if response.RecentTasks == nil || len(response.RecentTasks.Items) != 5 || (response.RecentTasks.Scope == "ALL") != tc.global || tasks.listQuery.GlobalRead != tc.global || tasks.listQuery.NodeID != node.NodeID || tasks.listQuery.SubjectID != "synthetic-reader" {
			t.Fatal("近期任务没有沿用节点和账号的读取范围")
		}
		if host := response.AgentDiagnostics.Host; host == nil || len(host.AddressPrefixes) != 2 || host.AddressPrefixes[0] != "192.0.2.0/24" || host.AddressPrefixes[1] != "2001:db8::/64" {
			t.Fatal("原始地址没有裁剪且去重为固定网段摘要")
		}
		encoded, _ := json.Marshal(response)
		if strings.Contains(string(encoded), "old-lease") || (!tc.global && strings.Contains(string(encoded), "private-execution")) {
			t.Fatal("无权或旧租约标识泄露")
		}
		for _, private := range []string{"192.0.2.99", "2001:db8::1234", "synthetic-private-database", "synthetic-private-table"} {
			if strings.Contains(string(encoded), private) {
				t.Fatal("节点摘要携带完整地址或任务业务对象")
			}
		}
	}
	server := &Server{tasks: &nodeObservationTaskStore{failed: true}}
	response := server.newAuthorizedExecutionNodeDetailResponse(context.Background(), identity.Principal{ID: "reader"}, node)
	if response.ActiveExecutions != nil {
		t.Fatal("读取失败不能投影为零活动执行")
	}
	node.ManagementState, node.EnvironmentCheck.Status, node.EnvironmentCheck.Code, node.Agent.CapacityTotal = "ENABLED", "PASSED", "TOOL_RUNTIME_READY", 1
	server = &Server{heartbeatTTL: time.Minute, nodeManagement: &failedNodeDiagnosticBudgetReader{}}
	response = server.newAuthorizedExecutionNodeDetailResponse(context.Background(), identity.Principal{ID: "reader", Type: identity.BrowserPrincipal}, node)
	if response.AcceptsNewTasks || response.ExecutionQuota != nil {
		t.Fatal("预算读取失败却仍授予详情执行资格")
	}
	node.Agent.EnvironmentFacts.RuntimeConfigurationDigest = strings.Repeat("b", 64)
	if server.newExecutionNodeDetailResponse(node).AgentDiagnostics.RuntimeVersions != nil {
		t.Fatal("旧配置版本进入当前安装版本")
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func Test详情接口提供检查记录且无权对象不调用诊断读取(t *testing.T) {
	now := time.Now().UTC()
	manager := &recordingNodeManagementStore{nodes: map[string]store.ExecutionNode{"node-allowed": {NodeID: "node-allowed", DisplayName: "合成节点", Platform: "WINDOWS_AMD64", ManagementState: "DISABLED", Revision: 1, CreatedAt: now, UpdatedAt: now}}}
	handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{Identity: browserOnlyIdentityProvider{}, Authorizer: nodeManageAuthorizer{allowedID: "node-allowed"}, NodeManagement: manager})
	for _, id := range []string{"node-allowed", "private-node"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/execution-nodes/"+id, nil))
		if id == "private-node" {
			if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "environmentCheck") {
				t.Fatal("无权对象显示诊断")
			}
			continue
		}
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"environmentCheck":{"status":"NOT_CHECKED"`) {
			t.Fatal("详情 HTTP 没有进入新投影")
		}
	}
}
