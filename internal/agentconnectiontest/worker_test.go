package agentconnectiontest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ob-data-orch/internal/agentwire"
)

func TestRunNextG2合成连接测试仅确认并回写固定结果(t *testing.T) {
	now := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	protocol := &protocolStub{grant: testGrant(now, agentwire.DataSourceConnectionTestG2Synthetic)}
	worker := testWorker(protocol, fixedClock(now))

	outcome, found, err := worker.RunNext(context.Background())
	if err != nil || !found {
		t.Fatalf("RunNext() found=%t error=%v", found, err)
	}
	if outcome.Status != agentwire.DataSourceConnectionTestSucceeded || outcome.EvidenceCode != "SYNTHETIC_OK" || outcome.VerificationSource != agentwire.DataSourceConnectionTestG2Synthetic {
		t.Fatalf("G2 合成结果 = %#v", outcome)
	}
	if want := []string{"claim", "acknowledge", "complete"}; !sameStrings(protocol.callNames(), want) {
		t.Fatalf("协议调用 = %#v，期望 %#v", protocol.callNames(), want)
	}
	completion := protocol.completion()
	if completion.Status != agentwire.DataSourceConnectionTestSucceeded || completion.EvidenceCode != "SYNTHETIC_OK" || completion.VerificationSource != agentwire.DataSourceConnectionTestG2Synthetic || completion.SysVerificationStatus != agentwire.DataSourceConnectionTestSysNotConfigured {
		t.Fatalf("G2 完成输入 = %#v", completion)
	}
}

func TestRunNext拒绝AgentJDBC租约且不确认或完成(t *testing.T) {
	now := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	protocol := &protocolStub{grant: testGrant(now, agentwire.DataSourceConnectionTestAgentJDBC)}
	worker := testWorker(protocol, fixedClock(now))

	_, found, err := worker.RunNext(context.Background())
	if !found || !errors.Is(err, ErrGrantRejected) {
		t.Fatalf("RunNext() found=%t error=%v", found, err)
	}
	if want := []string{"claim"}; !sameStrings(protocol.callNames(), want) {
		t.Fatalf("AGENT_JDBC 租约错误触发了后续动作: %#v", protocol.callNames())
	}
}

func TestRunNext允许显式JDBC运行器回写固定结果(t *testing.T) {
	now := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	protocol := &protocolStub{grant: testGrant(now, agentwire.DataSourceConnectionTestAgentJDBC)}
	worker := testWorker(protocol, fixedClock(now))
	worker.JDBCRunner = jdbcRunnerStub{}

	outcome, found, err := worker.RunNext(context.Background())
	if err != nil || !found {
		t.Fatalf("RunNext() found=%t error=%v", found, err)
	}
	if outcome.Status != agentwire.DataSourceConnectionTestSucceeded || outcome.EvidenceCode != "DATABASE_CONNECTED" || outcome.VerificationSource != agentwire.DataSourceConnectionTestAgentJDBC {
		t.Fatalf("JDBC 结果 = %#v", outcome)
	}
	if want := []string{"claim", "acknowledge", "complete"}; !sameStrings(protocol.callNames(), want) {
		t.Fatalf("JDBC 协议调用 = %#v，期望 %#v", protocol.callNames(), want)
	}
}

func TestRunNext在确认前拒绝漂移绑定(t *testing.T) {
	now := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	grant := testGrant(now, agentwire.DataSourceConnectionTestG2Synthetic)
	grant.Binding.NodeID = "node-other"
	protocol := &protocolStub{grant: grant}
	worker := testWorker(protocol, fixedClock(now))

	_, found, err := worker.RunNext(context.Background())
	if !found || !errors.Is(err, ErrGrantRejected) {
		t.Fatalf("RunNext() found=%t error=%v", found, err)
	}
	if want := []string{"claim"}; !sameStrings(protocol.callNames(), want) {
		t.Fatalf("漂移绑定后协议调用 = %#v，期望 %#v", protocol.callNames(), want)
	}
}

func TestRunNext确认后租约过期不完成(t *testing.T) {
	base := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	protocol := &protocolStub{grant: testGrant(base, agentwire.DataSourceConnectionTestG2Synthetic)}
	worker := testWorker(protocol, sequenceClock(base, base, base.Add(2*time.Minute)))

	_, found, err := worker.RunNext(context.Background())
	if !found || !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("RunNext() found=%t error=%v", found, err)
	}
	if want := []string{"claim", "acknowledge"}; !sameStrings(protocol.callNames(), want) {
		t.Fatalf("过期租约协议调用 = %#v，期望 %#v", protocol.callNames(), want)
	}
}

func TestRunNext同一Worker拒绝并发领取(t *testing.T) {
	now := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	protocol := &protocolStub{
		grant:        testGrant(now, agentwire.DataSourceConnectionTestG2Synthetic),
		claimStarted: make(chan struct{}), claimRelease: make(chan struct{}),
	}
	worker := testWorker(protocol, fixedClock(now))
	firstDone := make(chan error, 1)
	go func() {
		_, _, err := worker.RunNext(context.Background())
		firstDone <- err
	}()
	<-protocol.claimStarted
	if _, _, err := worker.RunNext(context.Background()); !errors.Is(err, ErrWorkerBusy) {
		t.Fatalf("第二次 RunNext() error=%v，期望 ErrWorkerBusy", err)
	}
	close(protocol.claimRelease)
	if err := <-firstDone; err != nil {
		t.Fatalf("首次 RunNext() error=%v", err)
	}
	if protocol.claimCount() != 1 {
		t.Fatalf("领取次数 = %d，期望 1", protocol.claimCount())
	}
}

func testWorker(protocol Protocol, clock func() time.Time) *Worker {
	return &Worker{Protocol: protocol, Clock: clock, BootID: "boot-1"}
}

func testGrant(now time.Time, source agentwire.DataSourceConnectionTestVerificationSource) agentwire.DataSourceConnectionTestGrant {
	return agentwire.DataSourceConnectionTestGrant{
		ConnectionTestID: "connection-test-1", LeaseID: "lease-1", LeaseEpoch: 3, ExpiresAt: now.Add(time.Minute),
		Binding: agentwire.DataSourceConnectionTestBinding{
			ConnectionTestID: "connection-test-1", DataSourceID: "source-1",
			ConnectionConfigDigest: "7e5bb836b6beeb4dc8115f2112be5ad1b917b7f6a5edffd7571e845be7b3a3a0",
			CredentialRevision:     4, NodeID: "node-1", NodeFactsRevision: 5,
		},
		BindingDigest: "e370fe7c62388ebc429070c9ee9e044da4c315a3c6de43f09e2aa57c70d6578d", VerificationSource: source,
	}
}

type protocolStub struct {
	mu sync.Mutex

	grant          agentwire.DataSourceConnectionTestGrant
	claimErr       error
	acknowledgeErr error
	completeErr    error
	noWork         bool

	completionInput agentwire.DataSourceConnectionTestCompletion
	calls           []string
	claimStarted    chan struct{}
	claimRelease    chan struct{}
}

type jdbcRunnerStub struct {
	sysStatus agentwire.DataSourceConnectionTestSysVerificationStatus
	sysCode   string
}

func (j jdbcRunnerStub) RunConnectionTest(_ context.Context, grant agentwire.DataSourceConnectionTestGrant) Outcome {
	outcome := Outcome{ConnectionTestID: grant.ConnectionTestID, Status: agentwire.DataSourceConnectionTestSucceeded, EvidenceCode: "DATABASE_CONNECTED", VerificationSource: agentwire.DataSourceConnectionTestAgentJDBC, SysVerificationStatus: agentwire.DataSourceConnectionTestSysNotConfigured}
	if grant.Binding.SysCredentialRevision > 0 {
		outcome.SysVerificationStatus = j.sysStatus
		outcome.SysEvidenceCode = j.sysCode
	}
	return outcome
}

func (p *protocolStub) AgentIdentity() (agentwire.AgentIdentity, error) {
	return agentwire.AgentIdentity{AgentID: "agent-1", NodeID: "node-1"}, nil
}

func (p *protocolStub) ClaimNextDataSourceConnectionTest(_ context.Context, _ agentwire.DataSourceConnectionTestClaimNext) (agentwire.DataSourceConnectionTestGrant, bool, error) {
	p.mu.Lock()
	p.calls = append(p.calls, "claim")
	started, release, err, noWork := p.claimStarted, p.claimRelease, p.claimErr, p.noWork
	p.mu.Unlock()
	if started != nil {
		close(started)
	}
	if release != nil {
		<-release
	}
	if err != nil {
		return agentwire.DataSourceConnectionTestGrant{}, false, err
	}
	if noWork {
		return agentwire.DataSourceConnectionTestGrant{}, false, nil
	}
	return p.grant, true, nil
}

func (p *protocolStub) AcknowledgeDataSourceConnectionTestLease(_ context.Context, _ agentwire.DataSourceConnectionTestLeaseAcknowledgement) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "acknowledge")
	return p.acknowledgeErr
}

func (p *protocolStub) CompleteDataSourceConnectionTest(_ context.Context, input agentwire.DataSourceConnectionTestCompletion) (agentwire.DataSourceConnectionTestStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "complete")
	p.completionInput = input
	if p.completeErr != nil {
		return "", p.completeErr
	}
	return input.Status, nil
}

func (p *protocolStub) callNames() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.calls...)
}

func (p *protocolStub) claimCount() int {
	count := 0
	for _, call := range p.callNames() {
		if call == "claim" {
			count++
		}
	}
	return count
}

func (p *protocolStub) completion() agentwire.DataSourceConnectionTestCompletion {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.completionInput
}

func fixedClock(now time.Time) func() time.Time {
	return func() time.Time { return now }
}

func sequenceClock(values ...time.Time) func() time.Time {
	var mutex sync.Mutex
	index := 0
	return func() time.Time {
		mutex.Lock()
		defer mutex.Unlock()
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
