package agentpreflight

import (
	"context"
	"encoding/json"

	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/directloadfiles"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
)

// DirectLoadInputScanner 只读取受认证旁路租约绑定的输入，不提供任意路径接口。
type DirectLoadInputScanner interface {
	ScanDirectLoadInput(context.Context, Request) (importfiles.Manifest, error)
}

// DirectLoadConfigurationReporter 只读取同一完整租约已完成的本机配置安全指纹，不触发秘密解析或新探测。
type DirectLoadConfigurationReporter interface {
	DirectLoadConfigFingerprint(context.Context, Request) string
}

// ValidDirectLoadContext 核对活动配置和平台根目录；普通导入配置不能冒充旁路。
func ValidDirectLoadContext(c *directload.Config, kind, mode string, platform commandgen.Platform, roots []string) bool {
	if c == nil || len(precheckcontract.DirectLoadChecks(kind)) == 0 || !validAllowedRoots(platform, roots) {
		return false
	}
	active, issues := directload.Normalize(*c, mode, string(platform), kind == "DIRECT_LOAD_PREFLIGHT")
	left, _ := json.Marshal(c)
	right, _ := json.Marshal(active)
	return len(issues) == 0 && active.DataSourceID != "" && active.NodeID != "" && active.Database != "" && active.Scenario != "" && active.Table != "" && active.FilePath != "" && active.Format != "" && string(left) == string(right)
}

func validateDirectLoadRequest(r Request) error {
	if !ValidDirectLoadContext(r.DirectLoad, r.CheckKind, r.CompatibilityMode, r.TargetPlatform, r.AllowedRoots) || r.NormalImport != nil || r.InputManifest != nil || string(r.Capability) != r.CheckKind || r.ActiveExecution ||
		r.PrecheckID == "" || r.AgentID == "" || r.LeaseID == "" || r.LeaseEpoch < 1 || r.Binding.DraftRevision < 1 || r.Binding.CredentialRevision < 1 || r.Binding.NodeFactsVersion < 1 ||
		r.PrecheckID != r.Binding.PrecheckID || r.NodeID != r.Binding.NodeID || r.DirectLoad.NodeID != r.NodeID || r.Database != r.DirectLoad.Database || r.ContentKind != "DATA_ONLY" ||
		r.OutputPath != "" || r.OutputKind != "" || r.LogPath != "" || r.StorageTarget != nil || r.SkipCheckDir || r.WeakRead || r.EnableHiddenPk || r.AddExtraMessage || len(r.Objects) != 0 || len(r.ObjectTypes) != 0 || len(r.Partitions) != 0 || len(r.IncludeColumnNames) != 0 || len(r.ExcludeColumnNames) != 0 {
		return ErrInvalidRequest
	}
	return nil
}

func runDirectLoad(ctx context.Context, r Request, p Probe) (Report, error) {
	if ctx == nil || p == nil || validateDirectLoadRequest(r) != nil {
		return Report{}, ErrInvalidRequest
	}
	report := Report{PrecheckID: r.PrecheckID, CheckKind: r.CheckKind, Succeeded: true}
	results := map[CheckID]Result{}
	for _, name := range precheckcontract.DirectLoadChecks(r.CheckKind) {
		check := CheckID(name)
		result := Result{Check: check, Status: StatusUnknown, EvidenceCode: "DIRECT_LOAD_UNAVAILABLE"}
		if name == "INPUT_FILES" {
			if scanner, ok := p.(DirectLoadInputScanner); ok {
				m, err := scanner.ScanDirectLoadInput(ctx, r)
				if err == nil && directloadfiles.Valid(m, *r.DirectLoad) {
					result.Status = StatusPassed
					result.EvidenceCode = "DIRECT_LOAD_VERIFIED"
					report.InputManifest = &m
					r.InputManifest = &m
				} else if ctx.Err() == nil {
					result.Status = StatusFailed
					result.EvidenceCode = "DIRECT_LOAD_REJECTED"
				}
			}
		} else if (name == "DATABASE_CONNECTIVITY" || name == "RPC_CONNECTIVITY" || name == "DIRECT_LOAD_VERSION" || name == "WRITE_ACCESS" || name == "DIRECT_LOAD_STRUCTURE") && results[CheckToolEnvironment].Status != StatusPassed {
			result.EvidenceCode = precheckcontract.EvidenceBlockedByTool
		} else if (name == "WRITE_ACCESS" || name == "DIRECT_LOAD_STRUCTURE") && results[CheckDatabaseConnectivity].Status != StatusPassed {
			result.EvidenceCode = precheckcontract.EvidenceBlockedByDatabase
		} else if observed, err := p.Probe(ctx, check, r); err == nil && observed.Check == check && precheckcontract.ValidDirectLoadResult(name, string(observed.Status), observed.EvidenceCode) {
			result = observed
		}
		if name == "DIRECT_LOAD_CONFIG" && result.Status == StatusPassed {
			if reporter, ok := p.(DirectLoadConfigurationReporter); ok && ctx.Err() == nil {
				report.DirectLoadConfigFingerprint = reporter.DirectLoadConfigFingerprint(ctx, r)
			}
			// 缺失或过期的安全投影不能仅凭 PASSED 枚举进入冻结快照。
			if ctx.Err() != nil || !validDirectLoadConfigFingerprint(report.DirectLoadConfigFingerprint) {
				report.DirectLoadConfigFingerprint = ""
				result = Result{Check: check, Status: StatusUnknown, EvidenceCode: "DIRECT_LOAD_UNAVAILABLE"}
			}
		}
		results[check] = result
		report.Results = append(report.Results, result)
		report.Succeeded = report.Succeeded && result.Status == StatusPassed
	}
	if ValidateDirectLoadReport(report) != nil {
		return Report{}, ErrInvalidRequest
	}
	return report, nil
}

// ValidateDirectLoadReport 复核独立清单、原因码与文件证据，未知或缺项不能升级成功。
func ValidateDirectLoadReport(report Report) error {
	checks := precheckcontract.DirectLoadChecks(report.CheckKind)
	if report.PrecheckID == "" || len(checks) == 0 || len(checks) != len(report.Results) {
		return ErrInvalidRequest
	}
	passed, files, configuration := true, false, false
	for i, name := range checks {
		r := report.Results[i]
		if string(r.Check) != name || !precheckcontract.ValidDirectLoadResult(name, string(r.Status), r.EvidenceCode) {
			return ErrInvalidRequest
		}
		passed = passed && r.Status == StatusPassed
		if name == "INPUT_FILES" {
			files = r.Status == StatusPassed
		}
		if name == "DIRECT_LOAD_CONFIG" {
			configuration = r.Status == StatusPassed
		}
	}
	if passed != report.Succeeded || files != (report.InputManifest != nil) ||
		configuration && !validDirectLoadConfigFingerprint(report.DirectLoadConfigFingerprint) ||
		!configuration && report.DirectLoadConfigFingerprint != "" {
		return ErrInvalidRequest
	}
	return nil
}

func validDirectLoadConfigFingerprint(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
