package agentstate

import (
	"errors"
	"testing"
	"time"
)

func TestPrecheckIsNodeBoundIdempotentAndSeparateFromExecution(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)}
	coordinator := newCoordinator(t, clock)
	binding := validBinding()
	if err := coordinator.SchedulePrecheck(binding); err != nil {
		t.Fatalf("SchedulePrecheck() error = %v", err)
	}
	request := validPrecheckClaim()
	request.NodeID = "wrong-node"
	if _, err := coordinator.ClaimPrecheck(request); !errors.Is(err, ErrClaimIneligible) {
		t.Fatalf("wrong node ClaimPrecheck() error = %v", err)
	}
	request.NodeID = binding.NodeID
	grant, err := coordinator.ClaimPrecheck(request)
	if err != nil {
		t.Fatalf("ClaimPrecheck() error = %v", err)
	}
	retry, err := coordinator.ClaimPrecheck(request)
	if err != nil || retry != grant {
		t.Fatalf("idempotent ClaimPrecheck() = %#v, %v", retry, err)
	}
	completion := PrecheckCompletion{RequestID: "precheck-complete-1", PrecheckID: grant.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, Binding: binding, Complete: true, Succeeded: true}
	completed, err := coordinator.CompletePrecheck(completion)
	if err != nil || completed.State != PrecheckSucceeded {
		t.Fatalf("CompletePrecheck() = %#v, %v", completed, err)
	}
	completedRetry, err := coordinator.CompletePrecheck(completion)
	if err != nil || completedRetry != completed {
		t.Fatalf("idempotent CompletePrecheck() = %#v, %v", completedRetry, err)
	}
	if _, err := coordinator.Snapshot("execution-not-created"); !errors.Is(err, ErrUnknownExecution) {
		t.Fatalf("precheck created execution error = %v", err)
	}
}

func TestPrecheckCompletionRejectsExpiredOldLeaseAndBindingDrift(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)}
	coordinator := newCoordinator(t, clock)
	binding := validBinding()
	if err := coordinator.SchedulePrecheck(binding); err != nil {
		t.Fatalf("SchedulePrecheck() error = %v", err)
	}
	grant, err := coordinator.ClaimPrecheck(validPrecheckClaim())
	if err != nil {
		t.Fatalf("ClaimPrecheck() error = %v", err)
	}
	completion := PrecheckCompletion{RequestID: "complete-1", PrecheckID: grant.PrecheckID, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, Binding: binding, Complete: true, Succeeded: true}
	drift := completion
	drift.RequestID, drift.Binding.ConfigFingerprint = "complete-drift", "different-fingerprint"
	if _, err := coordinator.CompletePrecheck(drift); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("binding drift completion error = %v", err)
	}
	old := completion
	old.RequestID, old.LeaseEpoch = "complete-old", 2
	if _, err := coordinator.CompletePrecheck(old); !errors.Is(err, ErrLeaseRejected) {
		t.Fatalf("old precheck lease error = %v", err)
	}
	clock.now = grant.ExpiresAt
	if _, err := coordinator.CompletePrecheck(completion); !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("expired completion error = %v", err)
	}
	snapshot, err := coordinator.PrecheckSnapshot(binding.PrecheckID)
	if err != nil || snapshot.State != PrecheckUnknown {
		t.Fatalf("expired precheck snapshot = %#v, %v", snapshot, err)
	}

	second := validBinding()
	second.PrecheckID = "precheck-2"
	if err := coordinator.SchedulePrecheck(second); err != nil {
		t.Fatalf("SchedulePrecheck(second) error = %v", err)
	}
	secondRequest := validPrecheckClaim()
	secondRequest.RequestID, secondRequest.PrecheckID, secondRequest.LeaseID = "precheck-claim-2", second.PrecheckID, "precheck-lease-2"
	secondGrant, err := coordinator.ClaimPrecheck(secondRequest)
	if err != nil {
		t.Fatalf("ClaimPrecheck(second) error = %v", err)
	}
	clock.now = secondGrant.ExpiresAt
	if expired := coordinator.ExpirePrechecks(); expired != 1 {
		t.Fatalf("ExpirePrechecks() = %d, want 1", expired)
	}
}

func TestPrecheckDoesNotCompeteWithActiveExecution(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)}
	coordinator := newCoordinator(t, clock)
	_ = claim(t, coordinator)
	binding := validBinding()
	if err := coordinator.SchedulePrecheck(binding); err != nil {
		t.Fatalf("SchedulePrecheck() error = %v", err)
	}
	if _, err := coordinator.ClaimPrecheck(validPrecheckClaim()); !errors.Is(err, ErrClaimIneligible) {
		t.Fatalf("active execution precheck claim error = %v", err)
	}
}

func validBinding() PrecheckBinding {
	return PrecheckBinding{PrecheckID: "precheck-1", NodeID: "node-1", DraftRevision: 2, ConfigFingerprint: "synthetic-fingerprint", CredentialRevision: 3, NodeFactsVersion: 4}
}

func validPrecheckClaim() PrecheckClaimRequest {
	return PrecheckClaimRequest{RequestID: "precheck-claim-1", PrecheckID: "precheck-1", NodeID: "node-1", AgentID: "agent-1", LeaseID: "precheck-lease-1", LeaseTTL: time.Minute}
}
