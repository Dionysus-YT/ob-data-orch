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

// SchedulingPolicyStore 限定本批真实调度设置的授权读取与原子保存端口。
type SchedulingPolicyStore interface {
	ReadAuthorizedSchedulingPolicy(context.Context, string) (schedulingpolicy.Policy, error)
	ChangeSchedulingPolicy(context.Context, schedulingpolicy.Change) (schedulingpolicy.Receipt, error)
}

// schedulingPolicyEntry 在读取正文或配置之前检查浏览器当前权限，机器身份没有此入口。
func (s *Server) schedulingPolicyEntry(w http.ResponseWriter, r *http.Request, principal identity.Principal) bool {
	if r.URL.Path != "/api/v1/settings/scheduling" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if s.schedulingPolicy == nil || s.authorizer == nil {
		schedulingHTTPError(w, r, schedulingpolicy.ErrUnavailable)
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
		schedulingHTTPError(w, r, schedulingpolicy.ErrInvalid)
		return true
	}
	switch r.Method {
	case http.MethodGet:
		policy, err := s.schedulingPolicy.ReadAuthorizedSchedulingPolicy(ctx, principal.ID)
		if err != nil {
			schedulingHTTPError(w, r, err)
			return true
		}
		if ctx.Err() != nil {
			schedulingHTTPError(w, r, ctx.Err())
			return true
		}
		if identity.Can(ctx, s.authorizer, principal, scope, "platform") != nil {
			notFound(w, r)
			return true
		}
		// 全局任务权限与系统管理权限独立，读取期间撤权后不得继续返回全平台数量。
		if policy.Usage.Visibility == "AVAILABLE" && identity.Can(ctx, s.authorizer, principal, identity.ScopeTaskReadAll, "*") != nil {
			policy.Usage.Visibility = "RESTRICTED"
			policy.Usage.HeldExecutions = nil
			policy.Usage.RunningExecutions = nil
		}
		w.Header().Set("ETag", fmt.Sprintf(`"rev-%d"`, policy.Revision))
		writeJSON(w, 200, map[string]any{"requestId": requestID(w), "item": policy})
	case http.MethodPatch:
		if s.csrf == nil {
			schedulingHTTPError(w, r, schedulingpolicy.ErrUnavailable)
			return true
		}
		if s.csrf.ValidateCSRF(r) != nil {
			writeError(w, 401, "CSRF_VALIDATION_FAILED", "请求安全校验失败。", false)
			return true
		}
		revision, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
		if !ok {
			writeError(w, 412, "REVISION_REQUIRED", "需要当前平台调度策略版本。", false)
			return true
		}
		var body struct {
			Maximum   *int    `json:"maxConcurrentExecutions"`
			Reason    *string `json:"reason"`
			Confirmed *bool   `json:"confirmed"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || body.Maximum == nil || body.Reason == nil || body.Confirmed == nil {
			schedulingHTTPError(w, r, schedulingpolicy.ErrInvalid)
			return true
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			schedulingHTTPError(w, r, schedulingpolicy.ErrInvalid)
			return true
		}
		result, err := s.schedulingPolicy.ChangeSchedulingPolicy(ctx, schedulingpolicy.Change{ActorID: principal.ID, ExpectedRevision: revision, MaxConcurrentExecutions: *body.Maximum, Reason: *body.Reason, Confirmed: *body.Confirmed, IdempotencyKey: r.Header.Get("Idempotency-Key"), RequestID: requestID(w), At: time.Now().UTC()})
		if err != nil {
			schedulingHTTPError(w, r, err)
			return true
		}
		writeJSON(w, 200, map[string]any{"requestId": requestID(w), "item": result})
	default:
		notFound(w, r)
	}
	return true
}

func schedulingHTTPError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, schedulingpolicy.ErrDenied):
		notFound(w, r)
	case errors.Is(err, schedulingpolicy.ErrRevision):
		writeError(w, 412, "REVISION_CONFLICT", "平台调度策略已变化，请重新读取后核对。", false)
	case errors.Is(err, schedulingpolicy.ErrIdempotency):
		writeError(w, 409, "IDEMPOTENCY_CONFLICT", "请求标识已用于不同变更。", false)
	case errors.Is(err, schedulingpolicy.ErrConfirmation):
		writeError(w, 422, "SCHEDULING_POLICY_CONFIRMATION_REQUIRED", "首次设置或降低并发上限将约束后续领取，请核对并确认。", false)
	case errors.Is(err, schedulingpolicy.ErrInvalid):
		writeError(w, 400, "SCHEDULING_POLICY_FIELDS_INVALID", "并发上限须为 1–1024 的整数，原因须为 1–200 字且不含秘密。", false)
	default:
		writeError(w, 503, "SCHEDULING_POLICY_UNAVAILABLE", "平台调度策略暂不可用，未确认变更成功。", true)
	}
}
