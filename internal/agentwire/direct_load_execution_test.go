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

	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/directloadfiles"
	"ob-data-orch/internal/importfiles"
)

func Test旁路执行协议拒绝跨类型清单错表错误版本与额外参数(t *testing.T) {
	c := directload.Config{DataSourceID: "source-1", NodeID: "node-1", Database: "synthetic", FilePath: `E:\synthetic`, Scenario: "PRIVATE_ODP", RPCPort: 2885, Table: "synthetic", Format: "CSV", Applicability: directload.Applicability{SingleTable: true, WholeTableCommit: true, NoCheckpoint: true, StructureLimits: true}}
	files := []importfiles.File{{Path: "synthetic.csv", Kind: "DATA", Object: "synthetic", Size: 2, SHA256: strings.Repeat("a", 64)}}
	b, _ := json.Marshal(files)
	h := sha256.Sum256(b)
	input := &directloadfiles.ExecutionInput{Config: c, CompatibilityMode: "MYSQL", DirectLoadConfigFingerprint: strings.Repeat("9", 64), Manifest: importfiles.Manifest{Files: files, FileCount: 1, TotalBytes: 2, Digest: hex.EncodeToString(h[:])}}
	argv := []string{"--host", "127.0.0.1", "--port", "2883", "--user", "synthetic", "--database", c.Database, "--file-path", c.FilePath, "--tenant", "synthetic-tenant", "--cluster", "synthetic-cluster", "--csv", "--direct", "--rpc-port", "2885", "--table", c.Table}
	for _, scenario := range []string{"有效", "旧Agent", "缺少清单", "错误版本", "普通导入参数", "错表", "缺少模式", "混合上下文"} {
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
				if scenario != "旧Agent" && (req.Payload.Capability != directload.TaskType || req.PayloadType != "OBLOADER_DIRECT_LOAD_CLAIM_NEXT") {
					t.Error("导入未显式领取")
				}
				payload := map[string]any{"taskType": directload.TaskType, "directLoad": input, "taskId": "task-normal", "executionId": "execution-normal", "leaseId": "lease-normal", "leaseEpoch": 1, "expiresAt": time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano), "envelopeDigest": strings.Repeat("a", 64), "argv": argv, "toolVersion": directload.ToolVersion, "metadataVersion": directload.MetadataVersion, "capabilityVersion": directload.CapabilityVersion, "realExecutionEnabled": true}
				if scenario == "缺少清单" {
					delete(payload, "directLoad")
				}
				if scenario == "错误版本" {
					payload["metadataVersion"] = "unverified"
				}
				if scenario == "错表" {
					invalid := *input
					invalid.Config.Table = "other"
					payload["directLoad"] = &invalid
				}
				if scenario == "缺少模式" {
					invalid := *input
					invalid.CompatibilityMode = ""
					payload["directLoad"] = &invalid
				}
				if scenario == "混合上下文" {
					payload["normalImport"] = map[string]any{"config": map[string]any{}, "manifest": map[string]any{}}
				}
				if scenario == "普通导入参数" {
					payload["argv"] = append(append([]string(nil), argv...), "--retry")
				}
				writeProtocolResponse(t, w, "EXECUTION_CLAIMED", payload)
			}))
			defer server.Close()
			state := prepareTLSEnrollment(t, server)
			if err := state.EnsureEnrollment(context.Background()); err != nil {
				t.Fatal(err)
			}
			kind := directload.TaskType
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
