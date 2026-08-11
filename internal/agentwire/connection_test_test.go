package agentwire

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func Test基础连接测试领取使用独立端点和稳定请求标识(t *testing.T) {
	var mutex sync.Mutex
	var enrolled enrollmentExchangeRequest
	var claims []dataSourceConnectionTestClaimNextRequest
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/agent/v1/enrollments:exchange":
			if err := json.NewDecoder(request.Body).Decode(&enrolled); err != nil {
				t.Fatalf("解析关联请求失败: %v", err)
			}
			writeProtocolResponse(t, writer, "ENROLLED", map[string]any{
				"agentId": enrolled.AgentID, "nodeId": enrolled.NodeID, "protocolVersion": Version,
				"replayed": false, "realExecutionEnabled": false,
			})
		case "/agent/v1/data-source-connection-tests:claim-next":
			body := decodeConnectionTestRequestBody(t, request)
			var claim dataSourceConnectionTestClaimNextRequest
			if err := json.Unmarshal(body, &claim); err != nil {
				t.Fatalf("解析连接测试领取请求失败: %v", err)
			}
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Fatalf("解析连接测试领取信封失败: %v", err)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(envelope["payload"], &payload); err != nil {
				t.Fatalf("解析连接测试领取载荷失败: %v", err)
			}
			for _, forbidden := range []string{"connectionTestId", "leaseId", "host", "password", "sql", "path", "command"} {
				if _, found := payload[forbidden]; found {
					t.Fatalf("领取载荷不应包含 %s", forbidden)
				}
			}
			mutex.Lock()
			if request.Header.Get("Authorization") != "Bearer "+enrolled.MachineCredential {
				mutex.Unlock()
				t.Fatal("连接测试领取未携带已关联机器凭据")
			}
			claims = append(claims, claim)
			attempt := len(claims)
			mutex.Unlock()
			if attempt == 1 {
				closeResponseConnection(t, writer)
				return
			}
			writeProtocolResponse(t, writer, "DATA_SOURCE_CONNECTION_TEST_CLAIMED", connectionTestClaimResponse("connection-test-1", "lease-1", DataSourceConnectionTestG2Synthetic))
		default:
			t.Fatalf("意外 Agent 请求路径: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	store := prepareTLSEnrollment(t, server)
	if err := store.EnsureEnrollment(context.Background()); err != nil {
		t.Fatalf("EnsureEnrollment() error = %v", err)
	}
	grant, found, err := store.ClaimNextDataSourceConnectionTest(context.Background(), DataSourceConnectionTestClaimNext{
		BootID: "boot-1", SentAt: time.Date(2026, 7, 27, 1, 2, 3, 0, time.UTC),
	})
	if err != nil || !found {
		t.Fatalf("ClaimNextDataSourceConnectionTest() found=%t error=%v", found, err)
	}
	if grant.ConnectionTestID != "connection-test-1" || grant.LeaseID != "lease-1" || grant.LeaseEpoch != 1 ||
		grant.Binding.ConnectionTestID != grant.ConnectionTestID || grant.Binding.NodeID != "node-1" || grant.VerificationSource != DataSourceConnectionTestG2Synthetic {
		t.Fatalf("连接测试租约不完整: %#v", grant)
	}

	mutex.Lock()
	defer mutex.Unlock()
	if len(claims) != 2 || claims[0].RequestID == "" || claims[0].RequestID != claims[1].RequestID {
		t.Fatalf("领取重试未复用 requestId: %#v", claims)
	}
	for _, claim := range claims {
		if claim.ProtocolVersion != Version || claim.AgentID != enrolled.AgentID || claim.NodeID != enrolled.NodeID ||
			claim.BootID != "boot-1" || claim.PayloadType != "DATA_SOURCE_CONNECTION_TEST_CLAIM_NEXT" || claim.Payload.Capability != "DATA_SOURCE_CONNECTION_TEST" {
			t.Fatalf("连接测试领取信封不严格: %#v", claim)
		}
	}
}

func Test基础连接测试确认和完成使用固定载荷(t *testing.T) {
	var mutex sync.Mutex
	var enrolled enrollmentExchangeRequest
	var acknowledgements []dataSourceConnectionTestLeaseAcknowledgementRequest
	var completions []dataSourceConnectionTestCompletionRequest
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/agent/v1/enrollments:exchange":
			if err := json.NewDecoder(request.Body).Decode(&enrolled); err != nil {
				t.Fatalf("解析关联请求失败: %v", err)
			}
			writeProtocolResponse(t, writer, "ENROLLED", map[string]any{
				"agentId": enrolled.AgentID, "nodeId": enrolled.NodeID, "protocolVersion": Version,
				"replayed": false, "realExecutionEnabled": false,
			})
		case "/agent/v1/data-source-connection-tests/connection-test-1:acknowledge-lease":
			var acknowledgement dataSourceConnectionTestLeaseAcknowledgementRequest
			if err := json.NewDecoder(request.Body).Decode(&acknowledgement); err != nil {
				t.Fatalf("解析连接测试确认请求失败: %v", err)
			}
			mutex.Lock()
			acknowledgements = append(acknowledgements, acknowledgement)
			mutex.Unlock()
			writeProtocolResponse(t, writer, "DATA_SOURCE_CONNECTION_TEST_LEASE_ACKNOWLEDGED", map[string]any{"realExecutionEnabled": false})
		case "/agent/v1/data-source-connection-tests/connection-test-1:complete":
			body := decodeConnectionTestRequestBody(t, request)
			var completion dataSourceConnectionTestCompletionRequest
			if err := json.Unmarshal(body, &completion); err != nil {
				t.Fatalf("解析连接测试完成请求失败: %v", err)
			}
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Fatalf("解析连接测试完成信封失败: %v", err)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(envelope["payload"], &payload); err != nil {
				t.Fatalf("解析连接测试完成载荷失败: %v", err)
			}
			for _, forbidden := range []string{"verificationSource", "host", "username", "password", "exception", "succeeded"} {
				if _, found := payload[forbidden]; found {
					t.Fatalf("完成载荷不应包含 %s", forbidden)
				}
			}
			mutex.Lock()
			completions = append(completions, completion)
			mutex.Unlock()
			writeProtocolResponse(t, writer, "DATA_SOURCE_CONNECTION_TEST_COMPLETED", map[string]any{
				"status": "SUCCEEDED", "evidenceCode": "SYNTHETIC_OK", "verificationSource": "G2_SYNTHETIC",
				"sysVerificationStatus": "NOT_CONFIGURED", "sysEvidenceCode": "", "realExecutionEnabled": false,
			})
		default:
			t.Fatalf("意外 Agent 请求路径: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	store := prepareTLSEnrollment(t, server)
	if err := store.EnsureEnrollment(context.Background()); err != nil {
		t.Fatalf("EnsureEnrollment() error = %v", err)
	}
	now := time.Date(2026, 7, 27, 1, 2, 3, 0, time.UTC)
	acknowledgement := DataSourceConnectionTestLeaseAcknowledgement{
		BootID: "boot-1", ConnectionTestID: "connection-test-1", LeaseID: "lease-1", LeaseEpoch: 1, BindingDigest: connectionTestBindingDigest, SentAt: now,
	}
	if err := store.AcknowledgeDataSourceConnectionTestLease(context.Background(), acknowledgement); err != nil {
		t.Fatalf("AcknowledgeDataSourceConnectionTestLease() error = %v", err)
	}
	state, err := store.CompleteDataSourceConnectionTest(context.Background(), DataSourceConnectionTestCompletion{
		BootID: "boot-1", ConnectionTestID: "connection-test-1", LeaseID: "lease-1", LeaseEpoch: 1, BindingDigest: connectionTestBindingDigest,
		Status: DataSourceConnectionTestSucceeded, EvidenceCode: "SYNTHETIC_OK", VerificationSource: DataSourceConnectionTestG2Synthetic,
		SysVerificationStatus: DataSourceConnectionTestSysNotConfigured, SentAt: now,
	})
	if err != nil || state != DataSourceConnectionTestSucceeded {
		t.Fatalf("CompleteDataSourceConnectionTest() = %q, %v", state, err)
	}

	mutex.Lock()
	defer mutex.Unlock()
	if len(acknowledgements) != 1 || acknowledgements[0].RequestID == "" || acknowledgements[0].PayloadType != "DATA_SOURCE_CONNECTION_TEST_ACKNOWLEDGE_LEASE" ||
		acknowledgements[0].Payload.LeaseID != "lease-1" || acknowledgements[0].Payload.LeaseEpoch != 1 || acknowledgements[0].Payload.BindingDigest != connectionTestBindingDigest {
		t.Fatalf("连接测试确认信封不严格: %#v", acknowledgements)
	}
	if len(completions) != 1 || completions[0].RequestID == "" || completions[0].PayloadType != "DATA_SOURCE_CONNECTION_TEST_COMPLETE" ||
		completions[0].Payload.LeaseID != "lease-1" || completions[0].Payload.LeaseEpoch != 1 || completions[0].Payload.BindingDigest != connectionTestBindingDigest ||
		completions[0].Payload.Status != DataSourceConnectionTestSucceeded || completions[0].Payload.EvidenceCode != "SYNTHETIC_OK" {
		t.Fatalf("连接测试完成信封不严格: %#v", completions)
	}
}

func Test基础连接测试槽位严格绑定当前租约并在销毁时清零(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(map[string]any)
		wantFail bool
	}{
		{name: "匹配响应"},
		{name: "绑定摘要不匹配", mutate: func(payload map[string]any) { payload["bindingDigest"] = connectionTestOtherDigest }, wantFail: true},
		{name: "请求标识不匹配", mutate: func(payload map[string]any) { payload["agentRequestId"] = "different-request" }, wantFail: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var enrolled enrollmentExchangeRequest
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/agent/v1/enrollments:exchange":
					if err := json.NewDecoder(request.Body).Decode(&enrolled); err != nil {
						t.Fatalf("解析关联请求失败: %v", err)
					}
					writeProtocolResponse(t, writer, "ENROLLED", map[string]any{
						"agentId": enrolled.AgentID, "nodeId": enrolled.NodeID, "protocolVersion": Version,
						"replayed": false, "realExecutionEnabled": false,
					})
				case "/agent/v1/data-source-connection-tests/connection-test-1/secret-slots:resolve":
					var resolved dataSourceConnectionTestSecretSlotRequest
					if err := json.NewDecoder(request.Body).Decode(&resolved); err != nil {
						t.Fatalf("解析连接测试槽位请求失败: %v", err)
					}
					payload := map[string]any{
						"agentRequestId": resolved.RequestID, "connectionTestId": "connection-test-1", "leaseId": "lease-1", "leaseEpoch": 1,
						"bindingDigest": connectionTestBindingDigest, "slot": "DATABASE_CONNECTION",
						"connection":           map[string]any{"host": "synthetic.example", "port": 2883, "username": []byte("synthetic-user"), "password": []byte("synthetic-password")},
						"realExecutionEnabled": false,
					}
					if test.mutate != nil {
						test.mutate(payload)
					}
					writeProtocolResponse(t, writer, "DATA_SOURCE_CONNECTION_TEST_SECRET_SLOTS_RESOLVED", payload)
				default:
					t.Fatalf("意外 Agent 请求路径: %s", request.URL.Path)
				}
			}))
			defer server.Close()

			store := prepareTLSEnrollment(t, server)
			if err := store.EnsureEnrollment(context.Background()); err != nil {
				t.Fatalf("EnsureEnrollment() error = %v", err)
			}
			slot, err := store.ResolveDataSourceConnectionTestDatabaseConnection(context.Background(), DataSourceConnectionTestSecretSlotRequest{
				BootID: "boot-1", ConnectionTestID: "connection-test-1", LeaseID: "lease-1", LeaseEpoch: 1,
				BindingDigest: connectionTestBindingDigest, SentAt: time.Date(2026, 7, 27, 1, 2, 3, 0, time.UTC),
			})
			if test.wantFail {
				if !errors.Is(err, ErrProtocolRejected) {
					t.Fatalf("ResolveDataSourceConnectionTestDatabaseConnection() error = %v", err)
				}
				return
			}
			if err != nil || slot.Host != "synthetic.example" || slot.Port != 2883 || string(slot.Username) != "synthetic-user" || string(slot.Password) != "synthetic-password" {
				slot.Destroy()
				t.Fatalf("ResolveDataSourceConnectionTestDatabaseConnection() = %#v, %v", slot, err)
			}
			username := slot.Username
			password := slot.Password
			slot.Destroy()
			if slot.Username != nil || slot.Password != nil || !allZeroBytes(username) || !allZeroBytes(password) {
				t.Fatal("连接测试槽位销毁后仍保留秘密缓冲区")
			}
		})
	}
}

func Test基础连接测试完成在发网前拒绝未枚举结果(t *testing.T) {
	var completionRequests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/agent/v1/enrollments:exchange":
			var enrollment enrollmentExchangeRequest
			if err := json.NewDecoder(request.Body).Decode(&enrollment); err != nil {
				t.Fatalf("解析关联请求失败: %v", err)
			}
			writeProtocolResponse(t, writer, "ENROLLED", map[string]any{
				"agentId": enrollment.AgentID, "nodeId": enrollment.NodeID, "protocolVersion": Version,
				"replayed": false, "realExecutionEnabled": false,
			})
		case "/agent/v1/data-source-connection-tests/connection-test-1:complete":
			completionRequests++
			t.Fatal("未枚举结果不应发送给控制面")
		default:
			t.Fatalf("意外 Agent 请求路径: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	store := prepareTLSEnrollment(t, server)
	if err := store.EnsureEnrollment(context.Background()); err != nil {
		t.Fatalf("EnsureEnrollment() error = %v", err)
	}
	_, err := store.CompleteDataSourceConnectionTest(context.Background(), DataSourceConnectionTestCompletion{
		BootID: "boot-1", ConnectionTestID: "connection-test-1", LeaseID: "lease-1", LeaseEpoch: 1, BindingDigest: connectionTestBindingDigest,
		Status: DataSourceConnectionTestSucceeded, EvidenceCode: "JDBC_CONNECTION_OK", VerificationSource: DataSourceConnectionTestAgentJDBC,
		SentAt: time.Date(2026, 7, 27, 1, 2, 3, 0, time.UTC),
	})
	if !errors.Is(err, ErrProtocolRejected) || completionRequests != 0 {
		t.Fatalf("CompleteDataSourceConnectionTest() error=%v, requests=%d", err, completionRequests)
	}
}

const (
	connectionTestBindingDigest = "e370fe7c62388ebc429070c9ee9e044da4c315a3c6de43f09e2aa57c70d6578d"
	connectionTestOtherDigest   = "d92d1a4b520dc211f753c09f243c1a53f10a9d91575b73aa07f36d04aa249035"
	connectionTestConfigDigest  = "7e5bb836b6beeb4dc8115f2112be5ad1b917b7f6a5edffd7571e845be7b3a3a0"
)

func connectionTestClaimResponse(connectionTestID, leaseID string, source DataSourceConnectionTestVerificationSource) map[string]any {
	return map[string]any{
		"connectionTestId": connectionTestID, "leaseId": leaseID, "leaseEpoch": 1,
		"expiresAt": time.Date(2026, 7, 27, 2, 2, 3, 0, time.UTC).Format(time.RFC3339Nano),
		"binding": map[string]any{
			"connectionTestId": connectionTestID, "dataSourceId": "source-1", "connectionConfigDigest": connectionTestConfigDigest,
			"credentialRevision": 3, "nodeId": "node-1", "nodeFactsRevision": 4,
		},
		"bindingDigest": connectionTestBindingDigest, "verificationSource": source, "realExecutionEnabled": false,
	}
}

func decodeConnectionTestRequestBody(t *testing.T, request *http.Request) []byte {
	t.Helper()
	var content json.RawMessage
	if err := json.NewDecoder(request.Body).Decode(&content); err != nil {
		t.Fatalf("读取连接测试请求失败: %v", err)
	}
	return append([]byte(nil), content...)
}

func allZeroBytes(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}
