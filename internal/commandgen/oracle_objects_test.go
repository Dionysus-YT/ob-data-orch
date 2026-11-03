package commandgen

import (
	"reflect"
	"testing"
)

func TestOracle对象仅新目录和包含结构的能力可发射(t *testing.T) {
	generator := mustGeneralizedGenerator(t)
	for _, capability := range []string{"export-odp-ddl-v1", "export-odp-ddl-csv-v1", "export-odp-ddl-cut-v1", "export-odp-ddl-sql-v1"} {
		t.Run(capability, func(t *testing.T) {
			request := validGeneralizedRequest(PlatformWindowsAMD64, capability)
			request.MetadataVersion = "obdumper-4.3.5-slice-v13-oracle-objects"
			for _, option := range []string{"--package", "--trigger", "--synonym", "--public-synonym", "--type"} {
				request.Fields = append(request.Fields, stringField(option, SourceUser, "synthetic_object"))
			}
			request.Fields = append(request.Fields, FieldInput{Name: "--ddl", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}})
			if capability != "export-odp-ddl-v1" {
				format := "--csv"
				if capability == "export-odp-ddl-cut-v1" {
					format = "--cut"
				}
				if capability == "export-odp-ddl-sql-v1" {
					format = "--sql"
				}
				request.Fields = append(request.Fields, FieldInput{Name: format, Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}})
			}
			result, err := generator.Generate(request)
			if err != nil {
				t.Fatal(err)
			}
			again, err := generator.Generate(request)
			if err != nil || !reflect.DeepEqual(result, again) {
				t.Fatal("命令或指纹不确定")
			}
			for _, option := range []string{"--package", "--trigger", "--synonym", "--public-synonym", "--type"} {
				if !containsToken(result.ArgvTemplate, option) {
					t.Fatalf("参数缺失: %s", option)
				}
			}
			request.MetadataVersion = "obdumper-4.3.5-slice-v12-schema-boolean"
			if _, err := generator.Generate(request); err == nil {
				t.Fatal("历史目录被扩展")
			}
			request.MetadataVersion = "obdumper-4.3.5-slice-v13-oracle-objects"
			removeField(&request, "--ddl")
			if _, err := generator.Generate(request); err == nil {
				t.Fatal("缺少 DDL 的对象参数被接受")
			}
			request.Fields = append(request.Fields, FieldInput{Name: "--ddl", Source: SourceFormat, Value: Value{Kind: ValueBoolean, Boolean: true}})
			fieldByName(t, &request, "--package").Value.String = "*"
			if _, err := generator.Generate(request); err == nil {
				t.Fatal("未授权通配范围被接受")
			}
		})
	}
}
