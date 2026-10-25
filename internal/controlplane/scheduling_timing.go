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
)

// SchedulingTimingStore 分离时间策略授权事务与控制面内部实际阈值读取。
type SchedulingTimingStore interface {
	ReadAuthorizedSchedulingTiming(context.Context, string, time.Duration) (schedulingpolicy.TimingPolicy, error)
	ChangeSchedulingTiming(context.Context, schedulingpolicy.TimingChange) (schedulingpolicy.Receipt, error)
	ReadCurrentSchedulingTiming(context.Context, time.Duration) (schedulingpolicy.TimingPolicy, error)
}

func (s *Server) schedulingTimingEntry(w http.ResponseWriter, r *http.Request, principal identity.Principal) bool {
	if r.URL.Path != "/api/v1/settings/scheduling/timing" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if s.schedulingTiming == nil || s.authorizer == nil {
		schedulingTimingHTTPError(w, r, schedulingpolicy.ErrUnavailable)
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
		schedulingTimingHTTPError(w, r, schedulingpolicy.ErrInvalid)
		return true
	}
	switch r.Method {
	case http.MethodGet:
		policy, err := s.schedulingTiming.ReadAuthorizedSchedulingTiming(ctx, principal.ID, s.heartbeatTTL)
		if err != nil {
			schedulingTimingHTTPError(w, r, err)
			return true
		}
		if ctx.Err() != nil {
			schedulingTimingHTTPError(w, r, ctx.Err())
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
			schedulingTimingHTTPError(w, r, schedulingpolicy.ErrUnavailable)
			return true
		}
		if s.csrf.ValidateCSRF(r) != nil {
			writeError(w, 401, "CSRF_VALIDATION_FAILED", "请求安全校验失败。", false)
			return true
		}
		revision, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
		if !ok {
			writeError(w, 412, "REVISION_REQUIRED", "需要当前调度时间策略版本。", false)
			return true
		}
		// RawMessage 区分缺失字段与明确 null；空覆盖必须是用户看到的真实未配置状态。
		var body struct {
			Heartbeat *int            `json:"heartbeatTimeoutSeconds"`
			Startup   json.RawMessage `json:"startupTimeoutSeconds"`
			Polling   json.RawMessage `json:"pollingIntervalSeconds"`
			Reason    *string         `json:"reason"`
			Confirmed *bool           `json:"confirmed"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || body.Heartbeat == nil || len(body.Startup) == 0 || len(body.Polling) == 0 || body.Reason == nil || body.Confirmed == nil {
			schedulingTimingHTTPError(w, r, schedulingpolicy.ErrInvalid)
			return true
		}
		var extra any
		var startup, polling *int
		if decoder.Decode(&extra) != io.EOF || json.Unmarshal(body.Startup, &startup) != nil || json.Unmarshal(body.Polling, &polling) != nil {
			schedulingTimingHTTPError(w, r, schedulingpolicy.ErrInvalid)
			return true
		}
		result, err := s.schedulingTiming.ChangeSchedulingTiming(ctx, schedulingpolicy.TimingChange{ActorID: principal.ID, ExpectedRevision: revision, BaselineHeartbeatTimeout: s.heartbeatTTL, HeartbeatTimeoutSeconds: *body.Heartbeat, StartupTimeoutSeconds: startup, PollingIntervalSeconds: polling, Reason: *body.Reason, Confirmed: *body.Confirmed, IdempotencyKey: r.Header.Get("Idempotency-Key"), RequestID: requestID(w), At: time.Now().UTC()})
		if err != nil {
			schedulingTimingHTTPError(w, r, err)
			return true
		}
		writeJSON(w, 200, map[string]any{"requestId": requestID(w), "item": result})
	default:
		notFound(w, r)
	}
	return true
}

func schedulingTimingHTTPError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, schedulingpolicy.ErrDenied):
		notFound(w, r)
	case errors.Is(err, schedulingpolicy.ErrInvalid):
		writeError(w, 400, "SCHEDULING_TIMING_FIELDS_INVALID", "请核对时间范围及原因；不允许清空已配置的启动或检查间隔。", false)
	case errors.Is(err, schedulingpolicy.ErrConfirmation):
		writeError(w, 422, "SCHEDULING_TIMING_CONFIRMATION_REQUIRED", "请确认缩短超时或改变检查间隔的影响。", false)
	case errors.Is(err, schedulingpolicy.ErrRevision):
		writeError(w, 412, "REVISION_CONFLICT", "调度时间策略已变化，请重新读取后核对。", false)
	case errors.Is(err, schedulingpolicy.ErrIdempotency):
		writeError(w, 409, "IDEMPOTENCY_CONFLICT", "请求标识已用于不同变更。", false)
	default:
		writeError(w, 503, "SCHEDULING_TIMING_UNAVAILABLE", "调度时间策略暂不可用，未确认变更成功。", true)
	}
}

// currentHeartbeatTimeout 对现役存储读取失败关闭；未装配时间设置的隔离依赖保留原启动阈值。
func (s *Server) currentHeartbeatTimeout(ctx context.Context) (time.Duration, error) {
	if s.schedulingTiming == nil {
		return s.heartbeatTTL, nil
	}
	policy, err := s.schedulingTiming.ReadCurrentSchedulingTiming(ctx, s.heartbeatTTL)
	if err != nil {
		return 0, err
	}
	return time.Duration(policy.HeartbeatTimeoutSeconds) * time.Second, nil
}

func (s *Server) heartbeatTimeoutForRequest(w http.ResponseWriter, r *http.Request) (time.Duration, bool) {
	ttl, err := s.currentHeartbeatTimeout(r.Context())
	if err != nil {
		schedulingTimingHTTPError(w, r, err)
		return 0, false
	}
	return ttl, true
}
