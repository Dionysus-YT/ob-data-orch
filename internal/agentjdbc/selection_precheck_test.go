package agentjdbc

import (
	"context"
	"reflect"
	"testing"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/jdbcprobe"
)

func Test冻结列与分区传入固定探针且变化不能复用旧结果(t *testing.T) {
	for _, access := range []jdbcprobe.ObjectAccess{jdbcprobe.ObjectAccessible, jdbcprobe.ObjectNotAccessible, jdbcprobe.ObjectUnavailable} {
		calls := 0
		request := validRequest()
		request.IncludeColumnNames = []string{"id"}
		request.Partitions = []string{"sp0"}
		probe := &PrecheckProbe{WorkspaceRoot: t.TempDir(), Resolver: &countingResolver{connection: Connection{Host: "synthetic.invalid", Port: 2883, Username: []byte("synthetic-user"), Password: []byte("synthetic-password")}},
			RunBatch: func(_ context.Context, _ credential.Workspace, _ jdbcprobe.Runtime, batch jdbcprobe.BatchPreflightRequest) (jdbcprobe.PreflightResult, error) {
				calls++
				if !reflect.DeepEqual(batch.IncludeColumnNames, request.IncludeColumnNames) || !reflect.DeepEqual(batch.Partitions, request.Partitions) {
					t.Fatal("未转交冻结筛选")
				}
				return jdbcprobe.PreflightResult{ObjectAccess: access}, nil
			},
		}
		_, _ = probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, request)
		result, err := probe.Probe(context.Background(), agentpreflight.CheckObjectAccess, request)
		want := map[jdbcprobe.ObjectAccess]string{jdbcprobe.ObjectAccessible: "OBJECT_SELECTION_VERIFIED", jdbcprobe.ObjectNotAccessible: "OBJECT_SELECTION_REJECTED", jdbcprobe.ObjectUnavailable: "OBJECT_ACCESS_UNAVAILABLE"}[access]
		if err != nil || result.EvidenceCode != want || calls != 1 {
			t.Fatal("筛选证据投影不正确", result, err)
		}
		request.IncludeColumnNames = []string{"changed"}
		result, _ = probe.Probe(context.Background(), agentpreflight.CheckObjectAccess, request)
		if result.Status != agentpreflight.StatusUnknown {
			t.Fatal("筛选变化复用了旧对象证据")
		}
		result, _ = probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, request)
		if result.Status != agentpreflight.StatusUnknown || calls != 1 {
			t.Fatal("同租约筛选变化未失败关闭")
		}
	}
}
