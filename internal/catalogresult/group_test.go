package catalogresult

import "testing"

func TestValidGroups(t *testing.T) {
	groups := []Group{
		{ObjectType: "TABLE", Objects: []string{"orders"}},
		{ObjectType: "VIEW", Objects: []string{"v_orders"}},
		{ObjectType: "FUNCTION", Objects: []string{}, Unavailable: true},
		{ObjectType: "PROCEDURE", Objects: []string{}},
		{ObjectType: "SEQUENCE", Objects: []string{"seq_id"}},
	}
	if !ValidGroups(groups) {
		t.Fatal("合法批量目录被拒绝")
	}
	invalid := append([]Group(nil), groups...)
	invalid[2].Objects = []string{"hidden"}
	if ValidGroups(invalid) {
		t.Fatal("不可用分类暴露对象名")
	}
	invalid = append([]Group(nil), groups...)
	invalid[1].ObjectType = "TABLE"
	if ValidGroups(invalid) {
		t.Fatal("重复分类被接受")
	}
	invalid = append([]Group(nil), groups...)
	invalid[0].Objects = []string{"orders", "orders"}
	if ValidGroups(invalid) {
		t.Fatal("重复对象名被接受")
	}
}
