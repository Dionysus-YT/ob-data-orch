package agentwire

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestValidDatabaseConnectionSlotStorageSegment 验证执行槽位中的对象存储凭据段
// 在协议解析层即失败关闭：provider 白名单、非空长度上限与禁止字节都必须校验。
func TestValidDatabaseConnectionSlotStorageSegment(t *testing.T) {
	t.Parallel()
	base := DatabaseConnectionSlot{Host: "127.0.0.1", Port: 2881, Username: []byte("user"), Password: []byte("password")}
	if !validDatabaseConnectionSlot(base) {
		t.Fatal("本地槽位必须有效")
	}
	base.StorageCredential = &StorageCredentialSlot{Provider: "OSS", AccessKey: []byte("access"), SecretKey: []byte("secret")}
	if !validDatabaseConnectionSlot(base) {
		t.Fatal("OSS 存储凭据段必须有效")
	}
	for _, provider := range []string{"S3", "COS", "OBS"} {
		withProvider := base
		withProvider.StorageCredential = &StorageCredentialSlot{Provider: provider, AccessKey: []byte("access"), SecretKey: []byte("secret")}
		if !validDatabaseConnectionSlot(withProvider) {
			t.Fatalf("provider %s 存储凭据段必须有效", provider)
		}
	}
	for name, mutate := range map[string]func(*DatabaseConnectionSlot){
		"未知 provider":      func(c *DatabaseConnectionSlot) { c.StorageCredential.Provider = "FTP" },
		"空 access-key":     func(c *DatabaseConnectionSlot) { c.StorageCredential.AccessKey = nil },
		"空 secret-key":     func(c *DatabaseConnectionSlot) { c.StorageCredential.SecretKey = nil },
		"access-key 超长":    func(c *DatabaseConnectionSlot) { c.StorageCredential.AccessKey = bytes.Repeat([]byte{'a'}, 4097) },
		"access-key 含换行":   func(c *DatabaseConnectionSlot) { c.StorageCredential.AccessKey = []byte("ac\ncess") },
		"secret-key 含 NUL": func(c *DatabaseConnectionSlot) { c.StorageCredential.SecretKey = []byte{'s', 0, 'e'} },
	} {
		invalid := base
		invalid.StorageCredential = &StorageCredentialSlot{Provider: base.StorageCredential.Provider, AccessKey: append([]byte(nil), base.StorageCredential.AccessKey...), SecretKey: append([]byte(nil), base.StorageCredential.SecretKey...)}
		mutate(&invalid)
		if validDatabaseConnectionSlot(invalid) {
			t.Fatalf("%s 必须被拒绝", name)
		}
	}
}

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
