package agentlocalpreflight

import (
	"context"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/directloadfiles"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/outputpath"
	"ob-data-orch/internal/precheckcontract"
)

// ScanDirectLoadInput 只读取匹配本机平台的允许根目录，保留旁路原始路径和独立单表判定。
func (p Probe) ScanDirectLoadInput(ctx context.Context, r agentpreflight.Request) (importfiles.Manifest, error) {
	if !agentpreflight.ValidDirectLoadContext(r.DirectLoad, r.CheckKind, r.CompatibilityMode, r.TargetPlatform, r.AllowedRoots) {
		return importfiles.Manifest{}, agentpreflight.ErrInvalidRequest
	}
	roots := make([]string, 0, len(r.AllowedRoots))
	for _, root := range r.AllowedRoots {
		native, ok := outputpath.LocalFilesystemPath(string(r.TargetPlatform), root)
		if !ok {
			return importfiles.Manifest{}, agentpreflight.ErrInvalidRequest
		}
		roots = append(roots, native)
	}
	return directloadfiles.Scan(ctx, *r.DirectLoad, roots)
}

// DirectLoadConfigFingerprint 仅转发本次完整旁路检查的已观测安全指纹，不借用普通导入端口。
func (p Probe) DirectLoadConfigFingerprint(ctx context.Context, r agentpreflight.Request) string {
	if ctx == nil || ctx.Err() != nil || r.DirectLoad == nil || r.NormalImport != nil || r.CheckKind != "DIRECT_LOAD_PREFLIGHT" {
		return ""
	}
	if reporter, ok := p.DirectLoadChecks.(agentpreflight.DirectLoadConfigurationReporter); ok {
		fingerprint := reporter.DirectLoadConfigFingerprint(ctx, r)
		if ctx.Err() == nil {
			return fingerprint
		}
	}
	return ""
}

func (p Probe) directLoadResult(ctx context.Context, check agentpreflight.CheckID, r agentpreflight.Request) (agentpreflight.Result, error) {
	result := agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: "DIRECT_LOAD_PROBE_NOT_CONFIGURED"}
	if ctx == nil || ctx.Err() != nil {
		return result, nil
	}
	// 复用的只有本机运行时布局与资源事实；不调用普通导入数据库探针，也不取得其秘密解析许可。
	if check == agentpreflight.CheckToolEnvironment || check == agentpreflight.CheckNodeResource {
		var observed agentpreflight.Result
		if check == agentpreflight.CheckToolEnvironment {
			copy := p
			copy.Runtime = p.ImportRuntime
			observed = copy.runtimeResult(ctx, check)
		} else {
			observed = nodeResourceResult(r)
		}
		result.Status = observed.Status
		switch observed.Status {
		case agentpreflight.StatusPassed:
			result.EvidenceCode = "DIRECT_LOAD_VERIFIED"
		case agentpreflight.StatusFailed:
			result.EvidenceCode = "DIRECT_LOAD_REJECTED"
		default:
			result.EvidenceCode = "DIRECT_LOAD_UNAVAILABLE"
		}
		return result, nil
	}
	if p.DirectLoadChecks != nil {
		observed, err := p.DirectLoadChecks.Probe(ctx, check, r)
		if err == nil && ctx.Err() == nil && observed.Check == check && precheckcontract.ValidDirectLoadResult(string(check), string(observed.Status), observed.EvidenceCode) {
			return observed, nil
		}
		result.EvidenceCode = "DIRECT_LOAD_UNAVAILABLE"
	}
	return result, nil
}
