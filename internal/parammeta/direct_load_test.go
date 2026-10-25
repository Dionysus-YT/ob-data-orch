package parammeta

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

func Test旁路目录完整性处置与冻结隔离(t *testing.T) {
	catalog, err := LoadDirectLoad()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../docs/02-design/normal-import-parameter-mapping.md")
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile("(?m)^\\| `(--[^`]+)` \\|")
	names := map[string]bool{}
	for _, match := range pattern.FindAllStringSubmatch(string(raw), -1) {
		names[match[1]] = true
	}
	if len(names) != 103 || len(catalog.Definitions()) != 103 {
		t.Fatal("参数全集不完整", len(names))
	}
	enabled := 0
	for _, definition := range catalog.Definitions() {
		if !names[definition.LongName] || !strings.HasPrefix(definition.DefinitionID, "obloader.direct.") || len(definition.OfficialEvidence) < 4 {
			t.Fatal("旁路定义缺证据或混入其他模块")
		}
		delete(names, definition.LongName)
		if definition.SupportState == "ENABLED" {
			enabled++
		}
		if definition.LongName == "--retry" && definition.SupportState != "UNSUPPORTED" || definition.LongName == "--session-config" && definition.SupportState != "VALIDATION_GATED" {
			t.Fatal("旁路不适用或未装配参数被开放")
		}
	}
	if len(names) != 0 || enabled != 23 {
		t.Fatal("旁路能力范围变化缺少新版本")
	}
	schema, err := os.ReadFile("../../contracts/direct-load.json")
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Parameters []struct{ Name, Status, SupportState string } `json:"parameterCatalog"`
	}
	if json.Unmarshal(schema, &contract) != nil || len(contract.Parameters) != 103 {
		t.Fatal("契约目录缺失")
	}
	for _, p := range contract.Parameters {
		definition, ok := catalog.Definition(p.Name)
		if !ok || p.Status == "" || definition.SupportState != p.SupportState {
			t.Fatal("契约与运行目录不一致")
		}
	}
}
