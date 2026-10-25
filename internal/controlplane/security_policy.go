package controlplane

import (
	"context"
	"net/http"
	"time"

	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/securitypolicy"
)

// SecurityPolicyReader 只提供非秘密安全状态，不提供配置写入、密钥访问或审计清理。
type SecurityPolicyReader interface {
	Read(context.Context) (securitypolicy.Snapshot, error)
}

func (s *Server) securityPolicyEntry(w http.ResponseWriter, r *http.Request, p identity.Principal) bool {
	if r.URL.Path != "/api/v1/settings/security" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	// 在读取启动事实之前复验当前 ACTIVE 系统管理员，不以旧角色或部署初始身份兜底。
	if identity.Can(r.Context(), s.authorizer, p, identity.ScopeSecurityPolicyRead, "platform") != nil {
		notFound(w, r)
		return true
	}
	if r.Method != http.MethodGet {
		notFound(w, r)
		return true
	}
	if r.URL.RawQuery != "" || r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
		writeError(w, 400, "SECURITY_POLICY_FIELDS_INVALID", "安全状态读取不接受查询条件或正文。", false)
		return true
	}
	if s.securityPolicy == nil {
		writeError(w, 503, "SECURITY_POLICY_UNAVAILABLE", "安全状态暂时不可用，请稍后重试。", true)
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	item, err := s.securityPolicy.Read(ctx)
	// 即使读取失败也先复验撤权，不能通过不同错误泄露已无权读取的安全状态。
	if identity.Can(ctx, s.authorizer, p, identity.ScopeSecurityPolicyRead, "platform") != nil {
		notFound(w, r)
		return true
	}
	if err != nil || ctx.Err() != nil {
		writeError(w, 503, "SECURITY_POLICY_UNAVAILABLE", "安全状态暂时不可用，请稍后重试。", true)
		return true
	}
	writeJSON(w, 200, map[string]any{"requestId": requestID(w), "item": item})
	return true
}
