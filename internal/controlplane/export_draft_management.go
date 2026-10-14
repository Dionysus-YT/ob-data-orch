package controlplane

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

// listExportDrafts 固定为当前主体所有的未提交草稿，不接受全部账号或其他主体筛选。
func (s *Server) listExportDrafts(w http.ResponseWriter, r *http.Request, principal identity.Principal) {
	reader, ok := s.drafts.(interface {
		ListOwnedExportDrafts(context.Context, store.ExportDraftListQuery) ([]store.ExportDraftListItem, int, error)
	})
	if !ok {
		writeError(w, 503, "EXPORT_DRAFT_QUERY_UNAVAILABLE", "草稿列表暂时不可读取", false)
		return
	}
	for name, values := range r.URL.Query() {
		if len(values) != 1 || (name != "keyword" && name != "limit" && name != "cursor") {
			writeError(w, 400, "DRAFT_FILTER_INVALID", "草稿筛选条件无效", false)
			return
		}
	}
	limit, ok := taskListPageSize(r)
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	if !ok || len(keyword) > 128 || strings.ContainsAny(keyword, "\x00\r\n") {
		writeError(w, 400, "DRAFT_FILTER_INVALID", "草稿筛选条件无效", false)
		return
	}
	query := store.ExportDraftListQuery{SubjectID: principal.ID, Keyword: keyword, Limit: limit}
	binding := fmt.Sprintf("%s|export-drafts|%d|%s", principal.ID, limit, keyword)
	if value := r.URL.Query().Get("cursor"); value != "" {
		position, valid := decodeTaskListCursor(value, binding)
		if !valid {
			writeError(w, 400, "CURSOR_INVALID", "草稿游标无效", false)
			return
		}
		query.BeforeUpdated, query.BeforeID = position.BeforeSubmitted, position.BeforeTaskID
	}
	items, total, err := reader.ListOwnedExportDrafts(r.Context(), query)
	if err != nil {
		writeError(w, 503, "EXPORT_DRAFT_QUERY_UNAVAILABLE", "草稿列表暂时不可读取", true)
		return
	}
	cursor := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		cursor = encodeTaskListCursor(binding, last.UpdatedAt, last.ID)
	}
	writeJSON(w, 200, map[string]any{"requestId": requestID(w), "items": items, "nextCursor": cursor, "totalPages": (total + limit - 1) / limit})
}

// deleteExportDraft 只删除主体自己的草稿；CSRF、版本及幂等缺失时失败关闭，领域事务复验提交与租约。
func (s *Server) deleteExportDraft(w http.ResponseWriter, r *http.Request, principal identity.Principal, id string) {
	writer, ok := s.drafts.(interface {
		DeleteOwnedExportDraft(context.Context, string, string, int64, string, string, time.Time) (bool, error)
	})
	if !ok || s.csrf == nil {
		writeError(w, 503, "EXPORT_DRAFT_DELETE_UNAVAILABLE", "草稿删除暂时不可用", false)
		return
	}
	if err := s.csrf.ValidateCSRF(r); err != nil {
		writeError(w, 401, "CSRF_VALIDATION_FAILED", "请求安全校验失败", false)
		return
	}
	revision, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
	if !ok {
		writeError(w, 412, "REVISION_REQUIRED", "需要有效的草稿版本号", false)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 16 || len(key) > 200 || strings.ContainsAny(key, "\r\n\x00") {
		writeError(w, 400, "IDEMPOTENCY_KEY_INVALID", "幂等键格式无效", false)
		return
	}
	replayed, err := writer.DeleteOwnedExportDraft(r.Context(), id, principal.ID, revision, key, requestID(w), time.Now().UTC())
	switch {
	case errors.Is(err, store.ErrDataSourceNotFound):
		notFound(w, r)
		return
	case errors.Is(err, store.ErrRevisionConflict):
		writeError(w, 412, "DRAFT_REVISION_CONFLICT", "草稿已发生变化，请刷新后重试", false)
		return
	case errors.Is(err, store.ErrIdempotencyConflict):
		writeError(w, 409, "IDEMPOTENCY_CONFLICT", "幂等键已用于不同请求", false)
		return
	case errors.Is(err, store.ErrExportDraftDeleteIneligible):
		writeError(w, 409, "DRAFT_DELETE_INELIGIBLE", "草稿已提交或预检查仍在执行，请刷新后核对", false)
		return
	case err != nil:
		writeError(w, 503, "EXPORT_DRAFT_DELETE_UNAVAILABLE", "草稿删除未完成", true)
		return
	}
	writeJSON(w, 200, map[string]any{"requestId": requestID(w), "deleted": true, "replayed": replayed})
}
