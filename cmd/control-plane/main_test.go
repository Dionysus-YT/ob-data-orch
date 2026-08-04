package main

import (
	"net/http"
	"testing"

	"ob-data-orch/internal/config"
)

func TestValidateLocalMVPConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  config.ControlPlane
		wantErr bool
	}{
		{name: "loopback TLS", config: config.ControlPlane{ListenAddress: "127.0.0.1:8080", TLSCertFile: "C:\\synthetic\\cert.pem", TLSKeyFile: "C:\\synthetic\\key.pem"}},
		{name: "loopback HTTP", config: config.ControlPlane{ListenAddress: "127.0.0.1:8080"}, wantErr: true},
		{name: "non-loopback address", config: config.ControlPlane{ListenAddress: "0.0.0.0:8080"}, wantErr: true},
		{name: "TLS certificate only", config: config.ControlPlane{ListenAddress: "127.0.0.1:8080", TLSCertFile: "C:\\synthetic\\cert.pem"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateLocalMVPConfiguration(tt.config); (err != nil) != tt.wantErr {
				t.Fatalf("validateLocalMVPConfiguration() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestControlPlaneServeFunctionSelectsTransport(t *testing.T) {
	t.Parallel()

	server := &http.Server{}
	transport, _ := controlPlaneServeFunction(server, config.ControlPlane{})
	if transport != "http" {
		t.Fatalf("未配置 TLS 时 transport = %q，want http", transport)
	}
	transport, _ = controlPlaneServeFunction(server, config.ControlPlane{TLSCertFile: "C:\\synthetic\\cert.pem", TLSKeyFile: "C:\\synthetic\\key.pem"})
	if transport != "https" {
		t.Fatalf("配置 TLS 时 transport = %q，want https", transport)
	}
}
