package controlplane

import (
	"testing"
	"time"

	"ob-data-orch/internal/store"
)

func TestCatalogQueryResponsePendingClaimDeadline(t *testing.T) {
	now := time.Date(2026, 9, 30, 1, 0, 0, 0, time.UTC)
	base := store.DataSourceConnectionTestRun{
		Status: "PENDING", CreatedAt: now.Add(-store.ExportObjectCatalogClaimTimeout), ValidUntil: now.Add(time.Minute),
	}
	if got := catalogQueryResponse(base, now)["status"]; got != "EXPIRED" {
		t.Fatalf("超过领取期限的查询状态 = %v", got)
	}
	base.CreatedAt = base.CreatedAt.Add(time.Nanosecond)
	if got := catalogQueryResponse(base, now)["status"]; got != "PENDING" {
		t.Fatalf("领取期限内的查询状态 = %v", got)
	}
	base.Status = "LEASED"
	base.CreatedAt = now.Add(-store.ExportObjectCatalogClaimTimeout - time.Minute)
	if got := catalogQueryResponse(base, now)["status"]; got != "LEASED" {
		t.Fatalf("已领取的查询不应受排队期限影响: %v", got)
	}
}

func Test导出数据库目录只接受空数据库名和受限类型(t *testing.T) {
	base := exportObjectCatalogRequest{NodeID: "synthetic-node", ObjectType: "DATABASE", Keyword: "app"}
	if !validExportCatalogRequest(base) {
		t.Fatal("应允许有界数据库目录查询")
	}
	base.Database = "synthetic_db"
	if validExportCatalogRequest(base) {
		t.Fatal("数据库目录不应接受指定数据库名")
	}
	base.ObjectType = "TABLE"
	if !validExportCatalogRequest(base) {
		t.Fatal("指定数据库的表查询应保持可用")
	}
	base.Database = ""
	if validExportCatalogRequest(base) {
		t.Fatal("表目录不能扩大到全库查询")
	}
	base.ObjectType = "SCHEMA"
	if validExportCatalogRequest(base) {
		t.Fatal("未知元数据类型必须失败关闭")
	}
}
