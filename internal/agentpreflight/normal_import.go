package agentpreflight

import (
	"context"
	"encoding/json"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
)

// ImportInputScanner 只读取当前已确认租约的固定输入，不能接受自由路径或切换任务。
type ImportInputScanner interface {
	ScanImportInput(context.Context, Request) (importfiles.Manifest, error)
}

// ValidNormalImportContext 统一协议与 Worker 的活动配置、用途和平台边界。
func ValidNormalImportContext(c *importdomain.Config, kind, mode string, platform commandgen.Platform, roots []string) bool {
	if c == nil || len(precheckcontract.NormalImportChecks(kind)) == 0 || (mode != "MYSQL" && mode != "ORACLE") || !validAllowedRoots(platform, roots) {
		return false
	}
	active, issues := importdomain.Normalize(*c, mode, string(platform), kind == "NORMAL_IMPORT_PREFLIGHT")
	if len(issues) != 0 || active.NodeID == "" || active.DataSourceID == "" || active.Database == "" || active.FilePath == "" || active.Content == "" || active.HasData() && active.Format == "" {
		return false
	}
	left, _ := json.Marshal(c)
	right, _ := json.Marshal(active)
	return string(left) == string(right)
}

func validateNormalImportRequest(r Request) error {
	if r.DirectLoad != nil || r.NormalImport == nil || r.InputManifest != nil || string(r.Capability) != r.CheckKind || !ValidNormalImportContext(r.NormalImport, r.CheckKind, r.CompatibilityMode, r.TargetPlatform, r.AllowedRoots) ||
		r.PrecheckID == "" || r.AgentID == "" || r.LeaseID == "" || r.LeaseEpoch < 1 || r.Binding.DraftRevision < 1 || r.Binding.CredentialRevision < 1 || r.Binding.NodeFactsVersion < 1 ||
		r.PrecheckID != r.Binding.PrecheckID || r.NodeID != r.Binding.NodeID || r.NormalImport.NodeID != r.NodeID || r.Database != r.NormalImport.Database || r.ActiveExecution ||
		r.OutputPath != "" || r.OutputKind != "" || r.LogPath != "" || r.StorageTarget != nil || r.WeakRead || r.EnableHiddenPk || r.AddExtraMessage || r.SkipCheckDir ||
		len(r.Objects) != 0 || len(r.ObjectTypes) != 0 || len(r.IncludeColumnNames) != 0 || len(r.ExcludeColumnNames) != 0 || len(r.Partitions) != 0 || r.ContentKind != r.NormalImport.Content {
		return ErrInvalidRequest
	}
	return nil
}

func runNormalImport(ctx context.Context, r Request, p Probe) (Report, error) {
	if ctx == nil || p == nil || validateNormalImportRequest(r) != nil {
		return Report{}, ErrInvalidRequest
	}
	report := Report{PrecheckID: r.PrecheckID, CheckKind: r.CheckKind, Succeeded: true}
	results := map[CheckID]Result{}
	// 数据库检查依赖已核对的工具环境；文件发现不解析数据库槽位。
	order := []string{"TOOL_ENVIRONMENT", "NODE_RESOURCE", "INPUT_FILES", "DATABASE_CONNECTIVITY", "WRITE_ACCESS", "INPUT_MAPPING"}
	if r.CheckKind == "NORMAL_IMPORT_FILES" {
		order = []string{"INPUT_FILES"}
	}
	for _, name := range order {
		check := CheckID(name)
		result := Result{Check: check, Status: StatusUnknown, EvidenceCode: "IMPORT_UNAVAILABLE"}
		if name == "INPUT_FILES" {
			if scanner, ok := p.(ImportInputScanner); ok {
				manifest, err := scanner.ScanImportInput(ctx, r)
				if err == nil && manifest.Valid(*r.NormalImport) {
					result.Status = StatusPassed
					result.EvidenceCode = "IMPORT_VERIFIED"
					report.InputManifest = &manifest
					r.InputManifest = &manifest
				} else {
					if ctx.Err() == nil {
						result.Status = StatusFailed
						result.EvidenceCode = "IMPORT_REJECTED"
					}
				}
			}
		} else if (name == "DATABASE_CONNECTIVITY" || name == "WRITE_ACCESS" || name == "INPUT_MAPPING") && results[CheckToolEnvironment].Status != StatusPassed {
			// 所有数据库相关项都明确说明尚未执行，避免将依赖阻断混为探针自身失败。
			result.EvidenceCode = precheckcontract.EvidenceBlockedByTool
		} else if (name == "WRITE_ACCESS" || name == "INPUT_MAPPING") && results[CheckDatabaseConnectivity].Status != StatusPassed {
			result.EvidenceCode = precheckcontract.EvidenceBlockedByDatabase
		} else {
			observed, err := p.Probe(ctx, check, r)
			if err == nil && observed.Check == check && precheckcontract.ValidResult(name, string(observed.Status), observed.EvidenceCode) {
				result = observed
			} else {
				if name == "TOOL_ENVIRONMENT" {
					result.EvidenceCode = "TOOL_RUNTIME_UNAVAILABLE"
				}
				if name == "NODE_RESOURCE" {
					result.EvidenceCode = "NODE_RESOURCE_UNAVAILABLE"
				}
				if name == "DATABASE_CONNECTIVITY" {
					result.EvidenceCode = "DATABASE_CONNECTION_UNAVAILABLE"
				}
			}
		}
		results[check] = result
	}
	for _, name := range precheckcontract.NormalImportChecks(r.CheckKind) {
		result := results[CheckID(name)]
		report.Results = append(report.Results, result)
		if result.Status != StatusPassed {
			report.Succeeded = false
		}
	}
	if ValidateNormalImportReport(report) != nil {
		return Report{}, ErrInvalidRequest
	}
	return report, nil
}

// ValidateNormalImportReport 复核独立清单与整体状态，输入证据还须按冻结上下文复验。
func ValidateNormalImportReport(report Report) error {
	checks := precheckcontract.NormalImportChecks(report.CheckKind)
	if len(checks) == 0 || len(report.Results) != len(checks) || report.PrecheckID == "" || report.DirectLoadConfigFingerprint != "" {
		return ErrInvalidRequest
	}
	passed := true
	filesPassed := false
	for index, name := range checks {
		result := report.Results[index]
		if string(result.Check) != name || !precheckcontract.ValidResult(name, string(result.Status), result.EvidenceCode) {
			return ErrInvalidRequest
		}
		passed = passed && result.Status == StatusPassed
		if name == "INPUT_FILES" {
			filesPassed = result.Status == StatusPassed
		}
	}
	if passed != report.Succeeded || filesPassed != (report.InputManifest != nil) {
		return ErrInvalidRequest
	}
	return nil
}
