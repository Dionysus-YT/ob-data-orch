package controlplane

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ob-data-orch/internal/buildinfo"
	"ob-data-orch/internal/store"
)

func Test节点物理内存成对投影且保留零可用量(t *testing.T) {
	now := time.Now().UTC()
	server := &Server{heartbeatTTL: time.Minute}
	for _, tc := range []struct {
		name      string
		total     uint64
		available uint64
		known     bool
	}{
		{"正常采样", 16 << 30, 4 << 30, true},
		{"内存耗尽", 16 << 30, 0, true},
		{"全部空闲", 16 << 30, 16 << 30, true},
		{"旧采样缺总量", 0, 0, false},
		{"可用量超总量", 16 << 30, 17 << 30, false},
		{"浏览器精度溢出", 9007199254740992, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := store.ExecutionNode{NodeID: "synthetic-node", Platform: "WINDOWS_AMD64", Agent: &store.ExecutionNodeAgent{
				LastHeartbeatAt: &now, EnvironmentFacts: store.AgentEnvironmentFacts{ObservedAt: now, MemoryTotalBytes: tc.total, MemoryAvailableBytes: tc.available},
			}}
			response := server.newExecutionNodeListResponse(node, server.heartbeatTTL)
			if response.AgentFacts == nil {
				t.Fatal("已观测节点未投影机器摘要")
			}
			encoded, err := json.Marshal(response.AgentFacts)
			if err != nil {
				t.Fatal(err)
			}
			var facts map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &facts); err != nil {
				t.Fatal(err)
			}
			_, hasTotal := facts["memoryTotalBytes"]
			_, hasAvailable := facts["memoryAvailableBytes"]
			if hasTotal != tc.known || hasAvailable != tc.known {
				t.Fatal("内存容量不完整或未知采样被伪造成零值")
			}
			if tc.known && (*response.AgentFacts.MemoryTotalBytes != tc.total || *response.AgentFacts.MemoryAvailableBytes != tc.available) {
				t.Fatal("内存容量投影改变了采样数值")
			}
		})
	}
}

func Test节点列表与详情接口返回同采样物理内存(t *testing.T) {
	for _, platform := range []struct{ name, os, arch string }{
		{"WINDOWS_AMD64", "WINDOWS", "AMD64"},
		{"LINUX_AMD64", "LINUX", "AMD64"},
		{"LINUX_ARM64", "LINUX", "ARM64"},
	} {
		t.Run(platform.name, func(t *testing.T) {
			for _, available := range []uint64{4 << 30, 0} {
				now := time.Now().UTC()
				manager := &recordingNodeManagementStore{nodes: map[string]store.ExecutionNode{
					"node-allowed": {
						NodeID: "node-allowed", DisplayName: "合成节点", Platform: platform.name, ManagementState: "DISABLED",
						Revision: 1, CreatedAt: now, UpdatedAt: now,
						Agent: &store.ExecutionNodeAgent{LastHeartbeatAt: &now, EnvironmentFacts: store.AgentEnvironmentFacts{
							OperatingSystem: platform.os, Architecture: platform.arch, ObservedAt: now,
							MemoryTotalBytes: 16 << 30, MemoryAvailableBytes: available,
						}},
					},
				}}
				handler := NewHandlerWithDependencies(buildinfo.Info{Version: "test"}, Dependencies{
					Identity: browserOnlyIdentityProvider{}, Authorizer: nodeManageAuthorizer{allowedID: "node-allowed"}, NodeManagement: manager,
				})
				for _, endpoint := range []string{"/api/v1/execution-nodes", "/api/v1/execution-nodes/node-allowed"} {
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, endpoint, nil))
					if response.Code != http.StatusOK {
						t.Fatalf("节点读取接口状态为 %d", response.Code)
					}
					var body struct {
						Items []executionNodeListResponse `json:"items"`
						Item  executionNodeListResponse   `json:"item"`
					}
					if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
						t.Fatal("节点读取响应无法解析")
					}
					item := body.Item
					if endpoint == "/api/v1/execution-nodes" {
						if len(body.Items) != 1 {
							t.Fatal("授权节点列表不完整")
						}
						item = body.Items[0]
					}
					if item.AgentFacts == nil || item.AgentFacts.MemoryTotalBytes == nil || item.AgentFacts.MemoryAvailableBytes == nil ||
						*item.AgentFacts.MemoryTotalBytes != 16<<30 || *item.AgentFacts.MemoryAvailableBytes != available {
						t.Fatal("列表或详情接口丢失物理内存采样，包括真实零可用量")
					}
				}
			}
		})
	}
}
