package importdomain

import (
	"encoding/json"
	"strings"
	"testing"

	"ob-data-orch/internal/commandgen"
)

func configured() Config {
	return Config{DataSourceID: "synthetic-source", NodeID: "synthetic-node", Database: "synthetic_db", FilePath: `E:\synthetic-import`, Content: "DATA_ONLY", Format: "CSV", Scope: "ALL", Cleanup: "NONE", TaskName: "合成任务", Mapping: &Mapping{Kind: "POSITION"}}
}

func Test配置严格解码与原样路径(t *testing.T) {
	c := configured()
	raw, _ := json.Marshal(c)
	decoded, err := Decode(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	n, issues := Normalize(decoded, "MYSQL", "WINDOWS_AMD64", true)
	if len(issues) != 0 || n.FilePath != c.FilePath {
		t.Fatalf("原样路径未保留：%v", issues)
	}
	for _, extra := range []string{`"direct":true`, `"querySql":"synthetic"`, `"password":"synthetic"`, `"sessionConfig":"synthetic"`} {
		contaminated := strings.TrimSuffix(string(raw), "}") + "," + extra + "}"
		if _, err := Decode(contaminated); err == nil {
			t.Fatalf("未知字段未拒绝：%s", extra)
		}
	}
	if _, err := Decode(string(raw) + " {}"); err == nil {
		t.Fatal("尾随对象未拒绝")
	}
	for _, path := range []string{"relative.csv", "s3://synthetic/input", "/E:/synthetic", `\\synthetic\share`} {
		c.FilePath = path
		if _, issues := Normalize(c, "MYSQL", "WINDOWS_AMD64", true); len(issues) == 0 {
			t.Fatal("越界路径被接受")
		}
	}
}

func Test仅定义隔离数据选项与模式差异(t *testing.T) {
	c := configured()
	c.Content = "DDL_ONLY"
	c.Parser = Parser{ColumnQuote: "xx", CompressionLevel: "999", Compress: true}
	c.Execution = Execution{Batch: "invalid", Thread: "2", Memory: "2G", NLSDateFormat: "YYYY-MM-DD"}
	c.ReplaceData = true
	c.Cleanup = "DELETE"
	n, issues := Normalize(c, "MYSQL", "WINDOWS_AMD64", true)
	if len(issues) != 0 || n.Format != "" || n.Mapping != nil || n.ReplaceData || n.Cleanup != "NONE" || n.Parser != (Parser{}) || n.Execution != (Execution{Thread: "2", Memory: "2G"}) {
		t.Fatalf("仅定义隔离失败：%v", issues)
	}
	c.Content = "DATA_ONLY"
	if _, issues := Normalize(c, "MYSQL", "WINDOWS_AMD64", true); len(issues) == 0 {
		t.Fatal("恢复活动后未重新校验")
	}
}

func Test映射压缩枚举与重复列负例(t *testing.T) {
	for _, change := range []func(*Config){
		func(c *Config) {
			c.Mapping = &Mapping{Kind: "EXPLICIT", IncludeColumns: "synthetic_id", ExcludeColumns: "synthetic_other"}
		},
		func(c *Config) { c.Format = "CUT"; c.Mapping = &Mapping{Kind: "AUTO"} },
		func(c *Config) { c.Parser = Parser{Compress: true, CompressionAlgo: "unknown"} },
		func(c *Config) { c.Parser = Parser{Compress: true, CompressionLevel: "23"} },
		func(c *Config) { c.Execution.MaxErrors = "-2" },
		func(c *Config) { c.Execution.Memory = "0G" },
		func(c *Config) { c.Execution.RW = "NaN" },
		func(c *Config) { c.Execution.RW = "Inf" },
		func(c *Config) { c.Content = "MIX" },
	} {
		c := configured()
		change(&c)
		if _, issues := Normalize(c, "MYSQL", "WINDOWS_AMD64", true); len(issues) == 0 {
			t.Fatal("非法组合未阻断")
		}
	}
	c := configured()
	c.Parser = Parser{Compress: true, CompressionAlgo: "gzip", CompressionLevel: "5"}
	n, issues := Normalize(c, "MYSQL", "WINDOWS_AMD64", true)
	if len(issues) != 0 || n.Parser.CompressionLevel != "" {
		t.Fatal("不适用等级未清除")
	}
}

func Test独立生成器与无秘密确定性(t *testing.T) {
	g, err := commandgen.NewNormalImport()
	if err != nil {
		t.Fatal(err)
	}
	c := configured()
	n, issues := Normalize(c, "MYSQL", "WINDOWS_AMD64", true)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	ref := commandgen.CredentialReference{CredentialID: "11111111-1111-4111-8111-111111111111", Revision: 1}
	fields := []commandgen.FieldInput{
		{Name: "--host", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: "synthetic.invalid"}},
		{Name: "--port", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: 2883}},
		{Name: "--user", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: "synthetic_user@synthetic_tenant#synthetic_cluster"}},
		{Name: "--database", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: c.Database}},
		{Name: "--password", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueSecretReference, Secret: &ref}},
		{Name: "--no-sys", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}},
	}
	request := commandgen.Request{Tool: "OBLOADER", ToolVersion: ToolVersion, MetadataVersion: MetadataVersion, CapabilityVersion: CapabilityVersion, ConnectionKind: commandgen.ConnectionODP, TargetPlatform: commandgen.PlatformWindowsAMD64, DataSourceFactVersion: "source-1", NodeFactVersion: "node-1", Fields: append(fields, Fields(n)...)}
	first, err := g.Generate(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := g.Generate(request)
	if err != nil || first.ConfigFingerprint != second.ConfigFingerprint {
		t.Fatal("生成不确定")
	}
	if !strings.HasPrefix(first.RedactedCommand, "obloader") || len(first.SecretSlots) != 1 {
		t.Fatal("工具或槽位边界错误")
	}
	for _, value := range first.ArgvTemplate {
		if strings.Contains(value, "password") || value == "******" {
			t.Fatal("密码或占位进入执行参数")
		}
	}
	for _, flag := range []string{"--direct", "--parallel", "--query-sql", "--skip-check-dir", "--retry"} {
		bad := request
		bad.Fields = append(append([]commandgen.FieldInput(nil), request.Fields...), commandgen.FieldInput{Name: flag, Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
		if _, err := g.Generate(bad); err == nil {
			t.Fatal("外来参数未拒绝")
		}
	}
	// 直接调用生成器同样失败关闭，不能只依赖向导或领域投影提前清理输入。
	for _, scenario := range []struct {
		name   string
		fields []commandgen.FieldInput
		remove string
	}{
		{name: "缺少固定无sys边界", remove: "--no-sys"},
		{name: "数据格式冲突", fields: []commandgen.FieldInput{{Name: "--cut", Source: commandgen.SourceFormat, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}}}},
		{name: "文件后缀注入选项", fields: []commandgen.FieldInput{{Name: "--file-suffix", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: "--direct"}}}},
		{name: "非有限读写比例", fields: []commandgen.FieldInput{{Name: "--rw", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: "NaN"}}}},
		{name: "数据量越界", fields: []commandgen.FieldInput{{Name: "--batch", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: 0}}}},
		{name: "未声明压缩输入", fields: []commandgen.FieldInput{{Name: "--compression-algo", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: "zstd"}}}},
		{name: "列名单相互冲突", fields: []commandgen.FieldInput{
			{Name: "--include-column-names", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: "synthetic_id"}},
			{Name: "--exclude-column-names", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: "synthetic_other"}},
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			bad := request
			bad.Fields = nil
			for _, field := range request.Fields {
				if field.Name != scenario.remove {
					bad.Fields = append(bad.Fields, field)
				}
			}
			bad.Fields = append(bad.Fields, scenario.fields...)
			if _, err := g.Generate(bad); err == nil {
				t.Fatal("绕过领域校验的非法生成请求未阻断")
			}
		})
	}
}
