package main

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/jdbcprobe"
)

type jdbcConnectionDialStub struct {
	err    error
	called bool
}

func (s *jdbcConnectionDialStub) dial(_ context.Context, network, address string) (net.Conn, error) {
	s.called = true
	if network != "tcp" || address != "synthetic.example:2881" {
		return nil, errors.New("固定 TCP 探针收到非冻结连接目标")
	}
	if s.err != nil {
		return nil, s.err
	}
	client, server := net.Pipe()
	go server.Close()
	return client, nil
}

func TestJDBCConnectionTestRunner成功时清零槽位并清理工作区(t *testing.T) {
	username := []byte("synthetic-user")
	password := []byte("synthetic-password")
	resolver := &jdbcConnectionResolverStub{slot: agentwire.DataSourceConnectionTestDatabaseConnectionSlot{
		Host: "synthetic.example", Port: 2881, Username: username, Password: password,
	}}
	root := t.TempDir()
	runner := jdbcConnectionTestRunner{
		resolver: resolver, workspaceRoot: root, bootID: "boot-1",
		dial: (&jdbcConnectionDialStub{}).dial,
		probe: func(_ context.Context, _ credential.Workspace, _ jdbcprobe.Runtime, request jdbcprobe.Request) (jdbcprobe.Result, error) {
			if request.Host != "synthetic.example" || request.Port != 2881 || string(request.Username) != "synthetic-user" || string(request.Password) != "synthetic-password" {
				t.Fatalf("固定 JDBC 探针输入不符合冻结槽位边界: %#v", request)
			}
			return jdbcprobe.Result{}, nil
		},
	}
	grant := jdbcConnectionTestGrant()
	outcome := runner.RunConnectionTest(context.Background(), grant)
	if outcome.Status != agentwire.DataSourceConnectionTestSucceeded || outcome.EvidenceCode != "DATABASE_CONNECTED" || outcome.VerificationSource != agentwire.DataSourceConnectionTestAgentJDBC {
		t.Fatalf("JDBC 成功结果 = %#v", outcome)
	}
	if resolver.request.BootID != "boot-1" || resolver.request.ConnectionTestID != grant.ConnectionTestID || resolver.request.LeaseID != grant.LeaseID || resolver.request.BindingDigest != grant.BindingDigest {
		t.Fatalf("槽位请求未绑定当前租约: %#v", resolver.request)
	}
	if !allZeroBytes(username) || !allZeroBytes(password) {
		t.Fatal("JDBC 运行器返回后仍保留短时槽位秘密")
	}
	if _, err := os.Stat(filepath.Join(root, "executions", "connection-test-"+grant.ConnectionTestID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("JDBC 工作区未清理: %v", err)
	}
}

func TestJDBCConnectionTestRunner连接失败与不可用均为固定结果(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		probeError error
		status     agentwire.DataSourceConnectionTestStatus
		code       string
	}{
		{name: "连接拒绝", probeError: jdbcprobe.ErrConnectionFailed, status: agentwire.DataSourceConnectionTestFailed, code: "DATABASE_CONNECTION_FAILED"},
		{name: "运行时不可用", probeError: jdbcprobe.ErrProbeFailed, status: agentwire.DataSourceConnectionTestUnknown, code: "DATABASE_CONNECTION_UNAVAILABLE"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runner := jdbcConnectionTestRunner{
				resolver:      &jdbcConnectionResolverStub{slot: agentwire.DataSourceConnectionTestDatabaseConnectionSlot{Host: "synthetic.example", Port: 2881, Username: []byte("user"), Password: []byte("password")}},
				workspaceRoot: t.TempDir(), bootID: "boot-1",
				dial: (&jdbcConnectionDialStub{}).dial,
				probe: func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.Request) (jdbcprobe.Result, error) {
					return jdbcprobe.Result{}, testCase.probeError
				},
			}
			outcome := runner.RunConnectionTest(context.Background(), jdbcConnectionTestGrant())
			if outcome.Status != testCase.status || outcome.EvidenceCode != testCase.code || outcome.VerificationSource != agentwire.DataSourceConnectionTestAgentJDBC {
				t.Fatalf("JDBC %s 结果 = %#v", testCase.name, outcome)
			}
		})
	}
}

func TestJDBCConnectionTestRunnerTCP失败时不启动JDBC(t *testing.T) {
	for _, testCase := range []struct {
		name string
		err  error
		code string
	}{
		{name: "连接拒绝", err: syscall.ECONNREFUSED, code: "DATABASE_TCP_REFUSED"},
		{name: "主机不能解析", err: &net.DNSError{IsNotFound: true}, code: "DATABASE_HOST_UNRESOLVABLE"},
		{name: "连接超时", err: syntheticTimeoutError{}, code: "DATABASE_TCP_TIMEOUT"},
		{name: "其他网络错误", err: errors.New("synthetic TCP failure"), code: "DATABASE_TCP_UNREACHABLE"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dial := &jdbcConnectionDialStub{err: testCase.err}
			probeCalled := false
			runner := jdbcConnectionTestRunner{
				resolver:      &jdbcConnectionResolverStub{slot: agentwire.DataSourceConnectionTestDatabaseConnectionSlot{Host: "synthetic.example", Port: 2881, Username: []byte("user"), Password: []byte("password")}},
				workspaceRoot: t.TempDir(), bootID: "boot-1", dial: dial.dial,
				probe: func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.Request) (jdbcprobe.Result, error) {
					probeCalled = true
					return jdbcprobe.Result{}, nil
				},
			}
			outcome := runner.RunConnectionTest(context.Background(), jdbcConnectionTestGrant())
			if !dial.called || probeCalled {
				t.Fatalf("TCP 失败时调用顺序错误: dial=%t probe=%t", dial.called, probeCalled)
			}
			if outcome.Status != agentwire.DataSourceConnectionTestFailed || outcome.EvidenceCode != testCase.code {
				t.Fatalf("TCP %s 结果 = %#v", testCase.name, outcome)
			}
		})
	}
}

type syntheticTimeoutError struct{}

func (syntheticTimeoutError) Error() string   { return "synthetic TCP timeout" }
func (syntheticTimeoutError) Timeout() bool   { return true }
func (syntheticTimeoutError) Temporary() bool { return true }

type jdbcConnectionResolverStub struct {
	slot    agentwire.DataSourceConnectionTestDatabaseConnectionSlot
	err     error
	request agentwire.DataSourceConnectionTestSecretSlotRequest
}

func (s *jdbcConnectionResolverStub) ResolveDataSourceConnectionTestDatabaseConnection(_ context.Context, request agentwire.DataSourceConnectionTestSecretSlotRequest) (agentwire.DataSourceConnectionTestDatabaseConnectionSlot, error) {
	s.request = request
	if s.err != nil {
		return agentwire.DataSourceConnectionTestDatabaseConnectionSlot{}, s.err
	}
	return s.slot, nil
}

func jdbcConnectionTestGrant() agentwire.DataSourceConnectionTestGrant {
	return agentwire.DataSourceConnectionTestGrant{
		ConnectionTestID: "connection-test-1", LeaseID: "lease-1", LeaseEpoch: 1,
		BindingDigest:      "e370fe7c62388ebc429070c9ee9e044da4c315a3c6de43f09e2aa57c70d6578d",
		VerificationSource: agentwire.DataSourceConnectionTestAgentJDBC,
	}
}

func allZeroBytes(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}
