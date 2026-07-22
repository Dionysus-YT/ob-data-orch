package jdbcprobe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverRuntimeLocksConnectorToOfficialPackageLayout(t *testing.T) {
	root := t.TempDir()
	javaPath := filepath.Join(root, "java")
	toolHome := filepath.Join(root, "ob-loader-dumper")
	connectorPath := filepath.Join(toolHome, "lib", connectorFileName)
	if err := os.WriteFile(javaPath, []byte("synthetic-java"), 0o600); err != nil {
		t.Fatalf("write java fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(connectorPath), 0o700); err != nil {
		t.Fatalf("create connector directory: %v", err)
	}
	if err := os.WriteFile(connectorPath, []byte("synthetic-connector"), 0o600); err != nil {
		t.Fatalf("write connector fixture: %v", err)
	}
	runtime, err := DiscoverRuntime(javaPath, toolHome, []string{"PATH=/synthetic"})
	if err != nil {
		t.Fatalf("DiscoverRuntime() 错误 = %v", err)
	}
	if runtime.ConnectorPath != connectorPath || runtime.JavaSHA256 == "" || runtime.ConnectorSHA256 == "" || runtime.ProbePath != "" {
		t.Fatalf("运行时清单不符合固定布局: %#v", runtime)
	}
}

func TestDiscoverRuntimeRejectsMissingConnectorAndUnsafeEnvironment(t *testing.T) {
	root := t.TempDir()
	javaPath := filepath.Join(root, "java")
	if err := os.WriteFile(javaPath, []byte("synthetic-java"), 0o600); err != nil {
		t.Fatalf("write java fixture: %v", err)
	}
	if _, err := DiscoverRuntime(javaPath, root, []string{"PATH=/synthetic"}); err == nil {
		t.Fatal("缺少固定 Connector/J 仍被接受")
	}
	if _, err := DiscoverRuntime(javaPath, root, []string{"JAVA_TOOL_OPTIONS=-unsafe"}); err == nil {
		t.Fatal("不安全 Java 环境仍被接受")
	}
}
