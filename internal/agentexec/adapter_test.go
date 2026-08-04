package agentexec

import (
	"context"
	"testing"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/commandgen"
)

func Test预检查失败绝不启动假工具(t *testing.T) {
	workspace := createWorkspace(t)
	defer cleanupWorkspace(t, workspace)
	tool := &countingTool{scriptedTool: scriptedTool{process: scriptedProcess{identity: validProcess()}}}
	probe := fixedPreflightProbe{status: agentpreflight.StatusFailed}
	result, err := ExecuteSyntheticExport(context.Background(), validPreflightRequest(), probe, workspace, validLaunch(), tool)
	if err != nil {
		t.Fatalf("ExecuteSyntheticExport() 错误 = %v", err)
	}
	if result.Preflight.Succeeded || result.Execution != nil || tool.starts != 0 {
		t.Fatalf("失败预检查结果 = %#v，假工具启动次数 = %d", result, tool.starts)
	}
}

func Test预检查通过后才启动假工具(t *testing.T) {
	workspace := createWorkspace(t)
	defer cleanupWorkspace(t, workspace)
	tool := &countingTool{scriptedTool: scriptedTool{process: scriptedProcess{identity: validProcess(), observation: Observation{ProcessExited: true, ToolTerminal: agentstate.ToolSuccess, ResultFacts: agentstate.ResultVerified}}}}
	result, err := ExecuteSyntheticExport(context.Background(), validPreflightRequest(), fixedPreflightProbe{status: agentpreflight.StatusPassed}, workspace, validLaunch(), tool)
	if err != nil || !result.Preflight.Succeeded || result.Execution == nil || tool.starts != 1 {
		t.Fatalf("ExecuteSyntheticExport() = %#v, %v，假工具启动次数 = %d", result, err, tool.starts)
	}
}

func validPreflightRequest() agentpreflight.Request {
	return agentpreflight.Request{Capability: agentpreflight.CapabilityExportPreflight, PrecheckID: "precheck-1", NodeID: "node-1", AgentID: "agent-1", LeaseID: "precheck-lease-1", LeaseEpoch: 1, Binding: agentstate.PrecheckBinding{PrecheckID: "precheck-1", NodeID: "node-1", DraftRevision: 1, ConfigFingerprint: "synthetic-fingerprint", CredentialRevision: 1, NodeFactsVersion: 1}, CompatibilityMode: "MYSQL", Database: "synthetic_db", Table: "synthetic_table", TargetPlatform: commandgen.PlatformWindowsAMD64, OutputPath: "/E:/synthetic/output", AllowedRoots: []string{`E:\synthetic`}}
}

type fixedPreflightProbe struct {
	status agentpreflight.Status
}

func (probe fixedPreflightProbe) Probe(_ context.Context, check agentpreflight.CheckID, _ agentpreflight.Request) (agentpreflight.Result, error) {
	return agentpreflight.Result{Check: check, Status: probe.status, EvidenceCode: "SYNTHETIC_OK"}, nil
}

type countingTool struct {
	scriptedTool
	starts int
}

func (tool *countingTool) Start(ctx context.Context, launch SyntheticLaunch) (SyntheticProcess, error) {
	tool.starts++
	return tool.scriptedTool.Start(ctx, launch)
}
