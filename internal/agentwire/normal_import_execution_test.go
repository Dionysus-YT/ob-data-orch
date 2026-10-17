package agentwire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
)

func Test普通导入协议只接受显式能力完整清单与匹配版本(t *testing.T) {
	c := importdomain.Config{Database: "synthetic", FilePath: `E:\synthetic`, Content: "DATA_ONLY", Format: "CSV", Scope: "ALL", Cleanup: "NONE"}
	files := []importfiles.File{{Path: "synthetic.csv", Kind: "DATA", Object: "synthetic", Size: 2, SHA256: strings.Repeat("a", 64)}}
	b, _ := json.Marshal(files)
	h := sha256.Sum256(b)
	input := &importfiles.ExecutionInput{Config: c, Manifest: importfiles.Manifest{Files: files, FileCount: 1, TotalBytes: 2, Digest: hex.EncodeToString(h[:])}}
	argv := []string{"--host", "127.0.0.1", "--port", "2883", "--user", "synthetic", "--database", c.Database, "--file-path", c.FilePath, "--all", "--csv", "--no-sys"}
	for _, scenario := range []string{"有效", "旧Agent", "缺少清单", "错误版本", "旁路参数"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/agent/v1/enrollments:exchange" {
					var req enrollmentExchangeRequest
					_ = json.NewDecoder(r.Body).Decode(&req)
					writeProtocolResponse(t, w, "ENROLLED", map[string]any{"agentId": req.AgentID, "nodeId": req.NodeID, "protocolVersion": Version, "replayed": false, "realExecutionEnabled": false})
					return
				}
				var req executionClaimNextRequest
				_ = json.NewDecoder(r.Body).Decode(&req)
				if scenario != "旧Agent" && (req.Payload.Capability != importdomain.TaskType || req.PayloadType != "OBLOADER_NORMAL_IMPORT_CLAIM_NEXT") {
					t.Error("导入未显式领取")
				}
				payload := map[string]any{"taskType": importdomain.TaskType, "normalImport": input, "taskId": "task-normal", "executionId": "execution-normal", "leaseId": "lease-normal", "leaseEpoch": 1, "expiresAt": time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano), "envelopeDigest": strings.Repeat("a", 64), "argv": argv, "toolVersion": importdomain.ToolVersion, "metadataVersion": importdomain.MetadataVersion, "capabilityVersion": importdomain.CapabilityVersion, "realExecutionEnabled": true}
				if scenario == "缺少清单" {
					delete(payload, "normalImport")
				}
				if scenario == "错误版本" {
					payload["metadataVersion"] = "unverified"
				}
				if scenario == "旁路参数" {
					payload["argv"] = append(append([]string(nil), argv...), "--direct")
				}
				writeProtocolResponse(t, w, "EXECUTION_CLAIMED", payload)
			}))
			defer server.Close()
			state := prepareTLSEnrollment(t, server)
			if err := state.EnsureEnrollment(context.Background()); err != nil {
				t.Fatal(err)
			}
			kind := importdomain.TaskType
			if scenario == "旧Agent" {
				kind = ""
			}
			_, found, err := state.ClaimNextExecution(context.Background(), ExecutionClaimNext{TaskType: kind, BootID: "boot", SentAt: time.Now()})
			if (err == nil && found) != (scenario == "有效") {
				t.Fatal("导入协议未失败关闭", err, found)
			}
		})
	}
}
