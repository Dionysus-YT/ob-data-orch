package agentconnectiontest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/catalogresult"
)

// largeCatalogRunner 只提供合成目录，验证 Worker 不截断回执。
type largeCatalogRunner struct{ catalogRunnerStub }

func (runner largeCatalogRunner) RunCatalog(ctx context.Context, grant agentwire.DataSourceConnectionTestGrant) Outcome {
	outcome := runner.catalogRunnerStub.RunCatalog(ctx, grant)
	names := make([]string, 10000)
	for index := range names {
		names[index] = fmt.Sprintf("synthetic_%d", index)
	}
	if grant.Binding.CatalogObjectType == "ALL" {
		outcome.CatalogObjects = nil
		for _, kind := range []string{"TABLE", "VIEW", "FUNCTION", "PROCEDURE", "SEQUENCE"} {
			outcome.CatalogGroups = append(outcome.CatalogGroups, catalogresult.Group{ObjectType: kind, Objects: names})
		}
	} else {
		outcome.CatalogObjects = names
	}
	return outcome
}

func Test大目录在Worker回执中保持完整(t *testing.T) {
	for _, kind := range []string{"TABLE", "ALL"} {
		now := time.Date(2026, 10, 8, 8, 0, 0, 0, time.UTC)
		grant := testGrant(now, agentwire.DataSourceConnectionTestAgentJDBC)
		grant.Binding.OperationKind = "EXPORT_OBJECT_CATALOG"
		grant.Binding.CatalogDatabase, grant.Binding.CatalogCompatibilityMode, grant.Binding.CatalogObjectType = "synthetic_db", "MYSQL", kind
		protocol := &protocolStub{grant: grant}
		worker := testWorker(protocol, fixedClock(now))
		worker.JDBCRunner = largeCatalogRunner{}
		if _, found, err := worker.RunNext(context.Background()); err != nil || !found {
			t.Fatalf("Worker 未完成: %v", err)
		}
		result := protocol.completion()
		if kind == "TABLE" && (len(result.CatalogObjects) != 10000 || result.CatalogObjects[9999] != "synthetic_9999") {
			t.Fatal("单类大目录回执丢失")
		}
		if kind == "ALL" && (len(result.CatalogGroups) != 5 || len(result.CatalogGroups[4].Objects) != 10000) {
			t.Fatal("五类大目录回执丢失")
		}
	}
}
