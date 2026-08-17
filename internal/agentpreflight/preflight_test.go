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
	if !reflect.DeepEqual(probe.checks, executionCheckOrder()) {
		t.Fatalf("检查执行顺序 = %#v，期望 %#v", probe.checks, executionCheckOrder())
	}
	if len(report.Results) != len(FixedChecks()) {
		t.Fatalf("结果数量 = %d，期望 %d", len(report.Results), len(FixedChecks()))
	}
	for index, check := range FixedChecks() {
		if report.Results[index].Check != check {
			t.Fatalf("报告结果顺序 %d = %q，期望 %q", index, report.Results[index].Check, check)
		}
	}
}

func Test本机前置失败时不解析数据库槽位(t *testing.T) {
	probe := &syntheticProbe{results: map[CheckID]Status{CheckToolEnvironment: StatusFailed}}
	report, err := Run(context.Background(), validRequest(), probe)
	if err != nil {
		t.Fatalf("Run() 错误 = %v", err)
	}
	if report.Succeeded {
		t.Fatalf("本机前置失败被错误判定为成功: %#v", report)
	}
	if !reflect.DeepEqual(probe.checks, localChecks) {
		t.Fatalf("本机前置失败后的 Probe 调用 = %#v，期望 %#v", probe.checks, localChecks)
	}
	if database := report.Results[0]; database != unavailableDatabaseResult() {
		t.Fatalf("数据库结果 = %#v，期望未尝试的未知结果", database)
	}
	if object := report.Results[1]; object != unavailableObjectResult() {
		t.Fatalf("对象结果 = %#v，期望未尝试的未知结果", object)
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
		{name: "兼容模式未知", mutate: func(request *Request) { request.CompatibilityMode = "UNKNOWN" }, want: ErrInvalidRequest},
		{name: "绑定漂移", mutate: func(request *Request) { request.Binding.NodeID = "other-node" }, want: ErrInvalidRequest},
		{name: "不支持平台", mutate: func(request *Request) { request.TargetPlatform = "OTHER" }, want: ErrInvalidRequest},
		{name: "输出路径不是绝对路径", mutate: func(request *Request) { request.OutputPath = "relative-output" }, want: ErrInvalidRequest},
		{name: "输出路径包含回退段", mutate: func(request *Request) { request.OutputPath = "/E:/synthetic/exports/../outside" }, want: ErrInvalidRequest},
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

func TestValidateRequest将输出根目录关系交给本机探针(t *testing.T) {
	request := validRequest()
	request.OutputPath = "/E:/synthetic-other/output"
	if err := ValidateRequest(request); err != nil {
		t.Fatalf("ValidateRequest() 错误 = %v；输出目录边界应由本机 Probe 解析后回传固定失败结果", err)
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
		{name: "编码后的不安全证据码", probe: &syntheticProbe{encodedEvidence: true}},
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

func TestValidateReport拒绝缺项乱序与伪造成功(t *testing.T) {
	report, err := Run(context.Background(), validRequest(), &syntheticProbe{})
	if err != nil {
		t.Fatalf("Run() 错误 = %v", err)
	}
	if err := ValidateReport(report); err != nil {
		t.Fatalf("ValidateReport() 错误 = %v", err)
	}

	for _, mutate := range []func(*Report){
		func(value *Report) { value.Results = value.Results[:len(value.Results)-1] },
		func(value *Report) { value.Results[0], value.Results[1] = value.Results[1], value.Results[0] },
		func(value *Report) { value.Results[0].Status = StatusFailed },
	} {
		candidate := Report{PrecheckID: report.PrecheckID, Succeeded: report.Succeeded, Results: append([]Result(nil), report.Results...)}
		mutate(&candidate)
		if !errors.Is(ValidateReport(candidate), ErrInvalidRequest) {
			t.Fatalf("ValidateReport() 接受了不安全报告: %#v", candidate)
		}
	}
}

func validRequest() Request {
	binding := agentstate.PrecheckBinding{PrecheckID: "precheck-1", NodeID: "node-1", DraftRevision: 1, ConfigFingerprint: "synthetic-fingerprint", CredentialRevision: 1, NodeFactsVersion: 1}
	return Request{Capability: CapabilityExportPreflight, PrecheckID: binding.PrecheckID, NodeID: binding.NodeID, AgentID: "agent-1", LeaseID: "lease-1", LeaseEpoch: 1, Binding: binding, CompatibilityMode: "MYSQL", Database: "synthetic_db", Objects: []string{"synthetic_table"}, ContentKind: "DATA_ONLY", TargetPlatform: commandgen.PlatformWindowsAMD64, OutputPath: "/E:/synthetic/output", AllowedRoots: []string{`E:\synthetic`}}
}

// validStorageRequest 构造对象存储输出任务的预检查请求（EX-I6 存储专用预检查）。
func validStorageRequest() Request {
	request := validRequest()
	request.OutputKind = OutputKindOSS
	request.OutputPath = "oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou.aliyuncs.com"
	request.LogPath = ""
	request.StorageTarget = &StorageTarget{
		Provider: "OSS", URI: request.OutputPath, Endpoint: "oss-cn-hangzhou.aliyuncs.com", TmpPath: "/E:/synthetic/tmp",
	}
	return request
}

// Test存储预检查执行存储形态清单且前置失败时存储检查保持未知 验证：
// 1) 对象存储输出使用六项存储形态清单（无 OUTPUT_PATH/OUTPUT_EMPTY，含两项存储检查）；
// 2) 本机前置失败时数据库与存储检查都保持 UNKNOWN 且不调用 Probe。
func Test存储预检查执行存储形态清单且前置失败时存储检查保持未知(t *testing.T) {
	probe := &syntheticProbe{}
	report, err := Run(context.Background(), validStorageRequest(), probe)
	if err != nil {
		t.Fatalf("Run() 错误 = %v", err)
	}
	expected := storageCheckList
	if len(report.Results) != len(expected) {
		t.Fatalf("结果数量 = %d，期望 %d", len(report.Results), len(expected))
	}
	for index, check := range expected {
		if report.Results[index].Check != check {
			t.Fatalf("报告结果顺序 %d = %q，期望 %q", index, report.Results[index].Check, check)
		}
	}
	if err := ValidateReportFor(OutputKindOSS, report); err != nil {
		t.Fatalf("ValidateReportFor(storage) 错误 = %v", err)
	}
	// 本机前置失败：TOOL_ENVIRONMENT 失败后数据库与存储检查全部 UNKNOWN 且不再调用 Probe。
	failed := &syntheticProbe{results: map[CheckID]Status{CheckToolEnvironment: StatusFailed}}
	report, err = Run(context.Background(), validStorageRequest(), failed)
	if err != nil {
		t.Fatalf("Run() 错误 = %v", err)
	}
	if reflect.DeepEqual(failed.checks, storageCheckList) {
		t.Fatalf("本机前置失败后仍执行了全部检查：%#v", failed.checks)
	}
	if report.Results[0].Status != StatusUnknown || report.Results[1].Status != StatusUnknown ||
		report.Results[4].Status != StatusUnknown || report.Results[4].EvidenceCode != "STORAGE_CONNECTIVITY_UNAVAILABLE" ||
		report.Results[5].Status != StatusUnknown || report.Results[5].EvidenceCode != "STORAGE_AUTH_UNAVAILABLE" {
		t.Fatalf("前置失败后的存储形态报告 = %#v", report.Results)
	}
}

// Test存储预检查拒绝不安全存储目标 验证存储形态的失败关闭：
// provider 与输出类型不一致、目标缺失、URI 含控制字符、非平台形态 tmp-path 均拒绝。
func Test存储预检查拒绝不安全存储目标(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Request)
	}{
		{name: "存储输出缺少目标", mutate: func(request *Request) { request.StorageTarget = nil }},
		{name: "provider 与输出类型不一致", mutate: func(request *Request) { request.StorageTarget.Provider = "S3" }},
		{name: "URI 含换行", mutate: func(request *Request) { request.OutputPath = "oss://bucket/path\n" }},
		{name: "URI 与控制目标不一致", mutate: func(request *Request) { request.StorageTarget.URI = "s3://bucket/path" }},
		{name: "tmp-path 不是平台绝对路径", mutate: func(request *Request) { request.StorageTarget.TmpPath = "relative/tmp" }},
		{name: "存储输出携带本地日志路径", mutate: func(request *Request) { request.LogPath = "/E:/synthetic/logs" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validStorageRequest()
			test.mutate(&request)
			if _, err := Run(context.Background(), request, &syntheticProbe{}); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("Run() 错误 = %v，期望 %v", err, ErrInvalidRequest)
			}
		})
	}
}

// TestValidateReportFor按输出类型拒绝形态混淆 验证存储形态报告不能通过本地校验，反之亦然。
func TestValidateReportFor按输出类型拒绝形态混淆(t *testing.T) {
	local, err := Run(context.Background(), validRequest(), &syntheticProbe{})
	if err != nil {
		t.Fatalf("Run(local) 错误 = %v", err)
	}
	if err := ValidateReportFor(OutputKindOSS, local); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("本地报告被存储形态校验接受: %v", err)
	}
	storage, err := Run(context.Background(), validStorageRequest(), &syntheticProbe{})
	if err != nil {
		t.Fatalf("Run(storage) 错误 = %v", err)
	}
	if err := ValidateReportFor(OutputKindLocal, storage); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("存储报告被本地形态校验接受: %v", err)
	}
}

func executionCheckOrder() []CheckID {
	return append(append([]CheckID(nil), localChecks...), databaseChecks...)
}

type syntheticProbe struct {
	checks          []CheckID
	results         map[CheckID]Status
	wrongCheck      bool
	emptyEvidence   bool
	invalidStatus   bool
	unsafeEvidence  bool
	encodedEvidence bool
	probeError      error
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
	if p.encodedEvidence {
		return Result{Check: check, Status: StatusFailed, EvidenceCode: "E_WORKSPACE_SECRET_ABC"}, nil
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
