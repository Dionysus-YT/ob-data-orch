package controlplane

import (
	"context"
	"net/http"
	"time"

	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/toolcompatibility"
)

// ToolCompatibilityReader 只提供内置版本与模块目录，不提供发布、安装或业务对象读取。
type ToolCompatibilityReader interface {
	Read(context.Context) (toolcompatibility.Snapshot, error)
}

func (s *Server) toolCompatibilityEntry(w http.ResponseWriter, r *http.Request, p identity.Principal) bool {
	if r.URL.Path != "/api/v1/settings/tools" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	// 读取和响应前均复验当前持久授权，不从内置非秘密内容推导公开访问。
	if identity.Can(r.Context(), s.authorizer, p, identity.ScopeToolCompatibilityRead, "platform") != nil {
		notFound(w, r)
		return true
	}
	if r.Method != http.MethodGet {
		notFound(w, r)
		return true
	}
	if r.URL.RawQuery != "" || r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
		writeError(w, 400, "TOOL_COMPATIBILITY_FIELDS_INVALID", "工具状态读取不接受查询条件或正文。", false)
		return true
	}
	if s.toolCompatibility == nil {
		writeError(w, 503, "TOOL_COMPATIBILITY_UNAVAILABLE", "工具与兼容状态暂时不可用，请稍后重试。", true)
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	item, err := s.toolCompatibility.Read(ctx)
	if identity.Can(ctx, s.authorizer, p, identity.ScopeToolCompatibilityRead, "platform") != nil {
		notFound(w, r)
		return true
	}
	if err != nil || ctx.Err() != nil {
		writeError(w, 503, "TOOL_COMPATIBILITY_UNAVAILABLE", "工具与兼容状态暂时不可用，请稍后重试。", true)
		return true
	}
	writeJSON(w, 200, map[string]any{"requestId": requestID(w), "item": item})
	return true
}
