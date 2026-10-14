package agentjdbc

import (
	"context"
	"testing"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/jdbcprobe"
)

func TestParameterPrecheckUsesDedicatedProbeAndClearsSysBuffers(t *testing.T) {
	for _, access := range []jdbcprobe.ObjectAccess{jdbcprobe.ObjectAccessible, jdbcprobe.ObjectNotAccessible, jdbcprobe.ObjectUnavailable} {
		t.Run(string(access), func(t *testing.T) {
			username, password := []byte("synthetic_user"), []byte("synthetic_password")
			sysUsername, sysPassword := []byte("synthetic_sys@sys"), []byte("synthetic_sys_password")
			resolver := &countingResolver{connection: Connection{Host: "synthetic.invalid", Port: 2883, Username: username, Password: password, SysUsername: sysUsername, SysPassword: sysPassword}}
			calls := 0
			probe := &PrecheckProbe{WorkspaceRoot: t.TempDir(), Resolver: resolver, RunParameters: func(_ context.Context, _ credential.Workspace, _ jdbcprobe.Runtime, input jdbcprobe.ParameterPreflightRequest) (jdbcprobe.PreflightResult, error) {
				calls++
				if !input.WeakRead || !input.EnableHiddenPk || !input.AddExtraMessage || string(input.SysConnection.Password) != "synthetic_sys_password" || len(input.Objects) != 1 {
					t.Fatal("固定参数、对象或 sys 槽位未传入专用探针")
				}
				return jdbcprobe.PreflightResult{ObjectAccess: access}, nil
			}, RunBatch: func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.BatchPreflightRequest) (jdbcprobe.PreflightResult, error) {
				t.Fatal("普通探针替代了参数检查")
				return jdbcprobe.PreflightResult{}, nil
			}}
			request := validRequest()
			request.ContentKind, request.WeakRead, request.EnableHiddenPk, request.AddExtraMessage = "DDL_AND_DATA", true, true, true
			_, _ = probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, request)
			result, _ := probe.Probe(context.Background(), agentpreflight.CheckObjectAccess, request)
			want := agentpreflight.StatusUnknown
			if access == jdbcprobe.ObjectAccessible {
				want = agentpreflight.StatusPassed
			}
			if access == jdbcprobe.ObjectNotAccessible {
				want = agentpreflight.StatusFailed
			}
			if result.Status != want || want == agentpreflight.StatusPassed && result.EvidenceCode != "PARAMETER_REQUIREMENTS_VERIFIED" || calls != 1 || resolver.calls != 1 {
				t.Fatalf("参数证据或一次性边界错误: %#v", result)
			}
			for _, buffer := range [][]byte{username, password, sysUsername, sysPassword} {
				if !allZero(buffer) {
					t.Fatal("槽位返回后仍有短时材料")
				}
			}
			request.WeakRead = false
			changed, _ := probe.Probe(context.Background(), agentpreflight.CheckObjectAccess, request)
			if changed.Status != agentpreflight.StatusUnknown {
				t.Fatal("参数变化复用了旧证据")
			}
		})
	}
}
