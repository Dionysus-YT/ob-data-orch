package controlplane

import (
	"context"
	"strings"
	"testing"

	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/store"
)

func Test旁路直连命令保留SQL身份和RPC租户(t *testing.T) {
	g, err := commandgen.NewDirectLoad()
	if err != nil {
		t.Fatal(err)
	}
	source, _ := (staticDataSourceReader{}).GetDataSourceSummary(context.Background(), "source-allowed")
	source.ConnectionKind, source.ClusterName = "OBSERVER_DIRECT", ""
	s := &Server{dataSource: staticDataSources{summaries: []store.DataSourceSummary{source}}, credentials: staticCredentialReferenceReader{}, nodes: staticNodeReader{}, directLoadGenerator: g}
	c := directload.Config{DataSourceID: source.DataSourceID, NodeID: "node-1", Database: "synthetic_db", Table: "synthetic_table", Scenario: "OBSERVER_DIRECT", RPCPort: 2882, FilePath: `E:\synthetic\input`, Format: "CSV", Applicability: directload.Applicability{SingleTable: true, WholeTableCommit: true, NoCheckpoint: true, StructureLimits: true}}
	command, _, _, err := s.generateDirectLoad(context.Background(), c)
	if err != nil {
		t.Fatal("直连生成失败", err)
	}
	joined := strings.Join(command.ArgvTemplate, " ")
	if !strings.Contains(joined, "--tenant synthetic-tenant") || !strings.Contains(joined, "synthetic-user@synthetic-tenant") || strings.Contains(joined, "--cluster") {
		t.Fatal("直连身份或租户映射错误")
	}
	for _, kind := range []string{"PUBLIC_CLOUD", "ODP"} {
		source.ConnectionKind = kind
		s.dataSource = staticDataSources{summaries: []store.DataSourceSummary{source}}
		if _, _, _, err := s.generateDirectLoad(context.Background(), c); err == nil {
			t.Fatal("连接方式错配被接受", kind)
		}
	}
}

func Test直连数据源字段拒绝代理集群(t *testing.T) {
	fields := dataSourceWritableFields{DisplayName: "synthetic", Environment: "TEST", ConnectionKind: "OBSERVER_DIRECT", CompatibilityMode: "MYSQL", Host: "synthetic.invalid", Port: 2881, TenantName: "synthetic_tenant", Username: "synthetic_user"}
	if len(validateDataSourceWriteFields(fields)) != 0 {
		t.Fatal("有效直连字段被拒绝")
	}
	fields.ClusterName = "synthetic_cluster"
	issues := validateDataSourceWriteFields(fields)
	if len(issues) != 1 || issues[0].Field != "clusterName" {
		t.Fatal("直连代理后缀未阻断")
	}
}
