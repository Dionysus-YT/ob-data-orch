package agentjdbc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/jdbcprobe"
)

func normalImportFixture(t *testing.T) agentpreflight.Request {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "synthetic_table.csv"), []byte("1,'synthetic'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	platform := commandgen.PlatformLinuxAMD64
	if runtime.GOOS == "windows" {
		platform = commandgen.PlatformWindowsAMD64
	}
	c, issues := importdomain.Normalize(importdomain.Config{DataSourceID: "source-1", NodeID: "node-1", Database: "synthetic_db", FilePath: root, Content: "DATA_ONLY", Format: "CSV", Scope: "ALL", Cleanup: "NONE"}, "MYSQL", string(platform), true)
	if len(issues) > 0 {
		t.Fatal("合成配置无效", issues)
	}
	manifest, err := importfiles.Scan(context.Background(), c, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	r := validRequest()
	r.CheckKind = "NORMAL_IMPORT_PREFLIGHT"
	r.NormalImport = &c
	r.InputManifest = &manifest
	r.TargetPlatform = platform
	r.AllowedRoots = []string{root}
	return r
}

func Test普通导入CSV空白损失不能被成功JDBC结果覆盖(t *testing.T) {
	r := normalImportFixture(t)
	if err := os.WriteFile(filepath.Join(r.NormalImport.FilePath, "synthetic_table.csv"), []byte("1, padded \n"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest, err := importfiles.Scan(context.Background(), *r.NormalImport, r.AllowedRoots)
	if err != nil {
		t.Fatal(err)
	}
	r.InputManifest = &manifest
	p := &NormalImportProbe{WorkspaceRoot: t.TempDir(), Resolver: &countingResolver{connection: Connection{Host: "synthetic.invalid", Port: 2883, Username: []byte("synthetic"), Password: []byte("synthetic-only")}}, Run: func(_ context.Context, _ credential.Workspace, _ jdbcprobe.Runtime, _ jdbcprobe.NormalImportRequest) (jdbcprobe.NormalImportResult, error) {
		return jdbcprobe.NormalImportResult{DatabaseAccess: jdbcprobe.ObjectAccessible, WriteAccess: jdbcprobe.ObjectAccessible, InputMapping: jdbcprobe.ObjectAccessible}, nil
	}}
	_, _ = p.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, r)
	result, err := p.Probe(context.Background(), "INPUT_MAPPING", r)
	if err != nil || result.Status != agentpreflight.StatusFailed || result.EvidenceCode != "IMPORT_CSV_WHITESPACE_UNQUOTED" {
		t.Fatal("JDBC通过覆盖了输入空白损失", result, err)
	}
}

func Test普通导入CUT换行错配不能被成功JDBC结果覆盖(t *testing.T) {
	r := normalImportFixture(t)
	line := "\n"
	if r.TargetPlatform != commandgen.PlatformWindowsAMD64 {
		line = "\r\n"
	}
	old := filepath.Join(r.NormalImport.FilePath, "synthetic_table.csv")
	if err := os.Remove(old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.NormalImport.FilePath, "synthetic_table.dat"), []byte("1|alpha|plain|"+line), 0600); err != nil {
		t.Fatal(err)
	}
	r.NormalImport.Format = "CUT"
	r.NormalImport.Parser = importdomain.Parser{ColumnSplitter: "|", TrailDelimiter: true}
	manifest, err := importfiles.Scan(context.Background(), *r.NormalImport, r.AllowedRoots)
	if err != nil {
		t.Fatal(err)
	}
	r.InputManifest = &manifest
	p := &NormalImportProbe{WorkspaceRoot: t.TempDir(), Resolver: &countingResolver{connection: Connection{Host: "synthetic.invalid", Port: 2883, Username: []byte("synthetic"), Password: []byte("synthetic-only")}}, Run: func(_ context.Context, _ credential.Workspace, _ jdbcprobe.Runtime, input jdbcprobe.NormalImportRequest) (jdbcprobe.NormalImportResult, error) {
		if input.MappingComplete {
			t.Fatal("执行平台默认换行不匹配仍声明输入完整")
		}
		return jdbcprobe.NormalImportResult{DatabaseAccess: jdbcprobe.ObjectAccessible, WriteAccess: jdbcprobe.ObjectAccessible, InputMapping: jdbcprobe.ObjectAccessible}, nil
	}}
	_, _ = p.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, r)
	result, err := p.Probe(context.Background(), "INPUT_MAPPING", r)
	if err != nil || result.Status != agentpreflight.StatusUnknown || result.EvidenceCode != "IMPORT_INPUT_MAPPING_UNSUPPORTED" {
		t.Fatal("成功JDBC覆盖了CUT换行未知", result, err)
	}
}

func Test普通导入三检查一次解析槽位并清除秘密和工作区(t *testing.T) {
	r := normalImportFixture(t)
	workspaceRoot := t.TempDir()
	username, password := []byte("synthetic-user"), []byte("synthetic-password")
	resolver := &countingResolver{connection: Connection{Host: "synthetic.invalid", Port: 2883, Username: username, Password: password}}
	calls := 0
	p := &NormalImportProbe{WorkspaceRoot: workspaceRoot, Resolver: resolver, Run: func(_ context.Context, w credential.Workspace, _ jdbcprobe.Runtime, input jdbcprobe.NormalImportRequest) (jdbcprobe.NormalImportResult, error) {
		calls++
		if !input.MappingComplete || len(input.Tables) != 1 || input.Tables[0].RequiredPrivileges != jdbcprobe.ImportPrivilegeInsert || input.Tables[0].Columns != 2 || input.Tables[0].Name != "synthetic_table" || string(input.Connection.Password) != "synthetic-password" {
			t.Fatal("普通导入未使用自身结构与短时业务槽位")
		}
		if _, err := os.Stat(w.RuntimeDirectory()); err != nil {
			t.Fatal(err)
		}
		return jdbcprobe.NormalImportResult{DatabaseAccess: jdbcprobe.ObjectAccessible, WriteAccess: jdbcprobe.ObjectAccessible, InputMapping: jdbcprobe.ObjectAccessible}, nil
	}}
	if result, _ := p.Probe(context.Background(), "WRITE_ACCESS", r); result.Status != agentpreflight.StatusUnknown || resolver.calls != 0 {
		t.Fatal("写检查先行时解析了槽位")
	}
	for _, check := range []agentpreflight.CheckID{agentpreflight.CheckDatabaseConnectivity, "WRITE_ACCESS", "INPUT_MAPPING"} {
		if result, err := p.Probe(context.Background(), check, r); err != nil || result.Status != agentpreflight.StatusPassed {
			t.Fatal("有效只读检查未通过", check, result, err)
		}
	}
	if resolver.calls != 1 || calls != 1 || !allZero(username) || !allZero(password) {
		t.Fatal("重复解析、重复探测或保留秘密")
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "executions", r.PrecheckID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("预检查工作区未清理")
	}
	r.Binding.CredentialRevision++
	if result, _ := p.Probe(context.Background(), "WRITE_ACCESS", r); result.Status != agentpreflight.StatusUnknown || resolver.calls != 1 {
		t.Fatal("新绑定复用了旧权限证据")
	}
}

func Test普通导入映射未知独立于连接权限且取消后不接纳成功(t *testing.T) {
	for _, kind := range []string{"changed-file", "unsupported-encoding", "unsupported-compression", "unsupported-control", "unsupported-footer", "unsupported-type", "cancelled", "connection-rejected", "unexpected-sys"} {
		t.Run(kind, func(t *testing.T) {
			r := normalImportFixture(t)
			if kind == "changed-file" {
				if err := os.WriteFile(filepath.Join(r.NormalImport.FilePath, "synthetic_table.csv"), []byte("2,'changed'\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "unsupported-encoding" {
				r.NormalImport.FileEncoding = "GBK"
			}
			if kind == "unsupported-compression" {
				r.NormalImport.Parser.Compress = true
				r.NormalImport.Parser.CompressionAlgo = "gzip"
				if err := os.Rename(filepath.Join(r.NormalImport.FilePath, "synthetic_table.csv"), filepath.Join(r.NormalImport.FilePath, "synthetic_table.csv.gz")); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "unsupported-control" {
				path := filepath.Join(r.NormalImport.FilePath, "synthetic_table.ctrl")
				if err := os.WriteFile(path, []byte(`lang=java (id "CONSTANT('synthetic')");`), 0600); err != nil {
					t.Fatal(err)
				}
				r.NormalImport.Mapping = &importdomain.Mapping{Kind: "CONTROL", CtlPath: path}
			}
			if kind == "unsupported-footer" {
				old := filepath.Join(r.NormalImport.FilePath, "synthetic_table.csv")
				if err := os.Rename(old, filepath.Join(r.NormalImport.FilePath, "synthetic_table.dat")); err != nil {
					t.Fatal(err)
				}
				r.NormalImport.Format = "CUT"
				r.NormalImport.Parser = importdomain.Parser{ColumnSplitter: "|", SkipFooter: true}
			}
			if kind == "unsupported-type" {
				r.NormalImport.Mapping = &importdomain.Mapping{Kind: "POSITION", ExcludeDataTypes: "UNSUPPORTED_SYNTHETIC_TYPE"}
			}
			if strings.HasPrefix(kind, "unsupported-") {
				manifest, err := importfiles.Scan(context.Background(), *r.NormalImport, r.AllowedRoots)
				if err != nil {
					t.Fatal(err)
				}
				r.InputManifest = &manifest
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			resolver := &countingResolver{connection: Connection{Host: "synthetic.invalid", Port: 2883, Username: []byte("user"), Password: []byte("synthetic-password")}}
			if kind == "unexpected-sys" {
				resolver.connection.SysPassword = []byte("synthetic-sys")
			}
			p := &NormalImportProbe{WorkspaceRoot: t.TempDir(), Resolver: resolver, Run: func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.NormalImportRequest) (jdbcprobe.NormalImportResult, error) {
				if kind == "cancelled" {
					cancel()
				}
				if kind == "connection-rejected" {
					return jdbcprobe.NormalImportResult{}, jdbcprobe.ErrConnectionFailed
				}
				return jdbcprobe.NormalImportResult{DatabaseAccess: jdbcprobe.ObjectAccessible, WriteAccess: jdbcprobe.ObjectAccessible, InputMapping: jdbcprobe.ObjectAccessible}, nil
			}}
			db, _ := p.Probe(ctx, agentpreflight.CheckDatabaseConnectivity, r)
			mapping, _ := p.Probe(ctx, "INPUT_MAPPING", r)
			if kind == "connection-rejected" {
				if db.Status != agentpreflight.StatusFailed || mapping.Status != agentpreflight.StatusUnknown {
					t.Fatal("连接失败被当作检查成功")
				}
			} else if kind == "cancelled" || kind == "unexpected-sys" {
				if db.Status != agentpreflight.StatusUnknown {
					t.Fatal("取消或越界秘密后仍接受成功")
				}
			} else {
				if db.Status != agentpreflight.StatusPassed || mapping.Status != agentpreflight.StatusUnknown {
					t.Fatal("映射不确定影响了独立事实或被放行")
				}
			}
			if !allZero(resolver.connection.Password) || !allZero(resolver.connection.SysPassword) {
				t.Fatal("失败路径遗留秘密")
			}
		})
	}
}

func Test普通导入权限需求只来自活动配置(t *testing.T) {
	data := importdomain.Config{Content: "DATA_ONLY", ReplaceData: true, Cleanup: "TRUNCATE"}
	if normalImportTablePrivileges(data, importfiles.MappingTable{HasData: true}) != jdbcprobe.ImportPrivilegeInsert|jdbcprobe.ImportPrivilegeUpdate|jdbcprobe.ImportPrivilegeDelete|jdbcprobe.ImportPrivilegeDrop {
		t.Fatal("数据写入权限需求不完整")
	}
	ddl := importdomain.Config{Content: "DDL_ONLY", ReplaceObject: true}
	if normalImportTablePrivileges(ddl, importfiles.MappingTable{HasDefinition: true, TargetColumns: []string{"id"}}) != jdbcprobe.ImportPrivilegeCreate|jdbcprobe.ImportPrivilegeDrop {
		t.Fatal("定义权限需求不完整")
	}
}

func Test清理权限不能以输入文件代替完整目标范围(t *testing.T) {
	plan := importfiles.MappingPlan{Tables: []importfiles.MappingTable{{Name: "synthetic_table", HasData: true}}}
	c := importdomain.Config{Cleanup: "DELETE", Scope: "ALL"}
	if normalImportCleanupScopeKnown(c, plan) {
		t.Fatal("全部清理范围被当成输入文件集合")
	}
	c.Scope = "SPECIFIC"
	c.Objects = map[string]string{"table": "synthetic_table,other_table"}
	if normalImportCleanupScopeKnown(c, plan) {
		t.Fatal("无输入文件的清理目标被遗漏")
	}
	c.Objects["table"] = "synthetic*"
	if normalImportCleanupScopeKnown(c, plan) {
		t.Fatal("通配清理范围未保持未知")
	}
	c.Objects["table"] = "synthetic_table"
	if !normalImportCleanupScopeKnown(c, plan) {
		t.Fatal("精确且覆盖完整的清理范围被误阻断")
	}
}

func Test普通导入同名删除建表按表核算且新原因码保持状态边界(t *testing.T) {
	r := normalImportFixture(t)
	r.NormalImport.Content = "DDL_AND_DATA"
	if err := os.WriteFile(filepath.Join(r.NormalImport.FilePath, "synthetic_table-schema.sql"), []byte("DROP TABLE IF EXISTS synthetic_table; CREATE TABLE synthetic_table(id INT NOT NULL,note VARCHAR(30));"), 0600); err != nil {
		t.Fatal(err)
	}
	manifest, err := importfiles.Scan(context.Background(), *r.NormalImport, r.AllowedRoots)
	if err != nil {
		t.Fatal(err)
	}
	r.InputManifest = &manifest
	p := &NormalImportProbe{WorkspaceRoot: t.TempDir(), Resolver: &countingResolver{connection: Connection{Host: "synthetic.invalid", Port: 2883, Username: []byte("user"), Password: []byte("synthetic-password")}}, Run: func(_ context.Context, _ credential.Workspace, _ jdbcprobe.Runtime, input jdbcprobe.NormalImportRequest) (jdbcprobe.NormalImportResult, error) {
		if !input.DDLComplete || len(input.Tables) != 1 || input.Tables[0].RequiredPrivileges != jdbcprobe.ImportPrivilegeInsert|jdbcprobe.ImportPrivilegeCreate|jdbcprobe.ImportPrivilegeDrop || len(input.Tables[0].TargetColumns) != 2 {
			t.Fatal("删除建表未形成已核算的逐表权限与列结构")
		}
		return jdbcprobe.NormalImportResult{DatabaseAccess: jdbcprobe.ObjectAccessible, WriteAccess: jdbcprobe.ObjectNotAccessible, WriteReason: "DROP_REQUIRED", InputMapping: jdbcprobe.ObjectUnavailable, MappingReason: "TARGET_METADATA_UNAVAILABLE"}, nil
	}}
	if result, _ := p.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, r); result.Status != agentpreflight.StatusPassed {
		t.Fatal("连接事实丢失")
	}
	write, _ := p.Probe(context.Background(), "WRITE_ACCESS", r)
	mapping, _ := p.Probe(context.Background(), "INPUT_MAPPING", r)
	if write.Status != agentpreflight.StatusFailed || write.EvidenceCode != "IMPORT_WRITE_DROP_REQUIRED" || mapping.Status != agentpreflight.StatusUnknown || mapping.EvidenceCode != "IMPORT_TARGET_METADATA_UNAVAILABLE" {
		t.Fatal("明确拒绝与元信息未知未正确投影")
	}
}
