package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/catalogresult"
	"ob-data-orch/internal/exportobject"
)

func TestOracle新分类与十类目录在前向迁移后回读(t *testing.T) {
	for _, kind := range []string{"PACKAGE", "TRIGGER", "SYNONYM", "PUBLIC_SYNONYM", "TYPE", "ALL"} {
		t.Run(kind, func(t *testing.T) {
			database, _ := openTestStore(t)
			seedBaseFixture(t, database)
			resetConnectionTestFacts(t, database)
			prepareConnectionTestAgentFacts(t, database)
			if _, err := database.db.Exec(`UPDATE data_sources SET compatibility_mode='ORACLE', state='ENABLED', last_test_status='SUCCEEDED', last_test_source='AGENT_JDBC' WHERE data_source_id='source-1'`); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			input := syntheticDataSourceConnectionTestInput("oracle-catalog-"+kind, "AGENT_JDBC")
			input.OperationKind, input.CatalogDatabase, input.CatalogObjectType = "EXPORT_OBJECT_CATALOG", "synthetic_schema", kind
			if _, err := database.RequestDataSourceConnectionTest(ctx, input); err != nil {
				t.Fatal(err)
			}
			run, err := database.GetDataSourceConnectionTestRun(ctx, input.ConnectionTestID)
			if err != nil || run.CatalogObjectType != kind {
				t.Fatalf("新分类未正确持久化: %v", err)
			}
			grant := claimAndAcknowledgeDataSourceConnectionTest(t, database, input.ConnectionTestID, testTime.Add(5*time.Second))
			completion := AgentDataSourceConnectionTestCompletion{AgentID: "agent-1", ConnectionTestID: input.ConnectionTestID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, BindingDigest: grant.Binding.BindingDigest, RequestID: "synthetic-oracle-complete", RequestDigest: strings.Repeat("e", 64), Status: "SUCCEEDED", EvidenceCode: "DATABASE_CONNECTED", VerificationSource: "AGENT_JDBC", SysVerificationStatus: "NOT_CONFIGURED", Now: testTime.Add(2 * time.Minute)}
			if kind == "ALL" {
				for _, objectType := range exportobject.Types() {
					completion.CatalogGroups = append(completion.CatalogGroups, catalogresult.Group{ObjectType: objectType, Objects: []string{"synthetic_same_name"}})
				}
				invalid := completion
				invalid.CatalogGroups = invalid.CatalogGroups[:5]
				if _, err := database.CompleteAgentDataSourceConnectionTest(ctx, invalid); !errors.Is(err, ErrDataSourceConnectionTestLeaseRejected) {
					t.Fatal("缺类结果未失败关闭")
				}
			} else {
				completion.CatalogObjects = []string{"synthetic_same_name"}
			}
			if _, err := database.CompleteAgentDataSourceConnectionTest(ctx, completion); err != nil {
				t.Fatal(err)
			}
			run, err = database.GetDataSourceConnectionTestRun(ctx, input.ConnectionTestID)
			if err != nil || run.CatalogObjectType != kind || kind == "ALL" && len(run.CatalogGroups) != 10 || kind != "ALL" && len(run.CatalogObjects) != 1 {
				t.Fatalf("对象目录回读丢失: %v", err)
			}
		})
	}
}

func TestMySQL目录拒绝Oracle新分类(t *testing.T) {
	database, _ := openTestStore(t)
	seedBaseFixture(t, database)
	resetConnectionTestFacts(t, database)
	prepareConnectionTestAgentFacts(t, database)
	if _, err := database.db.Exec(`UPDATE data_sources SET state='ENABLED', last_test_status='SUCCEEDED', last_test_source='AGENT_JDBC' WHERE data_source_id='source-1'`); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"PACKAGE", "TRIGGER", "SYNONYM", "PUBLIC_SYNONYM", "TYPE"} {
		input := syntheticDataSourceConnectionTestInput("rejected-oracle-"+kind, "AGENT_JDBC")
		input.OperationKind, input.CatalogDatabase, input.CatalogObjectType = "EXPORT_OBJECT_CATALOG", "synthetic_schema", kind
		if _, err := database.RequestDataSourceConnectionTest(context.Background(), input); !errors.Is(err, ErrDataSourceConnectionTestInvalid) {
			t.Fatalf("MySQL 接受 Oracle 对象 %s: %v", kind, err)
		}
	}
}
