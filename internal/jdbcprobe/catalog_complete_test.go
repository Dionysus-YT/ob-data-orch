package jdbcprobe

import (
	"encoding/json"
	"fmt"
	"testing"

	"ob-data-orch/internal/catalogresult"
)

func Test对象目录完整读取大集合并拒绝截断(t *testing.T) {
	names := make([]string, 10000)
	for index := range names {
		names[index] = fmt.Sprintf("synthetic_%d", index)
	}
	groups := make([]catalogresult.Group, 0, 5)
	for _, kind := range []string{"TABLE", "VIEW", "FUNCTION", "PROCEDURE", "SEQUENCE"} {
		body, _ := json.Marshal(map[string]any{"status": "SUCCESS", "objects": names, "truncated": false})
		result, err := parseCatalogResponseForType(body, kind)
		if err != nil || len(result.Objects) != len(names) || result.Objects[9999] != names[9999] {
			t.Fatalf("%s 大目录不完整: %v", kind, err)
		}
		groups = append(groups, catalogresult.Group{ObjectType: kind, Objects: names})
	}
	body, _ := json.Marshal(map[string]any{"status": "SUCCESS", "objects": []string{}, "groups": groups, "truncated": false})
	if result, err := parseCatalogResponseForType(body, "ALL"); err != nil || len(result.Groups[4].Objects) != 10000 {
		t.Fatalf("五类大目录不完整: %v", err)
	}
	groups[0].Truncated = true
	body, _ = json.Marshal(map[string]any{"status": "SUCCESS", "objects": []string{}, "groups": groups, "truncated": false})
	if _, err := parseCatalogResponseForType(body, "ALL"); err == nil {
		t.Fatal("旧节点截断分组被接受")
	}
	body, _ = json.Marshal(map[string]any{"status": "SUCCESS", "objects": names, "truncated": false})
	if _, err := parseCatalogResponseForType(body, "DATABASE"); err == nil {
		t.Fatal("数据库目录限制被意外取消")
	}
	if _, err := parseCatalogResponse([]byte(`{"status":"SUCCESS","objects":["synthetic"],"truncated":true}`)); err == nil {
		t.Fatal("单类截断结果被接受")
	}
}
