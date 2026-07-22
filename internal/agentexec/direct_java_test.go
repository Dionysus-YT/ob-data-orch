package agentexec

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"ob-data-orch/internal/credential"
)

func Test直接Java启动输入只接受私有安全配置与已校验Java(t *testing.T) {
	workspace, configuration := directJavaWorkspace(t)
	javaPath, toolHome, digest := directJavaFiles(t)
	launch := DirectJavaLaunch{
		Intent:                validStartIntent(),
		BootID:                "boot-1",
		JavaPath:              javaPath,
		JavaSHA256:            digest,
		ToolHome:              toolHome,
		SecurityConfiguration: configuration,
		JVMOptions:            []string{"-Xms64m", "-XX:+UseG1GC"},
		BusinessArguments:     []string{"--host", "127.0.0.1", "--csv"},
		Environment:           []string{"PATH=C:\\Windows\\System32", "SystemRoot=C:\\Windows"},
	}
	if err := validateDirectJavaLaunch(workspace, launch); err != nil {
		t.Fatalf("validateDirectJavaLaunch() 错误 = %v", err)
	}
	launch.SecurityConfiguration = filepath.Join(t.TempDir(), "security.properties")
	if err := validateDirectJavaLaunch(workspace, launch); err != ErrDirectJavaLaunchInvalid {
		t.Fatalf("外部安全配置错误 = %v", err)
	}
}

func Test直接Java启动拒绝密码参数与环境注入(t *testing.T) {
	workspace, configuration := directJavaWorkspace(t)
	javaPath, toolHome, digest := directJavaFiles(t)
	launch := DirectJavaLaunch{
		Intent:                validStartIntent(),
		BootID:                "boot-1",
		JavaPath:              javaPath,
		JavaSHA256:            digest,
		ToolHome:              toolHome,
		SecurityConfiguration: configuration,
		BusinessArguments:     []string{"--password", "synthetic-secret-not-allowed"},
		Environment:           []string{"PATH=C:\\Windows\\System32"},
	}
	if err := validateDirectJavaLaunch(workspace, launch); err != ErrDirectJavaLaunchInvalid {
		t.Fatalf("密码参数错误 = %v", err)
	}
	launch.BusinessArguments = []string{"--csv"}
	launch.Environment = []string{"PATH=C:\\Windows\\System32", "JAVA_TOOL_OPTIONS=-Dunsafe=true"}
	if err := validateDirectJavaLaunch(workspace, launch); err != ErrDirectJavaLaunchInvalid {
		t.Fatalf("环境注入错误 = %v", err)
	}
}

func directJavaWorkspace(t *testing.T) (credential.Workspace, string) {
	t.Helper()
	workspace := createWorkspace(t)
	t.Cleanup(func() { _ = workspace.Cleanup() })
	secret := []byte("synthetic-direct-java-secret")
	material, err := credential.GenerateSecurityMaterial(secret)
	credential.Zero(secret)
	if err != nil {
		t.Fatalf("GenerateSecurityMaterial() 错误 = %v", err)
	}
	defer material.Destroy()
	paths, err := workspace.WriteSecurityMaterial(&material)
	if err != nil {
		t.Fatalf("WriteSecurityMaterial() 错误 = %v", err)
	}
	return workspace, paths.SecurityConfiguration
}

func directJavaFiles(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	javaPath := filepath.Join(root, "java.exe")
	content := []byte("synthetic-java-binary")
	if err := os.WriteFile(javaPath, content, 0o600); err != nil {
		t.Fatalf("写入 Java 夹具错误 = %v", err)
	}
	toolHome := filepath.Join(root, "tool")
	if err := os.MkdirAll(filepath.Join(toolHome, "lib"), 0o700); err != nil {
		t.Fatalf("创建工具目录错误 = %v", err)
	}
	digest := sha256.Sum256(content)
	return javaPath, toolHome, hex.EncodeToString(digest[:])
}
