package exportdomain

import (
	"strings"
	"testing"
)

func TestWrapResultQuery(t *testing.T) {
	for _, test := range []struct{ mode, suffix string }{
		{"MYSQL", ") AS obdo_result LIMIT 1000"},
		{"ORACLE", ") obdo_result WHERE ROWNUM <= 1000"},
	} {
		got, err := WrapResultQuery(test.mode, " SELECT id FROM orders ORDER BY id ", 1000)
		if err != nil || !strings.HasSuffix(got, test.suffix) || !strings.Contains(got, "SELECT id FROM orders ORDER BY id") {
			t.Fatalf("mode=%s query=%q err=%v", test.mode, got, err)
		}
	}
}

func TestWrapResultQueryRejectsUnsafeOrUnsupportedInput(t *testing.T) {
	for _, query := range []string{"", "DELETE FROM orders", "SELECT 1; DELETE FROM orders", "SELECT 1 -- bypass", "SELECT 1 /* bypass */", "SELECT 1 FROM file://path"} {
		if _, err := WrapResultQuery("MYSQL", query, 1000); err == nil {
			t.Fatalf("query %q was accepted", query)
		}
	}
	for _, limit := range []int64{0, -1, MaxQueryResultRows + 1} {
		if _, err := WrapResultQuery("MYSQL", "SELECT 1", limit); err == nil {
			t.Fatalf("limit %d was accepted", limit)
		}
	}
	if _, err := WrapResultQuery("UNKNOWN", "SELECT 1", 1000); err == nil {
		t.Fatal("unknown compatibility mode was accepted")
	}
}
