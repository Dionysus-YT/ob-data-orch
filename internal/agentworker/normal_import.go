package agentworker

import (
	"context"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
)

func cloneNormalImport(config *importdomain.Config) *importdomain.Config {
	if config == nil {
		return nil
	}
	copy := *config
	copy.Objects = make(map[string]string, len(config.Objects))
	for kind, values := range config.Objects {
		copy.Objects[kind] = values
	}
	if config.Mapping != nil {
		mapping := *config.Mapping
		copy.Mapping = &mapping
	}
	return &copy
}

func checkSetForGrant(grant agentwire.PrecheckGrant) bool {
	if grant.Context.DirectLoad != nil {
		expected := precheckcontract.DirectLoadChecks(grant.Context.CheckKind)
		if len(expected) == 0 || len(expected) != len(grant.CheckSet) {
			return false
		}
		for i, name := range expected {
			if string(grant.CheckSet[i]) != name {
				return false
			}
		}
		return true
	}
	if grant.Context.NormalImport == nil {
		return checkSetForKind(kindForGrant(grant), grant.CheckSet)
	}
	expected := precheckcontract.NormalImportChecks(grant.Context.CheckKind)
	if len(expected) == 0 || len(expected) != len(grant.CheckSet) {
		return false
	}
	for index, check := range expected {
		if string(grant.CheckSet[index]) != check {
			return false
		}
	}
	return true
}

func validNormalImportExecutionContext(c agentwire.PrecheckExecutionContext) bool {
	return c.DirectLoad == nil && agentpreflight.ValidNormalImportContext(c.NormalImport, c.CheckKind, c.CompatibilityMode, c.TargetPlatform, c.AllowedRoots) &&
		c.Database == c.NormalImport.Database && c.ContentKind == c.NormalImport.Content && c.OutputKind == "" && c.OutputPath == "" && c.LogPath == "" &&
		c.StorageTarget == nil && !c.SkipCheckDir && !c.WeakRead && !c.EnableHiddenPk && !c.AddExtraMessage &&
		len(c.Objects) == 0 && len(c.ObjectTypes) == 0 && len(c.IncludeColumnNames) == 0 && len(c.ExcludeColumnNames) == 0 && len(c.Partitions) == 0
}

// ScanImportInput 只透传当前租约的固定输入检查，不开放数据库秘密解析窗口。
func (p secretGatedProbe) ScanImportInput(ctx context.Context, request agentpreflight.Request) (importfiles.Manifest, error) {
	if ctx == nil || p.delegate == nil || p.resolver == nil {
		return importfiles.Manifest{}, agentpreflight.ErrInvalidRequest
	}
	scanner, ok := p.delegate.(agentpreflight.ImportInputScanner)
	if !ok {
		return importfiles.Manifest{}, agentpreflight.ErrProbeFailed
	}
	return scanner.ScanImportInput(ctx, request)
}

func failedNormalImportReport(grant agentwire.PrecheckGrant) agentpreflight.Report {
	report := agentpreflight.Report{PrecheckID: grant.PrecheckID, CheckKind: grant.Context.CheckKind}
	for _, name := range precheckcontract.NormalImportChecks(report.CheckKind) {
		code := failureEvidenceCode(agentpreflight.CheckID(name))
		if name == "INPUT_FILES" || name == "INPUT_MAPPING" || name == "WRITE_ACCESS" {
			code = "IMPORT_UNAVAILABLE"
		}
		report.Results = append(report.Results, agentpreflight.Result{Check: agentpreflight.CheckID(name), Status: agentpreflight.StatusUnknown, EvidenceCode: code})
	}
	return report
}
