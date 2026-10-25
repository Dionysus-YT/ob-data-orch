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
	"ob-data-orch/internal/logretention"
)

// logRetentionEntry 在读取正文或配置之前检查浏览器当前权限，机器身份没有此入口。
func (s *Server) logRetentionEntry(w http.ResponseWriter, r *http.Request, principal identity.Principal) bool {
	if r.URL.Path != "/api/v1/settings/log-retention" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if s.logRetention == nil || s.authorizer == nil {
		retentionHTTPError(w, r, logretention.ErrUnavailable)
		return true
	}
	scope := identity.ScopeLogRetentionRead
	if r.Method == http.MethodPatch {
		scope = identity.ScopeLogRetentionWrite
	}
	if identity.Can(r.Context(), s.authorizer, principal, scope, "platform") != nil {
		notFound(w, r)
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if r.URL.RawQuery != "" {
		retentionHTTPError(w, r, logretention.ErrInvalid)
		return true
	}
	switch r.Method {
	case http.MethodGet:
		policy, err := s.logRetention.Read(ctx, principal.ID)
		if err != nil {
			retentionHTTPError(w, r, err)
			return true
		}
		if ctx.Err() != nil {
			retentionHTTPError(w, r, ctx.Err())
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
			retentionHTTPError(w, r, logretention.ErrUnavailable)
			return true
		}
		if s.csrf.ValidateCSRF(r) != nil {
			writeError(w, 401, "CSRF_VALIDATION_FAILED", "请求安全校验失败。", false)
			return true
		}
		revision, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
		if !ok {
			writeError(w, 412, "REVISION_REQUIRED", "需要当前普通日志策略版本。", false)
			return true
		}
		var body struct {
			Days      *int    `json:"retentionDays"`
			Reason    *string `json:"reason"`
			Confirmed *bool   `json:"confirmed"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || body.Days == nil || body.Reason == nil || body.Confirmed == nil {
			retentionHTTPError(w, r, logretention.ErrInvalid)
			return true
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			retentionHTTPError(w, r, logretention.ErrInvalid)
			return true
		}
		result, err := s.logRetention.Change(ctx, logretention.Change{ActorID: principal.ID, ExpectedRevision: revision, RetentionDays: *body.Days, Reason: *body.Reason, Confirmed: *body.Confirmed, IdempotencyKey: r.Header.Get("Idempotency-Key"), RequestID: requestID(w), At: time.Now().UTC()})
		if err != nil {
			retentionHTTPError(w, r, err)
			return true
		}
		writeJSON(w, 200, map[string]any{"requestId": requestID(w), "item": result})
	default:
		notFound(w, r)
	}
	return true
}

func retentionHTTPError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, logretention.ErrDenied):
		notFound(w, r)
	case errors.Is(err, logretention.ErrRevision):
		writeError(w, 412, "REVISION_CONFLICT", "普通日志策略已变化，请重新读取后核对。", false)
	case errors.Is(err, logretention.ErrIdempotency):
		writeError(w, 409, "IDEMPOTENCY_CONFLICT", "请求标识已用于不同变更。", false)
	case errors.Is(err, logretention.ErrConfirmation):
		writeError(w, 422, "LOG_RETENTION_CONFIRMATION_REQUIRED", "缩短期限会使更多已结束任务的普通日志进入清理范围，请核对并确认。", false)
	case errors.Is(err, logretention.ErrInvalid):
		writeError(w, 400, "LOG_RETENTION_FIELDS_INVALID", "期限须为 7–365 的整数，原因须为 1–200 字且不含秘密。", false)
	default:
		writeError(w, 503, "LOG_RETENTION_UNAVAILABLE", "普通日志策略暂不可用，未确认变更成功。", true)
	}
}
