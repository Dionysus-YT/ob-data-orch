package importdomain

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/executionargv"
	"ob-data-orch/internal/parammeta"
)

// 参数矩阵只验证合成配置到执行边界，不代替真实工具语义和目标库结果。
func Test普通导入72参数配置生成与执行边界矩阵(t *testing.T) {
	raw, err := os.ReadFile("testdata/normal-parameter-matrix.json")
	if err != nil {
		t.Fatal(err)
	}
	var matrix struct {
		MetadataVersion string `json:"metadataVersion"`
		Parameters      []struct {
			Name  string          `json:"name"`
			Mode  string          `json:"mode"`
			Patch json.RawMessage `json:"patch"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(raw, &matrix); err != nil {
		t.Fatal(err)
	}
	catalog, err := parammeta.LoadNormalImport()
	if err != nil {
		t.Fatal(err)
	}
	if matrix.MetadataVersion != catalog.MetadataVersion() || len(matrix.Parameters) != len(catalog.Definitions()) || len(matrix.Parameters) != 72 {
		t.Fatal("参数矩阵与冻结目录不一致")
	}
	g, err := commandgen.NewNormalImport()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, item := range matrix.Parameters {
		definition, ok := catalog.Definition(item.Name)
		if !ok || seen[item.Name] {
			t.Fatal("未知或重复的矩阵参数")
		}
		seen[item.Name] = true
		t.Run(item.Name, func(t *testing.T) {
			c := configured()
			c.TaskName = ""
			if err := json.Unmarshal(item.Patch, &c); err != nil {
				t.Fatal(err)
			}
			n, issues := Normalize(c, item.Mode, "WINDOWS_AMD64", true)
			if len(issues) != 0 {
				t.Fatalf("合法配置被拒绝：%v", issues)
			}
			request := matrixRequest(n)
			first, err := g.Generate(request)
			if err != nil {
				t.Fatalf("合法生成被拒绝：%v", err)
			}
			if !executionargv.ValidNormalImport(first.ArgvTemplate) {
				t.Fatal("生成器与 Agent 白名单不一致")
			}
			second, err := g.Generate(request)
			if err != nil || first.ConfigFingerprint != second.ConfigFingerprint || first.RedactedCommand != second.RedactedCommand {
				t.Fatal("重复生成不确定")
			}
			index := -1
			for i, field := range request.Fields {
				if field.Name == item.Name {
					index = i
				}
			}
			if index < 0 {
				t.Fatal("配置字段未进入唯一生成器")
			}
			if item.Name == "--password" {
				if len(first.SecretSlots) != 1 || !strings.Contains(first.RedactedCommand, "******") {
					t.Fatal("安全槽位或展示脱敏缺失")
				}
				for _, token := range first.ArgvTemplate {
					if strings.Contains(token, "password") || token == "******" {
						t.Fatal("密码进入执行参数")
					}
				}
			} else {
				found := false
				for _, token := range first.TokenEvidence {
					if token.Parameter != item.Name {
						continue
					}
					found = true
					if token.ArgvStart < 0 || token.ArgvLength < 1 || token.ArgvStart+token.ArgvLength > len(first.ArgvTemplate) {
						t.Fatal("令牌证据越界")
					}
					if item.Name == "--strict" && (token.ArgvLength != 1 || first.ArgvTemplate[token.ArgvStart] != "--strict=false") {
						t.Fatal("严格布尔未使用官方紧凑写法")
					}
				}
				if !found {
					t.Fatal("参数未发射，不能计作覆盖")
				}
			}
			for _, negative := range []string{"类型错误", "来源越权", "重复参数"} {
				t.Run(negative, func(t *testing.T) {
					bad := request
					bad.Fields = append([]commandgen.FieldInput(nil), request.Fields...)
					switch negative {
					case "类型错误":
						value := commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}
						if definition.ValueType == "flag" {
							value = commandgen.Value{Kind: commandgen.ValueString, String: "true"}
						}
						bad.Fields[index].Value = value
					case "来源越权":
						bad.Fields[index].Source = commandgen.ValueSource("UNTRUSTED")
					case "重复参数":
						bad.Fields = append(bad.Fields, bad.Fields[index])
					}
					if _, err := g.Generate(bad); err == nil {
						t.Fatal("非法生成请求未拒绝")
					}
				})
			}
			if definition.ValueType == "string" || definition.ValueType == "path" {
				t.Run("选项注入", func(t *testing.T) {
					bad := request
					bad.Fields = append([]commandgen.FieldInput(nil), request.Fields...)
					bad.Fields[index].Value = commandgen.Value{Kind: commandgen.ValueString, String: "--direct"}
					if _, err := g.Generate(bad); err == nil {
						t.Fatal("文本值被重新解释为选项")
					}
				})
			}
		})
	}
	for _, definition := range catalog.Definitions() {
		if !seen[definition.LongName] {
			t.Fatalf("遗漏参数：%s", definition.LongName)
		}
	}
}

// matrixRequest 模拟控制面受管上下文，仅使用合成引用且按配置派生清理确认。
func matrixRequest(c Config) commandgen.Request {
	fields := []commandgen.FieldInput{
		{Name: "--host", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: "synthetic.invalid"}},
		{Name: "--port", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueInteger, Integer: 2883}},
		{Name: "--user", Source: commandgen.SourceDataSource, Value: commandgen.Value{Kind: commandgen.ValueString, String: "synthetic_user@synthetic_tenant#synthetic_cluster"}},
		{Name: "--database", Source: commandgen.SourceUser, Value: commandgen.Value{Kind: commandgen.ValueString, String: c.Database}},
		{Name: "--password", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueSecretReference, Secret: &commandgen.CredentialReference{CredentialID: "11111111-1111-4111-8111-111111111111", Revision: 1}}},
		{Name: "--no-sys", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}},
	}
	if c.Cleanup == "TRUNCATE" || c.Cleanup == "DELETE" {
		fields = append(fields, commandgen.FieldInput{Name: "--yes", Source: commandgen.SourceSecurity, Value: commandgen.Value{Kind: commandgen.ValueBoolean, Boolean: true}})
	}
	return commandgen.Request{Tool: "OBLOADER", ToolVersion: ToolVersion, MetadataVersion: MetadataVersion, CapabilityVersion: CapabilityVersion, ConnectionKind: commandgen.ConnectionODP, TargetPlatform: commandgen.PlatformWindowsAMD64, DataSourceFactVersion: "synthetic-source-1", NodeFactVersion: "synthetic-node-1", Fields: append(fields, Fields(c)...)}
}

func Test普通导入MySQL兼容版本必须显式选择(t *testing.T) {
	g, err := commandgen.NewNormalImport()
	if err != nil {
		t.Fatal(err)
	}
	fingerprints := map[string]bool{}
	for _, version := range []string{"mysql56", "mysql57", "mysql80"} {
		c := configured()
		c.Content = "DDL_ONLY"
		c.CompatMysql = true
		c.CompatMysqlVersion = version
		n, issues := Normalize(c, "MYSQL", "WINDOWS_AMD64", true)
		if len(issues) != 0 {
			t.Fatal(issues)
		}
		result, err := g.Generate(matrixRequest(n))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for i, name := range result.ArgvTemplate {
			if name == "--compat-mode" && i+1 < len(result.ArgvTemplate) && result.ArgvTemplate[i+1] == version {
				found = true
			}
		}
		if !found {
			t.Fatal("兼容版本未进入冻结执行参数")
		}
		fingerprints[result.ConfigFingerprint] = true
		bad := matrixRequest(n)
		for i := range bad.Fields {
			if bad.Fields[i].Name == "--compat-mode" {
				bad.Fields[i].Value.String = "mysql"
			}
		}
		if _, err := g.Generate(bad); err == nil {
			t.Fatal("生成器接受固定工具不支持的兼容版本")
		}
	}
	if len(fingerprints) != 3 {
		t.Fatal("版本变化未使指纹失效")
	}
	legacy := configured()
	legacy.Content = "DDL_ONLY"
	legacy.CompatMysql = true
	if _, issues := Normalize(legacy, "MYSQL", "WINDOWS_AMD64", false); len(issues) != 0 {
		t.Fatal("旧草稿无法保留待补版本状态")
	}
	if _, issues := Normalize(legacy, "MYSQL", "WINDOWS_AMD64", true); len(issues) == 0 {
		t.Fatal("旧开关未选择源版本仍可完整提交")
	}
	legacy.CompatMysqlVersion = "mysql"
	if _, issues := Normalize(legacy, "MYSQL", "WINDOWS_AMD64", false); len(issues) == 0 {
		t.Fatal("非法兼容版本未拒绝")
	}
	for _, mode := range []string{"MYSQL", "ORACLE"} {
		legacy.Content = "DATA_ONLY"
		n, issues := Normalize(legacy, mode, "WINDOWS_AMD64", true)
		if len(issues) != 0 || n.CompatMysql || n.CompatMysqlVersion != "" {
			t.Fatal("非活动兼容版本未清除")
		}
	}
}

func Test普通导入参数取值与条件矩阵(t *testing.T) {
	// JSON 反序列化到已有配置，保留与当前条件无关的合成必填上下文。
	check := func(t *testing.T, patch, mode string, valid bool) Config {
		t.Helper()
		c := configured()
		if err := json.Unmarshal([]byte(patch), &c); err != nil {
			t.Fatal(err)
		}
		n, issues := Normalize(c, mode, "WINDOWS_AMD64", true)
		if (len(issues) == 0) != valid {
			t.Fatalf("取值验收不符：%v", issues)
		}
		return n
	}
	for _, field := range []string{"maxDiscards", "maxErrors", "thread", "batch", "blockSize", "maxTps", "maxWaitTimeout"} {
		min := "1"
		invalid := []string{"0", "-1", "1.5", "1e2", "9007199254740992", "9223372036854775808"}
		if field == "maxDiscards" || field == "maxErrors" {
			min = "-1"
			invalid = []string{"-2", "1.5", "1e2", "9007199254740992", "9223372036854775808"}
		}
		for _, value := range []string{min, "9007199254740991"} {
			t.Run(field+"合法边界"+value, func(t *testing.T) { check(t, `{"execution":{"`+field+`":"`+value+`"}}`, "MYSQL", true) })
		}
		for _, value := range invalid {
			t.Run(field+"非法边界"+value, func(t *testing.T) { check(t, `{"execution":{"`+field+`":"`+value+`"}}`, "MYSQL", false) })
		}
	}
	for _, test := range []struct {
		name, patch string
		valid       bool
	}{
		{"读写最小正值", `{"execution":{"rw":"0.01"}}`, true},
		{"读写上界", `{"execution":{"rw":"1"}}`, true},
		{"读写零", `{"execution":{"rw":"0"}}`, false},
		{"读写超界", `{"execution":{"rw":"1.01"}}`, false},
		{"读写非有限", `{"execution":{"rw":"NaN"}}`, false},
		{"读写指数", `{"execution":{"rw":"1e-1"}}`, false},
		{"内存MB", `{"execution":{"memory":"700M"}}`, true},
		{"内存GB", `{"execution":{"memory":"1G"}}`, true},
		{"内存零", `{"execution":{"memory":"0G"}}`, false},
		{"内存小数", `{"execution":{"memory":"0.7G"}}`, false},
		{"内存无单位", `{"execution":{"memory":"700"}}`, false},
		{"严格真", `{"execution":{"strict":"true"}}`, true},
		{"严格假", `{"execution":{"strict":"false"}}`, true},
		{"严格大小写", `{"execution":{"strict":"TRUE"}}`, false},
		{"严格数字", `{"execution":{"strict":"1"}}`, false},
		{"ZSTD低界", `{"parser":{"compress":true,"compressionAlgo":"zstd","compressionLevel":"1"}}`, true},
		{"ZSTD高界", `{"parser":{"compress":true,"compressionAlgo":"zstd","compressionLevel":"22"}}`, true},
		{"ZSTD越界", `{"parser":{"compress":true,"compressionAlgo":"zstd","compressionLevel":"23"}}`, false},
		{"ZLIB低界", `{"parser":{"compress":true,"compressionAlgo":"zlib","compressionLevel":"-1"}}`, true},
		{"ZLIB高界", `{"parser":{"compress":true,"compressionAlgo":"zlib","compressionLevel":"9"}}`, true},
		{"ZLIB越界", `{"parser":{"compress":true,"compressionAlgo":"zlib","compressionLevel":"10"}}`, false},
		{"未知压缩", `{"parser":{"compress":true,"compressionAlgo":"unknown"}}`, false},
		{"单字符包围", `{"parser":{"columnQuote":"\""}}`, true},
		{"多字符包围", `{"parser":{"columnQuote":"xx"}}`, false},
		{"多字符转义", `{"parser":{"escapeCharacter":"xx"}}`, false},
		{"空值标记冲突", `{"parser":{"nullString":"NIL","emptyString":"NIL"}}`, false},
		{"默认空值标记冲突", `{"parser":{"emptyString":"\\N"}}`, false},
		{"默认日期无格式", `{"parser":{"defaultDate":"2000-01-01"}}`, false},
		{"名单互斥", `{"mapping":{"kind":"EXPLICIT","includeColumns":"id","excludeColumns":"label"}}`, false},
		{"显式名单为空", `{"mapping":{"kind":"EXPLICIT"}}`, false},
		{"控制文件为空", `{"mapping":{"kind":"CONTROL"}}`, false},
		{"控制文件相对路径", `{"mapping":{"kind":"CONTROL","ctlPath":"relative.ctl"}}`, false},
		{"CUT自动映射", `{"format":"CUT","mapping":{"kind":"AUTO"}}`, false},
		{"未知对象", `{"scope":"SPECIFIC","objects":{"direct":"nivt_table"}}`, false},
		{"未知内容", `{"content":"MIX"}`, false},
		{"未知格式", `{"format":"ORC"}`, false},
		{"未知范围", `{"scope":"UNKNOWN"}`, false},
		{"未知清理", `{"cleanup":"DROP"}`, false},
		{"相对输入", `{"filePath":"relative.csv"}`, false},
		{"相对日志", `{"logPath":"relative"}`, false},
		{"UNC输入", `{"filePath":"\\\\synthetic\\share"}`, false},
		{"控制字符", `{"fileRegex":"a\nb"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) { check(t, test.patch, "MYSQL", test.valid) })
	}
	for _, algo := range []string{"gzip", "snappy"} {
		t.Run(algo+"清除等级", func(t *testing.T) {
			n := check(t, `{"parser":{"compress":true,"compressionAlgo":"`+algo+`","compressionLevel":"999"}}`, "MYSQL", true)
			if n.Parser.CompressionLevel != "" {
				t.Fatal("不活动等级参与执行")
			}
		})
	}
	t.Run("仅定义清除数据参数", func(t *testing.T) {
		n := check(t, `{"content":"DDL_ONLY","parser":{"compress":true,"skipFooter":true},"mapping":{"kind":"CONTROL","ctlPath":"relative"},"execution":{"strict":"invalid","batch":"0","memory":"1G","thread":"1"},"replaceData":true,"cleanup":"TRUNCATE"}`, "MYSQL", true)
		if n.Parser != (Parser{}) || n.Mapping != nil || n.ReplaceData || n.Cleanup != "NONE" || n.Execution.Strict != "" || n.Execution.Batch != "" {
			t.Fatal("定义配置污染数据行为")
		}
	})
	t.Run("SQL清除文本解析", func(t *testing.T) {
		n := check(t, `{"format":"SQL","parser":{"columnSeparator":"|","columnQuote":"xx","columnSplitter":"|","escapeCharacter":"xx","skipHeader":true,"skipFooter":true,"ignoreEscape":true,"trailDelimiter":true,"withTrim":true,"emptyString":"NIL","dateFormat":"date","defaultDate":"invalid"}}`, "MYSQL", true)
		if n.Parser != (Parser{}) {
			t.Fatal("非活动解析进入SQL")
		}
	})
	t.Run("MySQL清除Oracle参数", func(t *testing.T) {
		n := check(t, `{"content":"DDL_ONLY","scope":"SPECIFIC","objects":{"table":"nivt_table","role":"nivt_role","sequence":"nivt_sequence","synonym":"nivt_synonym","type":"nivt_type","type-body":"nivt_type_body","package":"nivt_package","package-body":"nivt_package_body"},"execution":{"nlsDateFormat":"date","nlsTimestampFormat":"timestamp","nlsTimestampTzFormat":"timezone"}}`, "MYSQL", true)
		if len(n.Objects) != 1 || n.Execution != (Execution{}) {
			t.Fatal("Oracle上下文泄漏到MySQL")
		}
	})
	t.Run("Oracle清除MySQL参数", func(t *testing.T) {
		n := check(t, `{"compatMysql":true,"parser":{"dateFormat":"date","defaultDate":"invalid"}}`, "ORACLE", true)
		if n.CompatMysql || n.Parser.DateFormat != "" || n.Parser.DefaultDate != "" {
			t.Fatal("MySQL上下文泄漏到Oracle")
		}
	})
}
