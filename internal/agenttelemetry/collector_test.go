package agenttelemetry

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ob-data-orch/internal/agentwire"
)

type runtimeConfigurationStub struct {
	configuration agentwire.RuntimeConfiguration
	err           error
}

func (s runtimeConfigurationStub) RuntimeConfiguration() (agentwire.RuntimeConfiguration, error) {
	return s.configuration, s.err
}

func Test采集器上报固定目录空间且不暴露路径(t *testing.T) {
	root := filepath.Join(t.TempDir(), "export-data")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	configuration := agentwire.RuntimeConfiguration{
		Platform:     "WINDOWS_AMD64",
		ToolHome:     root,
		JavaPath:     filepath.Join(root, "bin", "java.exe"),
		AllowedRoots: []string{root},
		Revision:     1,
		Digest:       strings.Repeat("a", 64),
	}
	snapshot, err := NewCollector(runtimeConfigurationStub{configuration: configuration}).Sample()
	if err != nil {
		t.Fatalf("Sample() error = %v", err)
	}
	if snapshot.RuntimeConfigurationDigest != configuration.Digest || len(snapshot.DataRootUsages) != 1 {
		t.Fatalf("采集结果不完整: %#v", snapshot)
	}
	usage := snapshot.DataRootUsages[0]
	if usage.RootDigest != DataRootDigest(root) || usage.RootDigest == root || usage.TotalBytes == 0 || usage.AvailableBytes > usage.TotalBytes {
		t.Fatalf("目录空间上报不安全或无效: %#v", usage)
	}
}

func Test采集器在固化配置不可用时失败关闭(t *testing.T) {
	if _, err := NewCollector(runtimeConfigurationStub{err: errors.New("configuration unavailable")}).Sample(); err == nil {
		t.Fatal("Sample() 在运行时配置不可用时成功")
	}
}
