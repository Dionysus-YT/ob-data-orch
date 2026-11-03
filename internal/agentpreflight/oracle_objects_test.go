package agentpreflight

import (
	"testing"

	"ob-data-orch/internal/exportobject"
)

func TestOracle十类冻结对象仅允许结构内容(t *testing.T) {
	request := validRequest()
	request.CompatibilityMode, request.ContentKind = "ORACLE", "DDL_AND_DATA"
	request.ObjectTypes = exportobject.Types()
	request.Objects = nil
	for range request.ObjectTypes {
		request.Objects = append(request.Objects, "synthetic_same_name")
	}
	if err := validateRequest(request); err != nil {
		t.Fatalf("Oracle 结构和表数据混选被拒绝: %v", err)
	}
	request.ContentKind = "DDL_ONLY"
	if err := validateRequest(request); err != nil {
		t.Fatal(err)
	}
	request.CompatibilityMode = "MYSQL"
	if validateRequest(request) == nil {
		t.Fatal("MySQL 接受了 Oracle 专属对象")
	}
	request.CompatibilityMode, request.ContentKind = "ORACLE", "DATA_ONLY"
	if validateRequest(request) == nil {
		t.Fatal("纯数据内容接受了非表对象")
	}
}
