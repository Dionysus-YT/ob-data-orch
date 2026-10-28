package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/store"
)

func initialAuditFixture(t *testing.T) (string, Settings, *store.Store) {
	t.Helper()
	directory := t.TempDir()
	settings, err := OpenSettings(directory, strings.NewReader("https://orch.example.test:8080\nsynthetic-password-1234\n"), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	settings.ListenAddress = "127.0.0.1:0"
	content, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "settings.json"), content, 0600); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(context.Background(), filepath.Join(directory, "local-mvp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	now := time.Now().UTC()
	if err = database.EnsureAuthSubject(context.Background(), store.AuthSubject{SubjectID: SubjectID, ExternalSubject: SubjectID, DisplayName: "合成管理员", AccountStatus: "ACTIVE", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err = database.BootstrapPermissionOwner(context.Background(), SubjectID, now); err != nil {
		t.Fatal(err)
	}
	return directory, settings, database
}

func Test部署首次审计必须原口令和完整确认且不改安装身份(t *testing.T) {
	for _, name := range []string{"错误密码", "取消", "密码中断", "确认中断", "超长密码", "确认主体不符", "成功"} {
		t.Run(name, func(t *testing.T) {
			directory, _, database := initialAuditFixture(t)
			settingsPath := filepath.Join(directory, "settings.json")
			before, err := os.ReadFile(settingsPath)
			if err != nil {
				t.Fatal(err)
			}
			input := "synthetic-password-1234\nGRANT_AUDIT_LOG admin\n"
			switch name {
			case "错误密码":
				input = "synthetic-wrong-password\nGRANT_AUDIT_LOG admin\n"
			case "取消":
				input = "synthetic-password-1234\ncancel\n"
			case "密码中断":
				input = "synthetic-password-1234"
			case "确认中断":
				input = "synthetic-password-1234\nGRANT_AUDIT_LOG admin"
			case "超长密码":
				input = strings.Repeat("x", 1025) + "\nGRANT_AUDIT_LOG admin\n"
			case "确认主体不符":
				input = "synthetic-password-1234\nGRANT_AUDIT_LOG another\n"
			}
			var output bytes.Buffer
			err = BootstrapInitialAuditAccess(context.Background(), directory, strings.NewReader(input), &output)
			if (err == nil) != (name == "成功") {
				t.Fatal("部署确认结果不符", err)
			}
			state, readErr := database.ReadInitialAuditAccess(context.Background(), SubjectID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			wantRevision := int64(1)
			if name == "成功" {
				wantRevision = 2
			}
			if state.Granted != (name == "成功") || state.Consumed != (name == "成功") || state.Revision != wantRevision {
				t.Fatal("失败留下授予或成功缺少标记")
			}
			after, err := os.ReadFile(settingsPath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("安装配置或密码被改写", err)
			}
			if strings.Contains(output.String(), "synthetic-password") || strings.Contains(output.String(), "synthetic-wrong") {
				t.Fatal("输出包含口令")
			}
			if name == "成功" {
				if err = BootstrapInitialAuditAccess(context.Background(), directory, strings.NewReader(input), io.Discard); err == nil {
					t.Fatal("重复部署授予仍通过")
				}
			}
		})
	}
}

func Test部署首次审计拒绝活动控制面且不读取密码(t *testing.T) {
	directory, settings, database := initialAuditFixture(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	settings.ListenAddress = listener.Addr().String()
	content, _ := json.Marshal(settings)
	if err = os.WriteFile(filepath.Join(directory, "settings.json"), content, 0600); err != nil {
		t.Fatal(err)
	}
	input := bytes.NewBufferString("synthetic-password-1234\nGRANT_AUDIT_LOG admin\n")
	before := input.Len()
	if err = BootstrapInitialAuditAccess(context.Background(), directory, input, io.Discard); err == nil || input.Len() != before {
		t.Fatal("活动端口仍进入口令或授权流程")
	}
	state, err := database.ReadInitialAuditAccess(context.Background(), SubjectID)
	if err != nil || state.Consumed || state.Granted {
		t.Fatal("活动控制面触发授权", err)
	}
}

func Test部署首次审计缺失或损坏配置不初始化(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		directory := t.TempDir()
		if corrupt {
			if err := os.WriteFile(filepath.Join(directory, "settings.json"), []byte("{}"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := BootstrapInitialAuditAccess(context.Background(), directory, strings.NewReader("synthetic-password-1234\nGRANT_AUDIT_LOG admin\n"), io.Discard); err == nil {
			t.Fatal("缺失或损坏配置仍授权")
		}
		if _, err := os.Stat(filepath.Join(directory, "local-mvp.db")); !os.IsNotExist(err) {
			t.Fatal("错误入口创建了数据库")
		}
	}
}
