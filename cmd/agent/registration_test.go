package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRegistrationCodeAcceptsStrictOneTimePayload(t *testing.T) {
	code := testRegistrationCode(t, "node-1", "enrollment-1", "synthetic-enrollment-material-0123456789")
	payload, err := readRegistrationCode(strings.NewReader(code + "\n"))
	if err != nil {
		t.Fatalf("readRegistrationCode() error = %v", err)
	}
	if payload.NodeID != "node-1" || payload.EnrollmentID != "enrollment-1" || payload.EnrollmentMaterial != "synthetic-enrollment-material-0123456789" {
		t.Fatal("注册码未恢复为原始关联输入")
	}
	if _, err := readRegistrationCode(strings.NewReader("obdo-r1.invalid")); err == nil {
		t.Fatal("无效注册码被接受")
	}
}

func TestLoadBundledAgentControlPlaneRequiresFixedLocalPackage(t *testing.T) {
	bundleDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(bundleDirectory, "control-plane-ca.pem"), []byte("synthetic ca"), 0o600); err != nil {
		t.Fatalf("写入测试 CA 失败: %v", err)
	}
	configBody := []byte(`{"formatVersion":"agent-bundle-config-v1","controlPlaneUrl":"https://127.0.0.1:8080","controlPlaneCaFile":"control-plane-ca.pem","stateDirectory":"agent-security"}`)
	if err := os.WriteFile(filepath.Join(bundleDirectory, agentBundleConfigFileName), configBody, 0o600); err != nil {
		t.Fatalf("写入测试 Agent 包配置失败: %v", err)
	}
	controlPlane, err := loadBundledAgentControlPlane(bundleDirectory, func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("loadBundledAgentControlPlane() error = %v", err)
	}
	if controlPlane.URL != defaultLocalControlPlaneURL || controlPlane.CAFile != filepath.Join(bundleDirectory, "control-plane-ca.pem") || controlPlane.StateDirectory != filepath.Join(bundleDirectory, "agent-security") {
		t.Fatal("Agent 包配置未被严格解析")
	}
}

func testRegistrationCode(t *testing.T, nodeID, enrollmentID, material string) string {
	t.Helper()
	payload, err := json.Marshal(registrationCodePayload{FormatVersion: "obdo-r1", NodeID: nodeID, EnrollmentID: enrollmentID, EnrollmentMaterial: material})
	if err != nil {
		t.Fatalf("编码测试注册码失败: %v", err)
	}
	return registrationCodePrefix + base64.RawURLEncoding.EncodeToString(payload)
}
