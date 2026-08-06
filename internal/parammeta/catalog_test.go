package parammeta

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadDefaultCatalog(t *testing.T) {
	t.Parallel()
	catalog, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault(): %v", err)
	}
	if catalog.ToolVersion() != "4.3.5-RELEASE" || catalog.MetadataVersion() != "obdumper-4.3.5-slice-v5" {
		t.Fatalf("unexpected catalog identity: %s / %s", catalog.ToolVersion(), catalog.MetadataVersion())
	}
	if catalog.BaseVersion() != "obdumper-4.3.5-slice-v1" || catalog.RevisionReason() == "" {
		t.Fatalf("missing compatibility revision trace: %s / %s", catalog.BaseVersion(), catalog.RevisionReason())
	}
	if catalog.CapabilityVersion() != "export-odp-single-table-csv-v1" {
		t.Fatalf("unexpected capability version: %s", catalog.CapabilityVersion())
	}
	definitions := catalog.Definitions()
	if len(definitions) != 18 {
		t.Fatalf("definition count = %d, want 18", len(definitions))
	}
	if got := catalog.CategoryOrder(); len(got) != 6 || got[0] != "CONNECTION" || got[5] != "OUTPUT_FILE" {
		t.Fatalf("unexpected category order: %#v", got)
	}
	password, ok := catalog.Definition("--password")
	if !ok || password.ShortName != "-p" || password.ValueType != "secret-slot" || password.Sensitivity != "SECRET" || password.EmissionTarget != "SECURITY_FILE" || password.SecurityProperty != "oceanbase.jdbc.password" {
		t.Fatalf("unsafe password metadata: %#v", password)
	}
	host, ok := catalog.Definition("--host")
	if !ok || host.ShortName != "-h" {
		t.Fatalf("host short parameter metadata: %#v", host)
	}
	if logPath, ok := catalog.Definition("--log-path"); !ok || logPath.ValueType != "path" || logPath.SupportState != "ENABLED" {
		t.Fatalf("log path metadata: %#v", logPath)
	}
	if skipCheckDir, ok := catalog.Definition("--skip-check-dir"); !ok || skipCheckDir.ValueType != "flag" || skipCheckDir.SupportState != "ENABLED" {
		t.Fatalf("skip check directory metadata: %#v", skipCheckDir)
	}
}

func TestCatalogReturnsDefensiveCopies(t *testing.T) {
	t.Parallel()
	catalog, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault(): %v", err)
	}
	definitions := catalog.Definitions()
	definitions[0].LongName = "--changed"
	definitions[0].OfficialEvidence[0] = "changed"
	got, ok := catalog.Definition("--host")
	if !ok || got.LongName != "--host" || got.OfficialEvidence[0] == "changed" {
		t.Fatal("catalog was mutated through a returned definition")
	}
}

// TestEmbeddedResourceInventoryIsLoadable 固化嵌入资源清单：
// resources/ 下只允许存在已确认可加载的元数据，新增资源必须同步提供加载路径与测试，
// 避免再次出现嵌入文件无法被解析却没有测试发现的情况。
func TestEmbeddedResourceInventoryIsLoadable(t *testing.T) {
	t.Parallel()
	entries, err := fs.ReadDir(resourceFiles, "resources")
	if err != nil {
		t.Fatalf("read embedded resources: %v", err)
	}
	known := map[string]struct{}{
		"obdumper-4.3.5-slice-v1.json": {},
		"obdumper-4.3.5-slice-v2.json": {},
		"obdumper-4.3.5-slice-v3.json": {},
		"obdumper-4.3.5-slice-v4.json": {},
		"obdumper-4.3.5-slice-v5.json": {},
	}
	for _, entry := range entries {
		if _, ok := known[entry.Name()]; !ok {
			t.Fatalf("unexpected embedded parameter metadata resource %q without loader support and tests", entry.Name())
		}
		delete(known, entry.Name())
	}
	if len(known) != 0 {
		t.Fatalf("missing embedded parameter metadata resources: %v", known)
	}
	if _, err := loadFromFS(resourceFiles, defaultRevisionResource); err != nil {
		t.Fatalf("embedded v5 revision is not loadable: %v", err)
	}
	base, err := resourceFiles.ReadFile("resources/obdumper-4.3.5-slice-v1.json")
	if err != nil {
		t.Fatalf("read embedded base: %v", err)
	}
	raw, err := decodeResource(base)
	if err != nil || raw.MetadataVersion != "obdumper-4.3.5-slice-v1" {
		t.Fatalf("embedded v1 base is not decodable: %v", err)
	}
}

func TestLoadRejectsTamperedBaseResource(t *testing.T) {
	t.Parallel()
	manifest, err := resourceFiles.ReadFile(defaultRevisionResource)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	base, err := resourceFiles.ReadFile("resources/obdumper-4.3.5-slice-v1.json")
	if err != nil {
		t.Fatalf("read base: %v", err)
	}
	tampered := bytes.Replace(base, []byte(`"--port"`), []byte(`"--host"`), 1)
	files := fstest.MapFS{
		defaultRevisionResource:                  &fstest.MapFile{Data: manifest},
		"resources/obdumper-4.3.5-slice-v1.json": &fstest.MapFile{Data: tampered},
	}
	if _, err := loadFromFS(files, defaultRevisionResource); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("loadFromFS() error = %v, want checksum mismatch", err)
	}
}
