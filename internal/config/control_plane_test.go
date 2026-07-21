package config

import "testing"

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
			got, err := LoadControlPlane(func(string) (string, bool) {
				return tt.value, tt.exists
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
