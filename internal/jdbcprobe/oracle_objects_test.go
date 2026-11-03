package jdbcprobe

import (
	"testing"

	"ob-data-orch/internal/catalogresult"
	"ob-data-orch/internal/exportobject"
)

func TestOracle十类冻结对象帧及目录完整性(t *testing.T) {
	request := BatchPreflightRequest{Connection: Request{Host: "synthetic.invalid", Port: 2883, Username: []byte("synthetic-user"), Password: []byte("synthetic-password")}, CompatibilityMode: CompatibilityModeOracle, Database: "synthetic_schema"}
	var groups []catalogresult.Group
	for _, kind := range exportobject.Types() {
		request.Objects = append(request.Objects, PreflightObject{Type: kind, Name: "synthetic_same_name"})
		groups = append(groups, catalogresult.Group{ObjectType: kind, Objects: []string{"synthetic_same_name"}})
	}
	if !validBatchPreflightRequest(request) || !catalogresult.ValidGroupsForMode(groups, "ORACLE") {
		t.Fatal("完整 Oracle 冻结对象未被接受")
	}
	request.CompatibilityMode = CompatibilityModeMySQL
	if validBatchPreflightRequest(request) || catalogresult.ValidGroupsForMode(groups, "MYSQL") {
		t.Fatal("MySQL 接受了 Oracle 新类型")
	}
	if catalogresult.ValidGroupsForMode(groups[:5], "ORACLE") {
		t.Fatal("缺类被接受为完整目录")
	}
}
