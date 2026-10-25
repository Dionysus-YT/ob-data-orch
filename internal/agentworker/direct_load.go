package agentworker

import (
	"context"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/precheckcontract"
)

func cloneDirectLoad(c *directload.Config) *directload.Config {
	if c == nil {
		return nil
	}
	copy := *c
	return &copy
}

func validDirectLoadExecutionContext(c agentwire.PrecheckExecutionContext) bool {
	return c.NormalImport == nil && agentpreflight.ValidDirectLoadContext(c.DirectLoad, c.CheckKind, c.CompatibilityMode, c.TargetPlatform, c.AllowedRoots) && c.Database == c.DirectLoad.Database && c.ContentKind == "DATA_ONLY" &&
		c.OutputPath == "" && c.LogPath == "" && c.OutputKind == "" && c.StorageTarget == nil && !c.SkipCheckDir && !c.WeakRead && !c.EnableHiddenPk && !c.AddExtraMessage && len(c.Objects) == 0 && len(c.ObjectTypes) == 0 && len(c.IncludeColumnNames) == 0 && len(c.ExcludeColumnNames) == 0 && len(c.Partitions) == 0
}

// ScanDirectLoadInput 透传当前已确认租约，读取输入期间不开放秘密解析窗口。
func (p secretGatedProbe) ScanDirectLoadInput(ctx context.Context, r agentpreflight.Request) (importfiles.Manifest, error) {
	if ctx == nil || p.delegate == nil || p.resolver == nil {
		return importfiles.Manifest{}, agentpreflight.ErrInvalidRequest
	}
	scanner, ok := p.delegate.(agentpreflight.DirectLoadInputScanner)
	if !ok {
		return importfiles.Manifest{}, agentpreflight.ErrProbeFailed
	}
	return scanner.ScanDirectLoadInput(ctx, r)
}

// DirectLoadConfigFingerprint 读取已完成的安全缓存，不为该只读接口开放任何秘密解析窗口。
func (p secretGatedProbe) DirectLoadConfigFingerprint(ctx context.Context, r agentpreflight.Request) string {
	if ctx == nil || ctx.Err() != nil || p.delegate == nil || p.resolver == nil || r.CheckKind != "DIRECT_LOAD_PREFLIGHT" || r.DirectLoad == nil || r.NormalImport != nil {
		return ""
	}
	if reporter, ok := p.delegate.(agentpreflight.DirectLoadConfigurationReporter); ok {
		fingerprint := reporter.DirectLoadConfigFingerprint(ctx, r)
		if ctx.Err() == nil {
			return fingerprint
		}
	}
	return ""
}

func failedDirectLoadReport(g agentwire.PrecheckGrant) agentpreflight.Report {
	r := agentpreflight.Report{PrecheckID: g.PrecheckID, CheckKind: g.Context.CheckKind}
	for _, name := range precheckcontract.DirectLoadChecks(r.CheckKind) {
		r.Results = append(r.Results, agentpreflight.Result{Check: agentpreflight.CheckID(name), Status: agentpreflight.StatusUnknown, EvidenceCode: "DIRECT_LOAD_UNAVAILABLE"})
	}
	return r
}
