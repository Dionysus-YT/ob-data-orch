package store

import (
	"fmt"
	"testing"

	"ob-data-orch/internal/catalogresult"
)

func Test大对象目录结果校验与部分结果失败关闭(t *testing.T) {
	names := make([]string, 10000)
	for index := range names {
		names[index] = fmt.Sprintf("synthetic_%d", index)
	}
	groups := []catalogresult.Group{}
	for _, kind := range []string{"TABLE", "VIEW", "FUNCTION", "PROCEDURE", "SEQUENCE"} {
		if !validCatalogResult("SUCCEEDED", names, nil, false, kind) {
			t.Fatalf("大目录被拒绝: %s", kind)
		}
		if validCatalogResult("SUCCEEDED", names, nil, true, kind) {
			t.Fatal("截断结果被接受")
		}
		groups = append(groups, catalogresult.Group{ObjectType: kind, Objects: names})
	}
	if !validCatalogResult("SUCCEEDED", nil, groups, false, "ALL") {
		t.Fatal("批量大目录被拒绝")
	}
	if validCatalogResult("SUCCEEDED", names, nil, false, "DATABASE") || validCatalogResult("FAILED", names, nil, false, "TABLE") {
		t.Fatal("失败关闭边界失效")
	}
	groups[4].Truncated = true
	if validCatalogResult("SUCCEEDED", nil, groups, false, "ALL") {
		t.Fatal("批量部分结果被接受")
	}
}
