package agentexec

import (
	"strings"
	"testing"

	"ob-data-orch/internal/logstream"
)

func Test普通导入固定主类与官方启动属性隔离(t *testing.T) {
	w, security := directJavaWorkspace(t)
	java, home, digest := directJavaFiles(t)
	profile, err := windowsOBLoaderReplicaArguments(w, home, security, 491)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(profile, " ")
	for _, required := range []string{obloaderMainClass, "-Dfs.page=1048576", "-Dfile.split=SAFE", "-Ddry.run=false", "-Drebalance.import=true", "-Dsecurity.configurationFile=" + security} {
		if !strings.Contains(joined, required) {
			t.Fatal("缺少导入启动属性")
		}
	}
	for _, forbidden := range []string{obdumperMainClass, "-Dupload.", "-Denable.table.", "--direct"} {
		if strings.Contains(joined, forbidden) {
			t.Fatal("导入混入无关配置")
		}
	}
	output := directJavaOutput(t)
	output.Stdout.SourceKind, output.Stderr.SourceKind = logstream.SourceLoaderStdout, logstream.SourceLoaderStderr
	launch := DirectJavaLaunch{Tool: "OBLOADER", Intent: validStartIntent(), BootID: "boot-1", JavaPath: java, JavaSHA256: digest, ToolHome: home, SecurityConfiguration: security, BusinessArguments: []string{"--host", "127.0.0.1", "--port", "2883", "--user", "synthetic", "--database", "synthetic", "--file-path", home, "--all", "--csv", "--no-sys"}, Environment: []string{"PATH=C:\\Windows\\System32"}, Output: output}
	if validateDirectJavaLaunch(w, launch) != nil {
		t.Fatal("有效导入配置被拒绝")
	}
	launch.BusinessArguments = append(launch.BusinessArguments, "--direct")
	if validateDirectJavaLaunch(w, launch) == nil {
		t.Fatal("旁路参数被接受")
	}
}
