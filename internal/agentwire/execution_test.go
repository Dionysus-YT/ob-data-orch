package agentwire

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClaimNextExecution接受完整冻结信封(t *testing.T) {
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
		case "/agent/v1/executions:claim-next":
			var claim executionClaimNextRequest
			if err := json.NewDecoder(request.Body).Decode(&claim); err != nil {
				t.Fatalf("解析执行领取请求失败: %v", err)
			}
			if claim.PayloadType != "OBDUMPER_EXPORT_CLAIM_NEXT" || claim.Payload.Capability != "OBDUMPER_EXPORT" {
				t.Fatalf("执行领取信封不严格: %#v", claim)
			}
			writeProtocolResponse(t, writer, "EXECUTION_CLAIMED", map[string]any{
				"taskId": "task-1", "executionId": "execution-1", "leaseId": "lease-1", "leaseEpoch": 1,
				"expiresAt": time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano), "envelopeDigest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"argv":        []string{"-h192.0.2.1", "-P2881", "-usynthetic_user@synthetic_tenant", "--database", "synthetic_db", "--table", "synthetic_table", "--csv", "--file-path", "/E:/tmp/output"},
				"toolVersion": "4.3.5-RELEASE", "metadataVersion": "obdumper-4.3.5-slice-v3", "capabilityVersion": "export-odp-single-table-csv-v1", "realExecutionEnabled": true,
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
	grant, found, err := store.ClaimNextExecution(context.Background(), ExecutionClaimNext{BootID: "boot-1", SentAt: time.Now().UTC()})
	if err != nil || !found || grant.TaskID != "task-1" || grant.ExecutionID != "execution-1" || len(grant.Argv) != 10 {
		t.Fatalf("ClaimNextExecution() = %#v, %t, %v", grant, found, err)
	}
}
