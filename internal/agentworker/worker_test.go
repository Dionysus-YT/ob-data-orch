package agentworker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/credential"
)

func TestRunNextAcknowledgesBeforeSecretResolutionAndCompletes(t *testing.T) {
	t.Parallel()
	clock := newTestClock(time.Date(2026, time.July, 27, 9, 0, 0, 0, time.UTC))
	protocol := &protocolStub{
		grant: testGrant(clock.Now()),
		slot: agentwire.DatabaseConnectionSlot{
			Host:     "odp.internal.test",
			Port:     2883,
			Username: []byte("synthetic-user"),
			Password: []byte("synthetic-password"),
		},
	}
	var resolver SecretResolver
	var factoryResolveErr error
	worker := testWorker(protocol, clock, func(input SecretResolver) agentpreflight.Probe {
		resolver = input
		_, factoryResolveErr = input.ResolveDatabaseConnection(context.Background(), protocol.grant.Binding)
		return probeFunc(func(ctx context.Context, check agentpreflight.CheckID, request agentpreflight.Request) (agentpreflight.Result, error) {
			if check == agentpreflight.CheckDatabaseConnectivity {
				connection, err := input.ResolveDatabaseConnection(ctx, request.Binding)
				if err != nil {
					return agentpreflight.Result{}, err
				}
				defer credential.Zero(connection.Username)
				defer credential.Zero(connection.Password)
				return agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: "DATABASE_CONNECTED"}, nil
			}
			return syntheticPassed(check), nil
		})
	})

	outcome, err := runClaimed(context.Background(), worker)
	if err != nil {
		t.Fatalf("RunNext() error = %v", err)
	}
	if outcome.State != agentwire.PrecheckSucceeded || !outcome.Report.Succeeded {
		t.Fatalf("RunNext() outcome = %#v, want successful report", outcome)
	}
	if want := []string{"claim", "acknowledge", "resolve", "complete"}; !sameStrings(protocol.callNames(), want) {
		t.Fatalf("protocol calls = %v, want %v", protocol.callNames(), want)
	}
	if protocol.resolveRequest.BootID != "boot-1" || protocol.resolveRequest.PrecheckID != "precheck-1" || protocol.resolveRequest.LeaseID != protocol.grant.LeaseID || protocol.resolveRequest.LeaseEpoch != 3 || protocol.resolveRequest.BindingDigest != "binding-digest-1" {
		t.Fatalf("resolve request = %#v, want current precheck lease binding", protocol.resolveRequest)
	}
	if !allZero(protocol.slot.Username) || !allZero(protocol.slot.Password) {
		t.Fatal("short-lived connection bytes were not cleared after the probe")
	}
	if resolver == nil {
		t.Fatal("ProbeFactory did not receive a resolver")
	}
	if !errors.Is(factoryResolveErr, ErrSecretResolution) {
		t.Fatalf("ProbeFactory 提前解析错误 = %v，期望 %v", factoryResolveErr, ErrSecretResolution)
	}
	if _, err := resolver.ResolveDatabaseConnection(context.Background(), protocol.grant.Binding); !errors.Is(err, ErrSecretResolution) {
		t.Fatalf("resolver after RunNext error = %v, want ErrSecretResolution", err)
	}
	if got := protocol.resolveCount(); got != 1 {
		t.Fatalf("resolve calls after closed resolver = %d, want 1", got)
	}
}

func TestRunNextDoesNotResolveSecretWhenLocalPrecheckFails(t *testing.T) {
	t.Parallel()
	clock := newTestClock(time.Date(2026, time.July, 27, 9, 0, 0, 0, time.UTC))
	protocol := &protocolStub{grant: testGrant(clock.Now())}
	var factoryResolveErr error
	var checks []agentpreflight.CheckID
	worker := testWorker(protocol, clock, func(resolver SecretResolver) agentpreflight.Probe {
		_, factoryResolveErr = resolver.ResolveDatabaseConnection(context.Background(), protocol.grant.Binding)
		return probeFunc(func(_ context.Context, check agentpreflight.CheckID, _ agentpreflight.Request) (agentpreflight.Result, error) {
			checks = append(checks, check)
			if check == agentpreflight.CheckToolEnvironment {
				return agentpreflight.Result{Check: check, Status: agentpreflight.StatusFailed, EvidenceCode: "TOOL_RUNTIME_INVALID"}, nil
			}
			return syntheticPassed(check), nil
		})
	})

	outcome, err := runClaimed(context.Background(), worker)
	if err != nil || outcome.State != agentwire.PrecheckFailed || outcome.Report.Succeeded {
		t.Fatalf("RunNext() = %#v, %v", outcome, err)
	}
	if !errors.Is(factoryResolveErr, ErrSecretResolution) {
		t.Fatalf("ProbeFactory 提前解析错误 = %v，期望 %v", factoryResolveErr, ErrSecretResolution)
	}
	if want := []agentpreflight.CheckID{agentpreflight.CheckToolEnvironment, agentpreflight.CheckOutputPath, agentpreflight.CheckOutputEmpty, agentpreflight.CheckAvailableSpace}; !sameChecks(checks, want) {
		t.Fatalf("本机前置检查 = %#v，期望 %#v", checks, want)
	}
	if protocol.resolveCount() != 0 {
		t.Fatalf("本机前置失败仍解析槽位 %d 次", protocol.resolveCount())
	}
	if want := []string{"claim", "acknowledge", "complete"}; !sameStrings(protocol.callNames(), want) {
		t.Fatalf("protocol calls = %v, want %v", protocol.callNames(), want)
	}
	if database := outcome.Report.Results[0]; database.Status != agentpreflight.StatusUnknown || database.EvidenceCode != "DATABASE_CONNECTION_UNAVAILABLE" {
		t.Fatalf("数据库结果 = %#v，期望未解析的未知结果", database)
	}
	if object := outcome.Report.Results[1]; object.Status != agentpreflight.StatusUnknown || object.EvidenceCode != "OBJECT_ACCESS_UNAVAILABLE" {
		t.Fatalf("对象结果 = %#v，期望未解析的未知结果", object)
	}
}

func TestRunNextDoesNotAllowObjectCheckToResolveSecret(t *testing.T) {
	t.Parallel()
	clock := newTestClock(time.Date(2026, time.July, 27, 9, 0, 0, 0, time.UTC))
	protocol := &protocolStub{grant: testGrant(clock.Now())}
	var objectResolveErr error
	worker := testWorker(protocol, clock, func(resolver SecretResolver) agentpreflight.Probe {
		return probeFunc(func(ctx context.Context, check agentpreflight.CheckID, request agentpreflight.Request) (agentpreflight.Result, error) {
			switch check {
			case agentpreflight.CheckDatabaseConnectivity:
				return agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: "DATABASE_CONNECTED"}, nil
			case agentpreflight.CheckObjectAccess:
				_, objectResolveErr = resolver.ResolveDatabaseConnection(ctx, request.Binding)
				return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: "OBJECT_ACCESS_UNAVAILABLE"}, nil
			default:
				return syntheticPassed(check), nil
			}
		})
	})

	outcome, err := runClaimed(context.Background(), worker)
	if err != nil || outcome.State != agentwire.PrecheckFailed || outcome.Report.Succeeded {
		t.Fatalf("RunNext() = %#v, %v", outcome, err)
	}
	if !errors.Is(objectResolveErr, ErrSecretResolution) {
		t.Fatalf("对象检查解析错误 = %v，期望 %v", objectResolveErr, ErrSecretResolution)
	}
	if protocol.resolveCount() != 0 {
		t.Fatalf("对象检查错误解析槽位 %d 次", protocol.resolveCount())
	}
	if want := []string{"claim", "acknowledge", "complete"}; !sameStrings(protocol.callNames(), want) {
		t.Fatalf("protocol calls = %v, want %v", protocol.callNames(), want)
	}
}

func TestRunNextReturnsNoWorkWithoutCreatingProbeOrResolvingSecret(t *testing.T) {
	t.Parallel()
	clock := newTestClock(time.Date(2026, time.July, 27, 9, 0, 0, 0, time.UTC))
	protocol := &protocolStub{noWork: true}
	factoryCalls := 0
	worker := testWorker(protocol, clock, func(SecretResolver) agentpreflight.Probe {
		factoryCalls++
		return passedProbe{}
	})

	outcome, found, err := worker.RunNext(context.Background())
	if err != nil || found || outcome.State != "" || outcome.Report.PrecheckID != "" || len(outcome.Report.Results) != 0 {
		t.Fatalf("RunNext() = %#v, %t, %v; want zero outcome, false, nil", outcome, found, err)
	}
	if factoryCalls != 0 {
		t.Fatalf("ProbeFactory calls = %d, want 0", factoryCalls)
	}
	if want := []string{"claim"}; !sameStrings(protocol.callNames(), want) {
		t.Fatalf("protocol calls = %v, want %v", protocol.callNames(), want)
	}
}

func TestRunNextRejectsGrantBeforeAcknowledgement(t *testing.T) {
	t.Parallel()
	clock := newTestClock(time.Date(2026, time.July, 27, 9, 0, 0, 0, time.UTC))
	grant := testGrant(clock.Now())
	grant.CheckSet[0] = "UNSUPPORTED"
	protocol := &protocolStub{grant: grant}
	factoryCalls := 0
	worker := testWorker(protocol, clock, func(SecretResolver) agentpreflight.Probe {
		factoryCalls++
		return passedProbe{}
	})

	_, err := runClaimed(context.Background(), worker)
	if !errors.Is(err, ErrGrantRejected) {
		t.Fatalf("RunNext() error = %v, want ErrGrantRejected", err)
	}
	if want := []string{"claim"}; !sameStrings(protocol.callNames(), want) {
		t.Fatalf("protocol calls = %v, want %v", protocol.callNames(), want)
	}
	if factoryCalls != 0 {
		t.Fatalf("ProbeFactory calls = %d, want 0", factoryCalls)
	}
}

func TestRunNextRejectsForeignNodeAndPlatformBeforeAcknowledgement(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*agentwire.PrecheckGrant)
	}{
		{name: "foreign node", mutate: func(grant *agentwire.PrecheckGrant) { grant.Binding.NodeID = "node-2" }},
		{name: "foreign platform", mutate: func(grant *agentwire.PrecheckGrant) { grant.Context.TargetPlatform = commandgen.PlatformLinuxAMD64 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clock := newTestClock(time.Date(2026, time.July, 27, 9, 0, 0, 0, time.UTC))
			grant := testGrant(clock.Now())
			test.mutate(&grant)
			protocol := &protocolStub{grant: grant}
			worker := testWorker(protocol, clock, func(SecretResolver) agentpreflight.Probe { return passedProbe{} })
			if _, err := runClaimed(context.Background(), worker); !errors.Is(err, ErrGrantRejected) {
				t.Fatalf("RunNext() error = %v", err)
			}
			if want := []string{"claim"}; !sameStrings(protocol.callNames(), want) {
				t.Fatalf("protocol calls = %v, want %v", protocol.callNames(), want)
			}
		})
	}
}

func TestRunNextDoesNotProbeAfterAcknowledgementFailure(t *testing.T) {
	t.Parallel()
	clock := newTestClock(time.Date(2026, time.July, 27, 9, 0, 0, 0, time.UTC))
	protocol := &protocolStub{grant: testGrant(clock.Now()), acknowledgeErr: errors.New("synthetic acknowledge failure")}
	factoryCalls := 0
	worker := testWorker(protocol, clock, func(SecretResolver) agentpreflight.Probe {
		factoryCalls++
		return passedProbe{}
	})

	_, err := runClaimed(context.Background(), worker)
	if !errors.Is(err, ErrLeaseAcknowledgement) {
		t.Fatalf("RunNext() error = %v, want ErrLeaseAcknowledgement", err)
	}
	if want := []string{"claim", "acknowledge"}; !sameStrings(protocol.callNames(), want) {
		t.Fatalf("protocol calls = %v, want %v", protocol.callNames(), want)
	}
	if factoryCalls != 0 || protocol.resolveCount() != 0 {
		t.Fatalf("ProbeFactory/resolve calls = %d/%d, want 0/0", factoryCalls, protocol.resolveCount())
	}
}

func TestRunNextCompletesWhenSecretSlotIsUnavailable(t *testing.T) {
	clock := newTestClock(time.Date(2026, time.July, 27, 9, 0, 0, 0, time.UTC))
	protocol := &protocolStub{grant: testGrant(clock.Now())}
	worker := testWorker(protocol, clock, func(resolver SecretResolver) agentpreflight.Probe {
		return probeFunc(func(ctx context.Context, check agentpreflight.CheckID, request agentpreflight.Request) (agentpreflight.Result, error) {
			if check == agentpreflight.CheckDatabaseConnectivity {
				if _, err := resolver.ResolveDatabaseConnection(ctx, request.Binding); !errors.Is(err, ErrSecretResolution) {
					return agentpreflight.Result{}, errors.New("unavailable slot unexpectedly resolved")
				}
				return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: "DATABASE_CONNECTION_UNAVAILABLE"}, nil
			}
			return syntheticPassed(check), nil
		})
	})
	outcome, err := runClaimed(context.Background(), worker)
	if err != nil || outcome.State != agentwire.PrecheckFailed || outcome.Report.Succeeded {
		t.Fatalf("RunNext() = %#v, %v", outcome, err)
	}
	if want := []string{"claim", "acknowledge", "resolve", "complete"}; !sameStrings(protocol.callNames(), want) {
		t.Fatalf("protocol calls = %v, want %v", protocol.callNames(), want)
	}
	if len(outcome.Report.Results) != len(agentpreflight.FixedChecks()) || outcome.Report.Results[0].EvidenceCode != "DATABASE_CONNECTION_UNAVAILABLE" {
		t.Fatalf("slot failure did not complete a full safe report: %#v", outcome.Report)
	}
}

func TestRunNextStopsWhenLeaseExpiresAfterAcknowledgement(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, time.July, 27, 9, 0, 0, 0, time.UTC)
	clock := newSequenceClock(base, base, base, base.Add(2*time.Minute))
	protocol := &protocolStub{grant: testGrant(base)}
	factoryCalls := 0
	worker := testWorker(protocol, clock, func(SecretResolver) agentpreflight.Probe {
		factoryCalls++
		return passedProbe{}
	})

	_, err := runClaimed(context.Background(), worker)
	if !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("RunNext() error = %v, want ErrLeaseExpired", err)
	}
	if want := []string{"claim", "acknowledge"}; !sameStrings(protocol.callNames(), want) {
		t.Fatalf("protocol calls = %v, want %v", protocol.callNames(), want)
	}
	if factoryCalls != 0 || protocol.resolveCount() != 0 {
		t.Fatalf("ProbeFactory/resolve calls = %d/%d, want 0/0", factoryCalls, protocol.resolveCount())
	}
}

func TestRunNextCompletesWithFailureReportForInvalidProbe(t *testing.T) {
	t.Parallel()
	clock := newTestClock(time.Date(2026, time.July, 27, 9, 0, 0, 0, time.UTC))
	protocol := &protocolStub{grant: testGrant(clock.Now())}
	worker := testWorker(protocol, clock, func(SecretResolver) agentpreflight.Probe {
		return probeFunc(func(_ context.Context, check agentpreflight.CheckID, _ agentpreflight.Request) (agentpreflight.Result, error) {
			return agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: "untrusted-evidence"}, nil
		})
	})

	outcome, err := runClaimed(context.Background(), worker)
	if err != nil || outcome.State != agentwire.PrecheckFailed || outcome.Report.Succeeded {
		t.Fatalf("RunNext() = %#v, %v", outcome, err)
	}
	if want := []string{"claim", "acknowledge", "complete"}; !sameStrings(protocol.callNames(), want) {
		t.Fatalf("protocol calls = %v, want %v", protocol.callNames(), want)
	}
	if len(outcome.Report.Results) != len(agentpreflight.FixedChecks()) || outcome.Report.Results[0].EvidenceCode != "DATABASE_CONNECTION_UNAVAILABLE" {
		t.Fatalf("无效 Probe 没有转换为完整失败关闭报告: %#v", outcome.Report)
	}
}

func TestRunNextBindsResolverToCurrentGrant(t *testing.T) {
	t.Parallel()
	clock := newTestClock(time.Date(2026, time.July, 27, 9, 0, 0, 0, time.UTC))
	protocol := &protocolStub{grant: testGrant(clock.Now())}
	worker := testWorker(protocol, clock, func(resolver SecretResolver) agentpreflight.Probe {
		return probeFunc(func(ctx context.Context, check agentpreflight.CheckID, request agentpreflight.Request) (agentpreflight.Result, error) {
			if check == agentpreflight.CheckDatabaseConnectivity {
				binding := request.Binding
				binding.CredentialRevision++
				if _, err := resolver.ResolveDatabaseConnection(ctx, binding); !errors.Is(err, ErrSecretResolution) {
					return agentpreflight.Result{}, errors.New("mismatched binding unexpectedly resolved")
				}
				return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: "DATABASE_CONNECTION_UNAVAILABLE"}, nil
			}
			return syntheticPassed(check), nil
		})
	})

	outcome, err := runClaimed(context.Background(), worker)
	if err != nil {
		t.Fatalf("RunNext() error = %v", err)
	}
	if outcome.State != agentwire.PrecheckFailed || outcome.Report.Succeeded {
		t.Fatalf("RunNext() outcome = %#v, want failed report", outcome)
	}
	if want := []string{"claim", "acknowledge", "complete"}; !sameStrings(protocol.callNames(), want) {
		t.Fatalf("protocol calls = %v, want %v", protocol.callNames(), want)
	}
}

func TestRunNextDoesNotRetryAfterCompletionFailure(t *testing.T) {
	t.Parallel()
	clock := newTestClock(time.Date(2026, time.July, 27, 9, 0, 0, 0, time.UTC))
	protocol := &protocolStub{grant: testGrant(clock.Now()), completeErr: errors.New("synthetic completion failure")}
	probeCalls := 0
	worker := testWorker(protocol, clock, func(SecretResolver) agentpreflight.Probe {
		return probeFunc(func(_ context.Context, check agentpreflight.CheckID, _ agentpreflight.Request) (agentpreflight.Result, error) {
			probeCalls++
			return syntheticPassed(check), nil
		})
	})

	_, err := runClaimed(context.Background(), worker)
	if !errors.Is(err, ErrCompletionRejected) {
		t.Fatalf("RunNext() error = %v, want ErrCompletionRejected", err)
	}
	if want := []string{"claim", "acknowledge", "complete"}; !sameStrings(protocol.callNames(), want) {
		t.Fatalf("protocol calls = %v, want %v", protocol.callNames(), want)
	}
	if probeCalls != len(agentpreflight.FixedChecks()) {
		t.Fatalf("Probe calls = %d, want %d", probeCalls, len(agentpreflight.FixedChecks()))
	}
}

func TestRunNextAllowsOnlyOneConcurrentClaim(t *testing.T) {
	t.Parallel()
	clock := newTestClock(time.Date(2026, time.July, 27, 9, 0, 0, 0, time.UTC))
	protocol := &protocolStub{
		grant:        testGrant(clock.Now()),
		claimStarted: make(chan struct{}),
		claimRelease: make(chan struct{}),
	}
	worker := testWorker(protocol, clock, func(SecretResolver) agentpreflight.Probe { return passedProbe{} })
	firstDone := make(chan error, 1)
	go func() {
		_, err := runClaimed(context.Background(), worker)
		firstDone <- err
	}()
	<-protocol.claimStarted

	_, err := runClaimed(context.Background(), worker)
	if !errors.Is(err, ErrWorkerBusy) {
		t.Fatalf("second RunNext() error = %v, want ErrWorkerBusy", err)
	}
	close(protocol.claimRelease)
	if err := <-firstDone; err != nil {
		t.Fatalf("first RunNext() error = %v", err)
	}
	if got := protocol.claimCount(); got != 1 {
		t.Fatalf("claim calls = %d, want 1", got)
	}
}

func testWorker(protocol Protocol, clock interface{ Now() time.Time }, factory ProbeFactory) *Worker {
	return &Worker{
		Protocol:      protocol,
		ProbeFactory:  factory,
		Clock:         clock.Now,
		BootID:        "boot-1",
		LocalPlatform: commandgen.PlatformWindowsAMD64,
	}
}

var errUnexpectedNoWork = errors.New("测试预期可领取预检查")

func runClaimed(ctx context.Context, worker *Worker) (Outcome, error) {
	outcome, found, err := worker.RunNext(ctx)
	if err == nil && !found {
		return Outcome{}, errUnexpectedNoWork
	}
	return outcome, err
}

func testGrant(now time.Time) agentwire.PrecheckGrant {
	return agentwire.PrecheckGrant{
		PrecheckID: "precheck-1",
		LeaseID:    "lease-1",
		LeaseEpoch: 3,
		ExpiresAt:  now.Add(time.Minute),
		Binding: agentstate.PrecheckBinding{
			PrecheckID:         "precheck-1",
			NodeID:             "node-1",
			DraftRevision:      2,
			ConfigFingerprint:  "config-fingerprint-1",
			CredentialRevision: 4,
			NodeFactsVersion:   5,
		},
		BindingDigest: "binding-digest-1",
		CheckSet:      agentpreflight.FixedChecks(),
		Context: agentwire.PrecheckExecutionContext{
			CompatibilityMode: "MYSQL",
			Database:          "test_database",
			Table:             "test_table",
			OutputPath:        "/E:/approved-output",
			TargetPlatform:    commandgen.PlatformWindowsAMD64,
			AllowedRoots:      []string{"E:\\approved-output"},
		},
	}
}

func syntheticPassed(check agentpreflight.CheckID) agentpreflight.Result {
	if check == agentpreflight.CheckDatabaseConnectivity {
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: "DATABASE_CONNECTED"}
	}
	return agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: "SYNTHETIC_OK"}
}

type probeFunc func(context.Context, agentpreflight.CheckID, agentpreflight.Request) (agentpreflight.Result, error)

func (f probeFunc) Probe(ctx context.Context, check agentpreflight.CheckID, request agentpreflight.Request) (agentpreflight.Result, error) {
	return f(ctx, check, request)
}

type passedProbe struct{}

func (passedProbe) Probe(_ context.Context, check agentpreflight.CheckID, _ agentpreflight.Request) (agentpreflight.Result, error) {
	return syntheticPassed(check), nil
}

type protocolStub struct {
	mu sync.Mutex

	grant          agentwire.PrecheckGrant
	slot           agentwire.DatabaseConnectionSlot
	acknowledgeErr error
	completeErr    error

	resolveRequest agentwire.PrecheckSecretSlotRequest
	calls          []string

	claimStarted chan struct{}
	claimRelease chan struct{}
	noWork       bool
}

func (p *protocolStub) AgentIdentity() (agentwire.AgentIdentity, error) {
	return agentwire.AgentIdentity{AgentID: "agent-1", NodeID: "node-1"}, nil
}

func (p *protocolStub) ClaimNextPrecheck(_ context.Context, _ agentwire.PrecheckClaimNext) (agentwire.PrecheckGrant, bool, error) {
	p.mu.Lock()
	p.calls = append(p.calls, "claim")
	started := p.claimStarted
	release := p.claimRelease
	noWork := p.noWork
	p.mu.Unlock()
	if started != nil {
		close(started)
	}
	if release != nil {
		<-release
	}
	if noWork {
		return agentwire.PrecheckGrant{}, false, nil
	}
	return p.grant, true, nil
}

func (p *protocolStub) AcknowledgePrecheckLease(_ context.Context, _ agentwire.PrecheckLeaseAcknowledgement) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "acknowledge")
	return p.acknowledgeErr
}

func (p *protocolStub) ResolvePrecheckDatabaseConnection(_ context.Context, request agentwire.PrecheckSecretSlotRequest) (agentwire.DatabaseConnectionSlot, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "resolve")
	p.resolveRequest = request
	return p.slot, nil
}

func (p *protocolStub) CompletePrecheck(_ context.Context, input agentwire.PrecheckCompletion) (agentwire.PrecheckState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "complete")
	if p.completeErr != nil {
		return "", p.completeErr
	}
	if input.Report.Succeeded {
		return agentwire.PrecheckSucceeded, nil
	}
	return agentwire.PrecheckFailed, nil
}

func (p *protocolStub) callNames() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.calls...)
}

func (p *protocolStub) resolveCount() int {
	count := 0
	for _, call := range p.callNames() {
		if call == "resolve" {
			count++
		}
	}
	return count
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

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock(now time.Time) *testClock {
	return &testClock{now: now}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

type sequenceClock struct {
	mu     sync.Mutex
	values []time.Time
	index  int
}

func newSequenceClock(values ...time.Time) *sequenceClock {
	return &sequenceClock{values: append([]time.Time(nil), values...)}
}

func (c *sequenceClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.index >= len(c.values) {
		return c.values[len(c.values)-1]
	}
	value := c.values[c.index]
	c.index++
	return value
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

func sameChecks(left, right []agentpreflight.CheckID) bool {
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

func allZero(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}
