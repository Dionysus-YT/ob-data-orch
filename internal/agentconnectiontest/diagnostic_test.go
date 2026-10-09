package agentconnectiontest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/agentwire"
)

func Test领取失败分类不泄漏原始错误并保留兼容判定(t *testing.T) {
	for _, test := range []struct {
		cause  error
		reason string
	}{
		{agentwire.ErrControlPlaneUnavailable, "CONTROL_PLANE_UNAVAILABLE"},
		{agentwire.ErrAgentAuthenticationDenied, "AUTHENTICATION_DENIED"},
		{agentwire.ErrProtocolRejected, "PROTOCOL_REJECTED"},
		{agentwire.ErrEnrollmentPending, "ENROLLMENT_PENDING"},
		{agentwire.ErrIdentityUnavailable, "IDENTITY_UNAVAILABLE"},
		{context.Canceled, "CANCELED"},
		{context.DeadlineExceeded, "TIMEOUT"},
		{errors.New("synthetic-private-marker"), "UNKNOWN"},
	} {
		protocol := &protocolStub{claimErr: fmt.Errorf("synthetic-private-marker: %w", test.cause)}
		worker := testWorker(protocol, fixedClock(time.Now()))
		_, found, err := worker.RunNext(context.Background())
		if found || !errors.Is(err, ErrGrantRejected) || !strings.Contains(err.Error(), test.reason) || strings.Contains(err.Error(), "synthetic-private-marker") {
			t.Fatalf("错误分类不符合边界：%s", test.reason)
		}
		if calls := protocol.callNames(); len(calls) != 1 || calls[0] != "claim" {
			t.Fatal("领取失败后继续执行")
		}
	}
}

// invalidOutcomeRunner 返回合成非法状态，确认结果校验错误不会被误报为领取失败。
type invalidOutcomeRunner struct{ jdbcRunnerStub }

func (invalidOutcomeRunner) RunConnectionTest(_ context.Context, grant agentwire.DataSourceConnectionTestGrant) Outcome {
	return Outcome{ConnectionTestID: grant.ConnectionTestID, VerificationSource: agentwire.DataSourceConnectionTestAgentJDBC, Status: "INVALID", EvidenceCode: "synthetic-private-marker", SysVerificationStatus: agentwire.DataSourceConnectionTestSysNotConfigured}
}

func Test租约和结果校验失败明确区分阶段(t *testing.T) {
	now := time.Now()
	grant := testGrant(now, agentwire.DataSourceConnectionTestAgentJDBC)
	protocol := &protocolStub{grant: grant}
	worker := testWorker(protocol, fixedClock(now))
	if _, found, err := worker.RunNext(context.Background()); !found || !errors.Is(err, ErrGrantRejected) || !strings.Contains(err.Error(), "GRANT_VALIDATION_FAILED") {
		t.Fatal("未配置运行器的错误阶段不明确")
	}
	if len(protocol.callNames()) != 1 {
		t.Fatal("租约未通过仍确认")
	}
	protocol = &protocolStub{grant: grant}
	worker = testWorker(protocol, fixedClock(now))
	worker.JDBCRunner = invalidOutcomeRunner{}
	if _, found, err := worker.RunNext(context.Background()); !found || !errors.Is(err, ErrGrantRejected) || !strings.Contains(err.Error(), "RESULT_VALIDATION_FAILED") || strings.Contains(err.Error(), "synthetic-private-marker") {
		t.Fatal("非法结果的错误阶段或日志边界不正确")
	}
	if calls := protocol.callNames(); len(calls) != 2 || calls[1] != "acknowledge" {
		t.Fatal("结果未通过仍上报")
	}
}
