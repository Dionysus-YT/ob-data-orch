package agentwire

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigurationRejectsDigestRollbackAndCrossPlatform(t *testing.T) {
	initial := RuntimeConfiguration{Platform: "WINDOWS_AMD64", ToolHome: `E:\synthetic\tools`, JavaPath: `C:\synthetic\java\bin\java.exe`, AllowedRoots: []string{`E:\synthetic\exports`}, Revision: 2}
	initial.Digest = runtimeConfigurationDigest(initial)
	configuration := initial
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-machine-configuration-0123456789" {
			w.WriteHeader(401)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"requestId": "synthetic-sync", "serverTime": time.Now().UTC().Format(time.RFC3339Nano), "status": "CONFIGURED", "payload": configuration})
	}))
	defer server.Close()
	directory := t.TempDir()
	ca := filepath.Join(directory, "ca.pem")
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	state, _ := OpenStateStore(filepath.Join(directory, "state"))
	identity := identityState{FormatVersion: identityStateFormatVersion, ControlPlaneURL: server.URL, CAFile: ca, ProtocolVersion: Version, AgentID: "agent-config", NodeID: "node-config", MachineCredential: []byte("synthetic-machine-configuration-0123456789"), RuntimeConfiguration: initial}
	defer identity.destroy()
	if err := state.saveState(&identity); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"digest", "rollback", "platform", "same-version"} {
		configuration = initial
		configuration.Revision = 3
		switch kind {
		case "digest":
			configuration.ToolHome = `E:\synthetic\changed`
		case "rollback":
			configuration.Revision = 1
		case "platform":
			configuration.Platform = "LINUX_AMD64"
			configuration.ToolHome = "/synthetic/tools"
			configuration.JavaPath = "/synthetic/java/bin/java"
			configuration.AllowedRoots = []string{"/synthetic/exports"}
			configuration.Digest = runtimeConfigurationDigest(configuration)
		case "same-version":
			configuration.Revision = 2
			configuration.ToolHome = `E:\synthetic\changed`
			configuration.Digest = runtimeConfigurationDigest(configuration)
		}
		if err := state.SyncRuntimeConfiguration(context.Background()); err == nil {
			t.Fatalf("无效配置被接受: %s", kind)
		}
		actual, err := state.RuntimeConfiguration()
		if err != nil || actual.Digest != initial.Digest {
			t.Fatal("失败后改变原配置")
		}
	}
}
