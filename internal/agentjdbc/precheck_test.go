package agentjdbc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/jdbcprobe"
)

func TestPrecheckProbeRunsOnlyConnectivityAndClearsConnectionBuffers(t *testing.T) {
	username := []byte("synthetic-user")
	password := []byte("synthetic-password")
	root := t.TempDir()
	probe := PrecheckProbe{
		WorkspaceRoot: root,
		Resolver:      staticResolver{connection: Connection{Host: "synthetic.example", Port: 2883, Username: username, Password: password}},
		Run: func(_ context.Context, workspace credential.Workspace, _ jdbcprobe.Runtime, request jdbcprobe.Request) (jdbcprobe.Result, error) {
			if filepath.Dir(workspace.RuntimeDirectory()) != filepath.Join(root, "executions", "precheck-1") || string(request.Password) != "synthetic-password" {
				t.Fatal("预检查未使用私有工作区或未取得短时连接输入")
			}
			return jdbcprobe.Result{ProductName: "OceanBase"}, nil
		},
	}
	result, err := probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, validRequest())
	if err != nil || result.Status != agentpreflight.StatusPassed || result.EvidenceCode != EvidenceDatabaseConnected {
		t.Fatalf("Probe() = %#v, %v", result, err)
	}
	if !allZero(username) || !allZero(password) {
		t.Fatal("预检查结束后仍保留连接输入")
	}
	if _, err := os.Stat(filepath.Join(root, "executions", "precheck-1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("预检查工作区未清理: %v", err)
	}
}

func TestPrecheckProbeFailsClosedWhenSlotOrRunnerUnavailable(t *testing.T) {
	request := validRequest()
	for _, probe := range []PrecheckProbe{
		{WorkspaceRoot: t.TempDir()},
		{WorkspaceRoot: t.TempDir(), Resolver: failingResolver{}},
	} {
		result, err := probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, request)
		if err != nil || result.Status != agentpreflight.StatusUnknown || result.EvidenceCode != EvidenceDatabaseUnavailable {
			t.Fatalf("不可用探针结果 = %#v, %v", result, err)
		}
	}
}

func TestPrecheckProbeClassifiesOnlyConfirmedConnectionFailure(t *testing.T) {
	for _, test := range []struct {
		err  error
		want agentpreflight.Status
		code string
	}{
		{err: jdbcprobe.ErrConnectionFailed, want: agentpreflight.StatusFailed, code: EvidenceDatabaseFailed},
		{err: jdbcprobe.ErrDriverUnavailable, want: agentpreflight.StatusUnknown, code: EvidenceDatabaseUnavailable},
	} {
		probe := PrecheckProbe{
			WorkspaceRoot: t.TempDir(),
			Resolver:      staticResolver{connection: Connection{Host: "synthetic.example", Port: 2883, Username: []byte("user"), Password: []byte("password")}},
			Run: func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.Request) (jdbcprobe.Result, error) {
				return jdbcprobe.Result{}, test.err
			},
		}
		result, err := probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, validRequest())
		if err != nil || result.Status != test.want || result.EvidenceCode != test.code {
			t.Fatalf("错误 %v 的结果 = %#v, %v", test.err, result, err)
		}
	}
}

func TestPrecheckProbeRejectsNonConnectivityCheck(t *testing.T) {
	_, err := (PrecheckProbe{}).Probe(context.Background(), agentpreflight.CheckObjectAccess, validRequest())
	if !errors.Is(err, ErrUnsupportedCheck) {
		t.Fatalf("非连接检查错误 = %v", err)
	}
}

type staticResolver struct{ connection Connection }

func (s staticResolver) ResolveDatabaseConnection(context.Context, agentstate.PrecheckBinding) (Connection, error) {
	return s.connection, nil
}

type failingResolver struct{}

func (failingResolver) ResolveDatabaseConnection(context.Context, agentstate.PrecheckBinding) (Connection, error) {
	return Connection{}, errors.New("synthetic resolver unavailable")
}

func validRequest() agentpreflight.Request {
	return agentpreflight.Request{
		Capability:     agentpreflight.CapabilityExportPreflight,
		PrecheckID:     "precheck-1",
		NodeID:         "node-1",
		AgentID:        "agent-1",
		LeaseID:        "lease-1",
		LeaseEpoch:     1,
		TargetPlatform: commandgen.PlatformWindowsAMD64,
		OutputPath:     `E:\output`,
		Binding: agentstate.PrecheckBinding{
			PrecheckID:         "precheck-1",
			NodeID:             "node-1",
			DraftRevision:      1,
			ConfigFingerprint:  "fingerprint",
			CredentialRevision: 1,
			NodeFactsVersion:   1,
		},
	}
}

func allZero(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}
