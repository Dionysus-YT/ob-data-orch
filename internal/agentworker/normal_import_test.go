package agentworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
)

type normalImportProtocol struct {
	protocolStub
	report agentpreflight.Report
}

func (p *normalImportProtocol) CompletePrecheck(ctx context.Context, input agentwire.PrecheckCompletion) (agentwire.PrecheckState, error) {
	p.report = input.Report
	return p.protocolStub.CompletePrecheck(ctx, input)
}

type normalImportProbe struct {
	checks        []agentpreflight.CheckID
	scans         int
	rejectTool    bool
	unknownWrites bool
}

func (p *normalImportProbe) Probe(_ context.Context, check agentpreflight.CheckID, _ agentpreflight.Request) (agentpreflight.Result, error) {
	p.checks = append(p.checks, check)
	if check == agentpreflight.CheckToolEnvironment && p.rejectTool {
		return agentpreflight.Result{}, agentpreflight.ErrProbeFailed
	}
	if (check == "WRITE_ACCESS" || check == "INPUT_MAPPING") && p.unknownWrites {
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: "IMPORT_UNAVAILABLE"}, nil
	}
	return syntheticPassed(check), nil
}
func (p *normalImportProbe) ScanImportInput(_ context.Context, r agentpreflight.Request) (importfiles.Manifest, error) {
	p.scans++
	files := []importfiles.File{{Path: "synthetic.csv", Kind: "DATA", Object: "synthetic", Size: 2, Modified: 1, SHA256: hex.EncodeToString(make([]byte, 32))}}
	raw, _ := json.Marshal(files)
	hash := sha256.Sum256(raw)
	return importfiles.Manifest{Files: files, FileCount: 1, TotalBytes: 2, Digest: hex.EncodeToString(hash[:])}, nil
}
func normalGrant(now time.Time, kind string) agentwire.PrecheckGrant {
	grant := testGrant(now)
	grant.CheckSet = nil
	for _, name := range precheckcontract.NormalImportChecks(kind) {
		grant.CheckSet = append(grant.CheckSet, agentpreflight.CheckID(name))
	}
	grant.Context = agentwire.PrecheckExecutionContext{CheckKind: kind, NormalImport: &importdomain.Config{DataSourceID: "source-1", NodeID: "node-1", Database: "synthetic_db", FilePath: `E:\tmp\input`, Content: "DATA_ONLY", Format: "CSV", Scope: "ALL", Cleanup: "NONE", TaskName: "合成任务", Mapping: &importdomain.Mapping{Kind: "POSITION"}, Objects: map[string]string{}}, CompatibilityMode: "MYSQL", Database: "synthetic_db", ContentKind: "DATA_ONLY", TargetPlatform: grant.Context.TargetPlatform, AllowedRoots: []string{`E:\tmp`}}
	return grant
}

func Test普通导入文件发现不执行其他检查或解析秘密(t *testing.T) {
	now := time.Now().UTC()
	protocol := &normalImportProtocol{protocolStub: protocolStub{grant: normalGrant(now, "NORMAL_IMPORT_FILES")}}
	probe := &normalImportProbe{}
	worker := testWorker(protocol, &testClock{now: now}, func(SecretResolver) agentpreflight.Probe { return probe })
	outcome, found, err := worker.RunNext(context.Background())
	if err != nil || !found || !outcome.Report.Succeeded || probe.scans != 1 || len(probe.checks) != 0 || protocol.resolveCount() != 0 {
		t.Fatal("文件发现越界或失败", err)
	}
	if protocol.report.CheckKind != "NORMAL_IMPORT_FILES" || protocol.report.InputManifest == nil || len(protocol.report.Results) != 1 {
		t.Fatal("文件证据没有保留用途")
	}
}
func Test普通导入缺少写入映射或工具证据不能通过(t *testing.T) {
	for _, toolUnavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "写权限与映射未知", true: "工具未知"}[toolUnavailable], func(t *testing.T) {
			now := time.Now().UTC()
			protocol := &normalImportProtocol{protocolStub: protocolStub{grant: normalGrant(now, "NORMAL_IMPORT_PREFLIGHT")}}
			probe := &normalImportProbe{unknownWrites: true, rejectTool: toolUnavailable}
			worker := testWorker(protocol, &testClock{now: now}, func(SecretResolver) agentpreflight.Probe { return probe })
			outcome, found, err := worker.RunNext(context.Background())
			if err != nil || !found || outcome.Report.Succeeded || protocol.report.Succeeded || len(protocol.report.Results) != 6 || agentpreflight.ValidateNormalImportReport(protocol.report) != nil {
				t.Fatal("未知证据未失败关闭", err)
			}
			if protocol.report.InputManifest == nil {
				t.Fatal("失败报告丢失已验证文件事实")
			}
			if toolUnavailable {
				for _, result := range protocol.report.Results {
					if result.Check == agentpreflight.CheckDatabaseConnectivity || result.Check == "WRITE_ACCESS" || result.Check == "INPUT_MAPPING" {
						if result.Status != agentpreflight.StatusUnknown || result.EvidenceCode != precheckcontract.EvidenceBlockedByTool {
							t.Fatal("工具前置阻断没有准确保留为未执行")
						}
					}
				}
				for _, check := range probe.checks {
					if check == agentpreflight.CheckDatabaseConnectivity || check == "WRITE_ACCESS" || check == "INPUT_MAPPING" {
						t.Fatal("工具未知仍访问数据库")
					}
				}
			}
		})
	}
}
func Test普通导入租约拒绝导出检查清单与输出字段(t *testing.T) {
	for _, foreign := range []string{"checks", "output"} {
		t.Run(foreign, func(t *testing.T) {
			now := time.Now().UTC()
			grant := normalGrant(now, "NORMAL_IMPORT_FILES")
			if foreign == "checks" {
				grant.CheckSet = agentpreflight.FixedChecks()
			} else {
				grant.Context.OutputPath = "/E:/tmp/output"
			}
			protocol := &normalImportProtocol{protocolStub: protocolStub{grant: grant}}
			calls := 0
			worker := testWorker(protocol, &testClock{now: now}, func(SecretResolver) agentpreflight.Probe { calls++; return &normalImportProbe{} })
			_, _, err := worker.RunNext(context.Background())
			if err == nil || calls != 0 || protocol.resolveCount() != 0 {
				t.Fatal("越界上下文进入探针")
			}
		})
	}
}
