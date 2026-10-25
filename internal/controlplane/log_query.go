package controlplane

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"ob-data-orch/internal/identity"
	"ob-data-orch/internal/logquery"
)

func (s *Server) logQueryEntry(w http.ResponseWriter, r *http.Request, p identity.Principal) bool {
	path := r.URL.Path
	if path != "/api/v1/logs/tasks" && path != "/api/v1/logs/task-context" && path != "/api/v1/logs/tasks:download" && path != "/api/v1/logs/task-context:copy" {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	if s.logQuery == nil {
		logQueryError(w, r, logquery.ErrUnavailable)
		return true
	}
	q, queryErr := url.ParseQuery(r.URL.RawQuery)
	if queryErr != nil {
		logQueryError(w, r, logquery.ErrInvalid)
		return true
	}
	var err error
	switch path {
	case "/api/v1/logs/tasks":
		if r.Method != http.MethodGet || r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
			notFound(w, r)
			return true
		}
		allowed := map[string]bool{"from": true, "to": true, "taskId": true, "source": true, "keyword": true, "limit": true, "cursor": true}
		for key, values := range q {
			if !allowed[key] || len(values) != 1 {
				logQueryError(w, r, logquery.ErrInvalid)
				return true
			}
		}
		var f logquery.Filter
		f.From, err = time.Parse(time.RFC3339Nano, q.Get("from"))
		if err == nil {
			f.To, err = time.Parse(time.RFC3339Nano, q.Get("to"))
		}
		if err != nil {
			logQueryError(w, r, logquery.ErrInvalid)
			return true
		}
		f.TaskID = q.Get("taskId")
		f.Source = q.Get("source")
		f.Keyword = q.Get("keyword")
		if q.Has("limit") {
			f.Limit, err = strconv.Atoi(q.Get("limit"))
			if err != nil || f.Limit < 1 {
				logQueryError(w, r, logquery.ErrInvalid)
				return true
			}
		}
		page, e := s.logQuery.Query(r.Context(), p.ID, f, q.Get("cursor"))
		if e != nil {
			logQueryError(w, r, e)
			return true
		}
		writeJSON(w, 200, map[string]any{"requestId": requestID(w), "item": page})
	case "/api/v1/logs/task-context":
		if r.Method != http.MethodGet || len(q) != 1 || len(q["token"]) != 1 || r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
			logQueryError(w, r, logquery.ErrInvalid)
			return true
		}
		item, e := s.logQuery.ReadContext(r.Context(), p.ID, q.Get("token"))
		if e != nil {
			logQueryError(w, r, e)
			return true
		}
		writeJSON(w, 200, map[string]any{"requestId": requestID(w), "item": item})
	default:
		if r.Method != http.MethodPost {
			notFound(w, r)
			return true
		}
		if s.csrf == nil {
			logQueryError(w, r, logquery.ErrUnavailable)
			return true
		}
		if s.csrf.ValidateCSRF(r) != nil {
			writeError(w, 401, "CSRF_VALIDATION_FAILED", "请求安全校验失败。", false)
			return true
		}
		if r.URL.RawQuery != "" || r.Header.Get("Content-Type") != "application/json" {
			logQueryError(w, r, logquery.ErrInvalid)
			return true
		}
		var body struct {
			Token    string `json:"token"`
			FullTask *bool  `json:"fullTask"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 12288))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || body.Token == "" || decoder.Decode(&struct{}{}) != io.EOF || path == "/api/v1/logs/task-context:copy" && body.FullTask != nil || path == "/api/v1/logs/tasks:download" && body.FullTask == nil {
			logQueryError(w, r, logquery.ErrInvalid)
			return true
		}
		if path == "/api/v1/logs/task-context:copy" {
			text, e := s.logQuery.CopyContext(r.Context(), p.ID, body.Token, requestID(w))
			if e != nil {
				logQueryError(w, r, e)
				return true
			}
			writeJSON(w, 200, map[string]any{"requestId": requestID(w), "text": text})
			return true
		}
		data, e := s.logQuery.Download(r.Context(), p.ID, body.Token, requestID(w), *body.FullTask)
		if e != nil {
			logQueryError(w, r, e)
			return true
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="task-logs.log"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(200)
		_, _ = w.Write(data)
	}
	return true
}

func logQueryError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, logquery.ErrDenied):
		notFound(w, r)
	case errors.Is(err, logquery.ErrInvalid):
		writeError(w, 400, "LOG_QUERY_INVALID", "请检查时间、任务 ID、通道与关键字；跨任务查询最多 24 小时。", false)
	case errors.Is(err, logquery.ErrCursor):
		writeError(w, 409, "LOG_QUERY_SNAPSHOT_INVALID", "日志快照已失效，请重新查询。", false)
	case errors.Is(err, logquery.ErrLimit):
		writeError(w, 422, "LOG_DOWNLOAD_LIMIT", "下载超过 100000 条或 100 MiB，请缩小条件。", false)
	default:
		writeError(w, 503, "LOG_QUERY_UNAVAILABLE", "脱敏日志暂时无法读取，请保留条件后重试。", true)
	}
}
