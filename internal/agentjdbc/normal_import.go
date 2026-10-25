package agentjdbc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/importfiles"
	"ob-data-orch/internal/jdbcprobe"
	"ob-data-orch/internal/outputpath"
	"ob-data-orch/internal/precheckcontract"
)

// NormalImportRunner 仅替换固定只读普通导入探针，测试不能通过该端口传入任意命令。
type NormalImportRunner func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.NormalImportRequest) (jdbcprobe.NormalImportResult, error)

// NormalImportProbe 独立处理普通导入的数据库、写权限与列映射；每个租约只解析一次业务连接槽位。
// 后续两项只读本次冻结绑定的安全结果，不复用导出 SELECT 权限或缓存明文。
type NormalImportProbe struct {
	WorkspaceRoot string
	Runtime       jdbcprobe.Runtime
	Resolver      SecretResolver
	Run           NormalImportRunner
	mu            sync.Mutex
	bound         string
	completed     bool
	results       map[agentpreflight.CheckID]agentpreflight.Result
}

// Probe 只允许数据库检查启动受控子进程；写权限和映射不能另行请求秘密。
func (p *NormalImportProbe) Probe(ctx context.Context, check agentpreflight.CheckID, request agentpreflight.Request) (agentpreflight.Result, error) {
	unknown := normalImportUnknown(check)
	if p == nil || ctx == nil || ctx.Err() != nil || request.NormalImport == nil || request.CheckKind != "NORMAL_IMPORT_PREFLIGHT" || !agentpreflight.ValidNormalImportContext(request.NormalImport, request.CheckKind, request.CompatibilityMode, request.TargetPlatform, request.AllowedRoots) || request.NormalImport.NodeID != request.NodeID || request.Binding.NodeID != request.NodeID {
		return unknown, nil
	}
	raw, _ := json.Marshal(struct {
		Config   *importdomain.Config
		Binding  any
		Manifest *importfiles.Manifest
	}{request.NormalImport, request.Binding, request.InputManifest})
	hash := sha256.Sum256(raw)
	key := hex.EncodeToString(hash[:])
	p.mu.Lock()
	if p.bound != "" {
		result, ok := p.results[check]
		valid := p.bound == key && p.completed && ok
		p.mu.Unlock()
		if valid {
			return result, nil
		}
		return unknown, nil
	}
	if check != agentpreflight.CheckDatabaseConnectivity {
		p.mu.Unlock()
		return unknown, nil
	}
	p.bound = key
	p.mu.Unlock()
	results := p.inspect(ctx, request)
	p.mu.Lock()
	p.results = results
	p.completed = true
	result := p.results[check]
	p.mu.Unlock()
	return result, nil
}

func normalImportUnknown(check agentpreflight.CheckID) agentpreflight.Result {
	code := "IMPORT_UNAVAILABLE"
	if check == agentpreflight.CheckDatabaseConnectivity {
		code = EvidenceDatabaseUnavailable
	}
	return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: code}
}

func (p *NormalImportProbe) inspect(ctx context.Context, request agentpreflight.Request) (results map[agentpreflight.CheckID]agentpreflight.Result) {
	results = map[agentpreflight.CheckID]agentpreflight.Result{}
	for _, check := range []agentpreflight.CheckID{agentpreflight.CheckDatabaseConnectivity, "WRITE_ACCESS", "INPUT_MAPPING"} {
		results[check] = normalImportUnknown(check)
	}
	if p.Resolver == nil || request.InputManifest == nil || !request.InputManifest.Valid(*request.NormalImport) {
		return results
	}
	config := *request.NormalImport
	roots := make([]string, 0, len(request.AllowedRoots))
	for _, root := range request.AllowedRoots {
		native, ok := outputpath.LocalFilesystemPath(string(request.TargetPlatform), root)
		if !ok {
			return results
		}
		roots = append(roots, native)
	}
	plan, planErr := importfiles.InspectMappingForPlatform(ctx, config, *request.InputManifest, roots, string(request.TargetPlatform))
	includes, includeOK := normalImportColumnNames(config.Mapping, true)
	excludes, excludeOK := normalImportColumnNames(config.Mapping, false)
	if !includeOK || !excludeOK {
		plan.Complete = false
		includes = nil
		excludes = nil
	}
	var excludedTypes []string
	if config.Mapping != nil && config.Mapping.ExcludeDataTypes != "" {
		for _, name := range strings.Split(config.Mapping.ExcludeDataTypes, ",") {
			excludedTypes = append(excludedTypes, strings.ToUpper(strings.TrimSpace(name)))
		}
		if !jdbcprobe.ValidImportDataTypes(excludedTypes) {
			plan.Complete = false
			excludedTypes = nil
		}
	}
	if ctx.Err() != nil {
		return results
	}
	workspace, err := credential.CreateWorkspace(p.WorkspaceRoot, request.PrecheckID)
	if err != nil {
		return results
	}
	defer func() {
		if workspace.Cleanup() != nil || ctx.Err() != nil {
			for check := range results {
				results[check] = normalImportUnknown(check)
			}
		}
	}()
	connection, err := p.Resolver.ResolveDatabaseConnection(ctx, request.Binding)
	defer credential.Zero(connection.Username)
	defer credential.Zero(connection.Password)
	defer credential.Zero(connection.SysUsername)
	defer credential.Zero(connection.SysPassword)
	if err != nil || len(connection.SysUsername) != 0 || len(connection.SysPassword) != 0 {
		return results
	}
	path, digest, err := jdbcprobe.Install(workspace)
	if err != nil {
		return results
	}
	runtime := p.Runtime
	runtime.ProbePath = path
	runtime.ProbeSHA256 = digest
	input := jdbcprobe.NormalImportRequest{Connection: jdbcprobe.Request{Host: connection.Host, Port: connection.Port, Username: connection.Username, Password: connection.Password}, CompatibilityMode: jdbcprobe.CompatibilityMode(request.CompatibilityMode), Database: config.Database, MappingComplete: planErr == nil && plan.Complete, DDLComplete: planErr == nil && plan.DDLComplete, HasDDL: config.HasDDL(), IncludeColumns: includes, ExcludeColumns: excludes, HasData: config.HasData()}
	input.AutoMapping = config.Mapping != nil && config.Mapping.Kind == "AUTO"
	input.ReplaceObject = config.ReplaceObject
	input.ExcludeDataTypes = excludedTypes
	for _, object := range plan.Objects {
		input.Objects = append(input.Objects, jdbcprobe.ImportObject{Kind: object.Kind, Name: object.Name, Reference: object.Reference})
	}
	for _, table := range plan.Tables {
		input.Tables = append(input.Tables, jdbcprobe.ImportTable{Name: table.Name, Columns: table.Columns, Names: table.Names, TargetColumns: table.TargetColumns, RequiredColumns: table.RequiredColumns, RequiredPrivileges: normalImportTablePrivileges(config, table), HasData: table.HasData, HasDefinition: table.HasDefinition})
	}
	run := p.Run
	if run == nil {
		run = func(ctx context.Context, _ credential.Workspace, runtime jdbcprobe.Runtime, input jdbcprobe.NormalImportRequest) (jdbcprobe.NormalImportResult, error) {
			return jdbcprobe.TestNormalImport(ctx, runtime, input)
		}
	}
	observed, err := run(ctx, workspace, runtime, input)
	if err != nil {
		if errors.Is(err, jdbcprobe.ErrConnectionFailed) {
			results[agentpreflight.CheckDatabaseConnectivity] = agentpreflight.Result{Check: agentpreflight.CheckDatabaseConnectivity, Status: agentpreflight.StatusFailed, EvidenceCode: EvidenceDatabaseFailed}
		}
		return results
	}
	project := func(check agentpreflight.CheckID, status jdbcprobe.ObjectAccess, passed, rejected, unavailable string) {
		value := agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: unavailable}
		if status == jdbcprobe.ObjectAccessible {
			value.Status = agentpreflight.StatusPassed
			value.EvidenceCode = passed
		}
		if status == jdbcprobe.ObjectNotAccessible {
			value.Status = agentpreflight.StatusFailed
			value.EvidenceCode = rejected
		}
		results[check] = value
	}
	project(agentpreflight.CheckDatabaseConnectivity, observed.DatabaseAccess, EvidenceDatabaseConnected, EvidenceDatabaseFailed, EvidenceDatabaseUnavailable)
	if observed.DatabaseAccess != jdbcprobe.ObjectAccessible {
		return results
	}
	project("WRITE_ACCESS", observed.WriteAccess, "IMPORT_VERIFIED", "IMPORT_WRITE_PERMISSION_REJECTED", "IMPORT_WRITE_PERMISSION_UNAVAILABLE")
	project("INPUT_MAPPING", observed.InputMapping, "IMPORT_VERIFIED", "IMPORT_INPUT_MAPPING_MISMATCH", "IMPORT_INPUT_MAPPING_UNAVAILABLE")
	for check, reason := range map[agentpreflight.CheckID]string{"WRITE_ACCESS": observed.WriteReason, "INPUT_MAPPING": observed.MappingReason} {
		if reason == "" {
			continue
		}
		result := results[check]
		code := normalImportReasonCode(reason)
		if code != "" && precheckcontract.ValidResult(string(check), string(result.Status), code) {
			result.EvidenceCode = code
			results[check] = result
		} else {
			results[check] = normalImportUnknown(check)
		}
	}
	if planErr != nil {
		results["WRITE_ACCESS"] = agentpreflight.Result{Check: "WRITE_ACCESS", Status: agentpreflight.StatusUnknown, EvidenceCode: "IMPORT_WRITE_INPUT_UNAVAILABLE"}
	} else if !normalImportCleanupScopeKnown(config, plan) {
		// 清理可能作用于没有输入文件的目标表，不能以本地匹配清单证明整个清理范围的权限。
		results["WRITE_ACCESS"] = agentpreflight.Result{Check: "WRITE_ACCESS", Status: agentpreflight.StatusUnknown, EvidenceCode: "IMPORT_WRITE_CLEANUP_SCOPE_UNAVAILABLE"}
	}
	if config.HasData() && (!plan.Complete || planErr != nil) {
		code := "IMPORT_INPUT_MAPPING_UNSUPPORTED"
		if planErr != nil {
			code = "IMPORT_INPUT_FILES_CHANGED"
		}
		results["INPUT_MAPPING"] = agentpreflight.Result{Check: "INPUT_MAPPING", Status: agentpreflight.StatusUnknown, EvidenceCode: code}
	}
	if planErr == nil && plan.CSVWhitespaceLoss {
		results["INPUT_MAPPING"] = agentpreflight.Result{Check: "INPUT_MAPPING", Status: agentpreflight.StatusFailed, EvidenceCode: "IMPORT_CSV_WHITESPACE_UNQUOTED"}
	}
	return results
}

func normalImportCleanupScopeKnown(config importdomain.Config, plan importfiles.MappingPlan) bool {
	if config.Cleanup != "DELETE" && config.Cleanup != "TRUNCATE" {
		return true
	}
	if config.Scope != "SPECIFIC" {
		return false
	}
	known := map[string]bool{}
	for _, table := range plan.Tables {
		if table.HasData {
			known[table.Name] = true
		}
	}
	targets := strings.Split(config.Objects["table"], ",")
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" || strings.Contains(target, "*") || !known[target] {
			return false
		}
	}
	return true
}

func normalImportDataPrivileges(config importdomain.Config) uint32 {
	var flags uint32
	if config.HasData() {
		flags |= jdbcprobe.ImportPrivilegeInsert
	}
	if config.ReplaceData {
		flags |= jdbcprobe.ImportPrivilegeUpdate | jdbcprobe.ImportPrivilegeDelete
	}
	if config.Cleanup == "DELETE" {
		flags |= jdbcprobe.ImportPrivilegeDelete
	}
	if config.Cleanup == "TRUNCATE" {
		flags |= jdbcprobe.ImportPrivilegeDrop
	}
	return flags
}

func normalImportTablePrivileges(config importdomain.Config, table importfiles.MappingTable) uint32 {
	var flags uint32
	if table.HasData {
		flags = normalImportDataPrivileges(config)
	}
	if table.HasDefinition && len(table.TargetColumns) > 0 {
		flags |= jdbcprobe.ImportPrivilegeCreate
		if config.ReplaceObject || table.DropBeforeCreate {
			flags |= jdbcprobe.ImportPrivilegeDrop
		}
	}
	return flags
}

// normalImportReasonCode 只投影固定分类，不携带授权文本、对象数据或异常原文。
func normalImportReasonCode(reason string) string {
	return map[string]string{
		"VERIFIED":        "IMPORT_VERIFIED",
		"INSERT_REQUIRED": "IMPORT_WRITE_INSERT_REQUIRED", "UPDATE_REQUIRED": "IMPORT_WRITE_UPDATE_REQUIRED", "DELETE_REQUIRED": "IMPORT_WRITE_DELETE_REQUIRED", "CREATE_REQUIRED": "IMPORT_WRITE_CREATE_REQUIRED", "ALTER_REQUIRED": "IMPORT_WRITE_ALTER_REQUIRED", "DROP_REQUIRED": "IMPORT_WRITE_DROP_REQUIRED",
		"GRANTS_QUERY_FAILED": "IMPORT_WRITE_GRANTS_UNAVAILABLE", "GRANTS_UNSUPPORTED": "IMPORT_WRITE_GRANTS_UNSUPPORTED", "DDL_UNSUPPORTED": "IMPORT_WRITE_DDL_UNSUPPORTED", "INPUT_CONTEXT_UNAVAILABLE": "IMPORT_WRITE_INPUT_UNAVAILABLE", "SCHEMA_PRIVILEGES_UNAVAILABLE": "IMPORT_WRITE_SCHEMA_UNAVAILABLE",
		"INPUT_UNSUPPORTED": "IMPORT_INPUT_MAPPING_UNSUPPORTED", "TARGET_METADATA_UNAVAILABLE": "IMPORT_TARGET_METADATA_UNAVAILABLE", "TARGET_STRUCTURE_MISSING": "IMPORT_TARGET_STRUCTURE_MISSING", "INPUT_DEFINITION_UNSUPPORTED": "IMPORT_INPUT_DEFINITION_UNSUPPORTED", "AUTO_COLUMNS_UNSUPPORTED": "IMPORT_AUTO_COLUMNS_UNSUPPORTED",
		"COLUMN_COUNT_MISMATCH": "IMPORT_COLUMN_COUNT_MISMATCH", "COLUMN_NAME_MISMATCH": "IMPORT_COLUMN_NAME_MISMATCH", "REQUIRED_COLUMN_MISSING": "IMPORT_REQUIRED_COLUMN_MISSING",
	}[reason]
}

func normalImportColumnNames(mapping *importdomain.Mapping, include bool) ([]string, bool) {
	if mapping == nil || mapping.Kind != "EXPLICIT" {
		return nil, true
	}
	value := mapping.ExcludeColumns
	if include {
		value = mapping.IncludeColumns
	}
	if strings.TrimSpace(value) == "" {
		return nil, true
	}
	var result []string
	seen := map[string]bool{}
	for _, name := range strings.Split(value, ",") {
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsAny(name, "*:;()'\"`\\") || seen[name] || len(result) >= 4096 {
			return nil, false
		}
		seen[name] = true
		result = append(result, name)
	}
	return result, true
}
