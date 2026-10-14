package precheckcontract

import "testing"

func Test名称仅作固定元数据字面值且非法列表失败关闭(t *testing.T) {
	for _, names := range [][]string{{""}, {"id", "id"}, {"*"}, {"a,b"}, {" id"}, {"id\n"}, {string([]byte{255})}} {
		if ValidSelectionIdentifiers(names) {
			t.Fatal("非法列表被接受")
		}
	}
	if !ValidSelectionIdentifiers([]string{"中文列", "Id"}) {
		t.Fatal("合法字面名称被拒绝")
	}
	if ValidSelectionIdentifiers(make([]string, 4097)) {
		t.Fatal("无限列表被接受")
	}
}
