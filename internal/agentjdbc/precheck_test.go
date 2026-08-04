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

func TestPrecheckProbeUsesOneConnectionForDatabaseAndObjectAndClearsBuffers(t *testing.T) {
	username := []byte("synthetic-user")
	password := []byte("synthetic-password")
	root := t.TempDir()
	resolver := &countingResolver{connection: Connection{Host: "synthetic.example", Port: 2883, Username: username, Password: password}}
	runCalls := 0
	probe := &PrecheckProbe{
		WorkspaceRoot: root,
		Resolver:      resolver,
		Run: func(_ context.Context, workspace credential.Workspace, _ jdbcprobe.Runtime, request jdbcprobe.PreflightRequest) (jdbcprobe.PreflightResult, error) {
			runCalls++
			if filepath.Dir(workspace.RuntimeDirectory()) != filepath.Join(root, "executions", "precheck-1") || string(request.Connection.Password) != "synthetic-password" || request.CompatibilityMode != jdbcprobe.CompatibilityModeMySQL || request.Database != "synthetic_db" || request.Table != "synthetic_table" {
				t.Fatal("预检查没有使用私有工作区、冻结对象或短时连接输入")
			}
			return jdbcprobe.PreflightResult{Connection: jdbcprobe.Result{ProductName: "OceanBase"}, ObjectAccess: jdbcprobe.ObjectAccessible}, nil
		},
	}
	request := validRequest()
	connection, err := probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, request)
	if err != nil || connection.Status != agentpreflight.StatusPassed || connection.EvidenceCode != EvidenceDatabaseConnected {
		t.Fatalf("连接 Probe() = %#v, %v", connection, err)
	}
	object, err := probe.Probe(context.Background(), agentpreflight.CheckObjectAccess, request)
	if err != nil || object.Status != agentpreflight.StatusPassed || object.EvidenceCode != EvidenceObjectAccessible {
		t.Fatalf("对象 Probe() = %#v, %v", object, err)
	}
	if resolver.calls != 1 || runCalls != 1 {
		t.Fatalf("resolver/run 调用次数 = %d/%d，期望 1/1", resolver.calls, runCalls)
	}
	if !allZero(username) || !allZero(password) {
		t.Fatal("预检查结束后仍保留连接输入")
	}
	if _, err := os.Stat(filepath.Join(root, "executions", "precheck-1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("预检查工作区未清理: %v", err)
	}
}

func TestPrecheckProbeFailsClosedForUnavailableOrUnreachableObject(t *testing.T) {
	for _, test := range []struct {
		name        string
		runnerError error
		access      jdbcprobe.ObjectAccess
		wantDB      agentpreflight.Status
		wantDBCode  string
		wantObject  agentpreflight.Status
		wantObjCode string
	}{
		{name: "连接被拒绝", runnerError: jdbcprobe.ErrConnectionFailed, wantDB: agentpreflight.StatusFailed, wantDBCode: EvidenceDatabaseFailed, wantObject: agentpreflight.StatusUnknown, wantObjCode: EvidenceObjectUnavailable},
		{name: "运行时不可用", runnerError: jdbcprobe.ErrDriverUnavailable, wantDB: agentpreflight.StatusUnknown, wantDBCode: EvidenceDatabaseUnavailable, wantObject: agentpreflight.StatusUnknown, wantObjCode: EvidenceObjectUnavailable},
		{name: "对象不存在或无权", access: jdbcprobe.ObjectNotFound, wantDB: agentpreflight.StatusPassed, wantDBCode: EvidenceDatabaseConnected, wantObject: agentpreflight.StatusFailed, wantObjCode: EvidenceObjectNotAccessible},
		{name: "对象元数据不可用", access: jdbcprobe.ObjectUnavailable, wantDB: agentpreflight.StatusPassed, wantDBCode: EvidenceDatabaseConnected, wantObject: agentpreflight.StatusUnknown, wantObjCode: EvidenceObjectUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			probe := &PrecheckProbe{
				WorkspaceRoot: t.TempDir(),
				Resolver:      &countingResolver{connection: Connection{Host: "synthetic.example", Port: 2883, Username: []byte("user"), Password: []byte("password")}},
				Run: func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.PreflightRequest) (jdbcprobe.PreflightResult, error) {
					return jdbcprobe.PreflightResult{ObjectAccess: test.access}, test.runnerError
				},
			}
			database, err := probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, validRequest())
			if err != nil || database.Status != test.wantDB || database.EvidenceCode != test.wantDBCode {
				t.Fatalf("连接结果 = %#v, %v", database, err)
			}
			object, err := probe.Probe(context.Background(), agentpreflight.CheckObjectAccess, validRequest())
			if err != nil || object.Status != test.wantObject || object.EvidenceCode != test.wantObjCode {
				t.Fatalf("对象结果 = %#v, %v", object, err)
			}
		})
	}
}

func TestPrecheckProbeDoesNotResolveSlotForObjectFirstOrMismatchedRequest(t *testing.T) {
	resolver := &countingResolver{connection: Connection{Host: "synthetic.example", Port: 2883, Username: []byte("user"), Password: []byte("password")}}
	runCalls := 0
	probe := &PrecheckProbe{
		WorkspaceRoot: t.TempDir(),
		Resolver:      resolver,
		Run: func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.PreflightRequest) (jdbcprobe.PreflightResult, error) {
			runCalls++
			return jdbcprobe.PreflightResult{ObjectAccess: jdbcprobe.ObjectAccessible}, nil
		},
	}
	object, err := probe.Probe(context.Background(), agentpreflight.CheckObjectAccess, validRequest())
	if err != nil || object.Status != agentpreflight.StatusUnknown || resolver.calls != 0 || runCalls != 0 {
		t.Fatalf("对象先行结果 = %#v, resolver/run=%d/%d, err=%v", object, resolver.calls, runCalls, err)
	}
	if _, err := probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, validRequest()); err != nil {
		t.Fatalf("连接检查错误 = %v", err)
	}
	mismatched := validRequest()
	mismatched.Table = "another_table"
	database, err := probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, mismatched)
	if err != nil || database.Status != agentpreflight.StatusUnknown || database.EvidenceCode != EvidenceDatabaseUnavailable || resolver.calls != 1 || runCalls != 1 {
		t.Fatalf("不匹配请求结果 = %#v, resolver/run=%d/%d, err=%v", database, resolver.calls, runCalls, err)
	}
}

func TestPrecheckProbeRejectsOtherChecks(t *testing.T) {
	_, err := (&PrecheckProbe{}).Probe(context.Background(), agentpreflight.CheckToolEnvironment, validRequest())
	if !errors.Is(err, ErrUnsupportedCheck) {
		t.Fatalf("非 JDBC 检查错误 = %v", err)
	}
}

type countingResolver struct {
	connection Connection
	calls      int
}

func (s *countingResolver) ResolveDatabaseConnection(context.Context, agentstate.PrecheckBinding) (Connection, error) {
	s.calls++
	return s.connection, nil
}

func validRequest() agentpreflight.Request {
	return agentpreflight.Request{
		Capability:        agentpreflight.CapabilityExportPreflight,
		PrecheckID:        "precheck-1",
		NodeID:            "node-1",
		AgentID:           "agent-1",
		LeaseID:           "lease-1",
		LeaseEpoch:        1,
		CompatibilityMode: "MYSQL",
		Database:          "synthetic_db",
		Table:             "synthetic_table",
		TargetPlatform:    commandgen.PlatformWindowsAMD64,
		OutputPath:        "/E:/output",
		AllowedRoots:      []string{`E:\output`},
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
