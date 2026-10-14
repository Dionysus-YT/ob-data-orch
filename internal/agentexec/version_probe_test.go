package agentexec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func init() {
	mode := os.Getenv("OBDO_SYNTHETIC_VERSION_HELPER")
	if mode == "" {
		return
	}
	switch mode {
	case "JAVA":
		fmt.Fprintln(os.Stderr, `java version "1.8.0_491"`)
	case "TOOL":
		fmt.Println("Version: 4.3.5-RELEASE")
	case "HANG":
		time.Sleep(30 * time.Second)
	case "OVERFLOW":
		fmt.Print(strings.Repeat("synthetic-private-output", 2048))
	case "INVALID":
		fmt.Print("synthetic-secret-output")
	}
	os.Exit(0)
}

func Test固定版本进程受时限预算和类型白名单约束(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		mode, kind, version string
		failure             error
	}{{"JAVA", "JAVA_RUNTIME", "1.8.0_491", nil}, {"TOOL", "OBDUMPER_RUNTIME", "4.3.5-RELEASE", nil}, {"TOOL", "OBLOADER_RUNTIME", "4.3.5-RELEASE", nil}, {"OVERFLOW", "JAVA_RUNTIME", "", ErrVersionOutputInvalid}, {"INVALID", "JAVA_RUNTIME", "", ErrVersionOutputInvalid}, {"HANG", "JAVA_RUNTIME", "", ErrVersionProbeTimeout}} {
		root := t.TempDir()
		prober := FixedVersionProber{JavaPath: executable, ToolHome: root, WorkspaceRoot: root, Environment: append(os.Environ(), "OBDO_SYNTHETIC_VERSION_HELPER="+test.mode)}
		ctx := context.Background()
		cancel := func() {}
		if test.mode == "HANG" {
			ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
		}
		version, err := prober.Probe(ctx, test.kind)
		cancel()
		if version != test.version || !errors.Is(err, test.failure) {
			t.Fatalf("固定合成探针结果不符合预期: %s", test.mode)
		}
		if err != nil && strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatal("错误带出原始输出")
		}
		entries, readErr := os.ReadDir(root)
		if readErr != nil || len(entries) != 0 {
			t.Fatal("版本查询未清理私有临时目录")
		}
		if _, err := prober.Probe(context.Background(), "ARBITRARY"); !errors.Is(err, ErrVersionProbeUnavailable) {
			t.Fatal("未登记工具能够执行")
		}
	}
}

func Test版本输出拒绝冲突和任意文本(t *testing.T) {
	for _, value := range []string{"Version: 4.3.5\nVersion: 4.3.6", "Version: /private/path", "synthetic-secret-output"} {
		if _, err := parseRuntimeVersion([]byte(value), "OBDUMPER_RUNTIME"); !errors.Is(err, ErrVersionOutputInvalid) {
			t.Fatal("非法输出成为版本")
		}
	}
}
