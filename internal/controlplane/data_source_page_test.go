package controlplane

import (
	"fmt"
	"net/url"
	"testing"
)

func TestDataSourcePageFiltersAndCursor(t *testing.T) {
	items := make([]dataSourceListResponse, 23)
	for i := range items {
		items[i].ID = fmt.Sprintf("source-%02d", i)
		items[i].DisplayName = "Synthetic"
		items[i].Environment = "TEST"
	}
	query := url.Values{"limit": {"10"}, "keyword": {" synthetic "}}
	first, next, total, err := dataSourcePage(items, query, "subject-a")
	if err != nil || len(first) != 10 || total != 23 || next == "" {
		t.Fatalf("first page: %d %d %v", len(first), total, err)
	}
	query.Set("cursor", next)
	second, next, _, err := dataSourcePage(items, query, "subject-a")
	if err != nil || len(second) != 10 || second[0].ID != "source-10" {
		t.Fatal("second page mismatch")
	}
	query.Set("cursor", next)
	last, next, _, err := dataSourcePage(items, query, "subject-a")
	if err != nil || len(last) != 3 || next != "" {
		t.Fatal("last page mismatch")
	}
	if _, _, _, err := dataSourcePage(items, query, "subject-b"); err == nil {
		t.Fatal("cross-subject cursor accepted")
	}
	query.Set("environment", "PRODUCTION")
	if _, _, _, err := dataSourcePage(items, query, "subject-a"); err == nil {
		t.Fatal("cross-filter cursor accepted")
	}
	query.Del("cursor")
	empty, _, count, err := dataSourcePage(items, query, "subject-a")
	if err != nil || len(empty) != 0 || count != 0 {
		t.Fatal("filter did not exclude rows")
	}
	query.Set("limit", "11")
	if _, _, _, err := dataSourcePage(items, query, "subject-a"); err == nil {
		t.Fatal("invalid page size accepted")
	}
}
