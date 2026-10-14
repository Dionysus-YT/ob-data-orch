package deployment

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type agentPackageFixture struct {
	handler  http.Handler
	settings Settings
	cookie   *http.Cookie
	root     string
}

func newAgentPackageFixture(t *testing.T) agentPackageFixture {
	t.Helper()
	settings := testSettings(t)
	settings.RealExecutionEnabled = true
	browser, err := NewBrowser(settings)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	files := map[string]string{
		"web/index.html":                   "local-mvp-csrf-v1",
		"data/control-plane-ca.pem":        "synthetic-ca-public",
		"agents/windows-x86_64/agent.exe":  "synthetic-windows-x86_64",
		"agents/windows-x86_64/启动.cmd":     "synthetic-windows-launcher",
		"agents/windows-x86_64/manage.ps1": "synthetic-windows-manager",
		"agents/linux-x86_64/agent":        "synthetic-linux-x86_64",
		"agents/linux-x86_64/start.sh":     "synthetic-linux-launcher",
		"agents/linux-aarch64/agent":       "synthetic-linux-aarch64",
		"agents/linux-aarch64/start.sh":    "synthetic-linux-launcher",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	handler, err := browser.Handler(http.NotFoundHandler(), filepath.Join(root, "web"), filepath.Join(root, "agents"), filepath.Join(root, "data"))
	if err != nil {
		t.Fatal(err)
	}
	// 通过正常登录取得合成会话，验证下载仍经过浏览器认证边界。
	request := httptest.NewRequest(http.MethodPost, settings.PublicURL+"/login", strings.NewReader(url.Values{"username": {"admin"}, "password": {"synthetic-password-1234"}}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", settings.PublicURL)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	cookies := response.Result().Cookies()
	if response.Code != http.StatusSeeOther || len(cookies) != 1 {
		t.Fatal("合成登录失败")
	}
	return agentPackageFixture{handler: handler, settings: settings, cookie: cookies[0], root: root}
}

func (fixture agentPackageFixture) request(method, target string, authenticated bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, fixture.settings.PublicURL+"/ob-data-orch-agent-"+target+".zip", nil)
	if authenticated {
		request.AddCookie(fixture.cookie)
	}
	response := httptest.NewRecorder()
	fixture.handler.ServeHTTP(response, request)
	return response
}

func TestAgentPackagePlatformNamesAndContents(t *testing.T) {
	fixture := newAgentPackageFixture(t)
	for _, target := range []string{"windows-x86_64", "linux-x86_64", "linux-aarch64"} {
		t.Run(target, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				if response := fixture.request(method, target, false); response.Code != http.StatusSeeOther {
					t.Fatal("匿名下载没有保持认证边界")
				}
				response := fixture.request(method, target, true)
				if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/zip" || response.Header().Get("Content-Disposition") != `attachment; filename="ob-data-orch-agent-`+target+`.zip"` {
					t.Fatalf("下载响应不匹配: %s %d", method, response.Code)
				}
				if method == http.MethodHead {
					if response.Body.Len() != 0 {
						t.Fatal("HEAD 不应发送安装包正文")
					}
					continue
				}
				archive, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
				if err != nil {
					t.Fatal(err)
				}
				files := make(map[string][]byte)
				for _, file := range archive.File {
					reader, err := file.Open()
					if err != nil {
						t.Fatal(err)
					}
					content, err := io.ReadAll(reader)
					reader.Close()
					if err != nil {
						t.Fatal(err)
					}
					files[file.Name] = content
					if file.Name == "agent" || file.Name == "start.sh" || file.Name == "agent.exe" {
						if file.Mode().Perm() != 0755 {
							t.Fatalf("安装包执行权限缺失: %s", file.Name)
						}
					}
				}
				executable, launcher, expectedCount := "agent", "start.sh", 4
				if target == "windows-x86_64" {
					executable, launcher, expectedCount = "agent.exe", "启动.cmd", 5
					if string(files["manage.ps1"]) != "synthetic-windows-manager" {
						t.Fatal("Windows 管理入口缺失")
					}
				}
				if len(files) != expectedCount || string(files[executable]) != "synthetic-"+target || len(files[launcher]) == 0 || string(files["control-plane-ca.pem"]) != "synthetic-ca-public" {
					t.Fatal("安装包平台、入口或信任材料不匹配")
				}
				var config struct {
					FormatVersion        string `json:"formatVersion"`
					ControlPlaneURL      string `json:"controlPlaneUrl"`
					ControlPlaneCAFile   string `json:"controlPlaneCaFile"`
					StateDirectory       string `json:"stateDirectory"`
					RealExecutionEnabled bool   `json:"realExecutionEnabled"`
				}
				if err := json.Unmarshal(files["agent-config.json"], &config); err != nil {
					t.Fatal(err)
				}
				if config.FormatVersion != "agent-bundle-config-v1" || config.ControlPlaneURL != fixture.settings.PublicURL || config.ControlPlaneCAFile != "control-plane-ca.pem" || config.StateDirectory != "data/agent-security" || config.RealExecutionEnabled != (target == "windows-x86_64") {
					t.Fatal("下载配置改变了控制面绑定、状态目录或执行开关边界")
				}
			}
		})
	}
	for _, target := range []string{"windows-aarch64", "windows-amd64", "linux-amd64", "linux-arm64", "linux-../linux-aarch64"} {
		if response := fixture.request(http.MethodGet, target, true); response.Code != http.StatusNotFound {
			t.Fatalf("未知或旧平台名称未拒绝: %s %d", target, response.Code)
		}
	}
	if response := fixture.request(http.MethodPost, "windows-x86_64", true); response.Code != http.StatusMethodNotAllowed {
		t.Fatal("下载端点接受了写方法")
	}
}

func TestAgentPackageMissingResourcesFailClosed(t *testing.T) {
	for _, missing := range []string{
		"agents/windows-x86_64/agent.exe", "agents/windows-x86_64/启动.cmd", "agents/windows-x86_64/manage.ps1",
		"agents/linux-x86_64/agent", "agents/linux-x86_64/start.sh",
		"agents/linux-aarch64/agent", "agents/linux-aarch64/start.sh", "data/control-plane-ca.pem",
	} {
		t.Run(missing, func(t *testing.T) {
			fixture := newAgentPackageFixture(t)
			if err := os.Remove(filepath.Join(fixture.root, filepath.FromSlash(missing))); err != nil {
				t.Fatal(err)
			}
			target := "windows-x86_64"
			if strings.HasPrefix(missing, "agents/linux-") {
				target = strings.Split(missing, "/")[1]
			}
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				response := fixture.request(method, target, true)
				if response.Code != http.StatusServiceUnavailable || response.Header().Get("Content-Type") == "application/zip" {
					t.Fatal("缺少资源时不得返回可下载的安装包")
				}
			}
		})
	}
}
