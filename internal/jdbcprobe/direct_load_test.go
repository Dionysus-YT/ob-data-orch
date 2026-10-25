package jdbcprobe

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func Test旁路固定协议独立于普通导入且无自由查询输入(t *testing.T) {
	r := DirectLoadRequest{Connection: Request{Host: "synthetic.invalid", Port: 2881, Username: []byte("synthetic-user"), Password: []byte("synthetic-password")}, CompatibilityMode: CompatibilityModeMySQL, Database: "synthetic_db", Table: "synthetic_table", Scenario: "OBSERVER_DIRECT", TruncateTable: true}
	if !validDirectLoadRequest(r) {
		t.Fatal("合成输入无效")
	}
	frame := encodeDirectLoadRequest(r)
	if binary.BigEndian.Uint32(frame[:4]) != 11 || binary.BigEndian.Uint32(frame[len(frame)-4:]) != 1 || !bytes.Contains(frame, []byte("synthetic_table")) {
		t.Fatal("旁路固定协议版本或字段错误")
	}
	for _, name := range []string{"synthetic*", "db.table", "x`,y", "-option", "a;b", "a b"} {
		r.Table = name
		if validDirectLoadRequest(r) {
			t.Fatal("自由表达式进入固定单表输入")
		}
	}
	r.Table = "synthetic_table"
	r.Scenario = "CLOUD_ODP"
	if validDirectLoadRequest(r) {
		t.Fatal("非本地场景进入固定探针")
	}
	r.Scenario = "ODP"
	if validDirectLoadRequest(r) {
		t.Fatal("非旁路场景被接纳")
	}
}

func Test旁路输出拒绝未知重复别名和矛盾事实(t *testing.T) {
	good := `{"status":"SUCCESS","databaseAccess":"ACCESSIBLE","writeAccess":"ACCESSIBLE","observerVersion":"4.3.5.2","odpVersion":"","tableFacts":{"exists":true,"empty":true,"hasBIT":false,"hasBinary":false,"hasVirtualGenerated":false,"hasIndex":false,"hasLOB":false,"hasUniqueIndex":false}}`
	got, err := parseDirectLoadResponse([]byte(good))
	if err != nil || got.TableFacts.Empty == nil || !*got.TableFacts.Empty || got.ToolVersion != "" || got.DirectLoadConfig != nil {
		t.Fatal("固定安全输出未解码")
	}
	for _, raw := range []string{
		strings.Replace(good, `"status":"SUCCESS"`, `"status":"SUCCESS","status":"SUCCESS"`, 1),
		strings.Replace(good, `"exists":true`, `"exists":true,"exists":false`, 1),
		strings.Replace(good, `"exists":true`, `"Exists":true`, 1),
		strings.Replace(good, `"databaseAccess"`, `"DatabaseAccess"`, 1),
		strings.Replace(good, `"status":"SUCCESS"`, `"status":"SUCCESS","toolVersion":"4.3.5-RELEASE"`, 1),
		strings.Replace(good, `"4.3.5.2"`, `"5.7-OceanBase-v4.3.5.2"`, 1),
		strings.Replace(good, `"databaseAccess":"ACCESSIBLE"`, `"databaseAccess":"UNAVAILABLE"`, 1),
		good + `{}`,
	} {
		if _, err := parseDirectLoadResponse([]byte(raw)); err == nil {
			t.Fatal("不可信输出未失败关闭")
		}
	}
}
