package agentexec

import (
	"ob-data-orch/internal/credential"
	"strings"
)

// windowsOBLoaderReplicaArguments 复刻 4.3.5 Windows obloader.bat 的固定默认分支。
// 只复用 JVM、安全材料与平台隔离规则；导出专用系统属性不进入导入进程。
func windowsOBLoaderReplicaArguments(workspace credential.Workspace, toolHome, security string, update int) ([]string, error) {
	profile, err := windowsOBDumperReplicaArguments(workspace, toolHome, security, update)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(profile))
	for _, option := range profile {
		if option == obdumperMainClass {
			result = append(result, obloaderMainClass)
			continue
		}
		if strings.HasPrefix(option, "-Denable.parallel.write=") || strings.HasPrefix(option, "-Dskip.tableName.check=") || strings.HasPrefix(option, "-Dupload.") || strings.HasPrefix(option, "-Denable.table.") {
			continue
		}
		if option == "-classpath" {
			result = append(result, "-Dfs.page=1048576", "-Dfile.split=SAFE", "-Ddry.run=false", "-Drebalance.import=true")
		}
		result = append(result, option)
	}
	return result, nil
}
