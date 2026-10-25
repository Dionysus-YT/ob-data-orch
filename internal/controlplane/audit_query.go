package controlplane

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"ob-data-orch/internal/auditquery"
	"ob-data-orch/internal/identity"
)

func (s *Server) auditQueryEntry(w http.ResponseWriter, r *http.Request, p identity.Principal) bool {
	path := r.URL.Path
	if path != "/api/v1/logs/audit-access" && path != "/api/v1/logs/audit" && path != "/api/v1/logs/audit-detail" && path != "/api/v1/logs/audit:download" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if s.auditQuery == nil {
		auditQueryError(w, r, auditquery.ErrUnavailable)
		return true
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		auditQueryError(w, r, auditquery.ErrInvalid)
		return true
	}
	if path != "/api/v1/logs/audit:download" && (r.Method != http.MethodGet || r.ContentLength != 0 || len(r.TransferEncoding) > 0) {
		notFound(w, r)
		return true
	}
	switch path {
	case "/api/v1/logs/audit-access":
		if len(q) > 0 {
			auditQueryError(w, r, auditquery.ErrInvalid)
			return true
		}
		allowed, e := s.auditQuery.Access(r.Context(), p.ID)
		if e != nil {
			auditQueryError(w, r, e)
			return true
		}
		writeJSON(w, 200, map[string]any{"requestId": requestID(w), "allowed": allowed})
	case "/api/v1/logs/audit":
		allowed := map[string]bool{"from": true, "to": true, "actorId": true, "objectType": true, "objectId": true, "action": true, "result": true, "keyword": true, "limit": true, "cursor": true}
		for key, values := range q {
			if !allowed[key] || len(values) != 1 {
				auditQueryError(w, r, auditquery.ErrInvalid)
				return true
			}
		}
		f := auditquery.Filter{ActorID: q.Get("actorId"), ObjectType: q.Get("objectType"), ObjectID: q.Get("objectId"), Action: q.Get("action"), Result: q.Get("result"), Keyword: q.Get("keyword")}
		f.From, err = time.Parse(time.RFC3339Nano, q.Get("from"))
		if err == nil {
			f.To, err = time.Parse(time.RFC3339Nano, q.Get("to"))
		}
		if err == nil && q.Has("limit") {
			f.Limit, err = strconv.Atoi(q.Get("limit"))
			if err == nil && f.Limit < 1 {
				err = auditquery.ErrInvalid
			}
		}
		if err != nil {
			auditQueryError(w, r, auditquery.ErrInvalid)
			return true
		}
		page, e := s.auditQuery.Query(r.Context(), p.ID, requestID(w), f, q.Get("cursor"))
		if e != nil {
			auditQueryError(w, r, e)
			return true
		}
		writeJSON(w, 200, map[string]any{"requestId": requestID(w), "item": page})
	case "/api/v1/logs/audit-detail":
		if len(q) != 1 || len(q["token"]) != 1 {
			auditQueryError(w, r, auditquery.ErrInvalid)
			return true
		}
		item, e := s.auditQuery.Detail(r.Context(), p.ID, requestID(w), q.Get("token"))
		if e != nil {
			auditQueryError(w, r, e)
			return true
		}
		writeJSON(w, 200, map[string]any{"requestId": requestID(w), "item": item})
	default:
		if r.Method != http.MethodPost {
			notFound(w, r)
			return true
		}
		if s.csrf == nil {
			auditQueryError(w, r, auditquery.ErrUnavailable)
			return true
		}
		if s.csrf.ValidateCSRF(r) != nil {
			writeError(w, 401, "CSRF_VALIDATION_FAILED", "请求安全校验失败。", false)
			return true
		}
		if r.URL.RawQuery != "" || r.Header.Get("Content-Type") != "application/json" {
			auditQueryError(w, r, auditquery.ErrInvalid)
			return true
		}
		var body struct {
			Token string `json:"token"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 12288))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || body.Token == "" || decoder.Decode(&struct{}{}) != io.EOF {
			auditQueryError(w, r, auditquery.ErrInvalid)
			return true
		}
		data, e := s.auditQuery.Download(r.Context(), p.ID, requestID(w), body.Token)
		if e != nil {
			auditQueryError(w, r, e)
			return true
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="audit-logs.log"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(200)
		_, _ = w.Write(data)
	}
	return true
}

func auditQueryError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auditquery.ErrDenied):
		notFound(w, r)
	case errors.Is(err, auditquery.ErrInvalid):
		writeError(w, 400, "AUDIT_QUERY_INVALID", "请检查时间、精确标识和关键字；审计查询最多 24 小时。", false)
	case errors.Is(err, auditquery.ErrCursor):
		writeError(w, 409, "AUDIT_QUERY_SNAPSHOT_INVALID", "审计快照已失效，请重新查询。", false)
	case errors.Is(err, auditquery.ErrLimit):
		writeError(w, 422, "AUDIT_DOWNLOAD_LIMIT", "审计下载达到扫描或文件限制，请缩小条件。", false)
	default:
		writeError(w, 503, "AUDIT_QUERY_UNAVAILABLE", "审计查询暂不可用，请保留条件后重试。", true)
	}
}
