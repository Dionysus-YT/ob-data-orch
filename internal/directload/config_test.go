package directload

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"ob-data-orch/internal/commandgen"
)

func configured() Config {
	return Config{DataSourceID: "synthetic-source", NodeID: "synthetic-node", Database: "synthetic_db", Scenario: "PRIVATE_ODP", RPCPort: 2885, Table: "synthetic_table", FilePath: `E:\synthetic\synthetic_table.csv`, Format: "CSV", Applicability: Applicability{true, true, true, true}}
}

func Test严格解码拒绝秘密事实重复和尾随(t *testing.T) {
	raw, _ := json.Marshal(configured())
	if _, err := Decode(string(raw)); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{`"password":"synthetic"`, `"argv":[]`, `"observerVersion":"4.3.5"`, `"tableEmpty":true`, `"retry":true`, `"rpcSource":"QUERY"`, `"rpcPort":2882`, `"RPCPort":2882`, `"Table":"synthetic_other"`, `"applicability":null`} {
		if _, err := Decode(strings.TrimSuffix(string(raw), "}") + "," + extra + "}"); err == nil {
			t.Fatalf("污染配置未拒绝：%s", extra)
		}
	}
	for _, input := range []string{string(raw) + " {}", "null", "[]", `{"applicability":{"singleTable":true,"singleTable":false}}`, `{"applicability":{"SingleTable":true}}`, `{"table":null}`, `{"rpcPort":1.5}`} {
		if _, err := Decode(input); err == nil {
			t.Fatal("非法JSON被接受")
		}
	}
}

func Test本地路径原样保留及平台边界(t *testing.T) {
	for _, item := range []struct{ platform, path string }{{"WINDOWS_AMD64", `E:\synthetic path\表.csv`}, {"WINDOWS_AMD64", "E:/synthetic path/table.csv"}, {"LINUX_AMD64", "/synthetic/table.csv"}, {"LINUX_ARM64", "/synthetic/table.csv"}} {
		c := configured()
		c.FilePath = item.path
		n, issues := Normalize(c, "MYSQL", item.platform, true)
		if len(issues) != 0 || n.FilePath != item.path {
			t.Fatalf("路径被改写或拒绝：%v", issues)
		}
	}
	for _, path := range []string{"relative.csv", "s3://synthetic/input", `\\synthetic\share`, "/synthetic/table.csv", `E:\synthetic\*.csv`, "E:/synthetic\n/table.csv"} {
		c := configured()
		c.FilePath = path
		if _, issues := Normalize(c, "MYSQL", "WINDOWS_AMD64", true); len(issues) == 0 {
			t.Fatal("非法本地路径被接受")
		}
	}
}

func Test单表格式确认和数字边界(t *testing.T) {
	for _, change := range []func(*Config){
		func(c *Config) { c.Table = "*" }, func(c *Config) { c.Table = "synthetic_a,synthetic_b" }, func(c *Config) { c.Table = "synthetic_db.synthetic_table" }, func(c *Config) { c.Table = "--direct" }, func(c *Config) { c.Database = "synthetic;command" },
		func(c *Config) { c.Scenario = "LOGICAL_DATABASE" }, func(c *Config) { c.Format = "CUT" }, func(c *Config) { c.Format = "PARQUET" }, func(c *Config) { c.RPCPort = 0 }, func(c *Config) { c.RPCPort = 65536 },
		func(c *Config) { c.Thread = "0" }, func(c *Config) { c.Parallel = "2147483648" }, func(c *Config) { c.Thread = "1e2" }, func(c *Config) { c.Parallel = "+1" }, func(c *Config) { c.Applicability.NoCheckpoint = false }, func(c *Config) { c.ColumnSeparator = "--retry" },
	} {
		c := configured()
		change(&c)
		if _, issues := Normalize(c, "MYSQL", "WINDOWS_AMD64", true); len(issues) == 0 {
			t.Fatal("非法组合被接受")
		}
	}
	for _, format := range []string{"CSV", "SQL", "PARQUET", "ORC"} {
		c := configured()
		c.Scenario = "CLOUD_ODP"
		c.Format = format
		c.ExternalData = true
		c.ColumnSeparator = "|"
		_, issues := Normalize(c, "ORACLE", "WINDOWS_AMD64", true)
		if len(issues) == 0 {
			t.Fatal("非本地场景被接受")
		}
		c.Scenario = "PRIVATE_ODP"
		n, issues := Normalize(c, "ORACLE", "WINDOWS_AMD64", true)
		if (len(issues) == 0) != (format == "CSV") {
			t.Fatal("本地格式边界未生效", format, issues)
		}
		if format != "CSV" && (n.ExternalData || n.ColumnSeparator != "") {
			t.Fatal("非活动CSV字段未清除")
		}
	}
	if _, issues := Normalize(Config{}, "MYSQL", "", false); len(issues) != 0 {
		t.Fatal("未完成草稿被拒绝", issues)
	}
}

func generatorRequest(c Config) commandgen.Request {
	ref := commandgen.CredentialReference{CredentialID: "11111111-1111-4111-8111-111111111111", Revision: 1}
	text := func(name, value string, source commandgen.ValueSource) commandgen.FieldInput {
		return commandgen.FieldInput{Name: "--" + name, Source: source, Value: commandgen.Value{Kind: commandgen.ValueString, String: value}}
	}
	fields := []commandgen.FieldInput{text("host", "synthetic.invalid", commandgen.SourceDataSource), text("user", "synthetic_user", commandgen.SourceDataSource), text("database", c.Database, commandgen.SourceUser), {Name: "--port", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: 2883}}, {Name: "--password", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueSecretReference, Secret: &ref}}}
	kind := commandgen.ConnectionObserverDirect
	if c.Scenario == "OBSERVER_DIRECT" {
		fields = append(fields, text("tenant", "synthetic_tenant", commandgen.SourceDataSource))
	}
	if c.Scenario == "PRIVATE_ODP" || c.Scenario == "CLOUD_ODP" {
		fields = append(fields, text("tenant", "synthetic_tenant", commandgen.SourceDataSource))
		kind = commandgen.ConnectionODP
		if c.Scenario == "PRIVATE_ODP" {
			fields = append(fields, text("cluster", "synthetic_cluster", commandgen.SourceDataSource))
		} else {
			kind = commandgen.ConnectionPublicCloud
			fields = append(fields, commandgen.FieldInput{Name: "--public-cloud", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
		}
	}
	if c.TruncateTable {
		fields = append(fields, commandgen.FieldInput{Name: "--yes", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
	}
	return commandgen.Request{Tool: "OBLOADER", ToolVersion: ToolVersion, MetadataVersion: MetadataVersion, CapabilityVersion: CapabilityVersion, ConnectionKind: kind, DataSourceFactVersion: "synthetic-source-1", NodeFactVersion: "synthetic-node-1", TargetPlatform: commandgen.PlatformWindowsAMD64, Fields: append(fields, Fields(c)...)}
}

func Test生成器本地场景格式确定性与秘密隔离(t *testing.T) {
	g, err := commandgen.NewDirectLoad()
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"PRIVATE_ODP", "CLOUD_ODP", "OBSERVER_DIRECT"} {
		for _, format := range []string{"CSV", "SQL", "PARQUET", "ORC"} {
			c := configured()
			c.Scenario = scenario
			c.Format = format
			request := generatorRequest(c)
			first, err := g.Generate(request)
			if format != "CSV" || scenario == "CLOUD_ODP" {
				if err == nil {
					t.Fatal("无证据格式组合被生成")
				}
				continue
			}
			if err != nil {
				t.Fatalf("已确认组合无法生成：%s/%s %v", scenario, format, err)
			}
			second, err := g.Generate(request)
			if err != nil || !reflect.DeepEqual(first, second) {
				t.Fatal("生成不确定")
			}
			if !strings.Contains(first.RedactedCommand, "--direct") || !strings.Contains(first.RedactedCommand, "******") || len(first.SecretSlots) != 1 {
				t.Fatal("秘密槽位或旁路固定参数无效")
			}
			for _, token := range first.ArgvTemplate {
				if token == "--thread" || token == "--parallel" || token == "--retry" || token == "--no-sys" || token == "-p" || token == "--password" {
					t.Fatal("未设置或不适用参数被生成")
				}
			}
		}
	}
}

func Test生成器直接调用不能绕过旁路边界(t *testing.T) {
	g, err := commandgen.NewDirectLoad()
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*commandgen.Request){
		func(r *commandgen.Request) {
			r.ConnectionKind = commandgen.ConnectionPublicCloud
			for i := range r.Fields {
				if r.Fields[i].Name == "--csv" {
					r.Fields[i].Name = "--sql"
				}
				if r.Fields[i].Name == "--cluster" {
					r.Fields[i] = commandgen.FieldInput{Name: "--public-cloud", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}}
				}
			}
			r.Fields = append(r.Fields, commandgen.FieldInput{Name: "--column-separator", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: "|"}})
		},
		func(r *commandgen.Request) {
			r.Fields = append(r.Fields, commandgen.FieldInput{Name: "--retry", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
		},
		func(r *commandgen.Request) {
			r.Fields = append(r.Fields, commandgen.FieldInput{Name: "--session-config", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueString, String: `E:\synthetic\session.config.json`}})
		},
		func(r *commandgen.Request) {
			r.Fields = append(r.Fields, commandgen.FieldInput{Name: "--max-errors", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: 1}})
		},
		func(r *commandgen.Request) {
			r.Fields = append(r.Fields, commandgen.FieldInput{Name: "--sql", Source: commandgen.SourceFormat, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
		},
		func(r *commandgen.Request) {
			r.Fields = append(r.Fields, commandgen.FieldInput{Name: "--table", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: "synthetic_table"}})
		},
		func(r *commandgen.Request) { r.CapabilityVersion = "import-odp-local-v1" }, func(r *commandgen.Request) { r.ConnectionKind = commandgen.ConnectionLogicalDB }, func(r *commandgen.Request) { r.OutputKind = "OSS" },
		func(r *commandgen.Request) {
			for i := range r.Fields {
				if r.Fields[i].Name == "--direct" {
					r.Fields[i].Value.Boolean = false
				}
			}
		},
		func(r *commandgen.Request) {
			for i := range r.Fields {
				if r.Fields[i].Name == "--rpc-port" {
					r.Fields[i].Value.Integer = 0
				}
			}
		},
		func(r *commandgen.Request) {
			for i := range r.Fields {
				if r.Fields[i].Name == "--table" {
					r.Fields[i].Value.String = "synthetic_a,synthetic_b"
				}
			}
		},
		func(r *commandgen.Request) {
			for i := range r.Fields {
				if r.Fields[i].Name == "--tenant" {
					r.Fields[i].Value.String = "--direct"
				}
			}
		},
		func(r *commandgen.Request) {
			for i := range r.Fields {
				if r.Fields[i].Name == "--password" {
					r.Fields[i].Value = commandgen.Value{Kind: commandgen.ValueString, String: "synthetic-secret"}
				}
			}
		},
	} {
		request := generatorRequest(configured())
		change(&request)
		if _, err := g.Generate(request); err == nil {
			t.Fatal("旁路生成器未失败关闭")
		}
	}
	c := configured()
	c.TruncateTable = true
	if _, err := g.Generate(generatorRequest(c)); err != nil {
		t.Fatal("清空平台确认派生失败", err)
	}
	request := generatorRequest(c)
	for i, f := range request.Fields {
		if f.Name == "--yes" {
			request.Fields = append(request.Fields[:i], request.Fields[i+1:]...)
			break
		}
	}
	if _, err := g.Generate(request); err == nil {
		t.Fatal("缺少非交互确认仍生成清空")
	}
	normal, err := commandgen.NewNormalImport()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := normal.Generate(generatorRequest(configured())); err == nil {
		t.Fatal("普通导入生成器接受旁路")
	}
}
