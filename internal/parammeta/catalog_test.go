package parammeta

import (
	"bytes"
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
	if catalog.ToolVersion() != "4.3.5-RELEASE" || catalog.MetadataVersion() != "obdumper-4.3.5-slice-v2" {
		t.Fatalf("unexpected catalog identity: %s / %s", catalog.ToolVersion(), catalog.MetadataVersion())
	}
	if catalog.BaseVersion() != "obdumper-4.3.5-slice-v1" || catalog.RevisionReason() == "" {
		t.Fatalf("missing compatibility revision trace: %s / %s", catalog.BaseVersion(), catalog.RevisionReason())
	}
	definitions := catalog.Definitions()
	if len(definitions) != 16 {
		t.Fatalf("definition count = %d, want 16", len(definitions))
	}
	if got := catalog.CategoryOrder(); len(got) != 6 || got[0] != "CONNECTION" || got[5] != "OUTPUT_FILE" {
		t.Fatalf("unexpected category order: %#v", got)
	}
	password, ok := catalog.Definition("--password")
	if !ok || password.ValueType != "secret-slot" || password.Sensitivity != "SECRET" || password.EmissionTarget != "SECURITY_FILE" || password.SecurityProperty != "oceanbase.jdbc.password" {
		t.Fatalf("unsafe password metadata: %#v", password)
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
