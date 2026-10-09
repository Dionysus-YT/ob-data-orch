package controlplane

import (
	"net/http/httptest"
	"strings"
	"testing"

	"ob-data-orch/internal/catalogresult"
)

func Test目录回执字节预算与普通机器接口隔离(t *testing.T) {
	body := `{"name":"` + strings.Repeat("a", 2<<20) + `"}`
	var target struct {
		Name string `json:"name"`
	}
	if decodeAgentJSON(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(body)), &target) {
		t.Fatal("普通 Agent 接口被放宽")
	}
	if !decodeAgentJSONLimited(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(body)), &target, catalogresult.MaxResponseBytes+(1<<20)) {
		t.Fatal("超过旧预算的目录回执被拒绝")
	}
	tooLarge := `{"name":"` + strings.Repeat("a", 18<<20) + `"}`
	if decodeAgentJSONLimited(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(tooLarge)), &target, catalogresult.MaxResponseBytes+(1<<20)) {
		t.Fatal("目录预算超限被接受")
	}
	if decodeAgentJSONLimited(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(`{"unknown":true}`)), &target, catalogresult.MaxResponseBytes+(1<<20)) {
		t.Fatal("未知字段校验被放宽")
	}
}
