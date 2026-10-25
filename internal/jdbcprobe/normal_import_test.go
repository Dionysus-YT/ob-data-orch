package jdbcprobe

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"
)

func Test普通导入V11仅携带固定连接逐表权限与结构字段(t *testing.T) {
	request := NormalImportRequest{Connection: Request{Host: "synthetic.invalid", Port: 2883, Username: []byte("synthetic-user"), Password: []byte("synthetic-secret")}, CompatibilityMode: CompatibilityModeMySQL, Database: "synthetic_db", MappingComplete: true, HasData: true, HasDDL: true, DDLComplete: true, Tables: []ImportTable{{Name: "synthetic_table", Columns: 2, RequiredPrivileges: ImportPrivilegeInsert | ImportPrivilegeCreate, HasData: true, HasDefinition: true, Names: []string{"id", "note"}, TargetColumns: []string{"id", "note"}, RequiredColumns: []string{"id"}}}}
	data, err := encodeNormalImportRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	defer zero(data)
	reader := bytes.NewReader(data)
	readInt := func() uint32 {
		t.Helper()
		var value uint32
		if binary.Read(reader, binary.BigEndian, &value) != nil {
			t.Fatal("结构帧读取失败")
		}
		return value
	}
	readText := func() string {
		t.Helper()
		size := readInt()
		value := make([]byte, size)
		if _, err := reader.Read(value); err != nil {
			t.Fatal("结构帧读取失败")
		}
		defer zero(value)
		return string(value)
	}
	if readInt() != 11 || readText() != "synthetic.invalid" || readInt() != 2883 {
		t.Fatal("固定连接头不符合预期")
	}
	readText()
	readText()
	if readText() != "MYSQL" || readText() != "synthetic_db" || readInt() != 51 {
		t.Fatal("普通导入上下文不符合预期")
	}
	if readInt() != 0 || readInt() != 0 || readInt() != 1 || readText() != "synthetic_table" || readInt() != 2 || readInt() != ImportPrivilegeInsert|ImportPrivilegeCreate || readInt() != 3 {
		t.Fatal("逐表权限与结构标志不符合预期")
	}
	if bytes.Contains(data, []byte("INSERT INTO")) || bytes.Contains(data, []byte("file-path")) {
		t.Fatal("数据SQL或路径进入JDBC结构帧")
	}
	if validImportNames([]string{"id", "id"}) || validImportNames([]string{"id\n"}) {
		t.Fatal("不安全列名单被接受")
	}
	if _, err := runFixedProbe(context.Background(), Runtime{}, nil, 100, time.Second, "java.lang.Runtime"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal("任意主类没有被阻断")
	}
	request.IncludeColumns = []string{strings.Repeat("x", 1024*1024)}
	if _, err := encodeNormalImportRequest(request); err == nil {
		t.Fatal("超限结构帧未拒绝")
	}
}

func Test普通导入只接受固定完整且一致的只读结果(t *testing.T) {
	good := []byte(`{"status":"SUCCESS","databaseAccess":"ACCESSIBLE","writeAccess":"ACCESSIBLE","inputMapping":"ACCESSIBLE","writeReason":"VERIFIED","mappingReason":"VERIFIED"}`)
	if _, err := parseNormalImportResponse(good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`{"status":"SUCCESS","databaseAccess":"ACCESSIBLE","writeAccess":"ACCESSIBLE"}`, `{"status":"SUCCESS","databaseAccess":"NOT_ACCESSIBLE","writeAccess":"ACCESSIBLE","inputMapping":"UNAVAILABLE"}`, `{"status":"SUCCESS","databaseAccess":"ACCESSIBLE","writeAccess":"ACCESSIBLE","inputMapping":"ACCESSIBLE","sql":"synthetic"}`, string(good) + ` {}`} {
		if _, err := parseNormalImportResponse([]byte(bad)); err == nil {
			t.Fatal("不完整、矛盾或未知响应被接受")
		}
	}
	for _, bad := range []string{strings.Replace(string(good), `"writeReason":"VERIFIED"`, `"writeReason":"DROP_REQUIRED"`, 1), strings.Replace(string(good), `"mappingReason":"VERIFIED"`, `"mappingReason":"synthetic-untrusted"`, 1)} {
		if _, err := parseNormalImportResponse([]byte(bad)); err == nil {
			t.Fatal("原因与状态不符或任意原因原文被接受")
		}
	}
}
