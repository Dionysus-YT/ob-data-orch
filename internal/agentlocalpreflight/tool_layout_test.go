package agentlocalpreflight

import (
	"path/filepath"
	"testing"

	"ob-data-orch/internal/commandgen"
)

func Test工具启动布局区分Windows子目录与Linux直接目录(t *testing.T) {
	root := t.TempDir()
	for _, tool := range []string{"obdumper", "obloader"} {
		for _, platform := range []commandgen.Platform{commandgen.PlatformWindowsAMD64, commandgen.PlatformLinuxAMD64, commandgen.PlatformLinuxARM64} {
			actual, ok := ToolLauncherPath(root, platform, tool)
			wanted := filepath.Join(root, "bin", tool)
			if platform == commandgen.PlatformWindowsAMD64 {
				wanted = filepath.Join(root, "bin", "windows", tool+".bat")
			}
			if !ok || actual != wanted {
				t.Fatal("启动布局与官方平台目录不一致")
			}
		}
	}
	for _, test := range []struct {
		root, tool string
		platform   commandgen.Platform
	}{{"relative", "obdumper", commandgen.PlatformWindowsAMD64}, {root, "other-tool", commandgen.PlatformWindowsAMD64}, {root, "obloader", "UNKNOWN"}} {
		if _, ok := ToolLauncherPath(test.root, test.platform, test.tool); ok {
			t.Fatal("非法目录、工具或平台通过固定布局边界")
		}
	}
}
