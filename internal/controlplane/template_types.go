package controlplane

import (
	"net/http"
	"strings"

	"ob-data-orch/internal/identity"
)

// templateTypeMatchesRoute 使旧导出入口保持类型边界；未知类型即使被存储端返回也不能进入写事务。
func templateTypeMatchesRoute(r *http.Request, kind string) bool {
	if kind == "" || kind == "OBDUMPER_EXPORT" {
		return true
	}
	return kind == "OBLOADER_NORMAL_IMPORT" && strings.HasPrefix(r.URL.Path, "/api/v1/config-templates/")
}

func (s *Server) templateTypeAllowed(w http.ResponseWriter, r *http.Request, p identity.Principal, id string) bool {
	item, err := s.templates.GetAuthorizedExportConfigTemplate(r.Context(), id, p.ID)
	if err != nil || !templateTypeMatchesRoute(r, item.TaskType) {
		notFound(w, r)
		return false
	}
	return true
}
