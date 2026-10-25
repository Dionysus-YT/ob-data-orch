package contracts

import "testing"

// Test日志中心契约保持任务权限与固定快照 验证四个固定入口、必需 CSRF 和严格投影，不扩为原始文件接口。
func Test日志中心契约保持任务权限与固定快照(t *testing.T) {
	spec := loadOpenAPI(t)
	paths := object(t, spec, "paths")
	schemas := object(t, object(t, spec, "components"), "schemas")
	for path, method := range map[string]string{"/api/v1/logs/tasks": "get", "/api/v1/logs/task-context": "get", "/api/v1/logs/tasks:download": "post", "/api/v1/logs/task-context:copy": "post"} {
		p := object(t, paths, path)
		if len(p) != 1 {
			t.Fatal("日志入口意外开放其他方法")
		}
		op := object(t, p, method)
		response := object(t, object(t, op, "responses"), "200")
		if object(t, object(t, object(t, response, "headers"), "Cache-Control"), "schema")["const"] != "no-store" {
			t.Fatal("日志响应允许缓存")
		}
		if method == "post" && len(toolContractArray(t, op, "parameters")) != 1 {
			t.Fatal("缺少独立 CSRF")
		}
	}
	for name, count := range map[string]int{"TaskLogQueryItem": 10, "TaskLogQueryPage": 12, "TaskLogQueryContext": 4, "TaskLogDownloadRequest": 2, "TaskLogContextCopyRequest": 1} {
		s := object(t, schemas, name)
		if s["additionalProperties"] != false || len(toolContractArray(t, s, "required")) != count {
			t.Fatal("日志投影未关闭未知字段", name)
		}
	}
	p := object(t, object(t, schemas, "TaskLogQueryPage"), "properties")
	if object(t, p, "scannedRecords")["maximum"] != float64(50000) || object(t, p, "scannedBytes")["maximum"] != float64(64<<20) {
		t.Fatal("扫描限制丢失")
	}
}
