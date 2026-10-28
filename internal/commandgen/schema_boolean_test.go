package commandgen

import "testing"

func Test结构布尔值保留显式关闭且历史语法不变(t *testing.T) {
	g := mustGeneralizedGenerator(t)
	for _, version := range []string{"obdumper-4.3.5-slice-v7", "obdumper-4.3.5-slice-v11-expert-parameters", "obdumper-4.3.5-slice-v12-schema-boolean"} {
		for _, value := range []bool{false, true} {
			req := validGeneralizedRequest(PlatformWindowsAMD64, "export-odp-ddl-v1")
			req.MetadataVersion = version
			req.Fields = append(req.Fields, stringField("--table", SourceUser, "synthetic_table"), FieldInput{Name: "--ddl", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}})
			without, err := g.Generate(req)
			if err != nil {
				t.Fatal(err)
			}
			req.Fields = append(req.Fields, boolField("--compact-schema", value))
			result, err := g.Generate(req)
			if err != nil {
				t.Fatal(err)
			}
			if version == "obdumper-4.3.5-slice-v12-schema-boolean" {
				want := "--compact-schema=false"
				if value {
					want = "--compact-schema=true"
				}
				if !containsToken(result.ArgvTemplate, want) || containsToken(result.ArgvTemplate, "--compact-schema") || result.ConfigFingerprint == without.ConfigFingerprint {
					t.Fatal("显式布尔令牌或指纹丢失")
				}
				for _, token := range result.TokenEvidence {
					if token.Parameter == "--compact-schema" && (token.ArgvLength != 1 || result.ArgvTemplate[token.ArgvStart] != want) {
						t.Fatal("布尔令牌证据不一致")
					}
				}
			} else if containsToken(result.ArgvTemplate, "--compact-schema") != value || (!value && result.ConfigFingerprint != without.ConfigFingerprint) {
				t.Fatal("历史 flag 行为发生变化")
			}
			replay, err := g.Generate(req)
			if err != nil || replay.ConfigFingerprint != result.ConfigFingerprint {
				t.Fatal("相同结构输入不能确定性重放")
			}
		}
	}
}
