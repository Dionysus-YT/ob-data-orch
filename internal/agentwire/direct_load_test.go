package agentwire

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
)

func Test旁路协议保留独立上下文并拒绝跨类型混合(t *testing.T) {
	config := &directload.Config{DataSourceID: "source-1", NodeID: "node-1", Database: "synthetic_db", Scenario: "PRIVATE_ODP", RPCPort: 2885, Table: "synthetic", Format: "CSV", FilePath: `E:\tmp\synthetic.csv`, Applicability: directload.Applicability{SingleTable: true, WholeTableCommit: true, NoCheckpoint: true, StructureLimits: true}}
	base := precheckExecutionContextPayload{DirectLoad: config, CheckKind: "DIRECT_LOAD_FILES", CompatibilityMode: "MYSQL", Database: config.Database, ContentKind: "DATA_ONLY", TargetPlatform: "WINDOWS_AMD64", AllowedRoots: []string{`E:\tmp`}}
	raw, _ := json.Marshal(base)
	var payload precheckExecutionContextPayload
	if err := decodeStrictJSON(raw, &payload); err != nil {
		t.Fatal(err)
	}
	g := validStoragePrecheckGrantForTest()
	g.Context = payload.toContext()
	g.CheckSet = []agentpreflight.CheckID{"INPUT_FILES"}
	if !validPrecheckGrant(g, "node-1") {
		t.Fatal("旁路上下文被丢弃或错误解释")
	}
	for _, kind := range []string{"normal", "scope", "checks", "node"} {
		t.Run(kind, func(t *testing.T) {
			copy := g
			switch kind {
			case "normal":
				copy.Context.NormalImport = &importdomain.Config{}
			case "scope":
				copy.Context.Objects = []string{"foreign"}
			case "checks":
				copy.CheckSet = agentpreflight.FixedChecks()
			case "node":
				copy.Binding.NodeID = "foreign"
			}
			if validPrecheckGrant(copy, "node-1") {
				t.Fatal("越界旁路协议被接受")
			}
		})
	}
	// 文件发现不要求尚未参与该检查的 RPC 和入口确认，完整预检仍要求这些输入。
	g.Context.DirectLoad.RPCPort = 0
	g.Context.DirectLoad.Applicability = directload.Applicability{}
	if !validPrecheckGrant(g, "node-1") {
		t.Fatal("未完成草稿的固定文件发现被阻断")
	}
	g.Context.CheckKind = "DIRECT_LOAD_PREFLIGHT"
	g.CheckSet = nil
	for _, name := range precheckcontract.DirectLoadChecks(g.Context.CheckKind) {
		g.CheckSet = append(g.CheckSet, agentpreflight.CheckID(name))
	}
	if validPrecheckGrant(g, "node-1") {
		t.Fatal("完整检查接受了未确认配置")
	}
}

func Test旁路报告拒绝普通导入成功码和缺失文件证据(t *testing.T) {
	r := agentpreflight.Report{CheckKind: "DIRECT_LOAD_FILES", PrecheckID: "synthetic-check", Succeeded: true, InputManifest: &importfiles.Manifest{}, Results: []agentpreflight.Result{{Check: "INPUT_FILES", Status: agentpreflight.StatusPassed, EvidenceCode: "DIRECT_LOAD_VERIFIED"}}}
	if !validLenientReport(r) {
		t.Fatal("旁路报告被当成普通导入")
	}
	r.Results[0].EvidenceCode = "IMPORT_VERIFIED"
	if validLenientReport(r) {
		t.Fatal("普通导入证据被旁路接受")
	}
	r.Results[0].EvidenceCode = "DIRECT_LOAD_VERIFIED"
	r.InputManifest = nil
	if validLenientReport(r) {
		t.Fatal("缺失输入证据未阻断")
	}
}

func directLoadCompleteReport() agentpreflight.Report {
	r := agentpreflight.Report{CheckKind: "DIRECT_LOAD_PREFLIGHT", PrecheckID: "synthetic-check", Succeeded: true, InputManifest: &importfiles.Manifest{}, DirectLoadConfigFingerprint: strings.Repeat("a", 64)}
	for _, name := range precheckcontract.DirectLoadChecks(r.CheckKind) {
		r.Results = append(r.Results, agentpreflight.Result{Check: agentpreflight.CheckID(name), Status: agentpreflight.StatusPassed, EvidenceCode: "DIRECT_LOAD_VERIFIED"})
	}
	return r
}

func Test旁路配置指纹只随完整检查有效安全投影提交(t *testing.T) {
	for _, kind := range []string{"valid", "missing", "uppercase", "short", "unknown-config", "failed-config", "files", "export", "normal"} {
		t.Run(kind, func(t *testing.T) {
			r := directLoadCompleteReport()
			switch kind {
			case "missing":
				r.DirectLoadConfigFingerprint = ""
			case "uppercase":
				r.DirectLoadConfigFingerprint = strings.Repeat("A", 64)
			case "short":
				r.DirectLoadConfigFingerprint = "abc"
			case "unknown-config", "failed-config":
				r.Succeeded = false
				r.Results[len(r.Results)-1].Status = agentpreflight.StatusUnknown
				r.Results[len(r.Results)-1].EvidenceCode = "DIRECT_LOAD_UNAVAILABLE"
				if kind == "failed-config" {
					r.Results[len(r.Results)-1].Status = agentpreflight.StatusFailed
					r.Results[len(r.Results)-1].EvidenceCode = "DIRECT_LOAD_REJECTED"
				}
			case "files":
				r.CheckKind = "DIRECT_LOAD_FILES"
				r.Results = []agentpreflight.Result{{Check: "INPUT_FILES", Status: agentpreflight.StatusPassed, EvidenceCode: "DIRECT_LOAD_VERIFIED"}}
			case "export":
				r = successfulPrecheckReport("synthetic-check")
				r.DirectLoadConfigFingerprint = strings.Repeat("a", 64)
			case "normal":
				r.CheckKind = "NORMAL_IMPORT_FILES"
				r.Results = []agentpreflight.Result{{Check: "INPUT_FILES", Status: agentpreflight.StatusPassed, EvidenceCode: "IMPORT_VERIFIED"}}
			}
			if validLenientReport(r) != (kind == "valid") || (agentpreflight.ValidateReport(r) == nil) != (kind == "valid") {
				t.Fatal("错误种类或无有效投影的报告携带了旁路指纹")
			}
		})
	}
}

func Test旁路完成请求传输安全配置投影指纹(t *testing.T) {
	var received bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/agent/v1/enrollments:exchange":
			var input enrollmentExchangeRequest
			if json.NewDecoder(request.Body).Decode(&input) != nil {
				t.Fatal("合成关联请求无效")
			}
			writeProtocolResponse(t, w, "ENROLLED", map[string]any{"agentId": input.AgentID, "nodeId": input.NodeID, "protocolVersion": Version, "replayed": false, "realExecutionEnabled": false})
		case "/agent/v1/prechecks/synthetic-check:complete":
			var input precheckCompletionRequest
			if json.NewDecoder(request.Body).Decode(&input) != nil || input.Payload.DirectLoadConfigFingerprint != strings.Repeat("a", 64) || len(input.Payload.Results) != 9 {
				t.Fatal("完成请求丢失安全配置投影指纹")
			}
			received = true
			writeProtocolResponse(t, w, "PRECHECK_COMPLETED", map[string]any{"status": "SUCCEEDED", "realExecutionEnabled": false})
		default:
			t.Fatal("旁路完成请求使用了未知端点")
		}
	}))
	defer server.Close()
	store := prepareTLSEnrollment(t, server)
	if err := store.EnsureEnrollment(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err := store.CompletePrecheck(context.Background(), PrecheckCompletion{BootID: "boot-1", PrecheckID: "synthetic-check", LeaseID: "lease-1", LeaseEpoch: 1, BindingDigest: precheckBindingDigest, SentAt: time.Now().UTC(), Report: directLoadCompleteReport()})
	if err != nil || state != PrecheckSucceeded || !received {
		t.Fatal("安全配置指纹没有进入固定完成协议", err)
	}
}
