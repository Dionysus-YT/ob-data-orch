package executionargv_test

import (
	"strings"
	"testing"

	"ob-data-orch/internal/executionargv"
	"ob-data-orch/internal/exportdomain"
)

// TestWrappedSelectCrossesExecutionBoundary 验证真实生成器包装形态可跨越共享执行边界。
func TestWrappedSelectCrossesExecutionBoundary(t *testing.T) {
	for _, mode := range []string{"MYSQL", "ORACLE"} {
		for _, query := range []string{"SELECT id FROM synthetic_table", "SELECT\n id\nFROM synthetic_table", "SELECT '" + strings.Repeat("x", 5000) + "' AS synthetic_value"} {
			wrapped, err := exportdomain.WrapResultQuery(mode, query, 100)
			if err != nil || !executionargv.Valid([]string{"--database", "synthetic_db", "--query-sql", wrapped, "--csv", "--file-path", "/E:/synthetic/output"}) {
				t.Fatal("已验证 SELECT 包装不应被执行边界拒绝")
			}
		}
	}
}

// TestQueryExceptionDoesNotRelaxOtherArguments 覆盖 SELECT 例外之外的秘密、路径和注入负例。
func TestQueryExceptionDoesNotRelaxOtherArguments(t *testing.T) {
	for _, argv := range [][]string{
		nil, {"--query-sql"}, {"--query-sql", "DELETE FROM synthetic_table"}, {"--query-sql", "SELECT 1; SELECT 2"},
		{"--query-sql", "SELECT 1 -- comment"}, {"--query-sql", "SELECT 1 FROM file://synthetic"},
		{"--query-sql", "SELECT 1", "--query-sql", "SELECT 2"}, {"--query-sql", "SELECT '" + strings.Repeat("x", 64<<10) + "'"},
		{"--table", "synthetic\nname"}, {"--file-path", "/E:/synthetic\r\noutput"}, {"--table", strings.Repeat("x", 4097)},
		{"--query-sql", "SELECT 1", "-p", "synthetic-secret"}, {"--query-sql", "SELECT 1", "--sys-password=synthetic-secret"},
	} {
		if executionargv.Valid(argv) {
			t.Fatal("SQL 值的受控例外不能扩大秘密、路径、多语句或普通参数权限")
		}
	}
}
