package agentjdbc

import (
	"context"
	"errors"
	"fmt"
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
		RunBatch: func(_ context.Context, workspace credential.Workspace, _ jdbcprobe.Runtime, request jdbcprobe.BatchPreflightRequest) (jdbcprobe.PreflightResult, error) {
			runCalls++
			if filepath.Dir(workspace.RuntimeDirectory()) != filepath.Join(root, "executions", "precheck-1") || string(request.Connection.Password) != "synthetic-password" || request.CompatibilityMode != jdbcprobe.CompatibilityModeMySQL || request.Database != "synthetic_db" || len(request.Objects) != 1 || request.Objects[0] != (jdbcprobe.PreflightObject{Type: "TABLE", Name: "synthetic_table"}) {
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
		{name: "对象不存在或无权", access: jdbcprobe.ObjectNotAccessible, wantDB: agentpreflight.StatusPassed, wantDBCode: EvidenceDatabaseConnected, wantObject: agentpreflight.StatusFailed, wantObjCode: EvidenceObjectNotAccessible},
		{name: "对象元数据不可用", access: jdbcprobe.ObjectUnavailable, wantDB: agentpreflight.StatusPassed, wantDBCode: EvidenceDatabaseConnected, wantObject: agentpreflight.StatusUnknown, wantObjCode: EvidenceObjectUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			probe := &PrecheckProbe{
				WorkspaceRoot: t.TempDir(),
				Resolver:      &countingResolver{connection: Connection{Host: "synthetic.example", Port: 2883, Username: []byte("user"), Password: []byte("password")}},
				RunBatch: func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.BatchPreflightRequest) (jdbcprobe.PreflightResult, error) {
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
		RunBatch: func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.BatchPreflightRequest) (jdbcprobe.PreflightResult, error) {
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
	mismatched.Objects = []string{"another_table"}
	database, err := probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, mismatched)
	if err != nil || database.Status != agentpreflight.StatusUnknown || database.EvidenceCode != EvidenceDatabaseUnavailable || resolver.calls != 1 || runCalls != 1 {
		t.Fatalf("不匹配请求结果 = %#v, resolver/run=%d/%d, err=%v", database, resolver.calls, runCalls, err)
	}
}

func TestPrecheckProbeDoesNotReuseResultForAmbiguousObjectNames(t *testing.T) {
	probe := &PrecheckProbe{
		WorkspaceRoot: t.TempDir(),
		Resolver: &countingResolver{connection: Connection{
			Host: "synthetic.example", Port: 2883, Username: []byte("user"), Password: []byte("password"),
		}},
		RunBatch: func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.BatchPreflightRequest) (jdbcprobe.PreflightResult, error) {
			return jdbcprobe.PreflightResult{ObjectAccess: jdbcprobe.ObjectAccessible}, nil
		},
	}
	first := validRequest()
	first.Objects = []string{"table_one", "table_two"}
	if result, err := probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, first); err != nil || result.Status != agentpreflight.StatusPassed {
		t.Fatalf("首个冻结清单结果 = %#v, %v", result, err)
	}
	second := validRequest()
	second.Objects = []string{"table_one,table_two"}
	if result, err := probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, second); err != nil || result.Status != agentpreflight.StatusUnknown {
		t.Fatalf("歧义对象名复用了已有结果：%#v, %v", result, err)
	}
}

func TestPrecheckProbeRejectsOtherChecks(t *testing.T) {
	_, err := (&PrecheckProbe{}).Probe(context.Background(), agentpreflight.CheckToolEnvironment, validRequest())
	if !errors.Is(err, ErrUnsupportedCheck) {
		t.Fatalf("非 JDBC 检查错误 = %v", err)
	}
}

// TestPrecheckProbeExploresEachFrozenObjectOnce 验证 SPECIFIED 多对象共用一次冻结探针和秘密槽位。
func TestPrecheckProbeExploresEachFrozenObjectOnce(t *testing.T) {
	resolver := &countingResolver{connection: Connection{Host: "synthetic.example", Port: 2883, Username: []byte("user"), Password: []byte("password")}}
	runCalls := 0
	probe := &PrecheckProbe{
		WorkspaceRoot: t.TempDir(),
		Resolver:      resolver,
		RunBatch: func(_ context.Context, workspace credential.Workspace, runtime jdbcprobe.Runtime, request jdbcprobe.BatchPreflightRequest) (jdbcprobe.PreflightResult, error) {
			runCalls++
			if runtime.ProbePath != filepath.Join(workspace.RuntimeDirectory(), "ob-data-orch-jdbc-probe.jar") || runtime.ProbeSHA256 == "" {
				t.Fatal("多对象预检查没有复用已安装的固定探针")
			}
			if _, _, err := jdbcprobe.Install(workspace); !errors.Is(err, jdbcprobe.ErrProbeFailed) {
				t.Fatalf("固定探针资产允许重复安装: %v", err)
			}
			want := []jdbcprobe.PreflightObject{{Type: "TABLE", Name: "table_one"}, {Type: "TABLE", Name: "table_two"}, {Type: "TABLE", Name: "view_one"}}
			if len(request.Objects) != len(want) {
				t.Fatalf("冻结对象数量 = %d", len(request.Objects))
			}
			for index := range want {
				if request.Objects[index] != want[index] {
					t.Fatalf("冻结对象[%d] = %#v", index, request.Objects[index])
				}
			}
			return jdbcprobe.PreflightResult{Connection: jdbcprobe.Result{ProductName: "OceanBase"}, ObjectAccess: jdbcprobe.ObjectAccessible}, nil
		},
	}
	request := validRequest()
	request.Objects = []string{"table_one", "table_two", "view_one"}
	request.ContentKind = "DDL_ONLY"
	connection, err := probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, request)
	if err != nil || connection.Status != agentpreflight.StatusPassed {
		t.Fatalf("多对象连接结果 = %#v, %v", connection, err)
	}
	object, err := probe.Probe(context.Background(), agentpreflight.CheckObjectAccess, request)
	if err != nil || object.Status != agentpreflight.StatusPassed || object.EvidenceCode != EvidenceObjectAccessible {
		t.Fatalf("多对象结果 = %#v, %v", object, err)
	}
	if runCalls != 1 || resolver.calls != 1 {
		t.Fatalf("批量探针/槽位解析调用 = %d/%d", runCalls, resolver.calls)
	}
}

// TestPrecheckProbeFailsClosedWhenAnyObjectInaccessible 验证任一对象不可达即整体失败。
func TestPrecheckProbeFailsClosedWhenAnyObjectInaccessible(t *testing.T) {
	probe := &PrecheckProbe{
		WorkspaceRoot: t.TempDir(),
		Resolver:      &countingResolver{connection: Connection{Host: "synthetic.example", Port: 2883, Username: []byte("user"), Password: []byte("password")}},
		RunBatch: func(_ context.Context, _ credential.Workspace, _ jdbcprobe.Runtime, request jdbcprobe.BatchPreflightRequest) (jdbcprobe.PreflightResult, error) {
			access := jdbcprobe.ObjectAccessible
			if len(request.Objects) == 2 && request.Objects[1].Name == "table_two" {
				access = jdbcprobe.ObjectNotAccessible
			}
			return jdbcprobe.PreflightResult{ObjectAccess: access}, nil
		},
	}
	request := validRequest()
	request.Objects = []string{"table_one", "table_two"}
	if _, err := probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, request); err != nil {
		t.Fatalf("连接检查错误 = %v", err)
	}
	object, err := probe.Probe(context.Background(), agentpreflight.CheckObjectAccess, request)
	if err != nil || object.Status != agentpreflight.StatusFailed || object.EvidenceCode != EvidenceObjectNotAccessible {
		t.Fatalf("部分不可达结果 = %#v, %v", object, err)
	}
}

// TestPrecheckProbeMixedObjectTypesUsesFrozenBatch 验证五类冻结对象一次传入并对缺失对象失败关闭。
func TestPrecheckProbeMixedObjectTypesUsesFrozenBatch(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint("missing=", missing), func(t *testing.T) {
			runCalls := 0
			probe := &PrecheckProbe{
				WorkspaceRoot: t.TempDir(),
				Resolver:      &countingResolver{connection: Connection{Host: "synthetic.example", Port: 2883, Username: []byte("user"), Password: []byte("password")}},
				RunBatch: func(_ context.Context, _ credential.Workspace, _ jdbcprobe.Runtime, request jdbcprobe.BatchPreflightRequest) (jdbcprobe.PreflightResult, error) {
					runCalls++
					wantTypes := []string{"TABLE", "VIEW", "FUNCTION", "PROCEDURE", "SEQUENCE"}
					if len(request.Objects) != len(wantTypes) {
						t.Fatalf("混合对象数量 = %d", len(request.Objects))
					}
					for index, kind := range wantTypes {
						if request.Objects[index].Type != kind {
							t.Fatalf("混合对象类型[%d] = %s", index, request.Objects[index].Type)
						}
					}
					access := jdbcprobe.ObjectAccessible
					if missing {
						access = jdbcprobe.ObjectNotAccessible
					}
					return jdbcprobe.PreflightResult{ObjectAccess: access}, nil
				},
			}
			request := validRequest()
			request.Objects = []string{"table_one", "view_one", "fn_one", "proc_one", "seq_one"}
			request.ObjectTypes = []string{"TABLE", "VIEW", "FUNCTION", "PROCEDURE", "SEQUENCE"}
			request.ContentKind = "DDL_ONLY"
			_, _ = probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, request)
			object, err := probe.Probe(context.Background(), agentpreflight.CheckObjectAccess, request)
			want := agentpreflight.StatusPassed
			if missing {
				want = agentpreflight.StatusFailed
			}
			if err != nil || object.Status != want || runCalls != 1 {
				t.Fatalf("冻结分类预检查 = %#v, %v; calls=%d", object, err, runCalls)
			}
		})
	}
}

// TestPrecheckProbeAllScopeProjectsDatabaseLevelAccess 验证 ALL 范围只运行连接探测并按可达性投影对象结论。
func TestPrecheckProbeAllScopeProjectsDatabaseLevelAccess(t *testing.T) {
	resolver := &countingResolver{connection: Connection{Host: "synthetic.example", Port: 2883, Username: []byte("user"), Password: []byte("password")}}
	preflightCalls := 0
	connectionCalls := 0
	probe := &PrecheckProbe{
		WorkspaceRoot: t.TempDir(),
		Resolver:      resolver,
		RunBatch: func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.BatchPreflightRequest) (jdbcprobe.PreflightResult, error) {
			preflightCalls++
			return jdbcprobe.PreflightResult{ObjectAccess: jdbcprobe.ObjectAccessible}, nil
		},
		RunConnection: func(_ context.Context, _ credential.Workspace, _ jdbcprobe.Runtime, _ jdbcprobe.Request) (jdbcprobe.Result, error) {
			connectionCalls++
			return jdbcprobe.Result{ProductName: "OceanBase"}, nil
		},
	}
	request := validRequest()
	request.Objects = nil
	request.ContentKind = "DATA_ONLY"
	connection, err := probe.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, request)
	if err != nil || connection.Status != agentpreflight.StatusPassed {
		t.Fatalf("ALL 连接结果 = %#v, %v", connection, err)
	}
	object, err := probe.Probe(context.Background(), agentpreflight.CheckObjectAccess, request)
	if err != nil || object.Status != agentpreflight.StatusPassed || object.EvidenceCode != EvidenceObjectAccessible {
		t.Fatalf("ALL 对象投影 = %#v, %v", object, err)
	}
	if preflightCalls != 0 || connectionCalls != 1 || resolver.calls != 1 {
		t.Fatalf("ALL 探测调用 = preflight/connection/resolver %d/%d/%d", preflightCalls, connectionCalls, resolver.calls)
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
		Objects:           []string{"synthetic_table"},
		ContentKind:       "DATA_ONLY",
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
