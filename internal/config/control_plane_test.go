package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadControlPlane(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		value       string
		exists      bool
		wantAddress string
		wantErr     bool
	}{
		{name: "default loopback", wantAddress: "127.0.0.1:8080"},
		{name: "configured", value: "0.0.0.0:9090", exists: true, wantAddress: "0.0.0.0:9090"},
		{name: "invalid", value: "9090", exists: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := LoadControlPlane(func(key string) (string, bool) {
				if key == ControlPlaneListenEnvironmentVariable {
					return tt.value, tt.exists
				}
				if key == "ProgramData" {
					return "C:\\ProgramData", true
				}
				if key == "HOME" {
					return "/home/synthetic", true
				}
				return "", false
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("LoadControlPlane() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got.ListenAddress != tt.wantAddress {
				t.Fatalf("LoadControlPlane() address = %q, want %q", got.ListenAddress, tt.wantAddress)
			}
		})
	}
}

func TestLoadControlPlaneTLSFiles(t *testing.T) {
	t.Parallel()

	temporaryDirectory := t.TempDir()
	certificateFile := filepath.Join(temporaryDirectory, "control-plane.crt")
	keyFile := filepath.Join(temporaryDirectory, "control-plane.key")
	if err := os.WriteFile(certificateFile, []byte("synthetic certificate"), 0o600); err != nil {
		t.Fatalf("写入合成证书失败: %v", err)
	}
	if err := os.WriteFile(keyFile, []byte("synthetic key"), 0o600); err != nil {
		t.Fatalf("写入合成私钥失败: %v", err)
	}

	tests := []struct {
		name    string
		cert    string
		key     string
		wantErr bool
	}{
		{name: "not configured"},
		{name: "certificate only", cert: certificateFile, wantErr: true},
		{name: "key only", key: keyFile, wantErr: true},
		{name: "explicitly empty certificate", cert: " ", key: keyFile, wantErr: true},
		{name: "relative certificate", cert: "control-plane.crt", key: keyFile, wantErr: true},
		{name: "directory certificate", cert: temporaryDirectory, key: keyFile, wantErr: true},
		{name: "paired regular files", cert: certificateFile, key: keyFile},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := LoadControlPlane(func(key string) (string, bool) {
				switch key {
				case TLSCertFileEnvironmentVariable:
					return tt.cert, tt.cert != ""
				case TLSKeyFileEnvironmentVariable:
					return tt.key, tt.key != ""
				case "ProgramData":
					return "C:\\ProgramData", true
				case "HOME":
					return "/home/synthetic", true
				default:
					return "", false
				}
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("LoadControlPlane() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.cert != "" {
				if got.TLSCertFile != filepath.Clean(certificateFile) || got.TLSKeyFile != filepath.Clean(keyFile) {
					t.Fatalf("TLS 文件路径 = (%q, %q)，未按预期规范化", got.TLSCertFile, got.TLSKeyFile)
				}
			}
		})
	}
}
