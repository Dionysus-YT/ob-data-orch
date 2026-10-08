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
		"obdumper-4.3.5-slice-v1.json":                           {},
		"obdumper-4.3.5-slice-v2.json":                           {},
		"obdumper-4.3.5-slice-v3.json":                           {},
		"obdumper-4.3.5-slice-v4.json":                           {},
		"obdumper-4.3.5-slice-v5.json":                           {},
		"obdumper-4.3.5-slice-v6.json":                           {},
		"obdumper-4.3.5-slice-v7.json":                           {},
		"obdumper-4.3.5-slice-v8-object-selection.json":          {},
		"obdumper-4.3.5-slice-v9-combined-object-selection.json": {},
		"obdumper-4.3.5-slice-v10-ddl-text-formats.json":         {},
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
	if _, err := loadFromFS(resourceFiles, generalizedRevisionResource); err != nil {
		t.Fatalf("embedded v7 revision is not loadable: %v", err)
	}
	if _, err := LoadObjectSelection(); err != nil {
		t.Fatalf("embedded object selection revision is not loadable: %v", err)
	}
	if _, err := loadFromFS(resourceFiles, legacyGeneralizedRevisionResource); err != nil {
		t.Fatalf("embedded v6 revision is not loadable for historical replay: %v", err)
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

// TestLoadGeneralizedCatalog 验证 v7 链式目录的身份、能力绑定与必填收缩。
func TestLoadGeneralizedCatalog(t *testing.T) {
	t.Parallel()
	catalog, err := LoadGeneralized()
	if err != nil {
		t.Fatalf("LoadGeneralized(): %v", err)
	}
	if catalog.MetadataVersion() != "obdumper-4.3.5-slice-v7" || catalog.BaseVersion() != "obdumper-4.3.5-slice-v1" || catalog.CapabilityVersion() != "" {
		t.Fatalf("unexpected generalized catalog identity: %s / %s / %s", catalog.MetadataVersion(), catalog.BaseVersion(), catalog.CapabilityVersion())
	}
	definitions := catalog.Definitions()
	// EX-I7 剩余参数第二批（2026-08-13）新增时间戳值格式 9 个、--partition/--exclude-data-types/
	// --enable-hidden-pk 与 --add-extra-message（59→72）。
	if len(definitions) != 72 {
		t.Fatalf("generalized definition count = %d, want 72", len(definitions))
	}
	for _, name := range []string{"--weak-read", "--retry"} {
		definition, ok := catalog.Definition(name)
		if !ok || definition.SupportState != "VALIDATION_GATED" {
			t.Fatalf("generalized definition %s = %#v, want VALIDATION_GATED", name, definition)
		}
	}
	// 第二批只有观察到可验收效果的四个参数进入 ENABLED。
	for _, name := range []string{"--partition", "--exclude-data-types", "--date-value-format", "--datetime-value-format"} {
		definition, ok := catalog.Definition(name)
		if !ok || definition.SupportState != "ENABLED" {
			t.Fatalf("generalized definition %s = %#v, want ENABLED", name, definition)
		}
	}
	for _, name := range []string{"--time-value-format", "--timestamp-value-format", "--timestamp-tz-value-format", "--timestamp-ltz-value-format", "--nls-date-format", "--nls-timestamp-format", "--nls-timestamp-tz-format", "--enable-hidden-pk", "--add-extra-message"} {
		definition, ok := catalog.Definition(name)
		if !ok || definition.SupportState != "VALIDATION_GATED" {
			t.Fatalf("generalized definition %s = %#v, want VALIDATION_GATED", name, definition)
		}
	}
	// --year-value-format 确认不存在（官网文档与 4.3.5 二进制均无该参数，2026-08-13 核实）。
	if _, ok := catalog.Definition("--year-value-format"); ok {
		t.Fatal("generalized catalog must not introduce --year-value-format")
	}
	if got := catalog.CategoryOrder(); len(got) != 9 || got[7] != "PERFORMANCE" || got[8] != "COMPRESSION" {
		t.Fatalf("unexpected generalized category order: %#v", got)
	}
	// 2026-08-13 4.3.5 --help 核实：-t 是 --tenant 的短选项，--table 无短选项（v7 撤销 v6 错误 override）。
	table, ok := catalog.Definition("--table")
	if !ok || table.ShortName != "" || table.RequiredWhen.Kind != "NEVER" {
		t.Fatalf("generalized table metadata: %#v", table)
	}
	for name, want := range map[string][]string{
		"--all":           {"export-odp-full-csv-v1", "export-odp-ddl-v1", "export-odp-ddl-csv-v1"},
		"--view":          {"export-odp-ddl-v1"},
		"--ddl":           {"export-odp-ddl-v1", "export-odp-ddl-csv-v1"},
		"--exclude-table": {"export-odp-full-csv-v1", "export-odp-ddl-v1", "export-odp-ddl-csv-v1"},
	} {
		definition, ok := catalog.Definition(name)
		if !ok || definition.SupportState != "ENABLED" || definition.EmissionTarget != "ARGV" || strings.Join(definition.CapabilityVersions, ",") != strings.Join(want, ",") {
			t.Fatalf("generalized %s metadata: %#v", name, definition)
		}
	}
	// v5 继承结果保持：密码秘密槽位与日志路径不受影响。
	password, ok := catalog.Definition("--password")
	if !ok || password.EmissionTarget != "SECURITY_FILE" || password.SecurityProperty != "oceanbase.jdbc.password" || password.RequiredWhen.Kind != "SLICE_SUBMISSION" {
		t.Fatalf("generalized password metadata: %#v", password)
	}
	if logPath, ok := catalog.Definition("--log-path"); !ok || logPath.SupportState != "ENABLED" {
		t.Fatalf("generalized log path metadata: %#v", logPath)
	}
	// 对象参数必填规则收缩为 NEVER，连接与输出必填保持。
	if csv, ok := catalog.Definition("--csv"); !ok || csv.RequiredWhen.Kind != "NEVER" {
		t.Fatalf("generalized csv required rule: %#v", csv)
	}
	if host, ok := catalog.Definition("--host"); !ok || host.RequiredWhen.Kind != "SLICE_SUBMISSION" {
		t.Fatalf("generalized host required rule: %#v", host)
	}
	// EX-I3：CSV 专属序列化参数从 VALIDATION_GATED 提升为 ENABLED，且仍绑定 CSV 格式活动。
	for _, name := range []string{"--skip-header", "--column-separator", "--column-quote", "--column-quote-mode"} {
		definition, ok := catalog.Definition(name)
		if !ok || definition.SupportState != "ENABLED" || definition.Activation.Kind != "FORMAT_IS" || definition.Activation.Value != "CSV" {
			t.Fatalf("EX-I3 %s metadata: %#v", name, definition)
		}
	}
	// EX-I4：跨格式共享的文本序列化参数改用 FORMAT_IN 多格式激活（EX-I5 起文件编码按官方格式表扩展到结构化格式）。
	for name, wantActivation := range map[string]string{
		"--escape-character": "CSV,CUT",
		"--null-string":      "CSV,CUT",
		"--line-separator":   "CSV,CUT,SQL",
		"--file-encoding":    "CSV,CUT,SQL,PARQUET,ORC,AVRO",
	} {
		definition, ok := catalog.Definition(name)
		if !ok || definition.SupportState != "ENABLED" || definition.Activation.Kind != "FORMAT_IN" || definition.Activation.Value != wantActivation {
			t.Fatalf("EX-I4 %s metadata: %#v", name, definition)
		}
	}
	// EX-I4：CUT/SQL/POS 数据格式参数的能力绑定与依赖规则；EX-I5：结构化格式参数。
	for name, wantCapabilities := range map[string][]string{
		"--cut":             {"export-odp-cut-v1"},
		"--sql":             {"export-odp-sql-v1"},
		"--pos":             {"export-odp-pos-v1"},
		"--par":             {"export-odp-parquet-v1"},
		"--orc":             {"export-odp-orc-v1"},
		"--avro":            {"export-odp-avro-v1"},
		"--trail-delimiter": {"export-odp-cut-v1"},
		"--remove-newline":  {"export-odp-cut-v1"},
		"--column-splitter": {"export-odp-cut-v1"},
		"--ctl-path":        {"export-odp-pos-v1"},
	} {
		definition, ok := catalog.Definition(name)
		if !ok || definition.SupportState != "ENABLED" || strings.Join(definition.CapabilityVersions, ",") != strings.Join(wantCapabilities, ",") {
			t.Fatalf("EX-I4 %s metadata: %#v", name, definition)
		}
	}
	if trail, ok := catalog.Definition("--trail-delimiter"); !ok || trail.Activation.Kind != "FORMAT_IS" || trail.Activation.Value != "CUT" || len(trail.DependsOn) != 1 || trail.DependsOn[0] != "--cut" {
		t.Fatalf("trail delimiter metadata: %#v", trail)
	}
	if remove, ok := catalog.Definition("--remove-newline"); !ok || remove.RiskLevel != "HIGH" || remove.Activation.Value != "CUT" || len(remove.DependsOn) != 1 || remove.DependsOn[0] != "--cut" {
		t.Fatalf("remove newline metadata: %#v", remove)
	}
	// EX-I4 POS 定版（2026-08-07 实测）：--pos 必须搭配 --ctl-path；--column-splitter 保持 CUT 专属。
	if pos, ok := catalog.Definition("--pos"); !ok || pos.SupportState != "ENABLED" || pos.SourcePolicy != "FORMAT" || len(pos.ConflictsWith) != 4 {
		t.Fatalf("pos metadata: %#v", pos)
	}
	if ctl, ok := catalog.Definition("--ctl-path"); !ok || ctl.SupportState != "ENABLED" || ctl.Activation.Kind != "FORMAT_IS" || ctl.Activation.Value != "POS" || len(ctl.DependsOn) != 1 || ctl.DependsOn[0] != "--pos" {
		t.Fatalf("ctl path metadata: %#v", ctl)
	}
	if splitter, ok := catalog.Definition("--column-splitter"); !ok || splitter.SupportState != "ENABLED" || splitter.Activation.Kind != "FORMAT_IS" || splitter.Activation.Value != "CUT" || len(splitter.DependsOn) != 1 || splitter.DependsOn[0] != "--cut" {
		t.Fatalf("column splitter metadata: %#v", splitter)
	}
	if algo, ok := catalog.Definition("--compression-algo"); !ok || strings.Join(algo.AllowedValues, ",") != "zstd,zlib,gzip,snappy" || len(algo.DependsOn) != 1 || algo.DependsOn[0] != "--compress" {
		t.Fatalf("compression algo metadata: %#v", algo)
	}
	if mode, ok := catalog.Definition("--column-quote-mode"); !ok || strings.Join(mode.AllowedValues, ",") != "all,all_not_null,minimal,non_numeric,none" {
		t.Fatalf("column quote mode metadata: %#v", mode)
	}
	if query, ok := catalog.Definition("--query-sql"); !ok || query.RiskLevel != "HIGH" || len(query.ConflictsWith) != 2 {
		t.Fatalf("query sql metadata: %#v", query)
	}
	// EX-I7 压缩等级（2026-08-10）：--compression-level 已接入（官方按算法分范围），依赖压缩与算法。
	if level, ok := catalog.Definition("--compression-level"); !ok || level.SupportState != "ENABLED" || len(level.DependsOn) != 2 || level.DependsOn[0] != "--compress" || level.DependsOn[1] != "--compression-algo" {
		t.Fatalf("compression level metadata: %#v", level)
	}
	// EX-I7 第二批只开放已观察到 MySQL 输出变化的 DATE/DATETIME 格式。
	for _, name := range []string{"--date-value-format", "--datetime-value-format"} {
		if definition, ok := catalog.Definition(name); !ok || definition.SupportState != "ENABLED" {
			t.Fatalf("timestamp value format %s metadata: %#v", name, definition)
		}
	}
	// 第二批筛选项已启用；依赖 sys 权限的 DDL 行为只登记定义并保持门禁。
	if partition, ok := catalog.Definition("--partition"); !ok || partition.SupportState != "ENABLED" || len(partition.ConflictsWith) != 1 || partition.ConflictsWith[0] != "--query-sql" {
		t.Fatalf("partition metadata: %#v", partition)
	}
	if extra, ok := catalog.Definition("--add-extra-message"); !ok || extra.SupportState != "VALIDATION_GATED" || len(extra.CapabilityVersions) != 2 {
		t.Fatalf("add extra message metadata: %#v", extra)
	}
}

// TestLoadObjectSelectionCatalog 验证五类对象目录独立版本及新增参数的 DDL 能力绑定。
func TestLoadObjectSelectionCatalog(t *testing.T) {
	t.Parallel()
	catalog, err := LoadObjectSelection()
	if err != nil {
		t.Fatal(err)
	}
	if catalog.MetadataVersion() != "obdumper-4.3.5-slice-v8-object-selection" || len(catalog.Definitions()) != 75 {
		t.Fatalf("五类对象目录身份或定义数不符：%s / %d", catalog.MetadataVersion(), len(catalog.Definitions()))
	}
	for _, name := range []string{"--function", "--procedure", "--sequence"} {
		definition, ok := catalog.Definition(name)
		if !ok || definition.SupportState != "ENABLED" || len(definition.CapabilityVersions) != 1 || definition.CapabilityVersions[0] != "export-odp-ddl-v1" {
			t.Fatalf("新增对象参数 %s 状态或能力错误：%#v", name, definition)
		}
	}
}

// TestLoadCombinedObjectSelectionCatalog 验证新修订只拓宽四类对象的结构与表数据组合能力。
func TestLoadCombinedObjectSelectionCatalog(t *testing.T) {
	t.Parallel()
	catalog, err := LoadCombinedObjectSelection()
	if err != nil {
		t.Fatal(err)
	}
	if catalog.MetadataVersion() != combinedObjectMetadataVersion || len(catalog.Definitions()) != 75 {
		t.Fatalf("组合目录身份或定义数错误：%s / %d", catalog.MetadataVersion(), len(catalog.Definitions()))
	}
	for _, name := range []string{"--view", "--function", "--procedure", "--sequence"} {
		definition, ok := catalog.Definition(name)
		if !ok || len(definition.CapabilityVersions) != 2 || definition.CapabilityVersions[1] != "export-odp-ddl-csv-v1" {
			t.Fatalf("组合对象参数 %s 能力错误：%#v", name, definition)
		}
	}
}

// TestLoadDDLTextFormatsCatalog 验证组合格式只获得对应的 DDL、对象及文本参数。
func TestLoadDDLTextFormatsCatalog(t *testing.T) {
	t.Parallel()
	catalog, err := LoadDDLTextFormats()
	if err != nil {
		t.Fatal(err)
	}
	if catalog.MetadataVersion() != ddlTextFormatsMetadataVersion || len(catalog.Definitions()) != 75 {
		t.Fatalf("组合文本目录身份或定义数错误：%s / %d", catalog.MetadataVersion(), len(catalog.Definitions()))
	}
	for _, name := range []string{"--ddl", "--cut", "--sql", "--view", "--function", "--sequence"} {
		definition, ok := catalog.Definition(name)
		if !ok || (!containsCapability(definition.CapabilityVersions, "export-odp-ddl-cut-v1") && !containsCapability(definition.CapabilityVersions, "export-odp-ddl-sql-v1")) {
			t.Fatalf("组合格式参数 %s 能力缺失：%#v", name, definition)
		}
	}
}

func containsCapability(versions []string, value string) bool {
	for _, version := range versions {
		if version == value {
			return true
		}
	}
	return false
}

// TestLoadLegacyGeneralizedCatalog 固化 v6 目录的字节级身份与旧 --table 短参数，
// 确保升级后只重放历史草稿，不把 v7 修订静默混入旧指纹。
func TestLoadLegacyGeneralizedCatalog(t *testing.T) {
	t.Parallel()
	catalog, err := LoadLegacyGeneralized()
	if err != nil {
		t.Fatalf("LoadLegacyGeneralized(): %v", err)
	}
	if catalog.MetadataVersion() != "obdumper-4.3.5-slice-v6" || len(catalog.Definitions()) != 59 {
		t.Fatalf("legacy generalized catalog identity: %s / %d", catalog.MetadataVersion(), len(catalog.Definitions()))
	}
	table, ok := catalog.Definition("--table")
	if !ok || table.ShortName != "-t" {
		t.Fatalf("legacy table definition changed: %#v", table)
	}
	if _, ok := catalog.Definition("--partition"); ok {
		t.Fatal("legacy v6 catalog must not contain v7-only parameters")
	}
}

const structuredCapabilities = ",export-odp-parquet-v1,export-odp-orc-v1,export-odp-avro-v1"

// EX-I3：新增参数的能力绑定、枚举与依赖规则（EX-I4 把压缩参数扩展到 CUT/SQL，官方复核后文件布局/筛选/性能参数同样绑定 CUT/SQL/POS 能力；EX-I5 结构化格式一并绑定；压缩保持可读格式专属）。
func TestGeneralizedStructuredFormatCapabilities(t *testing.T) {
	t.Parallel()
	catalog, err := LoadGeneralized()
	if err != nil {
		t.Fatalf("LoadGeneralized(): %v", err)
	}
	for name, wantBase := range map[string][]string{
		"--no-nested-dir":           {"export-odp-full-csv-v1", "export-odp-ddl-csv-v1", "export-odp-cut-v1", "export-odp-sql-v1", "export-odp-pos-v1"},
		"--max-file-size":           {"export-odp-full-csv-v1", "export-odp-ddl-csv-v1", "export-odp-cut-v1", "export-odp-sql-v1", "export-odp-pos-v1"},
		"--retain-empty-files":      {"export-odp-full-csv-v1", "export-odp-ddl-csv-v1", "export-odp-cut-v1", "export-odp-sql-v1", "export-odp-pos-v1"},
		"--query-sql":               {"export-odp-full-csv-v1", "export-odp-ddl-csv-v1", "export-odp-cut-v1", "export-odp-sql-v1", "export-odp-pos-v1"},
		"--include-column-names":    {"export-odp-full-csv-v1", "export-odp-ddl-csv-v1", "export-odp-cut-v1", "export-odp-sql-v1", "export-odp-pos-v1"},
		"--exclude-column-names":    {"export-odp-full-csv-v1", "export-odp-ddl-csv-v1", "export-odp-cut-v1", "export-odp-sql-v1", "export-odp-pos-v1"},
		"--exclude-virtual-columns": {"export-odp-full-csv-v1", "export-odp-ddl-csv-v1", "export-odp-cut-v1", "export-odp-sql-v1", "export-odp-pos-v1"},
		"--flashback-scn":           {"export-odp-full-csv-v1", "export-odp-ddl-csv-v1", "export-odp-cut-v1", "export-odp-sql-v1", "export-odp-pos-v1"},
		"--flashback-timestamp":     {"export-odp-full-csv-v1", "export-odp-ddl-csv-v1", "export-odp-cut-v1", "export-odp-sql-v1", "export-odp-pos-v1"},
		"--thread":                  {"export-odp-full-csv-v1", "export-odp-ddl-csv-v1", "export-odp-cut-v1", "export-odp-sql-v1", "export-odp-pos-v1"},
		"--page-size":               {"export-odp-full-csv-v1", "export-odp-ddl-csv-v1", "export-odp-cut-v1", "export-odp-sql-v1", "export-odp-pos-v1"},
		"--parallel-macro":          {"export-odp-full-csv-v1", "export-odp-ddl-csv-v1", "export-odp-cut-v1", "export-odp-sql-v1", "export-odp-pos-v1"},
		"--fetch-size":              {"export-odp-full-csv-v1", "export-odp-ddl-csv-v1", "export-odp-cut-v1", "export-odp-sql-v1", "export-odp-pos-v1"},
		"--mem":                     {"export-odp-full-csv-v1", "export-odp-ddl-csv-v1", "export-odp-cut-v1", "export-odp-sql-v1", "export-odp-pos-v1"},
	} {
		definition, ok := catalog.Definition(name)
		if !ok || definition.SupportState != "ENABLED" {
			t.Fatalf("EX-I5 %s metadata: %#v", name, definition)
		}
		want := append(append([]string(nil), wantBase...), "export-odp-parquet-v1", "export-odp-orc-v1", "export-odp-avro-v1")
		if strings.Join(definition.CapabilityVersions, ",") != strings.Join(want, ",") {
			t.Fatalf("EX-I5 %s capabilities = %#v, want %#v", name, definition.CapabilityVersions, want)
		}
	}
	// 压缩保持可读格式专属（不含结构化格式能力）。
	for _, name := range []string{"--compress", "--compression-algo"} {
		definition, ok := catalog.Definition(name)
		if !ok || strings.Contains(strings.Join(definition.CapabilityVersions, ","), structuredCapabilities) {
			t.Fatalf("EX-I5 %s must stay readable-format-only: %#v", name, definition)
		}
	}
	// 文件编码随官方格式表扩展到结构化格式。
	encoding, ok := catalog.Definition("--file-encoding")
	if !ok || encoding.Activation.Kind != "FORMAT_IN" || encoding.Activation.Value != "CSV,CUT,SQL,PARQUET,ORC,AVRO" {
		t.Fatalf("EX-I5 file encoding activation: %#v", encoding)
	}
	// EX-I6：--tmp-path 绑定全部已启用能力。
	tmpPath, ok := catalog.Definition("--tmp-path")
	if !ok || tmpPath.SupportState != "ENABLED" || len(tmpPath.CapabilityVersions) != 9 {
		t.Fatalf("EX-I6 tmp path metadata: %#v", tmpPath)
	}
	// with-trim 为 CSV/CUT 共享文本，不扩展到结构化格式。
	withTrim, ok := catalog.Definition("--with-trim")
	if !ok || strings.Join(withTrim.CapabilityVersions, ",") != "export-odp-full-csv-v1,export-odp-ddl-csv-v1,export-odp-cut-v1" {
		t.Fatalf("with-trim capabilities: %#v", withTrim)
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
