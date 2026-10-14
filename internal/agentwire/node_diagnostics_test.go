package agentwire

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/nodefacts"
)

func Test诊断事实通过受认证心跳且拒绝非法自报(t *testing.T) {
	now, started := time.Now().UTC(), time.Now().UTC().Add(-time.Minute)
	input := Heartbeat{BootID: "synthetic-boot", SentAt: now, Facts: EnvironmentFacts{OperatingSystem: "WINDOWS", Architecture: "AMD64", AgentVersion: "synthetic", ObservedAt: now, CapacityTotal: 1, StartedAt: &started,
		Host:                 &nodefacts.HostInventory{Hostname: "synthetic-host", Addresses: []string{"192.0.2.99"}, LogicalCPUs: 8},
		RuntimeVersions:      &nodefacts.RuntimeVersions{Java: &nodefacts.InstalledVersion{Version: "1.8.0_491", Source: "JAVA_RELEASE"}},
		ExecutionObservation: &nodefacts.ExecutionObservation{Active: []nodefacts.ActiveExecution{}}, LogQueue: &nodefacts.LogQueue{LastUploadResult: "UNKNOWN"}}}
	if !validHeartbeat(input) {
		t.Fatal("安全诊断事实被拒绝")
	}
	invalidGrant := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request heartbeatRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			t.Error("心跳无法严格解码")
		}
		if r.Header.Get("Authorization") == "" || request.Payload.StartedAt == nil || request.Payload.RuntimeVersions.Java.Version != "1.8.0_491" || request.Payload.ExecutionObservation == nil || request.Payload.ExecutionObservation.Active == nil || request.Payload.LogQueue == nil || request.Payload.Host == nil || request.Payload.Host.Hostname != "synthetic-host" {
			t.Error("诊断字段或机器认证未进入 HTTP")
		}
		payload := map[string]any{"factsRevision": 2, "environmentStatus": "NOT_CHECKED", "realExecutionEnabled": false, "environmentVersionProbeAllowed": true}
		if !invalidGrant {
			payload["environmentCheckId"] = "synthetic-check"
		}
		writeProtocolResponse(t, w, "ACCEPTED", payload)
	}))
	defer server.Close()
	client, err := newHTTPSClient(server.URL, writeServerCA(t, server))
	if err != nil {
		t.Fatal(err)
	}
	identity := &identityState{ProtocolVersion: Version, AgentID: "synthetic-agent", NodeID: "synthetic-node", MachineCredential: []byte(strings.Repeat("x", 40))}
	if result, err := client.sendHeartbeat(context.Background(), identity, "synthetic-request", input); err != nil || !result.EnvironmentVersionProbeAllowed || result.EnvironmentCheckID != "synthetic-check" {
		t.Fatal("心跳响应被拒绝", err)
	}
	invalidGrant = true
	if _, err := client.sendHeartbeat(context.Background(), identity, "synthetic-request-2", input); !errors.Is(err, ErrProtocolRejected) {
		t.Fatal("无检查标识的版本启动许可没有被拒绝")
	}
	input.Facts.LogQueue.PendingBytes = -1
	if validHeartbeat(input) {
		t.Fatal("非法队列数值可以被 Agent 发送")
	}
}
