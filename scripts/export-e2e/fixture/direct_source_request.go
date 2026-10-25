package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"unicode/utf8"

	"ob-data-orch/internal/credential"
)

// directSourceRequest 仅向匿名管道输出本轮 TEST 直连源请求；调用者不得转发 stdout 到日志或文件。
// 密码按字节转义成 JSON 字符串，避免创建不可清零的 Go 明文字符串。
func directSourceRequest(payload input) error {
	parts := strings.Split(payload.Username, "@")
	if len(parts) != 2 || strings.Contains(parts[1], "#") || !utf8.Valid(payload.Password) {
		return errors.New("直连请求身份无效")
	}
	mode, displayName, database := "MYSQL", "direct-api-"+payload.RunID[:12], payload.Database
	if payload.Action == "direct-oracle-source-request" {
		mode, displayName, database = "ORACLE", "direct-oracle-api-"+payload.RunID[:12], ""
		if parts[0] != strings.ToUpper(parts[0]) {
			return errors.New("Oracle 固定身份必须已登记为大写")
		}
	}
	fields := struct {
		DisplayName       string `json:"displayName"`
		Environment       string `json:"environment"`
		ConnectionKind    string `json:"connectionKind"`
		CompatibilityMode string `json:"compatibilityMode"`
		Host              string `json:"host"`
		Port              int    `json:"port"`
		ClusterName       string `json:"clusterName"`
		TenantName        string `json:"tenantName"`
		Username          string `json:"username"`
		DefaultDatabase   string `json:"defaultDatabase"`
	}{displayName, "TEST", "OBSERVER_DIRECT", mode, payload.Host, payload.Port, "", parts[1], parts[0], database}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	defer credential.Zero(encoded)
	output := bytes.NewBuffer(make([]byte, 0, len(encoded)+len(payload.Password)*6+24))
	defer func() { credential.Zero(output.Bytes()) }()
	output.Write(encoded[:len(encoded)-1])
	output.WriteString(`,"password":"`)
	const digits = "0123456789abcdef"
	for _, value := range payload.Password {
		switch {
		case value == '"' || value == '\\':
			output.WriteByte('\\')
			output.WriteByte(value)
		case value < 32:
			output.WriteString(`\u00`)
			output.WriteByte(digits[value>>4])
			output.WriteByte(digits[value&15])
		default:
			output.WriteByte(value)
		}
	}
	output.WriteString(`"}`)
	_, err = os.Stdout.Write(output.Bytes())
	return err
}
