package controlplane

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"ob-data-orch/internal/accesscontrol"
	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/store"
)

// PermissionStore 是已认证主体的授权端口，不允许通过本端口建立或修改认证身份。
type PermissionStore interface {
	ListPermissionSubjects(context.Context, string, accesscontrol.SubjectQuery) (accesscontrol.SubjectPage, error)
	ReadPermissionProfile(context.Context, string, string) (accesscontrol.Profile, error)
	ListPermissionCandidates(context.Context, string, accesscontrol.ScopeType, string, string, int) ([]accesscontrol.Candidate, bool, error)
	ChangeSubjectPermissions(context.Context, accesscontrol.Change) (accesscontrol.ChangeResult, error)
}

type permissionCursor struct {
	Version int    `json:"version"`
	Actor   string `json:"actor"`
	Kind    string `json:"kind"`
	Digest  string `json:"digest"`
	After   string `json:"after"`
}

// permissionEntry 在读取目录、目标或请求正文前校验平台权限管理能力。
func (s *Server) permissionEntry(w http.ResponseWriter, r *http.Request, p identity.Principal) bool {
	const prefix = "/api/v1/access-control/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if s.permissions == nil || s.authorizer == nil {
		writeError(w, 503, "PERMISSIONS_UNAVAILABLE", "权限配置依赖尚未接入。", false)
		return true
	}
	if identity.Can(r.Context(), s.authorizer, p, identity.ScopePermissionManage, "platform") != nil {
		notFound(w, r)
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	path := strings.TrimPrefix(r.URL.Path, prefix)
	switch {
	case path == "subjects" && r.Method == http.MethodGet:
		s.listPermissionSubjects(w, r, p)
	case path == "candidates" && r.Method == http.MethodGet:
		s.listPermissionCandidates(w, r, p)
	case strings.HasPrefix(path, "subjects/"):
		id := strings.TrimPrefix(path, "subjects/")
		if id == "" || len(id) > 200 || strings.ContainsAny(id, "/\\\r\n\t") {
			notFound(w, r)
			return true
		}
		switch r.Method {
		case http.MethodGet:
			s.getPermissionProfile(w, r, p, id)
		case http.MethodPatch:
			s.changePermissionProfile(w, r, p, id)
		default:
			notFound(w, r)
		}
	default:
		notFound(w, r)
	}
	return true
}

func permissionValues(r *http.Request, allowed ...string) (url.Values, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, accesscontrol.ErrInvalid
	}
	for key, items := range values {
		if !slices.Contains(allowed, key) || len(items) != 1 {
			return nil, accesscontrol.ErrInvalid
		}
	}
	return values, nil
}

func permissionPage(values url.Values, actor, kind string) (limit int, after, digest string, err error) {
	limit = 10
	if values.Has("limit") {
		limit, err = strconv.Atoi(values.Get("limit"))
		if err != nil || limit < 1 || limit > 50 {
			return 0, "", "", accesscontrol.ErrInvalid
		}
	}
	if !utf8.ValidString(values.Get("search")) || utf8.RuneCountInString(values.Get("search")) > 128 {
		return 0, "", "", accesscontrol.ErrInvalid
	}
	filters := url.Values{}
	for key, items := range values {
		if key != "cursor" {
			filters[key] = items
		}
	}
	hash := sha256.Sum256([]byte(filters.Encode()))
	digest = hex.EncodeToString(hash[:])
	if values.Has("cursor") {
		if len(values.Get("cursor")) > 2048 {
			return 0, "", "", accesscontrol.ErrInvalid
		}
		raw, e := base64.RawURLEncoding.DecodeString(values.Get("cursor"))
		if e != nil {
			return 0, "", "", accesscontrol.ErrInvalid
		}
		var cursor permissionCursor
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&cursor) != nil || cursor.Version != 1 || cursor.Actor != actor || cursor.Kind != kind || cursor.Digest != digest || cursor.After == "" || len(cursor.After) > 200 {
			return 0, "", "", accesscontrol.ErrInvalid
		}
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			return 0, "", "", accesscontrol.ErrInvalid
		}
		after = cursor.After
	}
	return limit, after, digest, nil
}

func permissionNext(actor, kind, digest, after string) string {
	raw, _ := json.Marshal(permissionCursor{1, actor, kind, digest, after})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func (s *Server) listPermissionSubjects(w http.ResponseWriter, r *http.Request, p identity.Principal) {
	values, err := permissionValues(r, "search", "role", "status", "capability", "limit", "cursor")
	if err != nil {
		s.permissionError(w, r, err)
		return
	}
	limit, after, digest, err := permissionPage(values, p.ID, "subjects")
	if err != nil {
		s.permissionError(w, r, err)
		return
	}
	role := accesscontrol.Role(values.Get("role"))
	capability := accesscontrol.Capability(values.Get("capability"))
	status := values.Get("status")
	if role != "" && accesscontrol.Validate(accesscontrol.Assignment{Roles: []accesscontrol.Role{role}}) != nil || !slices.Contains([]string{"", "ACTIVE", "DISABLED", "UNKNOWN"}, status) || capability != "" && !slices.Contains([]accesscontrol.Capability{accesscontrol.ProductionTask, accesscontrol.ExpertConfig, accesscontrol.SensitiveCommand, accesscontrol.AuditLog, accesscontrol.GlobalSchedulingLog}, capability) {
		s.permissionError(w, r, accesscontrol.ErrInvalid)
		return
	}
	page, err := s.permissions.ListPermissionSubjects(r.Context(), p.ID, accesscontrol.SubjectQuery{Search: values.Get("search"), Role: role, Status: status, Capability: capability, AfterID: after, Limit: limit})
	if err != nil {
		s.permissionError(w, r, err)
		return
	}
	if !s.permissionReadStillAllowed(w, r, p) {
		return
	}
	response := map[string]any{"requestId": requestID(w), "currentSubjectId": p.ID, "directorySource": "EXISTING_AUTH_SUBJECTS", "directoryStatus": "PROJECTED", "timeZone": "UTC", "observedAt": time.Now().UTC(), "items": page.Items, "hasMore": page.HasMore}
	if page.HasMore && len(page.Items) > 0 {
		response["nextCursor"] = permissionNext(p.ID, "subjects", digest, page.Items[len(page.Items)-1].Subject.ID)
	}
	writeJSON(w, 200, response)
}

func (s *Server) getPermissionProfile(w http.ResponseWriter, r *http.Request, p identity.Principal, id string) {
	if _, err := permissionValues(r); err != nil {
		s.permissionError(w, r, err)
		return
	}
	profile, err := s.permissions.ReadPermissionProfile(r.Context(), p.ID, id)
	if err != nil {
		s.permissionError(w, r, err)
		return
	}
	if !s.permissionReadStillAllowed(w, r, p) {
		return
	}
	writeJSON(w, 200, map[string]any{"requestId": requestID(w), "item": profile})
}

func (s *Server) listPermissionCandidates(w http.ResponseWriter, r *http.Request, p identity.Principal) {
	values, err := permissionValues(r, "type", "search", "limit", "cursor")
	if err != nil {
		s.permissionError(w, r, err)
		return
	}
	limit, after, digest, err := permissionPage(values, p.ID, "candidates")
	if err != nil {
		s.permissionError(w, r, err)
		return
	}
	items, more, err := s.permissions.ListPermissionCandidates(r.Context(), p.ID, accesscontrol.ScopeType(values.Get("type")), values.Get("search"), after, limit)
	if err != nil {
		s.permissionError(w, r, err)
		return
	}
	if !s.permissionReadStillAllowed(w, r, p) {
		return
	}
	response := map[string]any{"requestId": requestID(w), "items": items, "hasMore": more}
	if more && len(items) > 0 {
		response["nextCursor"] = permissionNext(p.ID, "candidates", digest, items[len(items)-1].ID)
	}
	writeJSON(w, 200, response)
}

func (s *Server) changePermissionProfile(w http.ResponseWriter, r *http.Request, p identity.Principal, id string) {
	if s.csrf == nil {
		writeError(w, 503, "PERMISSIONS_UNAVAILABLE", "权限写入安全依赖尚未接入。", false)
		return
	}
	if s.csrf.ValidateCSRF(r) != nil {
		writeError(w, 401, "CSRF_VALIDATION_FAILED", "请求安全校验失败。", false)
		return
	}
	if _, err := permissionValues(r); err != nil {
		s.permissionError(w, r, err)
		return
	}
	revision, ok := parseIfMatchRevision(r.Header.Get("If-Match"))
	if !ok {
		writeError(w, 412, "REVISION_REQUIRED", "需要当前权限版本。", false)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" || len(key) > 128 {
		s.permissionError(w, r, accesscontrol.ErrInvalid)
		return
	}
	var body struct {
		Roles        *[]accesscontrol.Role        `json:"roles"`
		Capabilities *[]accesscontrol.Capability  `json:"capabilities"`
		Scopes       *[]accesscontrol.ObjectScope `json:"scopes"`
		Reason       *string                      `json:"reason"`
		Confirmed    *bool                        `json:"confirmed"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || body.Roles == nil || body.Capabilities == nil || body.Scopes == nil || body.Reason == nil || body.Confirmed == nil {
		s.permissionError(w, r, accesscontrol.ErrInvalid)
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		s.permissionError(w, r, accesscontrol.ErrInvalid)
		return
	}
	result, err := s.permissions.ChangeSubjectPermissions(r.Context(), accesscontrol.Change{ActorID: p.ID, SubjectID: id, ExpectedRevision: revision, Assignment: accesscontrol.Assignment{Roles: *body.Roles, Capabilities: *body.Capabilities, Scopes: *body.Scopes}, Reason: *body.Reason, Confirmed: *body.Confirmed, IdempotencyKey: key, RequestID: requestID(w), At: time.Now().UTC()})
	if err != nil {
		s.permissionError(w, r, err)
		return
	}
	// 写入后不将旧读取合并为新配置；只回执版本，界面另读当前可信快照。
	writeJSON(w, 200, map[string]any{"requestId": requestID(w), "item": result})
}

func (s *Server) permissionReadStillAllowed(w http.ResponseWriter, r *http.Request, p identity.Principal) bool {
	if r.Context().Err() != nil {
		s.permissionError(w, r, r.Context().Err())
		return false
	}
	if identity.Can(r.Context(), s.authorizer, p, identity.ScopePermissionManage, "platform") != nil {
		notFound(w, r)
		return false
	}
	return true
}

func (s *Server) permissionError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrPermissionDenied):
		notFound(w, r)
	case errors.Is(err, store.ErrPermissionSelfReadOnly):
		writeError(w, 403, "PERMISSION_SELF_READ_ONLY", "当前用户自身权限只能查看。", false)
	case errors.Is(err, store.ErrRevisionConflict):
		writeError(w, 412, "REVISION_CONFLICT", "权限已发生变化，请刷新后重新比较。", false)
	case errors.Is(err, store.ErrIdempotencyConflict):
		writeError(w, 409, "IDEMPOTENCY_CONFLICT", "本次请求标识已用于不同变更。", false)
	case errors.Is(err, store.ErrPermissionConfirmation):
		writeError(w, 422, "PERMISSION_CONFIRMATION_REQUIRED", "请核对主体、权限差异与风险并确认。", false)
	case errors.Is(err, store.ErrPermissionAdditionGated):
		writeError(w, 422, "PERMISSION_ADDITION_GATED", "该独立能力或单独节点运行范围尚未接入完整门禁，只允许保留或撤销已有记录。", false)
	case errors.Is(err, accesscontrol.ErrInactiveAddition):
		writeError(w, 409, "PERMISSION_SUBJECT_INACTIVE", "账号停用或状态未知，只能撤销已有授权。", false)
	case errors.Is(err, accesscontrol.ErrInvalid):
		writeError(w, 400, "PERMISSION_FIELDS_INVALID", "权限字段、依赖、对象状态或变更原因无效。请检查后重试。", false)
	default:
		writeError(w, 503, "PERMISSIONS_UNAVAILABLE", "权限配置暂不可用，未确认变更成功。", true)
	}
}
