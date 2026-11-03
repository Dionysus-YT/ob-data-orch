package controlplane

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

// templateTypeMatchesRoute 使旧导出入口保持类型边界；未知类型即使被存储端返回也不能进入写事务。
func templateTypeMatchesRoute(r *http.Request, kind string) bool {
	if kind == "" || kind == "OBDUMPER_EXPORT" {
		return true
	}
	return kind == "OBLOADER_NORMAL_IMPORT" && strings.HasPrefix(r.URL.Path, "/api/v1/config-templates/")
}

// templateActions 只投影当前对象的使用与管理职责，不从列表可见推导写权限。
// 未启用持久授权的兼容部署沿用自有模板边界；启用后必须逐项读取当前授权。
func (s *Server) templateActions(ctx context.Context, principal identity.Principal, item store.ExportConfigTemplate) []string {
	actions := make([]string, 0, 3)
	legacyOwner := s.permissions == nil && s.authorizer != nil && item.OwnerSubjectID == principal.ID
	if legacyOwner || identity.Can(ctx, s.authorizer, principal, identity.ScopeTemplateUse, item.TemplateID) == nil {
		actions = append(actions, "CREATE_DRAFT")
	}
	if legacyOwner || identity.Can(ctx, s.authorizer, principal, identity.ScopeTemplateManage, item.TemplateID) == nil {
		actions = append(actions, "RENAME", "DELETE")
	}
	return actions
}

// templateWriteDigest 将全部业务输入纳入请求身份，避免同键异名称或异绑定被当作重放。
// 数组编码保持字段边界，不使用可与用户输入分隔符碰撞的字符串拼接。
func templateWriteDigest(operation string, values ...string) string {
	raw, _ := json.Marshal(append([]string{operation}, values...))
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func (s *Server) templateTypeAllowed(w http.ResponseWriter, r *http.Request, p identity.Principal, id string) bool {
	item, err := s.templates.GetAuthorizedExportConfigTemplate(r.Context(), id, p.ID)
	if err != nil || !templateTypeMatchesRoute(r, item.TaskType) {
		notFound(w, r)
		return false
	}
	return true
}
