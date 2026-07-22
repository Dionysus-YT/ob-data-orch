// Package agentstate implements the in-memory control-plane semantics for an
// execution lease. It deliberately has no HTTP, SQLite, agent process, or
// tool-execution dependency. Those adapters must preserve these rules.
package agentstate

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidRequest       = errors.New("invalid agent state request")
	ErrIdempotencyConflict  = errors.New("idempotency key was reused with different input")
	ErrTaskAlreadyClaimed   = errors.New("task already has an execution")
	ErrClaimIneligible      = errors.New("task is not eligible for this node")
	ErrUnknownExecution     = errors.New("unknown execution")
	ErrLeaseRejected        = errors.New("lease is not current")
	ErrLeaseExpired         = errors.New("lease has expired")
	ErrEventRejected        = errors.New("event rejected")
	ErrEventSequenceReused  = errors.New("event sequence was reused")
	ErrInvalidStateMutation = errors.New("event is invalid for the current state")
)

// Clock makes all lease decisions use a control-plane clock and permits
// deterministic tests. Agent-provided timestamps are intentionally absent.
type Clock interface {
	Now() time.Time
}

type ProductState string

const (
	StateWaiting    ProductState = "WAITING_SCHEDULE"
	StateStarting   ProductState = "STARTING"
	StateRunning    ProductState = "RUNNING"
	StateCancelling ProductState = "CANCELLING"
	StateSucceeded  ProductState = "SUCCEEDED"
	StateFailed     ProductState = "FAILED"
	StateCancelled  ProductState = "CANCELLED"
)

func (s ProductState) Terminal() bool {
	return s == StateSucceeded || s == StateFailed || s == StateCancelled
}

type EventType string

const (
	EventScheduled          EventType = "SCHEDULED"
	EventLeaseAcknowledged  EventType = "LEASE_ACKNOWLEDGED"
	EventStartRejected      EventType = "START_REJECTED"
	EventProcessStarted     EventType = "PROCESS_STARTED"
	EventProcessExited      EventType = "PROCESS_EXITED"
	EventToolTerminal       EventType = "TOOL_TERMINAL_OBSERVED"
	EventResultFacts        EventType = "RESULT_FACTS_OBSERVED"
	EventEvidenceConflict   EventType = "EVIDENCE_CONFLICT"
	EventLocalRecoveryStart EventType = "LOCAL_RECOVERY_STARTED"
	EventLocalRecoveryDone  EventType = "LOCAL_RECOVERY_RESULT"
)

type ToolTerminal string

const (
	ToolUnknown ToolTerminal = "UNKNOWN"
	ToolSuccess ToolTerminal = "SUCCEEDED"
	ToolFailure ToolTerminal = "FAILED"
)

type ResultFacts string

const (
	ResultUnknown  ResultFacts = "UNKNOWN"
	ResultVerified ResultFacts = "VERIFIED"
	ResultFailed   ResultFacts = "FAILED"
)

type ClaimRequest struct {
	RequestID   string
	TaskID      string
	ExecutionID string
	NodeID      string
	AgentID     string
	LeaseID     string
	LeaseTTL    time.Duration
}

// TaskSchedule is the control-plane fact that a submitted task is waiting for
// one user-selected node. This is not a scheduler: it has no priority,
// automatic node selection, or reassignment behavior.
type TaskSchedule struct {
	TaskID string
	NodeID string
}

type LeaseGrant struct {
	ExecutionID string
	LeaseID     string
	LeaseEpoch  int64
	ExpiresAt   time.Time
}

type RenewRequest struct {
	RequestID   string
	ExecutionID string
	LeaseID     string
	LeaseEpoch  int64
	LeaseTTL    time.Duration
}

type Event struct {
	EventID     string
	ExecutionID string
	LeaseID     string
	LeaseEpoch  int64
	Sequence    int64
	Type        EventType
	Facts       Facts
}

// Facts are trusted observations sent by an Agent, not a direct request to set
// a product status. A terminal state is only projected when the evidence matrix
// in the agent-state contract is complete.
type Facts struct {
	NoProcess     bool
	ProcessExited bool
	ToolTerminal  ToolTerminal
	ResultFacts   ResultFacts
}

type EventDecision string

const (
	EventAccepted  EventDecision = "ACCEPTED"
	EventDuplicate EventDecision = "DUPLICATE"
	EventGap       EventDecision = "GAP"
	EventStale     EventDecision = "STALE_LEASE"
	EventIgnored   EventDecision = "IGNORED_TERMINAL"
)

type EventResult struct {
	Decision         EventDecision
	ExpectedSequence int64
	Snapshot         Snapshot
}

type Snapshot struct {
	TaskID                 string
	ExecutionID            string
	NodeID                 string
	AgentID                string
	State                  ProductState
	ReconciliationRequired bool
	LeaseID                string
	LeaseEpoch             int64
	LeaseExpiresAt         time.Time
	NextEventSequence      int64
	Evidence               Evidence
}

type Evidence struct {
	ProcessStarted bool
	ProcessExited  bool
	ToolTerminal   ToolTerminal
	ResultFacts    ResultFacts
}

type rememberedRequest struct {
	signature string
	grant     LeaseGrant
}

type rememberedEvent struct {
	signature string
	result    EventResult
}

type execution struct {
	Snapshot
	requests  map[string]rememberedRequest
	events    map[string]rememberedEvent
	sequences map[int64]string
}

// Coordinator is an intentionally small fake control plane. Its storage is
// memory only; a later SQLite/HTTP adapter must make equivalent operations
// transactional rather than treating this type as the production datastore.
type Coordinator struct {
	mu        sync.Mutex
	clock     Clock
	byTask    map[string]string
	pending   map[string]string
	entries   map[string]*execution
	prechecks map[string]*precheck
}

func NewCoordinator(clock Clock) (*Coordinator, error) {
	if clock == nil {
		return nil, fmt.Errorf("%w: control-plane clock is required", ErrInvalidRequest)
	}
	return &Coordinator{
		clock:     clock,
		byTask:    make(map[string]string),
		pending:   make(map[string]string),
		entries:   make(map[string]*execution),
		prechecks: make(map[string]*precheck),
	}, nil
}

func (c *Coordinator) Schedule(task TaskSchedule) error {
	if blank(task.TaskID, task.NodeID) {
		return ErrInvalidRequest
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, claimed := c.byTask[task.TaskID]; claimed {
		return ErrTaskAlreadyClaimed
	}
	if existing, found := c.pending[task.TaskID]; found && existing != task.NodeID {
		return ErrIdempotencyConflict
	}
	c.pending[task.TaskID] = task.NodeID
	return nil
}

func (c *Coordinator) Claim(request ClaimRequest) (LeaseGrant, error) {
	if err := validateClaim(request); err != nil {
		return LeaseGrant{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	signature := claimSignature(request)
	for _, entry := range c.entries {
		if previous, ok := entry.requests[request.RequestID]; ok {
			if previous.signature != signature {
				return LeaseGrant{}, ErrIdempotencyConflict
			}
			return previous.grant, nil
		}
	}
	if _, exists := c.byTask[request.TaskID]; exists {
		return LeaseGrant{}, ErrTaskAlreadyClaimed
	}
	if expectedNode, exists := c.pending[request.TaskID]; !exists || expectedNode != request.NodeID {
		return LeaseGrant{}, ErrClaimIneligible
	}
	if _, exists := c.entries[request.ExecutionID]; exists {
		return LeaseGrant{}, fmt.Errorf("%w: execution ID already exists", ErrInvalidRequest)
	}
	now := c.clock.Now().UTC()
	grant := LeaseGrant{ExecutionID: request.ExecutionID, LeaseID: request.LeaseID, LeaseEpoch: 1, ExpiresAt: now.Add(request.LeaseTTL)}
	entry := &execution{
		Snapshot: Snapshot{
			TaskID: request.TaskID, ExecutionID: request.ExecutionID, NodeID: request.NodeID, AgentID: request.AgentID,
			State: StateStarting, LeaseID: grant.LeaseID, LeaseEpoch: grant.LeaseEpoch, LeaseExpiresAt: grant.ExpiresAt,
			NextEventSequence: 1,
			Evidence:          Evidence{ToolTerminal: ToolUnknown, ResultFacts: ResultUnknown},
		},
		requests:  map[string]rememberedRequest{request.RequestID: {signature: signature, grant: grant}},
		events:    make(map[string]rememberedEvent),
		sequences: make(map[int64]string),
	}
	c.byTask[request.TaskID] = request.ExecutionID
	delete(c.pending, request.TaskID)
	c.entries[request.ExecutionID] = entry
	return grant, nil
}

func (c *Coordinator) Renew(request RenewRequest) (LeaseGrant, error) {
	if err := validateRenew(request); err != nil {
		return LeaseGrant{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[request.ExecutionID]
	if !ok {
		return LeaseGrant{}, ErrUnknownExecution
	}
	signature := renewSignature(request)
	if previous, ok := entry.requests[request.RequestID]; ok {
		if previous.signature != signature {
			return LeaseGrant{}, ErrIdempotencyConflict
		}
		return previous.grant, nil
	}
	if !entry.matches(request.LeaseID, request.LeaseEpoch) {
		return LeaseGrant{}, ErrLeaseRejected
	}
	now := c.clock.Now().UTC()
	if !now.Before(entry.LeaseExpiresAt) {
		entry.ReconciliationRequired = true
		return LeaseGrant{}, ErrLeaseExpired
	}
	grant := LeaseGrant{ExecutionID: entry.ExecutionID, LeaseID: entry.LeaseID, LeaseEpoch: entry.LeaseEpoch, ExpiresAt: now.Add(request.LeaseTTL)}
	entry.LeaseExpiresAt = grant.ExpiresAt
	entry.requests[request.RequestID] = rememberedRequest{signature: signature, grant: grant}
	return grant, nil
}

func (c *Coordinator) Append(event Event) (EventResult, error) {
	if err := validateEvent(event); err != nil {
		return EventResult{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[event.ExecutionID]
	if !ok {
		return EventResult{}, ErrUnknownExecution
	}
	signature := eventSignature(event)
	if previous, ok := entry.events[event.EventID]; ok {
		if previous.signature != signature {
			return EventResult{}, ErrIdempotencyConflict
		}
		return EventResult{Decision: EventDuplicate, ExpectedSequence: entry.NextEventSequence, Snapshot: entry.Snapshot}, nil
	}
	if !entry.matches(event.LeaseID, event.LeaseEpoch) {
		return EventResult{Decision: EventStale, ExpectedSequence: entry.NextEventSequence, Snapshot: entry.Snapshot}, nil
	}
	if event.Sequence > entry.NextEventSequence {
		return EventResult{Decision: EventGap, ExpectedSequence: entry.NextEventSequence, Snapshot: entry.Snapshot}, nil
	}
	if event.Sequence < entry.NextEventSequence {
		return EventResult{}, ErrEventSequenceReused
	}
	if event.Sequence == entry.NextEventSequence {
		if _, exists := entry.sequences[event.Sequence]; exists {
			return EventResult{}, ErrEventSequenceReused
		}
	}

	result := EventResult{Decision: EventAccepted}
	if entry.State.Terminal() {
		result.Decision = EventIgnored
	} else if err := applyEvent(&entry.Snapshot, event); err != nil {
		return EventResult{}, err
	}
	entry.sequences[event.Sequence] = event.EventID
	entry.NextEventSequence++
	result.ExpectedSequence = entry.NextEventSequence
	result.Snapshot = entry.Snapshot
	entry.events[event.EventID] = rememberedEvent{signature: signature, result: result}
	return result, nil
}

// ExpireLeases marks unknown work for reconciliation; it never changes a
// STARTING/RUNNING task to FAILED and never creates another execution.
func (c *Coordinator) ExpireLeases() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock.Now().UTC()
	count := 0
	for _, entry := range c.entries {
		if !entry.State.Terminal() && !now.Before(entry.LeaseExpiresAt) && !entry.ReconciliationRequired {
			entry.ReconciliationRequired = true
			count++
		}
	}
	return count
}

// MarkAgentDisconnected records uncertainty without inventing a terminal
// conclusion. It is deliberately independent of the Agent's clock.
func (c *Coordinator) MarkAgentDisconnected(agentID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for _, entry := range c.entries {
		if entry.AgentID == agentID && !entry.State.Terminal() && !entry.ReconciliationRequired {
			entry.ReconciliationRequired = true
			count++
		}
	}
	return count
}

func (c *Coordinator) Snapshot(executionID string) (Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[executionID]
	if !ok {
		return Snapshot{}, ErrUnknownExecution
	}
	return entry.Snapshot, nil
}

func (e *execution) matches(leaseID string, epoch int64) bool {
	return e.LeaseID == leaseID && e.LeaseEpoch == epoch
}

func applyEvent(snapshot *Snapshot, event Event) error {
	switch event.Type {
	case EventScheduled:
		return nil
	case EventLeaseAcknowledged:
		return nil
	case EventProcessStarted:
		if snapshot.State != StateStarting {
			return ErrInvalidStateMutation
		}
		snapshot.Evidence.ProcessStarted = true
		snapshot.State = StateRunning
	case EventStartRejected:
		if snapshot.State != StateStarting || !event.Facts.NoProcess {
			return ErrInvalidStateMutation
		}
		snapshot.State = StateFailed
	case EventProcessExited:
		snapshot.Evidence.ProcessExited = true
		if snapshot.State == StateStarting && !snapshot.Evidence.ProcessStarted {
			snapshot.ReconciliationRequired = true
		}
	case EventToolTerminal:
		if event.Facts.ToolTerminal != ToolSuccess && event.Facts.ToolTerminal != ToolFailure {
			return fmt.Errorf("%w: tool terminal fact is required", ErrInvalidRequest)
		}
		snapshot.Evidence.ToolTerminal = event.Facts.ToolTerminal
		if snapshot.State == StateStarting && !snapshot.Evidence.ProcessStarted {
			snapshot.ReconciliationRequired = true
		}
	case EventResultFacts:
		if event.Facts.ResultFacts != ResultVerified && event.Facts.ResultFacts != ResultFailed {
			return fmt.Errorf("%w: result fact is required", ErrInvalidRequest)
		}
		snapshot.Evidence.ResultFacts = event.Facts.ResultFacts
		if snapshot.State == StateStarting && !snapshot.Evidence.ProcessStarted {
			snapshot.ReconciliationRequired = true
		}
	case EventEvidenceConflict, EventLocalRecoveryStart, EventLocalRecoveryDone:
		snapshot.ReconciliationRequired = true
	default:
		return fmt.Errorf("%w: unsupported event type %q", ErrInvalidRequest, event.Type)
	}
	projectTerminal(snapshot)
	return nil
}

func projectTerminal(snapshot *Snapshot) {
	if snapshot.State != StateRunning || !snapshot.Evidence.ProcessExited {
		return
	}
	if snapshot.Evidence.ToolTerminal == ToolFailure || snapshot.Evidence.ResultFacts == ResultFailed {
		snapshot.State = StateFailed
		return
	}
	if snapshot.Evidence.ToolTerminal == ToolSuccess && snapshot.Evidence.ResultFacts == ResultVerified {
		snapshot.State = StateSucceeded
	}
}

func validateClaim(request ClaimRequest) error {
	if blank(request.RequestID, request.TaskID, request.ExecutionID, request.NodeID, request.AgentID, request.LeaseID) || request.LeaseTTL <= 0 {
		return ErrInvalidRequest
	}
	return nil
}

func validateRenew(request RenewRequest) error {
	if blank(request.RequestID, request.ExecutionID, request.LeaseID) || request.LeaseEpoch <= 0 || request.LeaseTTL <= 0 {
		return ErrInvalidRequest
	}
	return nil
}

func validateEvent(event Event) error {
	if blank(event.EventID, event.ExecutionID, event.LeaseID) || event.LeaseEpoch <= 0 || event.Sequence <= 0 || event.Type == "" {
		return ErrInvalidRequest
	}
	return nil
}

func blank(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}

func claimSignature(request ClaimRequest) string {
	return strings.Join([]string{request.TaskID, request.ExecutionID, request.NodeID, request.AgentID, request.LeaseID, request.LeaseTTL.String()}, "|")
}

func renewSignature(request RenewRequest) string {
	return strings.Join([]string{request.ExecutionID, request.LeaseID, fmt.Sprint(request.LeaseEpoch), request.LeaseTTL.String()}, "|")
}

func eventSignature(event Event) string {
	return strings.Join([]string{event.ExecutionID, event.LeaseID, fmt.Sprint(event.LeaseEpoch), fmt.Sprint(event.Sequence), string(event.Type), fmt.Sprintf("%t,%t,%s,%s", event.Facts.NoProcess, event.Facts.ProcessExited, event.Facts.ToolTerminal, event.Facts.ResultFacts)}, "|")
}
