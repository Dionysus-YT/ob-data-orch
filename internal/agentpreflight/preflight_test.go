package agentpreflight

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/commandgen"
)

func Test固定预检查执行完整清单且不接收自定义操作(t *testing.T) {
	probe := &syntheticProbe{}
	report, err := Run(context.Background(), validRequest(), probe)
	if err != nil {
		t.Fatalf("Run() 错误 = %v", err)
	}
	if !report.Succeeded {
		t.Fatalf("Run() 成功结果 = %#v", report)
	}
	if !reflect.DeepEqual(probe.checks, FixedChecks()) {
		t.Fatalf("检查顺序 = %#v，期望 %#v", probe.checks, FixedChecks())
	}
	if len(report.Results) != len(FixedChecks()) {
		t.Fatalf("结果数量 = %d，期望 %d", len(report.Results), len(FixedChecks()))
	}
}

func Test预检查失败与未知均不伪造成功(t *testing.T) {
	for _, status := range []Status{StatusFailed, StatusUnknown} {
		t.Run(string(status), func(t *testing.T) {
			probe := &syntheticProbe{results: map[CheckID]Status{CheckObjectAccess: status}}
			report, err := Run(context.Background(), validRequest(), probe)
			if err != nil {
				t.Fatalf("Run() 错误 = %v", err)
			}
			if report.Succeeded {
				t.Fatalf("状态 %s 被错误判定为成功", status)
			}
		})
	}
}

func Test预检查在不安全输入前失败关闭(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Request)
		want   error
	}{
		{name: "未知能力", mutate: func(request *Request) { request.Capability = "ARBITRARY_SHELL" }, want: ErrUnsupportedCapability},
		{name: "活动执行", mutate: func(request *Request) { request.ActiveExecution = true }, want: ErrActiveExecution},
		{name: "租约缺失", mutate: func(request *Request) { request.LeaseID = "" }, want: ErrInvalidRequest},
		{name: "绑定漂移", mutate: func(request *Request) { request.Binding.NodeID = "other-node" }, want: ErrInvalidRequest},
		{name: "不支持平台", mutate: func(request *Request) { request.TargetPlatform = "OTHER" }, want: ErrInvalidRequest},
		{name: "输出路径不是绝对路径", mutate: func(request *Request) { request.OutputPath = "relative-output" }, want: ErrInvalidRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validRequest()
			test.mutate(&request)
			probe := &syntheticProbe{}
			if _, err := Run(context.Background(), request, probe); !errors.Is(err, test.want) {
				t.Fatalf("Run() 错误 = %v，期望 %v", err, test.want)
			}
			if len(probe.checks) != 0 {
				t.Fatalf("拒绝请求仍执行了检查：%#v", probe.checks)
			}
		})
	}
}

func Test预检查拒绝不完整或不匹配的合成事实(t *testing.T) {
	tests := []struct {
		name  string
		probe *syntheticProbe
	}{
		{name: "错误检查标识", probe: &syntheticProbe{wrongCheck: true}},
		{name: "缺失证据码", probe: &syntheticProbe{emptyEvidence: true}},
		{name: "不安全证据码", probe: &syntheticProbe{unsafeEvidence: true}},
		{name: "未知状态", probe: &syntheticProbe{invalidStatus: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Run(context.Background(), validRequest(), test.probe); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("Run() 错误 = %v，期望 %v", err, ErrInvalidRequest)
			}
		})
	}
}

func Test预检查不泄露探针内部错误(t *testing.T) {
	probe := &syntheticProbe{probeError: errors.New("synthetic-secret-not-allowed")}
	if _, err := Run(context.Background(), validRequest(), probe); !errors.Is(err, ErrProbeFailed) {
		t.Fatalf("Run() 错误 = %v，期望 %v", err, ErrProbeFailed)
	}
}

func validRequest() Request {
	binding := agentstate.PrecheckBinding{PrecheckID: "precheck-1", NodeID: "node-1", DraftRevision: 1, ConfigFingerprint: "synthetic-fingerprint", CredentialRevision: 1, NodeFactsVersion: 1}
	return Request{Capability: CapabilityExportPreflight, PrecheckID: binding.PrecheckID, NodeID: binding.NodeID, AgentID: "agent-1", LeaseID: "lease-1", LeaseEpoch: 1, Binding: binding, TargetPlatform: commandgen.PlatformWindowsAMD64, OutputPath: `E:\synthetic\output`}
}

type syntheticProbe struct {
	checks         []CheckID
	results        map[CheckID]Status
	wrongCheck     bool
	emptyEvidence  bool
	invalidStatus  bool
	unsafeEvidence bool
	probeError     error
}

func (p *syntheticProbe) Probe(_ context.Context, check CheckID, _ Request) (Result, error) {
	p.checks = append(p.checks, check)
	if p.probeError != nil {
		return Result{}, p.probeError
	}
	if p.wrongCheck {
		return Result{Check: CheckAvailableSpace, Status: StatusPassed, EvidenceCode: "SYNTHETIC_OK"}, nil
	}
	if p.emptyEvidence {
		return Result{Check: check, Status: StatusPassed}, nil
	}
	if p.unsafeEvidence {
		return Result{Check: check, Status: StatusPassed, EvidenceCode: "synthetic-secret-not-allowed"}, nil
	}
	if p.invalidStatus {
		return Result{Check: check, Status: "UNTRUSTED", EvidenceCode: "SYNTHETIC_OK"}, nil
	}
	status := StatusPassed
	if p.results != nil {
		if configured, found := p.results[check]; found {
			status = configured
		}
	}
	return Result{Check: check, Status: status, EvidenceCode: "SYNTHETIC_OK"}, nil
}
