package contracts

import "testing"

// Test审计契约关闭原始差异入口 验证固定 API、会话/CSRF、缓存与有界严格投影。
func Test审计契约关闭原始差异入口(t *testing.T) {
	spec := loadOpenAPI(t)
	paths := object(t, spec, "paths")
	schemas := object(t, object(t, spec, "components"), "schemas")
	for path, method := range map[string]string{"/api/v1/logs/audit-access": "get", "/api/v1/logs/audit": "get", "/api/v1/logs/audit-detail": "get", "/api/v1/logs/audit:download": "post"} {
		p := object(t, paths, path)
		if len(p) != 1 {
			t.Fatal("审计开放了其他方法")
		}
		op := object(t, p, method)
		if len(toolContractArray(t, op, "security")) != 1 {
			t.Fatal("审计未绑定浏览器会话")
		}
		response := object(t, object(t, op, "responses"), "200")
		if object(t, object(t, object(t, response, "headers"), "Cache-Control"), "schema")["const"] != "no-store" {
			t.Fatal("审计允许缓存")
		}
		if method == "post" && len(toolContractArray(t, op, "parameters")) != 1 {
			t.Fatal("下载未要求CSRF")
		}
	}
	for name, count := range map[string]int{"AuditQueryItem": 12, "AuditQueryPage": 8, "AuditDownloadRequest": 1, "AuditAccessResponse": 2} {
		s := object(t, schemas, name)
		if s["additionalProperties"] != false || len(toolContractArray(t, s, "required")) != count {
			t.Fatal("投影未关闭未知字段", name)
		}
	}
	p := object(t, object(t, schemas, "AuditQueryPage"), "properties")
	if object(t, p, "scannedRecords")["maximum"] != float64(50000) || object(t, p, "scannedBytes")["maximum"] != float64(64<<20) {
		t.Fatal("扫描限制缺失")
	}
}
