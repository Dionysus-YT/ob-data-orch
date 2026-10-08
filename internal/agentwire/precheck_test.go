package agentwire

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/commandgen"
)

func TestValidPrecheckGrantAcceptsMoreThanOneHundredObjects(t *testing.T) {
	grant := validStoragePrecheckGrantForTest()
	grant.Context.Objects = make([]string, 101)
	for index := range grant.Context.Objects {
		grant.Context.Objects[index] = "table_" + strconv.Itoa(index)
	}
	if !validPrecheckGrant(grant, "node-1") {
		t.Fatal("101 个冻结对象应通过 Agent 租约结构校验")
	}
}

func TestClaimNextPrecheckRetriesWithSameRequestIDAndMachineCredential(t *testing.T) {
	var mutex sync.Mutex
	var enrolled enrollmentExchangeRequest
	var claims []precheckClaimNextRequest
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
		case "/agent/v1/prechecks:claim-next":
			body := decodePrecheckRequestBody(t, request)
			var claim precheckClaimNextRequest
			if err := json.Unmarshal(body, &claim); err != nil {
				t.Fatalf("解析预检查领取请求失败: %v", err)
			}
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Fatalf("解析预检查领取信封失败: %v", err)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(envelope["payload"], &payload); err != nil {
				t.Fatalf("解析预检查领取载荷失败: %v", err)
			}
			for _, forbidden := range []string{"precheckId", "leaseId", "path", "command", "sql"} {
				if _, found := payload[forbidden]; found {
					t.Fatalf("claim-next 载荷不应包含 %s", forbidden)
				}
			}
			mutex.Lock()
			if request.Header.Get("Authorization") != "Bearer "+enrolled.MachineCredential {
				mutex.Unlock()
				t.Fatal("预检查领取未携带已关联机器凭据")
			}
			claims = append(claims, claim)
			attempt := len(claims)
			mutex.Unlock()
			if attempt == 1 {
				closeResponseConnection(t, writer)
				return
			}
			response := precheckClaimResponse("precheck-1", "lease-1")
			response["realExecutionEnabled"] = true
			writeProtocolResponse(t, writer, "PRECHECK_CLAIMED", response)
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
	grant, found, err := store.ClaimNextPrecheck(context.Background(), PrecheckClaimNext{
		BootID: "boot-1", SentAt: now,
	})
	if err != nil || !found {
		t.Fatalf("ClaimNextPrecheck() found=%t error=%v", found, err)
	}
	if grant.PrecheckID != "precheck-1" || grant.LeaseID != "lease-1" || grant.LeaseEpoch != 1 || grant.Binding.PrecheckID != grant.PrecheckID || grant.Binding.NodeID != "node-1" || !validPrecheckCheckSet(grant.Context.OutputKind, grant.CheckSet) {
		t.Fatalf("预检查租约不完整: %#v", grant)
	}

	mutex.Lock()
	defer mutex.Unlock()
	if len(claims) != 2 || claims[0].RequestID == "" || claims[0].RequestID != claims[1].RequestID {
		t.Fatalf("领取重试未复用 requestId: %#v", claims)
	}
	for _, claim := range claims {
		if claim.ProtocolVersion != Version || claim.AgentID != enrolled.AgentID || claim.NodeID != enrolled.NodeID || claim.BootID != "boot-1" || claim.PayloadType != "EXPORT_PREFLIGHT_CLAIM_NEXT" || claim.Payload.Capability != string(agentpreflight.CapabilityExportPreflight) {
			t.Fatalf("预检查领取信封不严格: %#v", claim)
		}
	}
}

func TestClaimNextPrecheckReturnsNoWorkForNoContent(t *testing.T) {
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
		case "/agent/v1/prechecks:claim-next":
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("意外 Agent 请求路径: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	store := prepareTLSEnrollment(t, server)
	if err := store.EnsureEnrollment(context.Background()); err != nil {
		t.Fatalf("EnsureEnrollment() error = %v", err)
	}
	grant, found, err := store.ClaimNextPrecheck(context.Background(), PrecheckClaimNext{
		BootID: "boot-1", SentAt: time.Date(2026, 7, 27, 1, 2, 3, 0, time.UTC),
	})
	if err != nil || found || grant.PrecheckID != "" || grant.LeaseID != "" || grant.LeaseEpoch != 0 || !grant.ExpiresAt.IsZero() || grant.Binding.PrecheckID != "" || len(grant.CheckSet) != 0 || len(grant.Context.AllowedRoots) != 0 {
		t.Fatalf("ClaimNextPrecheck() = %#v, %t, %v; want zero grant, false, nil", grant, found, err)
	}
}

func TestAcknowledgeAndCompletePrecheckUseFixedPayloads(t *testing.T) {
	var mutex sync.Mutex
	var enrolled enrollmentExchangeRequest
	var acknowledgements []precheckLeaseAcknowledgementRequest
	var completions []precheckCompletionRequest
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
		case "/agent/v1/prechecks/precheck-1:acknowledge-lease":
			body := decodePrecheckRequestBody(t, request)
			var acknowledgement precheckLeaseAcknowledgementRequest
			if err := json.Unmarshal(body, &acknowledgement); err != nil {
				t.Fatalf("解析预检查确认请求失败: %v", err)
			}
			mutex.Lock()
			if request.Header.Get("Authorization") != "Bearer "+enrolled.MachineCredential {
				mutex.Unlock()
				t.Fatal("预检查租约确认未携带已关联机器凭据")
			}
			acknowledgements = append(acknowledgements, acknowledgement)
			mutex.Unlock()
			writeProtocolResponse(t, writer, "PRECHECK_LEASE_ACKNOWLEDGED", map[string]any{"realExecutionEnabled": true})
		case "/agent/v1/prechecks/precheck-1:complete":
			body := decodePrecheckRequestBody(t, request)
			var completion precheckCompletionRequest
			if err := json.Unmarshal(body, &completion); err != nil {
				t.Fatalf("解析预检查完成请求失败: %v", err)
			}
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Fatalf("解析预检查完成信封失败: %v", err)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(envelope["payload"], &payload); err != nil {
				t.Fatalf("解析预检查完成载荷失败: %v", err)
			}
			if _, found := payload["succeeded"]; found {
				t.Fatal("预检查完成载荷不应发送独立成功布尔值")
			}
			var results []map[string]json.RawMessage
			if err := json.Unmarshal(payload["results"], &results); err != nil {
				t.Fatalf("解析预检查完成结果载荷失败: %v", err)
			}
			if len(results) != len(agentpreflight.FixedChecks()) {
				t.Fatalf("预检查完成结果数量 = %d", len(results))
			}
			for _, result := range results {
				if result["check"] == nil || result["status"] == nil || result["evidenceCode"] == nil || result["Check"] != nil || result["Status"] != nil || result["EvidenceCode"] != nil {
					t.Fatalf("预检查结果未使用严格 lowerCamel 字段: %#v", result)
				}
			}
			mutex.Lock()
			if request.Header.Get("Authorization") != "Bearer "+enrolled.MachineCredential {
				mutex.Unlock()
				t.Fatal("预检查完成未携带已关联机器凭据")
			}
			completions = append(completions, completion)
			mutex.Unlock()
			writeProtocolResponse(t, writer, "PRECHECK_COMPLETED", map[string]any{"status": "SUCCEEDED", "realExecutionEnabled": true})
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
	acknowledgement := PrecheckLeaseAcknowledgement{
		BootID: "boot-1", PrecheckID: "precheck-1", LeaseID: "lease-1", LeaseEpoch: 1, BindingDigest: precheckBindingDigest, SentAt: now,
	}
	if err := store.AcknowledgePrecheckLease(context.Background(), acknowledgement); err != nil {
		t.Fatalf("AcknowledgePrecheckLease() error = %v", err)
	}
	report := successfulPrecheckReport("precheck-1")
	state, err := store.CompletePrecheck(context.Background(), PrecheckCompletion{
		BootID: "boot-1", PrecheckID: "precheck-1", LeaseID: "lease-1", LeaseEpoch: 1, BindingDigest: precheckBindingDigest, SentAt: now, Report: report,
	})
	if err != nil {
		t.Fatalf("CompletePrecheck() error = %v", err)
	}
	if state != PrecheckSucceeded {
		t.Fatalf("预检查完成状态 = %q", state)
	}

	mutex.Lock()
	defer mutex.Unlock()
	if len(acknowledgements) != 1 || acknowledgements[0].RequestID == "" || acknowledgements[0].ProtocolVersion != Version || acknowledgements[0].PayloadType != "EXPORT_PREFLIGHT_ACKNOWLEDGE_LEASE" || acknowledgements[0].Payload.LeaseID != "lease-1" || acknowledgements[0].Payload.LeaseEpoch != 1 || acknowledgements[0].Payload.BindingDigest != precheckBindingDigest {
		t.Fatalf("预检查确认信封不严格: %#v", acknowledgements)
	}
	if len(completions) != 1 || completions[0].RequestID == "" || completions[0].ProtocolVersion != Version || completions[0].PayloadType != "EXPORT_PREFLIGHT_COMPLETE" || completions[0].Payload.LeaseID != "lease-1" || completions[0].Payload.LeaseEpoch != 1 || completions[0].Payload.BindingDigest != precheckBindingDigest || len(completions[0].Payload.Results) != len(agentpreflight.FixedChecks()) {
		t.Fatalf("预检查完成信封不严格: %#v", completions)
	}
	for index, check := range agentpreflight.FixedChecks() {
		if completions[0].Payload.Results[index].Check != check || completions[0].Payload.Results[index].Status != agentpreflight.StatusPassed {
			t.Fatalf("预检查完成结果集不严格: %#v", completions[0].Payload.Results)
		}
	}
}

func TestCompletePrecheckAcceptsExecutionEnabledResponse(t *testing.T) {
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
		case "/agent/v1/prechecks/precheck-1:complete":
			writeProtocolResponse(t, writer, "PRECHECK_COMPLETED", map[string]any{"status": "SUCCEEDED", "realExecutionEnabled": true})
		default:
			t.Fatalf("意外 Agent 请求路径: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	store := prepareTLSEnrollment(t, server)
	if err := store.EnsureEnrollment(context.Background()); err != nil {
		t.Fatalf("EnsureEnrollment() error = %v", err)
	}
	state, err := store.CompletePrecheck(context.Background(), PrecheckCompletion{
		BootID: "boot-1", PrecheckID: "precheck-1", LeaseID: "lease-1", LeaseEpoch: 1, BindingDigest: precheckBindingDigest,
		SentAt: time.Date(2026, 7, 27, 1, 2, 3, 0, time.UTC), Report: successfulPrecheckReport("precheck-1"),
	})
	if err != nil || state != PrecheckSucceeded {
		t.Fatalf("CompletePrecheck() = %q, %v; want SUCCEEDED, nil", state, err)
	}
}

func TestCompletePrecheckRejectsInvalidReportBeforeNetworkRequest(t *testing.T) {
	var mutex sync.Mutex
	completionRequests := 0
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
		case "/agent/v1/prechecks/precheck-1:complete":
			mutex.Lock()
			completionRequests++
			mutex.Unlock()
			t.Fatal("无效检查报告不应发送到控制面")
		default:
			t.Fatalf("意外 Agent 请求路径: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	store := prepareTLSEnrollment(t, server)
	if err := store.EnsureEnrollment(context.Background()); err != nil {
		t.Fatalf("EnsureEnrollment() error = %v", err)
	}
	report := successfulPrecheckReport("precheck-1")
	report.Results = report.Results[:len(report.Results)-1]
	_, err := store.CompletePrecheck(context.Background(), PrecheckCompletion{
		BootID: "boot-1", PrecheckID: "precheck-1", LeaseID: "lease-1", LeaseEpoch: 1, BindingDigest: precheckBindingDigest,
		SentAt: time.Date(2026, 7, 27, 1, 2, 3, 0, time.UTC), Report: report,
	})
	if !errors.Is(err, ErrProtocolRejected) {
		t.Fatalf("CompletePrecheck() error = %v, want ErrProtocolRejected", err)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if completionRequests != 0 {
		t.Fatalf("无效报告意外发送了 %d 次", completionRequests)
	}
}

func TestResolvePrecheckDatabaseConnectionBindsResponseToCurrentLease(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(map[string]any)
		wantFail bool
	}{
		{name: "匹配响应"},
		{name: "租约摘要不匹配", mutate: func(payload map[string]any) { payload["bindingDigest"] = "different-binding" }, wantFail: true},
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
				case "/agent/v1/prechecks/precheck-1/secret-slots:resolve":
					var resolved precheckSecretSlotRequest
					if err := json.NewDecoder(request.Body).Decode(&resolved); err != nil {
						t.Fatalf("解析秘密槽位请求失败: %v", err)
					}
					if request.Header.Get("Authorization") != "Bearer "+enrolled.MachineCredential {
						t.Fatal("秘密槽位请求未携带已关联机器凭据")
					}
					payload := map[string]any{
						"agentRequestId": resolved.RequestID,
						"precheckId":     "precheck-1",
						"leaseId":        "lease-1",
						"leaseEpoch":     1,
						"bindingDigest":  precheckBindingDigest,
						"slot":           "DATABASE_CONNECTION",
						"connection": map[string]any{
							"host": "synthetic.example", "port": 2883,
							"username": []byte("synthetic-user"), "password": []byte("synthetic-password"),
						},
						"realExecutionEnabled": true,
					}
					if test.mutate != nil {
						test.mutate(payload)
					}
					writeProtocolResponse(t, writer, "PRECHECK_SECRET_SLOTS_RESOLVED", payload)
				default:
					t.Fatalf("意外 Agent 请求路径: %s", request.URL.Path)
				}
			}))
			defer server.Close()

			store := prepareTLSEnrollment(t, server)
			if err := store.EnsureEnrollment(context.Background()); err != nil {
				t.Fatalf("EnsureEnrollment() error = %v", err)
			}
			connection, err := store.ResolvePrecheckDatabaseConnection(context.Background(), PrecheckSecretSlotRequest{
				BootID: "boot-1", PrecheckID: "precheck-1", LeaseID: "lease-1", LeaseEpoch: 1,
				BindingDigest: precheckBindingDigest, SentAt: time.Date(2026, 7, 27, 1, 2, 3, 0, time.UTC),
			})
			if test.wantFail {
				if !errors.Is(err, ErrProtocolRejected) {
					t.Fatalf("ResolvePrecheckDatabaseConnection() error = %v, want ErrProtocolRejected", err)
				}
				return
			}
			if err != nil || connection.Host != "synthetic.example" || connection.Port != 2883 || string(connection.Username) != "synthetic-user" || string(connection.Password) != "synthetic-password" {
				connection.Destroy()
				t.Fatalf("ResolvePrecheckDatabaseConnection() = %#v, %v", connection, err)
			}
			connection.Destroy()
			if connection.Username != nil || connection.Password != nil {
				t.Fatal("连接槽位销毁后仍保留凭据缓冲区")
			}
		})
	}
}

func TestResolvePrecheckStorageCredentialUsesDedicatedSlot(t *testing.T) {
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
		case "/agent/v1/prechecks/precheck-1/secret-slots:resolve":
			var resolved precheckSecretSlotRequest
			if err := json.NewDecoder(request.Body).Decode(&resolved); err != nil {
				t.Fatalf("解析存储秘密槽位请求失败: %v", err)
			}
			if resolved.Payload.Slot != "STORAGE_CREDENTIAL" || request.Header.Get("Authorization") != "Bearer "+enrolled.MachineCredential {
				t.Fatal("存储秘密槽位请求未使用受控槽位或机器凭据")
			}
			writeProtocolResponse(t, writer, "PRECHECK_SECRET_SLOTS_RESOLVED", map[string]any{
				"agentRequestId": resolved.RequestID, "precheckId": "precheck-1", "leaseId": "lease-1", "leaseEpoch": 1,
				"bindingDigest": precheckBindingDigest, "slot": "STORAGE_CREDENTIAL",
				"storageCredential": map[string]any{
					"provider": "OSS", "accessKey": []byte("synthetic-access"), "secretKey": []byte("synthetic-secret"),
				},
				"realExecutionEnabled": false,
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
	storageCredential, err := store.ResolvePrecheckStorageCredential(context.Background(), PrecheckSecretSlotRequest{
		BootID: "boot-1", PrecheckID: "precheck-1", LeaseID: "lease-1", LeaseEpoch: 1,
		BindingDigest: precheckBindingDigest, SentAt: time.Date(2026, 7, 27, 1, 2, 3, 0, time.UTC),
	})
	if err != nil || storageCredential.Provider != "OSS" || string(storageCredential.AccessKey) != "synthetic-access" || string(storageCredential.SecretKey) != "synthetic-secret" {
		storageCredential.Destroy()
		t.Fatalf("ResolvePrecheckStorageCredential() = %#v, %v", storageCredential, err)
	}
	storageCredential.Destroy()
	if storageCredential.AccessKey != nil || storageCredential.SecretKey != nil {
		t.Fatal("存储凭据槽位销毁后仍保留凭据缓冲区")
	}
}

const precheckBindingDigest = "1ba7902c43683fd0c978e6f9402f98626e643f2031dc50458e4db7502513b1da"

func precheckClaimResponse(precheckID, leaseID string) map[string]any {
	return map[string]any{
		"precheckId": precheckID,
		"leaseId":    leaseID,
		"leaseEpoch": 1,
		"expiresAt":  time.Date(2026, 7, 27, 2, 2, 3, 0, time.UTC).Format(time.RFC3339Nano),
		"binding": map[string]any{
			"precheckId": precheckID, "nodeId": "node-1", "draftRevision": 2,
			"configFingerprint": "synthetic-fingerprint", "credentialRevision": 3, "nodeFactsVersion": 4,
		},
		"bindingDigest": precheckBindingDigest,
		"checkSet":      agentpreflight.FixedChecks(),
		"executionContext": map[string]any{
			"compatibilityMode": "MYSQL",
			"database":          "synthetic_db",
			"objects":           []string{"synthetic_table"},
			"contentKind":       "DATA_ONLY",
			"outputPath":        "/E:/tmp/output",
			"targetPlatform":    "WINDOWS_AMD64",
			"allowedRoots":      []string{`E:\tmp`},
		},
		"realExecutionEnabled": false,
	}
}

func successfulPrecheckReport(precheckID string) agentpreflight.Report {
	checks := agentpreflight.FixedChecks()
	results := make([]agentpreflight.Result, 0, len(checks))
	for _, check := range checks {
		results = append(results, agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: "SYNTHETIC_OK"})
	}
	return agentpreflight.Report{PrecheckID: precheckID, Succeeded: true, Results: results}
}

// TestClaimNextPrecheck接受存储形态上下文与检查清单 验证 EX-I6 对象存储预检查租约：
// 受控存储目标段、输出类型与存储形态检查清单必须原样解析且通过失败关闭校验。
func TestClaimNextPrecheck接受存储形态上下文与检查清单(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/agent/v1/enrollments:exchange":
			var enrolled enrollmentExchangeRequest
			if err := json.NewDecoder(request.Body).Decode(&enrolled); err != nil {
				t.Fatalf("解析关联请求失败: %v", err)
			}
			writeProtocolResponse(t, writer, "ENROLLED", map[string]any{
				"agentId": enrolled.AgentID, "nodeId": enrolled.NodeID, "protocolVersion": Version,
				"replayed": false, "realExecutionEnabled": false,
			})
		case "/agent/v1/prechecks:claim-next":
			response := precheckClaimResponse("precheck-storage", "lease-storage")
			response["checkSet"] = agentpreflight.ChecksForOutputKind(agentpreflight.OutputKindOSS)
			response["executionContext"] = map[string]any{
				"compatibilityMode": "MYSQL",
				"database":          "synthetic_db",
				"objects":           []string{"synthetic_table"},
				"contentKind":       "DATA_ONLY",
				"outputPath":        "oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou.aliyuncs.com",
				"targetPlatform":    "WINDOWS_AMD64",
				"allowedRoots":      []string{`E:\tmp`},
				"outputKind":        "OSS",
				"storageTarget": map[string]any{
					"provider": "OSS", "uri": "oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou.aliyuncs.com",
					"endpoint": "oss-cn-hangzhou.aliyuncs.com", "tmpPath": "/E:/tmp/upload",
				},
			}
			writeProtocolResponse(t, writer, "PRECHECK_CLAIMED", response)
		default:
			t.Fatalf("意外 Agent 请求路径: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	store := prepareTLSEnrollment(t, server)
	if err := store.EnsureEnrollment(context.Background()); err != nil {
		t.Fatalf("EnsureEnrollment() error = %v", err)
	}
	grant, found, err := store.ClaimNextPrecheck(context.Background(), PrecheckClaimNext{
		BootID: "boot-1", SentAt: time.Date(2026, 7, 27, 1, 2, 3, 0, time.UTC),
	})
	if err != nil || !found {
		t.Fatalf("ClaimNextPrecheck() found=%t error=%v", found, err)
	}
	if grant.Context.OutputKind != agentpreflight.OutputKindOSS || grant.Context.StorageTarget == nil ||
		grant.Context.StorageTarget.Endpoint != "oss-cn-hangzhou.aliyuncs.com" || grant.Context.StorageTarget.TmpPath != "/E:/tmp/upload" {
		t.Fatalf("存储上下文解析失败: %#v", grant.Context)
	}
	if len(grant.CheckSet) != len(agentpreflight.ChecksForOutputKind(agentpreflight.OutputKindOSS)) || grant.CheckSet[4] != agentpreflight.CheckStorageConnectivity || grant.CheckSet[5] != agentpreflight.CheckStorageAuth {
		t.Fatalf("存储检查清单解析失败: %#v", grant.CheckSet)
	}
}

// TestCompletePrecheck接受存储形态结果 验证 Agent 客户端对存储形态报告的宽松结构复核
// （顺序与形态的权威校验由控制面按冻结草稿执行）。
func TestCompletePrecheck接受存储形态结果(t *testing.T) {
	report := agentpreflight.Report{PrecheckID: "precheck-storage", Succeeded: true, Results: []agentpreflight.Result{
		{Check: agentpreflight.CheckDatabaseConnectivity, Status: agentpreflight.StatusPassed, EvidenceCode: "DATABASE_CONNECTED"},
		{Check: agentpreflight.CheckObjectAccess, Status: agentpreflight.StatusPassed, EvidenceCode: "OBJECT_ACCESSIBLE"},
		{Check: agentpreflight.CheckToolEnvironment, Status: agentpreflight.StatusPassed, EvidenceCode: "TOOL_RUNTIME_READY"},
		{Check: agentpreflight.CheckAvailableSpace, Status: agentpreflight.StatusPassed, EvidenceCode: "OUTPUT_SPACE_SUFFICIENT"},
		{Check: agentpreflight.CheckStorageConnectivity, Status: agentpreflight.StatusPassed, EvidenceCode: "STORAGE_ENDPOINT_REACHABLE"},
		{Check: agentpreflight.CheckStorageAuth, Status: agentpreflight.StatusPassed, EvidenceCode: "STORAGE_CREDENTIAL_VERIFIED"},
	}}
	input := PrecheckCompletion{BootID: "boot-1", PrecheckID: "precheck-storage", LeaseID: "lease-storage", LeaseEpoch: 1, BindingDigest: precheckBindingDigest, SentAt: time.Date(2026, 7, 27, 1, 2, 3, 0, time.UTC), Report: report}
	if !validPrecheckCompletion(input) {
		t.Fatal("存储形态报告被客户端校验拒绝")
	}
	// 伪造成功结论（结果含 UNKNOWN 但 Succeeded=true）必须拒绝。
	forged := report
	forged.Results[4].Status = agentpreflight.StatusUnknown
	forged.Results[4].EvidenceCode = "STORAGE_CONNECTIVITY_UNAVAILABLE"
	input.Report = forged
	if validPrecheckCompletion(input) {
		t.Fatal("伪造成功的存储形态报告被接受")
	}
}

// TestValidPrecheckGrant拒绝畸形存储上下文 验证 Agent 信封侧的结构边界。
// Agent 不重新解释 URI 的 scheme/查询语义，但仍必须拒绝缺段、控制字符、路径和输出类型漂移。
func TestValidPrecheckGrant拒绝畸形存储上下文(t *testing.T) {
	if !validPrecheckGrant(validStoragePrecheckGrantForTest(), "node-1") {
		t.Fatal("有效对象存储预检查租约未通过结构校验")
	}

	cases := map[string]func(*PrecheckGrant){
		"缺少存储目标": func(grant *PrecheckGrant) {
			grant.Context.StorageTarget = nil
		},
		"provider 与输出类型不一致": func(grant *PrecheckGrant) {
			grant.Context.StorageTarget.Provider = "S3"
		},
		"输出路径含控制字符": func(grant *PrecheckGrant) {
			grant.Context.OutputPath += "\n"
		},
		"存储 URI 含控制字符": func(grant *PrecheckGrant) {
			grant.Context.StorageTarget.URI += "\x00"
		},
		"endpoint 含控制字符": func(grant *PrecheckGrant) {
			grant.Context.StorageTarget.Endpoint += "\r"
		},
		"携带本地日志路径": func(grant *PrecheckGrant) {
			grant.Context.LogPath = "/E:/tmp/export.log"
		},
		"tmp 路径非目标平台绝对路径": func(grant *PrecheckGrant) {
			grant.Context.StorageTarget.TmpPath = "relative/tmp"
		},
		"存储输出路径为空": func(grant *PrecheckGrant) {
			grant.Context.OutputPath = ""
		},
		"检查清单漂移": func(grant *PrecheckGrant) {
			grant.CheckSet = agentpreflight.ChecksForOutputKind(agentpreflight.OutputKindLocal)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			grant := validStoragePrecheckGrantForTest()
			mutate(&grant)
			if validPrecheckGrant(grant, "node-1") {
				t.Fatal("畸形对象存储预检查租约被接受")
			}
		})
	}
}

func validStoragePrecheckGrantForTest() PrecheckGrant {
	return PrecheckGrant{
		PrecheckID:    "precheck-storage",
		LeaseID:       "lease-storage",
		LeaseEpoch:    1,
		ExpiresAt:     time.Date(2026, 7, 27, 2, 2, 3, 0, time.UTC),
		BindingDigest: precheckBindingDigest,
		Binding: agentstate.PrecheckBinding{
			PrecheckID:         "precheck-storage",
			NodeID:             "node-1",
			DraftRevision:      2,
			ConfigFingerprint:  "synthetic-fingerprint",
			CredentialRevision: 3,
			NodeFactsVersion:   4,
		},
		CheckSet: agentpreflight.ChecksForOutputKind(agentpreflight.OutputKindOSS),
		Context: PrecheckExecutionContext{
			CompatibilityMode: "MYSQL",
			Database:          "synthetic_db",
			Objects:           []string{"synthetic_table"},
			ContentKind:       "DATA_ONLY",
			OutputPath:        "oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou.aliyuncs.com",
			TargetPlatform:    commandgen.PlatformWindowsAMD64,
			AllowedRoots:      []string{`E:\tmp`},
			OutputKind:        agentpreflight.OutputKindOSS,
			StorageTarget: &PrecheckStorageTarget{
				Provider: "OSS",
				URI:      "oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou.aliyuncs.com",
				Endpoint: "oss-cn-hangzhou.aliyuncs.com",
				TmpPath:  "/E:/tmp/upload",
			},
		},
	}
}

func decodePrecheckRequestBody(t *testing.T, request *http.Request) []byte {
	t.Helper()
	var content json.RawMessage
	if err := json.NewDecoder(request.Body).Decode(&content); err != nil {
		t.Fatalf("读取预检查请求失败: %v", err)
	}
	return append([]byte(nil), content...)
}
