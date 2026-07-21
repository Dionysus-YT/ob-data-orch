package parammeta

import (
	"bytes"
	"strings"
	"testing"
)

func TestLoadDefaultCatalog(t *testing.T) {
	t.Parallel()
	catalog, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault(): %v", err)
	}
	if catalog.ToolVersion() != "4.3.5-RELEASE" || catalog.MetadataVersion() != "obdumper-4.3.5-slice-v1" {
		t.Fatalf("unexpected catalog identity: %s / %s", catalog.ToolVersion(), catalog.MetadataVersion())
	}
	definitions := catalog.Definitions()
	if len(definitions) != 16 {
		t.Fatalf("definition count = %d, want 16", len(definitions))
	}
	if got := catalog.CategoryOrder(); len(got) != 6 || got[0] != "CONNECTION" || got[5] != "OUTPUT_FILE" {
		t.Fatalf("unexpected category order: %#v", got)
	}
	password, ok := catalog.Definition("--password")
	if !ok || password.ValueType != "secret-slot" || password.Sensitivity != "SECRET" {
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

func TestLoadRejectsDuplicateParameter(t *testing.T) {
	t.Parallel()
	tampered := bytes.Replace(defaultResource, []byte(`"--port"`), []byte(`"--host"`), 1)
	if _, err := load(tampered); err == nil || !strings.Contains(err.Error(), "duplicate parameter") {
		t.Fatalf("load() error = %v, want duplicate parameter failure", err)
	}
}
