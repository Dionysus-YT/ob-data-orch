package deployment

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (b *Browser) agentPackage(w http.ResponseWriter, r *http.Request, packages, data string) {
	name := strings.TrimPrefix(r.URL.Path, "/ob-data-orch-agent-")
	// 下载名称和分发目录采用操作系统与 lscpu 架构名，Go 的 GOARCH 值仅用于构建。
	targets := map[string]string{"windows-x86_64.zip": "windows-x86_64", "linux-x86_64.zip": "linux-x86_64", "linux-aarch64.zip": "linux-aarch64"}
	target, ok := targets[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	executable := "agent"
	if target == "windows-x86_64" {
		executable += ".exe"
	}
	path := filepath.Join(packages, target, executable)
	binary, err := os.Open(path)
	if err != nil {
		http.Error(w, "此平台安装包尚未构建", http.StatusServiceUnavailable)
		return
	}
	defer binary.Close()
	ca, err := os.ReadFile(filepath.Join(data, "control-plane-ca.pem"))
	if err != nil {
		http.Error(w, "服务信任配置不可用", 503)
		return
	}
	config, _ := json.Marshal(map[string]any{"formatVersion": "agent-bundle-config-v1", "controlPlaneUrl": b.settings.PublicURL, "controlPlaneCaFile": "control-plane-ca.pem", "stateDirectory": "data/agent-security", "realExecutionEnabled": b.settings.RealExecutionEnabled && target == "windows-x86_64", "normalImportExecutionEnabled": b.settings.RealExecutionEnabled && b.settings.NormalImportExecutionEnabled && target == "windows-x86_64", "directLoadProbeEnabled": b.settings.RealExecutionEnabled && b.settings.DirectLoadProbeEnabled && target == "windows-x86_64", "directLoadExecutionEnabled": b.settings.RealExecutionEnabled && b.settings.DirectLoadProbeEnabled && b.settings.DirectLoadExecutionEnabled && target == "windows-x86_64"})
	launcher := "启动.cmd"
	if target != "windows-x86_64" {
		launcher = "start.sh"
	}
	files := map[string][]byte{"control-plane-ca.pem": ca, "agent-config.json": config}
	for _, file := range []string{launcher, "manage.ps1"} {
		if target != "windows-x86_64" && file == "manage.ps1" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(packages, target, file))
		if err != nil {
			http.Error(w, "安装入口缺失，请重新构建安装包", 503)
			return
		}
		files[file] = content
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="ob-data-orch-agent-`+name+`"`)
	if r.Method == http.MethodHead {
		return
	}
	archive := zip.NewWriter(w)
	header := &zip.FileHeader{Name: executable, Method: zip.Deflate}
	header.SetMode(0755)
	entry, err := archive.CreateHeader(header)
	if err != nil {
		return
	}
	if _, err = io.Copy(entry, binary); err != nil {
		return
	}
	for name, content := range files {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		if name == "start.sh" {
			header.SetMode(0755)
		} else {
			header.SetMode(0600)
		}
		entry, err := archive.CreateHeader(header)
		if err != nil {
			return
		}
		if _, err = entry.Write(content); err != nil {
			return
		}
	}
	_ = archive.Close()
}
