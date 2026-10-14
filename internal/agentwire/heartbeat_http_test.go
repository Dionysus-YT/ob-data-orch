package agentwire

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHeartbeatHTTPRejectionDistinguishesAuthentication(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 409, 422, 429, 500, 503} {
		t.Run(fmt.Sprintf("HTTP_%d", status), func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/agent/v1/heartbeats" {
					t.Error("心跳发送到意外的协议端点")
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte("synthetic-server-detail"))
			}))
			defer server.Close()
			client, err := newHTTPSClient(server.URL, writeServerCA(t, server))
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.sendHeartbeat(context.Background(), &identityState{
				ProtocolVersion: Version, AgentID: "synthetic-agent", NodeID: "synthetic-node",
				MachineCredential: []byte("synthetic-machine-credential-0123456789"),
			}, "synthetic-request", Heartbeat{BootID: "synthetic-boot", SentAt: time.Now()})
			want := ErrProtocolRejected
			if status == http.StatusUnauthorized || status == http.StatusForbidden {
				want = ErrAgentAuthenticationDenied
			} else if status >= http.StatusInternalServerError {
				want = ErrControlPlaneUnavailable
			}
			if !errors.Is(err, want) {
				t.Fatalf("错误分类不符合 HTTP %d 的预期", status)
			}
			if want == ErrProtocolRejected && (!strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", status)) || errors.Is(err, ErrAgentAuthenticationDenied)) {
				t.Fatal("协议拒绝缺少安全状态码或被误报为机器认证失败")
			}
			if strings.Contains(err.Error(), "synthetic-server-detail") || strings.Contains(err.Error(), "synthetic-machine-credential") {
				t.Fatal("诊断错误泄漏响应正文或凭据")
			}
		})
	}
}

func TestLegacyHeartbeatRejectionPreservesIdentity(t *testing.T) {
	var enrollmentRequests, heartbeatRequests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/agent/v1/enrollments:exchange":
			enrollmentRequests++
			var request enrollmentExchangeRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error("合成关联请求不能解析")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			writeProtocolResponse(t, w, "ENROLLED", map[string]any{
				"agentId": request.AgentID, "nodeId": request.NodeID, "protocolVersion": Version,
				"replayed": false, "realExecutionEnabled": false,
			})
		case "/agent/v1/heartbeats":
			heartbeatRequests++
			// 模拟旧控制面的严格 DTO：新资源字段在机器认证后被请求解码拒绝。
			var request struct {
				heartbeatRequest
				Payload struct {
					OperatingSystem string `json:"operatingSystem"`
					Architecture    string `json:"architecture"`
					AgentVersion    string `json:"agentVersion"`
					ObservedAt      string `json:"observedAt"`
					CapacityTotal   int    `json:"capacityTotal"`
					CapacityUsed    int    `json:"capacityUsed"`
				} `json:"payload"`
			}
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err == nil || !strings.Contains(err.Error(), "memoryTotalBytes") {
				t.Error("旧协议夹具没有拒绝新资源字段")
			}
			if r.Header.Get("Authorization") == "" {
				t.Error("资源心跳未携带机器认证")
			}
			w.WriteHeader(http.StatusBadRequest)
		default:
			t.Error("意外协议端点")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	store := prepareTLSEnrollment(t, server)
	if err := store.EnsureEnrollment(context.Background()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.statePath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = store.SendHeartbeat(context.Background(), Heartbeat{
		BootID: "synthetic-boot", SentAt: now,
		Facts: EnvironmentFacts{
			OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic", ObservedAt: now,
			CapacityTotal: 16, MemoryTotalBytes: 16 << 30, MemoryAvailableBytes: 8 << 30,
		},
	})
	if !errors.Is(err, ErrProtocolRejected) || errors.Is(err, ErrAgentAuthenticationDenied) {
		t.Fatal("旧控制面拒绝新字段应分类为协议不匹配")
	}
	if err := store.EnsureEnrollment(context.Background()); err != nil {
		t.Fatal("协议拒绝破坏了已关联身份")
	}
	after, err := os.ReadFile(store.statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || enrollmentRequests != 1 || heartbeatRequests != 1 {
		t.Fatal("协议拒绝改写身份、自动重新注册或重试无效请求")
	}
}
