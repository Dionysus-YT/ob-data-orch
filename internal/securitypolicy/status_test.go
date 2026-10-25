package securitypolicy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func startupFixture() Startup {
	return Startup{LoadedAt: time.Now().Add(-time.Second), CredentialKeyID: "synthetic-key-v1", RealExecutionEnabled: true, NormalImportExecutionEnabled: true, DirectLoadProbeEnabled: true, DirectLoadExecutionEnabled: true}
}

func Test安全状态仅投影启动事实且不返回秘密(t *testing.T) {
	input := startupFixture()
	service, err := New(input, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	input.CredentialKeyID = "modified-after-startup"
	item, err := service.Read(context.Background())
	if err != nil || item.CredentialKeyID != "synthetic-key-v1" || item.Source != "STARTUP_CONFIGURATION" || item.RootKeyStatus != "LOADED" || !item.BuiltInRedaction || item.AuditRetentionDays != nil || item.AuditAutomaticCleanup || item.SensitiveCommandView != "NOT_AVAILABLE" || item.LoadedAt.After(item.CapturedAt) {
		t.Fatalf("状态=%+v 错误=%v", item, err)
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"ciphertext", "protectedKey", "password", "privateKey", "rootKeyDigest", "filePath"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("安全投影包含禁止字段")
		}
	}
}

func Test安全状态缺依赖与矛盾门禁失败关闭(t *testing.T) {
	for _, mutate := range []func(*Startup){func(s *Startup) { s.LoadedAt = time.Time{} }, func(s *Startup) { s.CredentialKeyID = "../synthetic" }, func(s *Startup) { s.RealExecutionEnabled = false }, func(s *Startup) { s.DirectLoadProbeEnabled = false }} {
		input := startupFixture()
		mutate(&input)
		if _, err := New(input, func() bool { return true }); !errors.Is(err, ErrUnavailable) {
			t.Fatal("非法启动事实未阻断")
		}
	}
	if _, err := New(startupFixture(), nil); !errors.Is(err, ErrUnavailable) {
		t.Fatal("缺少密钥可用性端口未阻断")
	}
	var empty *Service
	if _, err := empty.Read(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatal("缺少服务未阻断")
	}
}

func Test密钥失效取消及未来载入时间不返回旧状态(t *testing.T) {
	ready := true
	service, _ := New(startupFixture(), func() bool { return ready })
	ready = false
	if _, err := service.Read(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatal("密钥失效仍返回已载入")
	}
	ready = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Read(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatal("取消请求仍返回状态")
	}
	input := startupFixture()
	input.LoadedAt = time.Now().Add(time.Hour)
	future, _ := New(input, func() bool { return true })
	if _, err := future.Read(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatal("未来载入时间未阻断")
	}
}
