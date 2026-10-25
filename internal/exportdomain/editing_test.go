package exportdomain

import (
	"testing"

	"ob-data-orch/internal/store"
)

func Test编辑草稿允许缺项但拒绝未开放能力与任意输入(t *testing.T) {
	base := store.ExportConfig{ObjectScope: store.ObjectScope{ScopeKind: "SPECIFIED"}, ContentSelection: store.ContentSelection{ContentKind: ""}, OutputConfig: store.OutputConfig{OutputKind: "S3", StorageLocationDraft: &store.ExportStorageLocationDraft{Bucket: "synthetic-bucket"}}}
	if err := ValidateEditingConfig(&base); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*store.ExportConfig){
		func(c *store.ExportConfig) { c.ContentSelection.ContentKind = "AUTO" },
		func(c *store.ExportConfig) { c.DataFormat.FormatKind = "ORC" },
		func(c *store.ExportConfig) { c.PerformanceConfig.Retry = true },
		func(c *store.ExportConfig) { c.ObjectScope.Database = "synthetic\x00db" },
		func(c *store.ExportConfig) { c.OutputConfig.OutputKind = "LOCAL" },
		func(c *store.ExportConfig) {
			c.OutputConfig.FilePath = "s3://synthetic-bucket/path?access-key=synthetic"
		},
		func(c *store.ExportConfig) {
			c.OutputConfig.FilePath = "s3://synthetic-user:synthetic-secret@synthetic-bucket/path"
		},
		func(c *store.ExportConfig) {
			c.OutputConfig.StorageLocationDraft = &store.ExportStorageLocationDraft{Endpoint: "https://synthetic-user:synthetic-secret@storage.example.test"}
		},
	} {
		config := base
		mutate(&config)
		if ValidateEditingConfig(&config) == nil {
			t.Fatal("非法编辑配置未阻断")
		}
	}
}
