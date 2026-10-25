package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/schedulingpolicy"
	"ob-data-orch/internal/store"
)

// SchedulingCandidateStore 隔离设置事务和普通任务候选读取，不向普通用户公开设置修改能力。
type SchedulingCandidateStore interface {
	ReadCurrentCandidatePolicy(context.Context) (schedulingpolicy.CandidatePolicy, error)
	ReadAuthorizedCandidatePolicy(context.Context, string) (schedulingpolicy.CandidatePolicy, error)
	ChangeCandidatePolicy(context.Context, schedulingpolicy.CandidateChange) (schedulingpolicy.Receipt, error)
}

type candidateHeartbeatReader interface {
	ReadCandidateHeartbeat(context.Context, string) (*time.Time, error)
}

type taskNodeSelectionReader interface {
	ReadTaskNodeSelection(context.Context, string) ([]store.ExecutionNode, map[string]store.NodeBudgetUsage, error)
}

func (s *Server) listTaskNodeSelection(w http.ResponseWriter, r *http.Request, principal identity.Principal) {
	reader, ok := s.nodeManagement.(taskNodeSelectionReader)
	if !ok || s.authorizer == nil {
		candidatePolicyHTTPError(w, r, schedulingpolicy.ErrUnavailable)
		return
	}
	nodes, budgets, err := reader.ReadTaskNodeSelection(r.Context(), principal.ID)
	if err != nil {
		candidatePolicyHTTPError(w, r, err)
		return
	}
	ttl, err := s.currentHeartbeatTimeout(r.Context())
	if err != nil {
		candidatePolicyHTTPError(w, r, err)
		return
	}
	items := []executionNodeListResponse{}
	ids := []string{}
	for _, node := range nodes {
		if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeNodeManage, node.NodeID) != nil {
			continue
		}
		item := s.newExecutionNodeListResponse(node, ttl)
		attachNodeBudget(&item, node, budgets[node.NodeID])
		items = append(items, item)
		ids = append(ids, item.ID)
	}
	indices, err := s.candidatePermutation(r.Context(), principal, ids, identity.ScopeNodeManage)
	if err != nil {
		candidatePolicyHTTPError(w, r, err)
		return
	}
	ordered := make([]executionNodeListResponse, 0, len(items))
	for _, index := range indices {
		if identity.Can(r.Context(), s.authorizer, principal, identity.ScopeNodeManage, items[index].ID) == nil {
			ordered = append(ordered, items[index])
		}
	}
	writeJSON(w, 200, map[string]any{"requestId": requestID(w), "items": ordered})
}

func (s *Server) schedulingCandidateEntry(w http.ResponseWriter, r *http.Request, principal identity.Principal) bool {
	if r.URL.Path != "/api/v1/settings/scheduling/candidates" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if s.schedulingCandidates == nil || s.authorizer == nil {
		candidatePolicyHTTPError(w, r, schedulingpolicy.ErrUnavailable)
		return true
	}
	scope := identity.ScopeSchedulingPolicyRead
	if r.Method == http.MethodPatch {
		scope = identity.ScopeSchedulingPolicyWrite
	}
	if identity.Can(r.Context(), s.authorizer, principal, scope, "platform") != nil {
		notFound(w, r)
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if r.URL.RawQuery != "" {
		candidatePolicyHTTPError(w, r, schedulingpolicy.ErrInvalid)
		return true
	}
	switch r.Method {
	case http.MethodGet:
		policy, err := s.schedulingCandidates.ReadAuthorizedCandidatePolicy(ctx, principal.ID)
		if err == nil {
			err = schedulingpolicy.ValidateCandidatePolicy(policy)
		}
		if err != nil {
			candidatePolicyHTTPError(w, r, err)
			return true
		}
		if ctx.Err() != nil {
			candidatePolicyHTTPError(w, r, ctx.Err())
			return true
		}
		if identity.Can(ctx, s.authorizer, principal, scope, "platform") != nil {
			notFound(w, r)
			return true
		}
		w.Header().Set("ETag", fmt.Sprintf(`"rev-%d"`, policy.Revision))
		writeJSON(w, 200, map[string]any{"requestId": requestID(w), "item": policy})
	case http.MethodPatch:
		if s.csrf == nil {
			candidatePolicyHTTPError(w, r, schedulingpolicy.ErrUnavailable)
			return true
		}
		if s.csrf.ValidateCSRF(r) != nil {
			writeError(w, 401, "CSRF_VALIDATION_FAILED", "请求安全校验失败。", false)
			return true
		}
		revision, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
		if !ok {
			writeError(w, 412, "REVISION_REQUIRED", "需要当前候选顺序版本。", false)
			return true
		}
		var body struct {
			Order     *string `json:"order"`
			Reason    *string `json:"reason"`
			Confirmed *bool   `json:"confirmed"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || body.Order == nil || body.Reason == nil || body.Confirmed == nil || !schedulingpolicy.ValidCandidateOrder(*body.Order) {
			candidatePolicyHTTPError(w, r, schedulingpolicy.ErrInvalid)
			return true
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			candidatePolicyHTTPError(w, r, schedulingpolicy.ErrInvalid)
			return true
		}
		result, err := s.schedulingCandidates.ChangeCandidatePolicy(ctx, schedulingpolicy.CandidateChange{ActorID: principal.ID, ExpectedRevision: revision, Order: *body.Order, Reason: *body.Reason, Confirmed: *body.Confirmed, IdempotencyKey: r.Header.Get("Idempotency-Key"), RequestID: requestID(w), At: time.Now().UTC()})
		if err != nil {
			candidatePolicyHTTPError(w, r, err)
			return true
		}
		writeJSON(w, 200, map[string]any{"requestId": requestID(w), "item": result})
	default:
		notFound(w, r)
	}
	return true
}

func candidatePolicyHTTPError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, schedulingpolicy.ErrDenied):
		notFound(w, r)
	case errors.Is(err, schedulingpolicy.ErrInvalid):
		writeError(w, 400, "CANDIDATE_ORDER_FIELDS_INVALID", "请核对候选顺序及变更原因。", false)
	case errors.Is(err, schedulingpolicy.ErrConfirmation):
		writeError(w, 422, "CANDIDATE_ORDER_CONFIRMATION_REQUIRED", "请确认候选顺序变化的影响。", false)
	case errors.Is(err, schedulingpolicy.ErrRevision):
		writeError(w, 412, "REVISION_CONFLICT", "候选顺序已变化，请重新读取后核对。", false)
	case errors.Is(err, schedulingpolicy.ErrIdempotency):
		writeError(w, 409, "IDEMPOTENCY_CONFLICT", "请求标识已用于不同变更。", false)
	default:
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/execution-nodes" {
			writeError(w, 503, "EXECUTION_NODE_QUERY_UNAVAILABLE", "执行节点候选暂不可用，请稍后重试。", true)
			return
		}
		writeError(w, 503, "CANDIDATE_ORDER_UNAVAILABLE", "候选顺序暂不可用，未确认变更成功。", true)
	}
}

// candidatePermutation 先复验对象权限再读取最小心跳，禁止隐藏节点影响排序或错误。
// 未装配设置端口的隔离夹具沿用旧顺序；正式组合根必须装配持久化策略。
func (s *Server) candidatePermutation(ctx context.Context, principal identity.Principal, ids []string, scope identity.Scope) ([]int, error) {
	heartbeats := make([]*time.Time, len(ids))
	order := schedulingpolicy.ExistingOrder
	if s.schedulingCandidates != nil {
		policy, err := s.schedulingCandidates.ReadCurrentCandidatePolicy(ctx)
		if err != nil {
			return nil, err
		}
		if err = schedulingpolicy.ValidateCandidatePolicy(policy); err != nil {
			return nil, err
		}
		order = policy.Order
	}
	var ttl time.Duration
	if order == schedulingpolicy.OnlineFirst {
		var err error
		ttl, err = s.currentHeartbeatTimeout(ctx)
		if err != nil {
			return nil, err
		}
		reader, ok := s.nodeCandidates.(candidateHeartbeatReader)
		if !ok {
			return nil, schedulingpolicy.ErrUnavailable
		}
		for index, id := range ids {
			if identity.Can(ctx, s.authorizer, principal, scope, id) != nil {
				continue
			}
			heartbeats[index], err = reader.ReadCandidateHeartbeat(ctx, id)
			if err != nil {
				return nil, err
			}
		}
	}
	return schedulingpolicy.CandidateOrderIndices(order, heartbeats, time.Now().UTC(), ttl)
}
