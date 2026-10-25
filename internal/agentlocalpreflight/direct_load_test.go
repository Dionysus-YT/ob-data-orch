package agentlocalpreflight

import (
	"context"
	"testing"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/precheckcontract"
)

func Test旁路默认不调用已准入普通导入探针(t *testing.T) {
	p := Probe{JDBC: rejectDirectProbe{t}, ImportDatabase: rejectDirectProbe{t}, ImportChecks: rejectDirectProbe{t}}
	for _, name := range []string{"DATABASE_CONNECTIVITY", "RPC_CONNECTIVITY", "DIRECT_LOAD_VERSION", "WRITE_ACCESS", "DIRECT_LOAD_STRUCTURE", "DIRECT_LOAD_CONFIG"} {
		r, err := p.Probe(context.Background(), agentpreflight.CheckID(name), agentpreflight.Request{DirectLoad: &directload.Config{}})
		if err != nil || r.Status != agentpreflight.StatusUnknown || r.EvidenceCode != "DIRECT_LOAD_PROBE_NOT_CONFIGURED" || !precheckcontract.ValidDirectLoadResult(name, string(r.Status), r.EvidenceCode) {
			t.Fatal("旁路未装配事实被提升", err)
		}
	}
}

type rejectDirectProbe struct{ t *testing.T }

func (p rejectDirectProbe) Probe(context.Context, agentpreflight.CheckID, agentpreflight.Request) (agentpreflight.Result, error) {
	p.t.Fatal("旁路借用了其他业务探针")
	return agentpreflight.Result{}, nil
}

func Test旁路只消费专用检查端口且拒绝迟到和跨模块原因码(t *testing.T) {
	for _, kind := range []string{"valid", "wrong-check", "ordinary-code", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := Probe{DirectLoadChecks: directResultProbe{run: func(check agentpreflight.CheckID) agentpreflight.Result {
				result := agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: "DIRECT_LOAD_VERIFIED"}
				if kind == "wrong-check" {
					result.Check = "WRITE_ACCESS"
				}
				if kind == "ordinary-code" {
					result.EvidenceCode = "IMPORT_VERIFIED"
				}
				if kind == "cancelled" {
					cancel()
				}
				return result
			}}}
			r, err := p.Probe(ctx, "RPC_CONNECTIVITY", agentpreflight.Request{DirectLoad: &directload.Config{}})
			want := agentpreflight.StatusUnknown
			if kind == "valid" {
				want = agentpreflight.StatusPassed
			}
			if err != nil || r.Status != want {
				t.Fatal("专用端口边界不成立", r, err)
			}
		})
	}
}

type directResultProbe struct {
	run func(agentpreflight.CheckID) agentpreflight.Result
}

func (p directResultProbe) Probe(_ context.Context, c agentpreflight.CheckID, _ agentpreflight.Request) (agentpreflight.Result, error) {
	return p.run(c), nil
}

type directFingerprintProbe struct {
	directResultProbe
	cancel context.CancelFunc
	calls  int
}

func (p *directFingerprintProbe) DirectLoadConfigFingerprint(context.Context, agentpreflight.Request) string {
	p.calls++
	if p.cancel != nil {
		p.cancel()
	}
	return "synthetic-safe-projection"
}

func Test旁路配置指纹仅转发专用完整检查且丢弃迟到结果(t *testing.T) {
	for _, kind := range []string{"valid", "files", "normal", "no-direct", "cancel-before", "cancel-late", "unconfigured"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			port := &directFingerprintProbe{}
			p := Probe{DirectLoadChecks: port}
			r := agentpreflight.Request{DirectLoad: &directload.Config{}, CheckKind: "DIRECT_LOAD_PREFLIGHT"}
			switch kind {
			case "files":
				r.CheckKind = "DIRECT_LOAD_FILES"
			case "normal":
				r.NormalImport = &importdomain.Config{}
			case "no-direct":
				r.DirectLoad = nil
			case "cancel-before":
				cancel()
			case "cancel-late":
				port.cancel = cancel
			case "unconfigured":
				p.DirectLoadChecks = nil
			}
			value := p.DirectLoadConfigFingerprint(ctx, r)
			if (value != "") != (kind == "valid") || port.calls != map[bool]int{true: 1, false: 0}[kind == "valid" || kind == "cancel-late"] {
				t.Fatal("本机配置安全指纹端口越界或接受迟到事实")
			}
		})
	}
}
