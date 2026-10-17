package jdbcprobe

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"strings"
	"time"
)

const normalImportMainClass = "com.obdataorch.jdbcprobe.NormalImportProbe"

const (
	// ImportPrivilegeInsert 表示本次数据写入需要 INSERT。
	ImportPrivilegeInsert uint32 = 1
	// ImportPrivilegeUpdate 表示替换数据需要 UPDATE。
	ImportPrivilegeUpdate uint32 = 2
	// ImportPrivilegeDelete 表示替换或清理需要 DELETE。
	ImportPrivilegeDelete uint32 = 4
	// ImportPrivilegeCreate 表示对象定义需要 CREATE。
	ImportPrivilegeCreate uint32 = 8
	// ImportPrivilegeAlter 保留为已确认 ALTER 需求的固定权限位。
	ImportPrivilegeAlter uint32 = 16
	// ImportPrivilegeDrop 表示替换对象或 TRUNCATE 需要 DROP。
	ImportPrivilegeDrop uint32 = 32
)

// ImportTable 只包含当前输入文件的表名、列数和可选列名，不携带 SQL 或数据内容。
type ImportTable struct {
	Name    string
	Columns int
	Names   []string
	// RequiredPrivileges 按本表实际输入、清理与定义操作派生，不要求其他表的权限。
	RequiredPrivileges uint32
	HasData            bool
	HasDefinition      bool
	// TargetColumns 是文件结构探针已识别的目标定义，仅在新建或替换目标时使用。
	TargetColumns []string
	// RequiredColumns 是该定义中不能省略的目标列，必须属于 TargetColumns。
	RequiredColumns []string
}

// NormalImportRequest 是固定 v10 普通导入只读检查输入；权限位不接受任意权限表达式。
type NormalImportRequest struct {
	Connection        Request
	CompatibilityMode CompatibilityMode
	Database          string
	MappingComplete   bool
	DDLComplete       bool
	HasDDL            bool
	Tables            []ImportTable
	IncludeColumns    []string
	ExcludeColumns    []string
	HasData           bool
	AutoMapping       bool
	ReplaceObject     bool
}

// NormalImportResult 的三个固定结果只表达只读检查事实，不证明导入已经执行。
type NormalImportResult struct {
	DatabaseAccess ObjectAccess
	WriteAccess    ObjectAccess
	InputMapping   ObjectAccess
	// WriteReason 与 MappingReason 只接受与状态一致的固定原因，不透传 JDBC 错误或授权原文。
	WriteReason   string
	MappingReason string
}

// TestNormalImport 运行固定只读元信息探针，不执行文件 SQL、DML、DDL 或官方 OBLOADER。
func TestNormalImport(ctx context.Context, runtime Runtime, request NormalImportRequest) (NormalImportResult, error) {
	if ctx == nil || !validRuntime(runtime) {
		return NormalImportResult{}, ErrInvalidRuntime
	}
	if !validRequest(request.Connection) || !validCompatibilityMode(request.CompatibilityMode) || !validObjectIdentifier(request.Database) || len(request.Tables) > 1024 || !validImportNames(request.IncludeColumns) || !validImportNames(request.ExcludeColumns) || len(request.IncludeColumns) > 0 && len(request.ExcludeColumns) > 0 {
		return NormalImportResult{}, ErrInvalidRequest
	}
	seen := map[string]bool{}
	hasData, hasDefinition := false, false
	for _, table := range request.Tables {
		if !table.HasDefinition && (len(table.TargetColumns) > 0 || len(table.RequiredColumns) > 0) || request.DDLComplete && table.HasDefinition && len(table.TargetColumns) == 0 {
			return NormalImportResult{}, ErrInvalidRequest
		}
		if !validObjectIdentifier(table.Name) || strings.ContainsAny(table.Name, "*,") || seen[table.Name] || table.Columns < 0 || table.Columns > 4096 || table.RequiredPrivileges > 63 || table.RequiredPrivileges == 0 && (!request.HasDDL || request.DDLComplete) || !table.HasData && !table.HasDefinition || !validImportNames(table.Names) || !validImportNames(table.TargetColumns) || !validImportNames(table.RequiredColumns) {
			return NormalImportResult{}, ErrInvalidRequest
		}
		seen[table.Name] = true
		hasData = hasData || table.HasData
		hasDefinition = hasDefinition || table.HasDefinition
		target := map[string]bool{}
		for _, name := range table.TargetColumns {
			target[name] = true
		}
		for _, name := range table.RequiredColumns {
			if !target[name] {
				return NormalImportResult{}, ErrInvalidRequest
			}
		}
	}
	if !request.HasData && hasData || !request.HasDDL && hasDefinition || request.MappingComplete && request.HasData && !hasData || request.DDLComplete && request.HasDDL && !hasDefinition {
		return NormalImportResult{}, ErrInvalidRequest
	}
	input, err := encodeNormalImportRequest(request)
	if err != nil {
		return NormalImportResult{}, err
	}
	defer zero(input)
	output, err := runFixedProbe(ctx, runtime, input, maxOutputBytes, 90*time.Second, normalImportMainClass)
	if err != nil {
		return NormalImportResult{}, err
	}
	defer zero(output)
	return parseNormalImportResponse(output)
}

func encodeNormalImportRequest(request NormalImportRequest) ([]byte, error) {
	// 先编码非秘密结构，再一次性拼入连接帧，避免 append 扩容遗留未清零的密码副本。
	body := []byte{}
	overflow := false
	appendInt := func(value uint32) {
		var data [4]byte
		binary.BigEndian.PutUint32(data[:], value)
		body = append(body, data[:]...)
	}
	appendText := func(value string) {
		if len(body)+4+len(value) > 1024*1024 {
			overflow = true
			return
		}
		appendInt(uint32(len(value)))
		body = append(body, value...)
	}
	appendNames := func(names []string) {
		appendInt(uint32(len(names)))
		for _, name := range names {
			appendText(name)
		}
	}
	flags := uint32(0)
	if request.MappingComplete {
		flags |= 1
	}
	if request.HasData {
		flags |= 2
	}
	if request.AutoMapping {
		flags |= 4
	}
	if request.ReplaceObject {
		flags |= 8
	}
	if request.HasDDL {
		flags |= 16
	}
	if request.DDLComplete {
		flags |= 32
	}
	appendInt(flags)
	appendNames(request.IncludeColumns)
	appendNames(request.ExcludeColumns)
	appendInt(uint32(len(request.Tables)))
	for _, table := range request.Tables {
		appendText(table.Name)
		appendInt(uint32(table.Columns))
		appendInt(table.RequiredPrivileges)
		tableFlags := uint32(0)
		if table.HasData {
			tableFlags |= 1
		}
		if table.HasDefinition {
			tableFlags |= 2
		}
		appendInt(tableFlags)
		appendNames(table.Names)
		appendNames(table.TargetColumns)
		appendNames(table.RequiredColumns)
		if overflow {
			return nil, ErrInvalidRequest
		}
	}
	base := encodeProbeRequest(10, request.Connection, [][]byte{[]byte(request.CompatibilityMode), []byte(request.Database)})
	defer zero(base)
	if overflow || len(base)+len(body) > 1024*1024 {
		return nil, ErrInvalidRequest
	}
	input := make([]byte, len(base)+len(body))
	copy(input, base)
	copy(input[len(base):], body)
	return input, nil
}

func validImportNames(names []string) bool {
	if len(names) > 4096 {
		return false
	}
	seen := map[string]bool{}
	for _, name := range names {
		if !validObjectIdentifier(name) || strings.ContainsAny(name, "*,") || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}

func parseNormalImportResponse(output []byte) (NormalImportResult, error) {
	var value struct {
		Status         string       `json:"status"`
		DatabaseAccess ObjectAccess `json:"databaseAccess"`
		WriteAccess    ObjectAccess `json:"writeAccess"`
		InputMapping   ObjectAccess `json:"inputMapping"`
		WriteReason    string       `json:"writeReason"`
		MappingReason  string       `json:"mappingReason"`
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()
	valid := func(status ObjectAccess) bool {
		return status == ObjectAccessible || status == ObjectNotAccessible || status == ObjectUnavailable
	}
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF || value.Status != "SUCCESS" || !valid(value.DatabaseAccess) || !valid(value.WriteAccess) || !valid(value.InputMapping) {
		return NormalImportResult{}, ErrProbeFailed
	}
	if value.DatabaseAccess != ObjectAccessible && (value.WriteAccess != ObjectUnavailable || value.InputMapping != ObjectUnavailable) {
		return NormalImportResult{}, ErrProbeFailed
	}
	if !validImportReason(value.WriteAccess, value.WriteReason, true) || !validImportReason(value.InputMapping, value.MappingReason, false) {
		return NormalImportResult{}, ErrProbeFailed
	}
	return NormalImportResult{DatabaseAccess: value.DatabaseAccess, WriteAccess: value.WriteAccess, InputMapping: value.InputMapping, WriteReason: value.WriteReason, MappingReason: value.MappingReason}, nil
}

func validImportReason(status ObjectAccess, reason string, write bool) bool {
	if status == ObjectAccessible {
		return reason == "VERIFIED"
	}
	if status == ObjectNotAccessible {
		if write {
			switch reason {
			case "INSERT_REQUIRED", "UPDATE_REQUIRED", "DELETE_REQUIRED", "CREATE_REQUIRED", "ALTER_REQUIRED", "DROP_REQUIRED":
				return true
			default:
				return false
			}
		}
		return reason == "COLUMN_COUNT_MISMATCH" || reason == "COLUMN_NAME_MISMATCH" || reason == "REQUIRED_COLUMN_MISSING"
	}
	if status != ObjectUnavailable {
		return false
	}
	if reason == "DATABASE_UNAVAILABLE" {
		return true
	}
	if write {
		return reason == "GRANTS_QUERY_FAILED" || reason == "GRANTS_UNSUPPORTED" || reason == "DDL_UNSUPPORTED" || reason == "SCHEMA_PRIVILEGES_UNAVAILABLE" || reason == "INPUT_CONTEXT_UNAVAILABLE"
	}
	return reason == "INPUT_UNSUPPORTED" || reason == "TARGET_METADATA_UNAVAILABLE" || reason == "TARGET_STRUCTURE_MISSING" || reason == "INPUT_DEFINITION_UNSUPPORTED" || reason == "AUTO_COLUMNS_UNSUPPORTED"
}
