package agentworker

import (
	"context"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
)

type directLoadProbe struct {
	normalImportProbe
	unknown, foreign, missingFingerprint bool
	fingerprint                          string
}

func (p *directLoadProbe) DirectLoadConfigFingerprint(context.Context, agentpreflight.Request) string {
	if p.missingFingerprint {
		return ""
	}
	if p.fingerprint != "" {
		return p.fingerprint
	}
	return strings.Repeat("a", 64)
}

func (p *directLoadProbe) Probe(_ context.Context, check agentpreflight.CheckID, _ agentpreflight.Request) (agentpreflight.Result, error) {
	p.checks = append(p.checks, check)
	code, status := "DIRECT_LOAD_VERIFIED", agentpreflight.StatusPassed
	if p.foreign {
		code = "IMPORT_VERIFIED"
	}
	if p.unknown && check == "DIRECT_LOAD_STRUCTURE" {
		code = "DIRECT_LOAD_UNAVAILABLE"
		status = agentpreflight.StatusUnknown
	}
	return agentpreflight.Result{Check: check, Status: status, EvidenceCode: code}, nil
}

func (p *directLoadProbe) ScanDirectLoadInput(ctx context.Context, r agentpreflight.Request) (importfiles.Manifest, error) {
	return p.normalImportProbe.ScanImportInput(ctx, r)
}

func directGrant(now time.Time, kind string) agentwire.PrecheckGrant {
	g := normalGrant(now, "NORMAL_IMPORT_FILES")
	g.Context.NormalImport = nil
	g.Context.DirectLoad = &directload.Config{DataSourceID: "source-1", NodeID: "node-1", Database: "synthetic_db", Scenario: "PRIVATE_ODP", RPCPort: 2885, Table: "synthetic", Format: "CSV", FilePath: `E:\tmp\input`, Applicability: directload.Applicability{SingleTable: true, WholeTableCommit: true, NoCheckpoint: true, StructureLimits: true}}
	g.Context.CheckKind = kind
	g.CheckSet = nil
	for _, name := range precheckcontract.DirectLoadChecks(kind) {
		g.CheckSet = append(g.CheckSet, agentpreflight.CheckID(name))
	}
	return g
}

func Test旁路文件发现隔离秘密与数据库检查(t *testing.T) {
	now := time.Now().UTC()
	protocol := &normalImportProtocol{protocolStub: protocolStub{grant: directGrant(now, "DIRECT_LOAD_FILES")}}
	probe := &directLoadProbe{}
	w := testWorker(protocol, &testClock{now: now}, func(SecretResolver) agentpreflight.Probe { return probe })
	outcome, found, err := w.RunNext(context.Background())
	if err != nil || !found || !outcome.Report.Succeeded || outcome.Report.DirectLoadConfigFingerprint != "" || probe.scans != 1 || len(probe.checks) != 0 || protocol.resolveCount() != 0 || agentpreflight.ValidateDirectLoadReport(outcome.Report) != nil {
		t.Fatal("旁路文件发现隔离失败", err)
	}
}

func Test旁路完整检查拒绝未知与普通导入证据(t *testing.T) {
	for _, kind := range []string{"complete", "unknown", "foreign", "missing-fingerprint", "invalid-fingerprint"} {
		t.Run(kind, func(t *testing.T) {
			now := time.Now().UTC()
			p := &normalImportProtocol{protocolStub: protocolStub{grant: directGrant(now, "DIRECT_LOAD_PREFLIGHT")}}
			probe := &directLoadProbe{unknown: kind == "unknown", foreign: kind == "foreign", missingFingerprint: kind == "missing-fingerprint"}
			if kind == "invalid-fingerprint" {
				probe.fingerprint = strings.Repeat("A", 64)
			}
			w := testWorker(p, &testClock{now: now}, func(SecretResolver) agentpreflight.Probe { return probe })
			outcome, found, err := w.RunNext(context.Background())
			if err != nil || !found || outcome.Report.Succeeded != (kind == "complete") || len(outcome.Report.Results) != 9 || outcome.Report.InputManifest == nil || p.resolveCount() != 0 {
				t.Fatal("旁路证据未独立失败关闭", err)
			}
			wantFingerprint := strings.Repeat("a", 64)
			if kind == "foreign" || kind == "missing-fingerprint" || kind == "invalid-fingerprint" {
				wantFingerprint = ""
			}
			if outcome.Report.DirectLoadConfigFingerprint != wantFingerprint || agentpreflight.ValidateDirectLoadReport(outcome.Report) != nil {
				t.Fatal("旁路配置安全指纹没有随检查事实失败关闭")
			}
		})
	}
}

func Test旁路租约拒绝混合配置与其他检查集(t *testing.T) {
	for _, kind := range []string{"normal", "checks", "output", "node"} {
		t.Run(kind, func(t *testing.T) {
			now := time.Now().UTC()
			g := directGrant(now, "DIRECT_LOAD_FILES")
			switch kind {
			case "normal":
				g.Context.NormalImport = &importdomain.Config{}
			case "checks":
				g.CheckSet = agentpreflight.FixedChecks()
			case "output":
				g.Context.OutputPath = "/synthetic"
			case "node":
				g.Context.DirectLoad.NodeID = "foreign"
			}
			p := &normalImportProtocol{protocolStub: protocolStub{grant: g}}
			calls := 0
			w := testWorker(p, &testClock{now: now}, func(SecretResolver) agentpreflight.Probe { calls++; return &directLoadProbe{} })
			if _, _, err := w.RunNext(context.Background()); err == nil || calls != 0 || p.resolveCount() != 0 {
				t.Fatal("混合租约未在确认前拒绝")
			}
		})
	}
}
