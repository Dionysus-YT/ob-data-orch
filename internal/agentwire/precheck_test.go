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

	"ob-data-orch/internal/agentpreflight"
)

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
	if grant.PrecheckID != "precheck-1" || grant.LeaseID != "lease-1" || grant.LeaseEpoch != 1 || grant.Binding.PrecheckID != grant.PrecheckID || grant.Binding.NodeID != "node-1" || !validPrecheckCheckSet(grant.CheckSet) {
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
			"table":             "synthetic_table",
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

func decodePrecheckRequestBody(t *testing.T, request *http.Request) []byte {
	t.Helper()
	var content json.RawMessage
	if err := json.NewDecoder(request.Body).Decode(&content); err != nil {
		t.Fatalf("读取预检查请求失败: %v", err)
	}
	return append([]byte(nil), content...)
}
