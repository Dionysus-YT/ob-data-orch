package agentexec

import (
	"context"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/credential"
)

// AdapterResult 是固定预检查与假工具执行的合成链路结果。
// Execution 为空表示预检查未通过或被拒绝，因而没有调用任何工具端口。
type AdapterResult struct {
	Preflight agentpreflight.Report
	Execution *RunResult
}

// ExecuteSyntheticExport 先运行固定 EXPORT_PREFLIGHT，只有全部通过后才运行假工具。
// 该入口仅用于 G2 合成验证，不具备真实凭据、网络、OBDUMPER 或操作系统进程能力。
func ExecuteSyntheticExport(ctx context.Context, preflight agentpreflight.Request, probe agentpreflight.Probe, workspace credential.Workspace, launch SyntheticLaunch, tool SyntheticTool) (AdapterResult, error) {
	report, err := agentpreflight.Run(ctx, preflight, probe)
	if err != nil {
		return AdapterResult{}, err
	}
	result := AdapterResult{Preflight: report}
	if !report.Succeeded {
		return result, nil
	}
	execution, err := RunSynthetic(ctx, workspace, launch, tool)
	if err != nil {
		return result, err
	}
	result.Execution = &execution
	return result, nil
}
