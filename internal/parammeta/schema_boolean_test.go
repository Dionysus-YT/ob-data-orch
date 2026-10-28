package parammeta

import "testing"

func Test显式布尔语法只允许新目录的固定结构参数(t *testing.T) {
	catalog, err := LoadSchemaBoolean()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.definitions) != 79 {
		t.Fatal("结构修订改变了参数集合")
	}
	definition, ok := catalog.Definition("--compact-schema")
	if !ok || !definition.ExplicitBoolean {
		t.Fatal("显式布尔发射未登记")
	}
	for _, mutate := range []func(*resource){
		func(raw *resource) { raw.MetadataVersion = expertParametersMetadataVersion },
		func(raw *resource) {
			for i := range raw.Definitions {
				if raw.Definitions[i].LongName == "--drop-object" {
					raw.Definitions[i].ExplicitBoolean = true
				}
			}
		},
	} {
		raw := resource{MetadataVersion: catalog.metadataVersion, Tool: catalog.tool, ToolVersion: catalog.toolVersion, CategoryOrder: catalog.CategoryOrder(), SourceDocuments: []string{"synthetic-evidence"}, Definitions: catalog.Definitions()}
		mutate(&raw)
		if validateResource(raw) == nil {
			t.Fatal("未知或历史布尔语法没有失败关闭")
		}
	}
	legacy, err := LoadExpertParameters()
	if err != nil {
		t.Fatal(err)
	}
	old, _ := legacy.Definition("--compact-schema")
	if old.ExplicitBoolean {
		t.Fatal("冻结目录被原地修改")
	}
}
