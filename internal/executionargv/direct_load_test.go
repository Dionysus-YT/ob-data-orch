package executionargv_test

import (
	"testing"

	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/executionargv"
)

func Test旁路执行参数仅允许单一冻结场景(t *testing.T) {
	c := directload.Config{Database: "synthetic_db", Scenario: "PRIVATE_ODP", RPCPort: 2885, Table: "synthetic", FilePath: `E:\synthetic\input`, Format: "CSV", Thread: "2", Parallel: "3"}
	base := []string{"--host", "127.0.0.1", "--port", "2883", "--user", "synthetic", "--tenant", "synthetic_tenant", "--cluster", "synthetic_cluster", "--database", c.Database, "--file-path", c.FilePath, "--table", c.Table, "--csv", "--direct", "--rpc-port", "2885", "--thread", "2", "--parallel", "3"}
	if !executionargv.ValidDirectLoad(base) || !executionargv.MatchesDirectLoad(base, c) {
		t.Fatal("已冻结旁路参数被拒绝")
	}
	for _, extra := range [][]string{{"--query-sql", "SELECT 1"}, {"--retry"}, {"--no-sys"}, {"--password", "synthetic-secret"}, {"--sys-user", "synthetic"}, {"--csv"}, {"--sql"}, {"--public-cloud"}, {"--replace-data"}, {"--truncate-table"}, {"--yes"}, {"--rpc-port", "2885"}, {"--unknown"}} {
		if executionargv.MatchesDirectLoad(append(append([]string{}, base...), extra...), c) {
			t.Fatal("越界或未冻结参数被接受")
		}
	}
	for _, mutate := range []func(*directload.Config){func(c *directload.Config) { c.Table = "other" }, func(c *directload.Config) { c.Database = "other" }, func(c *directload.Config) { c.FilePath += "-other" }, func(c *directload.Config) { c.RPCPort++ }, func(c *directload.Config) { c.Thread = "3" }, func(c *directload.Config) { c.Format = "SQL" }, func(c *directload.Config) { c.Scenario = "CLOUD_ODP" }} {
		other := c
		mutate(&other)
		if executionargv.MatchesDirectLoad(base, other) {
			t.Fatal("冻结配置漂移被接受")
		}
	}
}

func Test旁路参数拒绝缺项非法整数与确认不配对(t *testing.T) {
	base := []string{"--host", "127.0.0.1", "--port", "2881", "--user", "synthetic", "--database", "synthetic_db", "--file-path", "/synthetic/input", "--table", "synthetic", "--csv", "--direct", "--rpc-port", "2882"}
	for _, flag := range []string{"--sql", "--par", "--orc"} {
		argv := append([]string{}, base...)
		argv[12] = flag
		if executionargv.ValidDirectLoad(argv) {
			t.Fatal("历史目录不能授予本地未确认格式执行资格", flag)
		}
	}
	if executionargv.ValidDirectLoad(append(append([]string{}, base...), "--public-cloud", "--tenant", "synthetic_tenant")) {
		t.Fatal("历史目录不能授予其他部署场景执行资格")
	}
	for _, raw := range []string{"0", "65536", "01", "-1", "1.5", "unknown"} {
		argv := append([]string{}, base...)
		argv[len(argv)-1] = raw
		if executionargv.ValidDirectLoad(argv) {
			t.Fatal("非法 RPC 端口被接受")
		}
	}
	for _, argv := range [][]string{nil, base[:len(base)-1], base[:len(base)-2], append(append([]string{}, base...), "--truncate-table"), append(append([]string{}, base...), "--yes"), append(append([]string{}, base...), "--thread", "0")} {
		if executionargv.ValidDirectLoad(argv) {
			t.Fatal("不完整或确认未配对的命令被接受")
		}
	}
	c := directload.Config{Database: "synthetic_db", Scenario: "OBSERVER_DIRECT", RPCPort: 2882, Table: "synthetic", FilePath: "/synthetic/input", Format: "CSV", TruncateTable: true}
	if executionargv.MatchesDirectLoad(append(append([]string{}, base...), "--truncate-table", "--yes"), c) {
		t.Fatal("直连缺少 RPC 租户参数未被阻断")
	}
	if !executionargv.MatchesDirectLoad(append(append([]string{}, base...), "--tenant", "synthetic_tenant", "--truncate-table", "--yes"), c) {
		t.Fatal("服务端派生的清空确认被拒绝")
	}
}
