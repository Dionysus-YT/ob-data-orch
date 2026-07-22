package jdbcprobe

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ob-data-orch/internal/credential"
)

func Test连接探针请求使用网络字节序且不含额外字段(t *testing.T) {
	request := Request{Host: "127.0.0.1", Port: 2883, Username: []byte("synthetic-user"), Password: []byte("synthetic-password")}
	encoded := encodeRequest(request)
	defer zero(encoded)
	readInt := func(offset int) int { return int(binary.BigEndian.Uint32(encoded[offset : offset+4])) }
	if readInt(0) != 1 || readInt(4) != len(request.Host) {
		t.Fatalf("请求头不是网络字节序: %v", encoded[:8])
	}
	if !bytes.Contains(encoded, request.Password) || !bytes.Contains(encoded, request.Username) {
		t.Fatal("请求没有保留短时标准输入字段")
	}
}

func Test连接探针只释放到预检查私有运行目录(t *testing.T) {
	workspace, err := credential.CreateWorkspace(t.TempDir(), "precheck-1")
	if err != nil {
		t.Fatalf("CreateWorkspace() 错误 = %v", err)
	}
	t.Cleanup(func() { _ = workspace.Cleanup() })
	path, digest, err := Install(workspace)
	if err != nil {
		t.Fatalf("Install() 错误 = %v", err)
	}
	if filepath.Dir(path) != workspace.RuntimeDirectory() || len(digest) != 64 {
		t.Fatalf("探针位置或摘要错误: %q, %q", path, digest)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取探针错误 = %v", err)
	}
	defer zero(content)
	if got := sha256Digest(content); got != digest {
		t.Fatalf("探针摘要 = %q，期望 %q", got, digest)
	}
	if _, _, err := Install(workspace); !errors.Is(err, ErrProbeFailed) {
		t.Fatalf("重复释放错误 = %v", err)
	}
}

func sha256Digest(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func Test连接探针拒绝不安全运行时和请求(t *testing.T) {
	if validRequest(Request{Host: "host;unsafe", Port: 2883, Username: []byte("user"), Password: []byte("password")}) {
		t.Fatal("不安全主机被接受")
	}
	if validRequest(Request{Host: "odp.example", Port: 2883, Username: []byte("user"), Password: []byte("password\nunsafe")}) {
		t.Fatal("换行密码被接受")
	}
	if validRuntime(Runtime{Environment: []string{"PATH=C:\\Windows", "JAVA_TOOL_OPTIONS=-Dunsafe=true"}}) {
		t.Fatal("注入环境被接受")
	}
	if _, err := parseResponse([]byte(`{"status":"FAILED","code":"CONNECTION_FAILED"}`)); !errors.Is(err, ErrConnectionFailed) {
		t.Fatalf("失败响应错误 = %v", err)
	}
}

func Test连接探针失败只接受固定安全状态码(t *testing.T) {
	for _, test := range []struct {
		body string
		want error
	}{
		{body: `{"status":"FAILED","code":"DRIVER_UNAVAILABLE"}`, want: ErrDriverUnavailable},
		{body: `{"status":"FAILED","code":"INVALID_INPUT"}`, want: ErrInvalidRequest},
		{body: `{"status":"FAILED","code":"UNSAFE_DETAIL"}`, want: ErrProbeFailed},
		{body: `{"status":"FAILED","code":"CONNECTION_FAILED","driverName":"leak"}`, want: ErrProbeFailed},
	} {
		if _, err := parseResponse([]byte(test.body)); !errors.Is(err, test.want) {
			t.Fatalf("响应 %s 错误 = %v，期望 %v", test.body, err, test.want)
		}
	}
}

func Test连接探针只接受完整安全成功响应(t *testing.T) {
	result, err := parseResponse([]byte(`{"status":"SUCCESS","productName":"OceanBase","productVersion":"4.3","driverName":"OceanBase Connector/J","driverVersion":"2.4.14"}`))
	if err != nil || result.DriverVersion != "2.4.14" {
		t.Fatalf("parseResponse() = %#v, %v", result, err)
	}
	if _, err := parseResponse([]byte(`{"status":"SUCCESS","productName":"unsafe\nvalue","productVersion":"4.3","driverName":"driver","driverVersion":"2"}`)); !errors.Is(err, ErrProbeFailed) {
		t.Fatalf("换行元信息错误 = %v", err)
	}
}
