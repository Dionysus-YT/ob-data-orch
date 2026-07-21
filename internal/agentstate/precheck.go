package agentstate

import (
	"fmt"
	"strings"
	"time"
)

// PrecheckBinding is immutable control-plane input. It intentionally contains
// identifiers and version facts only: a precheck does not accept arbitrary SQL,
// shell text, paths, or credential plaintext.
type PrecheckBinding struct {
	PrecheckID         string
	NodeID             string
	DraftRevision      int64
	ConfigFingerprint  string
	CredentialRevision int64
	NodeFactsVersion   int64
}

type PrecheckState string

const (
	PrecheckWaiting   PrecheckState = "WAITING"
	PrecheckClaimed   PrecheckState = "CLAIMED"
	PrecheckSucceeded PrecheckState = "SUCCEEDED"
	PrecheckFailed    PrecheckState = "FAILED"
	PrecheckUnknown   PrecheckState = "UNKNOWN"
)

type PrecheckClaimRequest struct {
	RequestID  string
	PrecheckID string
	NodeID     string
	AgentID    string
	LeaseID    string
	LeaseTTL   time.Duration
}

type PrecheckLeaseGrant struct {
	PrecheckID string
	LeaseID    string
	LeaseEpoch int64
	ExpiresAt  time.Time
	Binding    PrecheckBinding
}

type PrecheckCompletion struct {
	RequestID  string
	PrecheckID string
	LeaseID    string
	LeaseEpoch int64
	Binding    PrecheckBinding
	Complete   bool
	Succeeded  bool
}

type PrecheckSnapshot struct {
	Binding        PrecheckBinding
	AgentID        string
	LeaseID        string
	LeaseEpoch     int64
	LeaseExpiresAt time.Time
	State          PrecheckState
}

type rememberedPrecheckRequest struct {
	signature string
	grant     PrecheckLeaseGrant
}

type rememberedCompletion struct {
	signature string
	snapshot  PrecheckSnapshot
}

type precheck struct {
	PrecheckSnapshot
	claims      map[string]rememberedPrecheckRequest
	completions map[string]rememberedCompletion
}

func (c *Coordinator) SchedulePrecheck(binding PrecheckBinding) error {
	if err := validateBinding(binding); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, found := c.prechecks[binding.PrecheckID]; found {
		if existing.Binding != binding {
			return ErrIdempotencyConflict
		}
		return nil
	}
	c.prechecks[binding.PrecheckID] = &precheck{
		PrecheckSnapshot: PrecheckSnapshot{Binding: binding, State: PrecheckWaiting},
		claims:           make(map[string]rememberedPrecheckRequest),
		completions:      make(map[string]rememberedCompletion),
	}
	return nil
}

// ClaimPrecheck enforces the single-Agent first-slice capacity rule. It only
// grants a fixed EXPORT_PREFLIGHT binding and never creates an execution.
func (c *Coordinator) ClaimPrecheck(request PrecheckClaimRequest) (PrecheckLeaseGrant, error) {
	if err := validatePrecheckClaim(request); err != nil {
		return PrecheckLeaseGrant{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, found := c.prechecks[request.PrecheckID]
	if !found {
		return PrecheckLeaseGrant{}, ErrUnknownExecution
	}
	signature := precheckClaimSignature(request)
	if previous, found := entry.claims[request.RequestID]; found {
		if previous.signature != signature {
			return PrecheckLeaseGrant{}, ErrIdempotencyConflict
		}
		return previous.grant, nil
	}
	if entry.State != PrecheckWaiting || entry.Binding.NodeID != request.NodeID || c.hasActiveExecution(request.AgentID) {
		return PrecheckLeaseGrant{}, ErrClaimIneligible
	}
	now := c.clock.Now().UTC()
	grant := PrecheckLeaseGrant{PrecheckID: request.PrecheckID, LeaseID: request.LeaseID, LeaseEpoch: 1, ExpiresAt: now.Add(request.LeaseTTL), Binding: entry.Binding}
	entry.AgentID, entry.LeaseID, entry.LeaseEpoch, entry.LeaseExpiresAt, entry.State = request.AgentID, grant.LeaseID, grant.LeaseEpoch, grant.ExpiresAt, PrecheckClaimed
	entry.claims[request.RequestID] = rememberedPrecheckRequest{signature: signature, grant: grant}
	return grant, nil
}

// CompletePrecheck accepts only a complete result attached to the current,
// unexpired short lease and its immutable binding. A stale or incomplete result
// cannot be promoted into a submission gate.
func (c *Coordinator) CompletePrecheck(completion PrecheckCompletion) (PrecheckSnapshot, error) {
	if err := validateCompletion(completion); err != nil {
		return PrecheckSnapshot{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, found := c.prechecks[completion.PrecheckID]
	if !found {
		return PrecheckSnapshot{}, ErrUnknownExecution
	}
	signature := completionSignature(completion)
	if previous, found := entry.completions[completion.RequestID]; found {
		if previous.signature != signature {
			return PrecheckSnapshot{}, ErrIdempotencyConflict
		}
		return previous.snapshot, nil
	}
	if entry.State != PrecheckClaimed || entry.LeaseID != completion.LeaseID || entry.LeaseEpoch != completion.LeaseEpoch {
		return PrecheckSnapshot{}, ErrLeaseRejected
	}
	if !c.clock.Now().UTC().Before(entry.LeaseExpiresAt) {
		entry.State = PrecheckUnknown
		return PrecheckSnapshot{}, ErrLeaseExpired
	}
	if entry.Binding != completion.Binding {
		return PrecheckSnapshot{}, fmt.Errorf("%w: precheck binding changed", ErrInvalidRequest)
	}
	entry.State = PrecheckFailed
	if completion.Succeeded {
		entry.State = PrecheckSucceeded
	}
	entry.completions[completion.RequestID] = rememberedCompletion{signature: signature, snapshot: entry.PrecheckSnapshot}
	return entry.PrecheckSnapshot, nil
}

// ExpirePrechecks turns a missing result into UNKNOWN. It cannot create a task
// or alter a task state, which keeps preflight distinct from execution.
func (c *Coordinator) ExpirePrechecks() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock.Now().UTC()
	count := 0
	for _, entry := range c.prechecks {
		if entry.State == PrecheckClaimed && !now.Before(entry.LeaseExpiresAt) {
			entry.State = PrecheckUnknown
			count++
		}
	}
	return count
}

func (c *Coordinator) PrecheckSnapshot(precheckID string) (PrecheckSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, found := c.prechecks[precheckID]
	if !found {
		return PrecheckSnapshot{}, ErrUnknownExecution
	}
	return entry.PrecheckSnapshot, nil
}

func (c *Coordinator) hasActiveExecution(agentID string) bool {
	for _, entry := range c.entries {
		if entry.AgentID == agentID && !entry.State.Terminal() {
			return true
		}
	}
	return false
}

func validateBinding(binding PrecheckBinding) error {
	if blank(binding.PrecheckID, binding.NodeID, binding.ConfigFingerprint) || binding.DraftRevision <= 0 || binding.CredentialRevision <= 0 || binding.NodeFactsVersion <= 0 {
		return ErrInvalidRequest
	}
	return nil
}

func validatePrecheckClaim(request PrecheckClaimRequest) error {
	if blank(request.RequestID, request.PrecheckID, request.NodeID, request.AgentID, request.LeaseID) || request.LeaseTTL <= 0 {
		return ErrInvalidRequest
	}
	return nil
}

func validateCompletion(completion PrecheckCompletion) error {
	if blank(completion.RequestID, completion.PrecheckID, completion.LeaseID) || completion.LeaseEpoch <= 0 || !completion.Complete {
		return ErrInvalidRequest
	}
	return validateBinding(completion.Binding)
}

func precheckClaimSignature(request PrecheckClaimRequest) string {
	return strings.Join([]string{request.PrecheckID, request.NodeID, request.AgentID, request.LeaseID, request.LeaseTTL.String()}, "|")
}

func completionSignature(completion PrecheckCompletion) string {
	return strings.Join([]string{completion.PrecheckID, completion.LeaseID, fmt.Sprint(completion.LeaseEpoch), fmt.Sprint(completion.Binding), fmt.Sprint(completion.Complete), fmt.Sprint(completion.Succeeded)}, "|")
}
