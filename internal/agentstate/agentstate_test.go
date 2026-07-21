package agentstate

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func TestClaimIsIdempotentAndSingleTaskOnly(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)}
	coordinator := newCoordinator(t, clock)
	request := validClaim()
	if err := coordinator.Schedule(TaskSchedule{TaskID: request.TaskID, NodeID: request.NodeID}); err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	grant, err := coordinator.Claim(request)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	retry, err := coordinator.Claim(request)
	if err != nil || retry != grant {
		t.Fatalf("idempotent Claim() = %#v, %v; want %#v, nil", retry, err, grant)
	}
	changed := request
	changed.LeaseID = "lease-changed"
	if _, err := coordinator.Claim(changed); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed request ID reuse error = %v", err)
	}
	other := request
	other.RequestID, other.ExecutionID, other.LeaseID = "claim-2", "execution-2", "lease-2"
	if _, err := coordinator.Claim(other); !errors.Is(err, ErrTaskAlreadyClaimed) {
		t.Fatalf("second task claim error = %v", err)
	}
}

func TestConcurrentClaimOnlyCreatesOneExecution(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)}
	coordinator := newCoordinator(t, clock)
	requests := []ClaimRequest{validClaim(), validClaim()}
	if err := coordinator.Schedule(TaskSchedule{TaskID: requests[0].TaskID, NodeID: requests[0].NodeID}); err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	requests[1].RequestID, requests[1].ExecutionID, requests[1].LeaseID = "claim-other", "execution-other", "lease-other"
	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, request := range requests {
		go func(request ClaimRequest) {
			ready.Done()
			<-start
			_, err := coordinator.Claim(request)
			results <- err
		}(request)
	}
	ready.Wait()
	close(start)
	var success, rejected int
	for range requests {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, ErrTaskAlreadyClaimed) {
			rejected++
		} else {
			t.Fatalf("concurrent Claim() error = %v", err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("claims success=%d rejected=%d", success, rejected)
	}
}

func TestClaimRejectsTaskNotScheduledForTheRequestNode(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)}
	coordinator := newCoordinator(t, clock)
	request := validClaim()
	if err := coordinator.Schedule(TaskSchedule{TaskID: request.TaskID, NodeID: "node-authorized"}); err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	if _, err := coordinator.Claim(request); !errors.Is(err, ErrClaimIneligible) {
		t.Fatalf("node-mismatched Claim() error = %v", err)
	}
}

func TestEventSequenceIdempotencyGapAndOldEpoch(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)}
	coordinator := newCoordinator(t, clock)
	grant := claim(t, coordinator)

	gap, err := coordinator.Append(event(grant, "event-2", 2, EventProcessStarted, Facts{}))
	if err != nil || gap.Decision != EventGap || gap.ExpectedSequence != 1 {
		t.Fatalf("gap = %#v, %v", gap, err)
	}
	started := event(grant, "event-1", 1, EventProcessStarted, Facts{})
	accepted, err := coordinator.Append(started)
	if err != nil || accepted.Decision != EventAccepted || accepted.Snapshot.State != StateRunning {
		t.Fatalf("start event = %#v, %v", accepted, err)
	}
	duplicate, err := coordinator.Append(started)
	if err != nil || duplicate.Decision != EventDuplicate {
		t.Fatalf("duplicate = %#v, %v", duplicate, err)
	}
	old := event(grant, "stale", 2, EventProcessExited, Facts{ProcessExited: true})
	old.LeaseEpoch = 99
	stale, err := coordinator.Append(old)
	if err != nil || stale.Decision != EventStale || stale.Snapshot.State != StateRunning {
		t.Fatalf("old epoch = %#v, %v", stale, err)
	}
	wrongReuse := event(grant, "new-event", 1, EventProcessExited, Facts{ProcessExited: true})
	if _, err := coordinator.Append(wrongReuse); !errors.Is(err, ErrEventSequenceReused) {
		t.Fatalf("reused sequence error = %v", err)
	}
}

func TestTerminalEvidenceMatrixAndTerminalIsIrreversible(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)}
	coordinator := newCoordinator(t, clock)
	grant := claim(t, coordinator)
	appendEvent(t, coordinator, event(grant, "1", 1, EventProcessStarted, Facts{}))
	appendEvent(t, coordinator, event(grant, "2", 2, EventProcessExited, Facts{ProcessExited: true}))
	appendEvent(t, coordinator, event(grant, "3", 3, EventToolTerminal, Facts{ToolTerminal: ToolSuccess}))
	missingOutput := appendEvent(t, coordinator, event(grant, "4", 4, EventResultFacts, Facts{ResultFacts: ResultFailed}))
	if missingOutput.Snapshot.State != StateFailed {
		t.Fatalf("success tool + missing output state = %s, want FAILED", missingOutput.Snapshot.State)
	}
	late := appendEvent(t, coordinator, event(grant, "5", 5, EventResultFacts, Facts{ResultFacts: ResultVerified}))
	if late.Decision != EventIgnored || late.Snapshot.State != StateFailed {
		t.Fatalf("late terminal event = %#v", late)
	}

	startFailure := newCoordinator(t, clock)
	startGrant := claim(t, startFailure)
	failed := appendEvent(t, startFailure, event(startGrant, "start-rejected", 1, EventStartRejected, Facts{NoProcess: true}))
	if failed.Snapshot.State != StateFailed {
		t.Fatalf("start rejected state = %s", failed.Snapshot.State)
	}

	toolFailure := newCoordinator(t, clock)
	toolGrant := claim(t, toolFailure)
	appendEvent(t, toolFailure, event(toolGrant, "tool-1", 1, EventProcessStarted, Facts{}))
	appendEvent(t, toolFailure, event(toolGrant, "tool-2", 2, EventProcessExited, Facts{ProcessExited: true}))
	toolFailed := appendEvent(t, toolFailure, event(toolGrant, "tool-3", 3, EventToolTerminal, Facts{ToolTerminal: ToolFailure}))
	if toolFailed.Snapshot.State != StateFailed {
		t.Fatalf("tool failure state = %s, want FAILED", toolFailed.Snapshot.State)
	}

	missingStart := newCoordinator(t, clock)
	missingGrant := claim(t, missingStart)
	unknown := appendEvent(t, missingStart, event(missingGrant, "exit-without-start", 1, EventProcessExited, Facts{ProcessExited: true}))
	if unknown.Snapshot.State != StateStarting || !unknown.Snapshot.ReconciliationRequired {
		t.Fatalf("missing start evidence snapshot = %#v", unknown.Snapshot)
	}
}

func TestLeaseExpiryDisconnectAndRenewUseControlPlaneClock(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)}
	coordinator := newCoordinator(t, clock)
	grant := claim(t, coordinator)
	clock.now = clock.now.Add(30 * time.Second)
	renew, err := coordinator.Renew(RenewRequest{RequestID: "renew-1", ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, LeaseTTL: time.Minute})
	if err != nil || !renew.ExpiresAt.After(grant.ExpiresAt) {
		t.Fatalf("Renew() = %#v, %v", renew, err)
	}
	clock.now = renew.ExpiresAt
	if affected := coordinator.ExpireLeases(); affected != 1 {
		t.Fatalf("ExpireLeases() = %d, want 1", affected)
	}
	snapshot, err := coordinator.Snapshot(grant.ExecutionID)
	if err != nil || snapshot.State != StateStarting || !snapshot.ReconciliationRequired {
		t.Fatalf("expired snapshot = %#v, %v", snapshot, err)
	}
	if _, err := coordinator.Renew(RenewRequest{RequestID: "renew-late", ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, LeaseTTL: time.Minute}); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("late renew error = %v", err)
	}
	if affected := coordinator.MarkAgentDisconnected("agent-1"); affected != 0 {
		t.Fatalf("already reconciled disconnect count = %d", affected)
	}
}

func newCoordinator(t *testing.T, clock Clock) *Coordinator {
	t.Helper()
	coordinator, err := NewCoordinator(clock)
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}
	return coordinator
}

func validClaim() ClaimRequest {
	return ClaimRequest{RequestID: "claim-1", TaskID: "task-1", ExecutionID: "execution-1", NodeID: "node-1", AgentID: "agent-1", LeaseID: "lease-1", LeaseTTL: time.Minute}
}

func claim(t *testing.T, coordinator *Coordinator) LeaseGrant {
	t.Helper()
	request := validClaim()
	if err := coordinator.Schedule(TaskSchedule{TaskID: request.TaskID, NodeID: request.NodeID}); err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}
	grant, err := coordinator.Claim(request)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	return grant
}

func event(grant LeaseGrant, id string, sequence int64, kind EventType, facts Facts) Event {
	return Event{EventID: id, ExecutionID: grant.ExecutionID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, Sequence: sequence, Type: kind, Facts: facts}
}

func appendEvent(t *testing.T, coordinator *Coordinator, event Event) EventResult {
	t.Helper()
	result, err := coordinator.Append(event)
	if err != nil {
		t.Fatalf("Append(%s) error = %v", event.Type, err)
	}
	return result
}
