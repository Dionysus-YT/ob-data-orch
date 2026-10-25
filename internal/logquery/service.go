package logquery

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ob-data-orch/internal/logstream"
)

const (
	queryRecords          = 50000
	queryBytes      int64 = 64 << 20
	downloadRecords       = 100000
	downloadBytes   int64 = 100 << 20
	cursorLifetime        = time.Hour
)

// Service 是查询会话与限制的唯一所有者；进程重启使旧游标失效，不能降级信任明文游标。
type Service struct {
	index  Index
	reader Reader
	cipher cipher.AEAD
	now    func() time.Time
}

// New 创建进程内 AEAD 游标密钥；熵源或正文依赖不可用时失败关闭。
func New(index Index, reader Reader) (*Service, error) {
	if index == nil || reader == nil {
		return nil, ErrUnavailable
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, ErrUnavailable
	}
	block, err := aes.NewCipher(key)
	clear(key)
	if err != nil {
		return nil, ErrUnavailable
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrUnavailable
	}
	return &Service{index: index, reader: reader, cipher: aead, now: time.Now}, nil
}

type cursor struct {
	Version     int
	Kind        string
	Subject     string
	Fingerprint string
	Filter      Filter
	Watermark   int64
	SnapshotAt  time.Time
	Key         string
	Offset      int
	Include     bool
	Scanned     int
	Bytes       int64
	TaskID      string
	StreamID    string
	Sequence    int64
	AllRetained bool
	Incomplete  bool
}

func validFilter(f Filter) bool {
	if f.From.IsZero() || !f.From.Before(f.To) || f.From.Year() < 1970 || f.To.Year() > 9999 || f.TaskID == "" && f.To.Sub(f.From) > 24*time.Hour || f.Limit < 1 || f.Limit > 500 {
		return false
	}
	if len(f.TaskID) > 200 || len(f.Keyword) > 256 || !utf8.ValidString(f.Keyword) {
		return false
	}
	for _, r := range f.TaskID {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("-_.", r) {
			return false
		}
	}
	for _, r := range f.Keyword {
		if unicode.IsControl(r) {
			return false
		}
	}
	return f.Source == "" || slices.Contains([]string{"OBDUMPER_STDOUT", "OBDUMPER_STDERR", "OBDUMPER_FILE", "OBLOADER_STDOUT", "OBLOADER_STDERR"}, f.Source)
}

func (s *Service) seal(c cursor) (string, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return "", ErrUnavailable
	}
	nonce := make([]byte, s.cipher.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", ErrUnavailable
	}
	return base64.RawURLEncoding.EncodeToString(s.cipher.Seal(nonce, nonce, raw, []byte("task-log-query-v1"))), nil
}

func (s *Service) open(value, subject, kind string) (cursor, error) {
	var c cursor
	if len(value) > 8192 {
		return c, ErrCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) < s.cipher.NonceSize()+s.cipher.Overhead() {
		return c, ErrCursor
	}
	raw, err = s.cipher.Open(nil, raw[:s.cipher.NonceSize()], raw[s.cipher.NonceSize():], []byte("task-log-query-v1"))
	if err != nil || json.Unmarshal(raw, &c) != nil || c.Version != 1 || c.Subject != subject || c.Kind != kind || !validFilter(c.Filter) || c.Watermark < 0 || c.Offset < 0 || c.Scanned < 0 || c.Bytes < 0 || c.SnapshotAt.After(s.now().UTC()) || s.now().Sub(c.SnapshotAt) > cursorLifetime {
		return cursor{}, ErrCursor
	}
	return c, nil
}

func (s *Service) check(ctx context.Context, c cursor) (Scope, error) {
	scope, err := s.index.LogQueryScope(ctx, c.Subject, c.Filter)
	if err != nil {
		return scope, err
	}
	if scope.Fingerprint != c.Fingerprint {
		return scope, ErrCursor
	}
	return scope, nil
}

// Query 从授权索引开始读取固定水位，关键字永远只接触已验证、已二次脱敏的正文。
func (s *Service) Query(ctx context.Context, subject string, f Filter, value string) (Page, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if f.Limit == 0 {
		f.Limit = 200
	}
	f.From = f.From.UTC()
	f.To = f.To.UTC()
	if !validFilter(f) {
		return Page{}, ErrInvalid
	}
	var c cursor
	var scope Scope
	var err error
	if value == "" {
		scope, err = s.index.LogQueryScope(ctx, subject, f)
		c = cursor{Version: 1, Kind: "page", Subject: subject, Fingerprint: scope.Fingerprint, Filter: f, Watermark: scope.Watermark, SnapshotAt: s.now().UTC()}
	} else {
		c, err = s.open(value, subject, "page")
		if err == nil && c.Filter != f {
			err = ErrCursor
		}
		if err == nil {
			scope, err = s.check(ctx, c)
		}
	}
	if err != nil {
		return Page{}, err
	}
	start := c
	start.Kind = "snapshot"
	start.Key = ""
	start.Offset = 0
	start.Include = false
	start.Scanned = 0
	start.Bytes = 0
	token, err := s.seal(start)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: []Item{}, SnapshotToken: token, SnapshotAt: c.SnapshotAt, Sources: scope.Sources, Expired: scope.Expired, Availability: "RETAINED"}
	if !scope.Retained {
		if scope.Expired {
			page.Availability = "EXPIRED"
		} else {
			page.Availability = "NOT_COLLECTED"
		}
	}
	page.PageCursor, err = s.seal(c)
	if err != nil {
		return Page{}, err
	}
	items, next, incomplete, truncated, err := s.scan(ctx, &c, f.Limit, queryRecords, queryBytes, true)
	// 失败同样先复验授权，不能通过损坏错误与无权错误的差异发现对象。
	if _, authErr := s.check(ctx, c); authErr != nil {
		return Page{}, authErr
	}
	if err != nil {
		if errors.Is(err, ErrDenied) || errors.Is(err, ErrCursor) {
			return Page{}, err
		}
		return Page{}, ErrUnavailable
	}
	page.Items = items
	page.Incomplete = incomplete
	page.Truncated = truncated
	page.ScannedRecords = c.Scanned
	page.ScannedBytes = c.Bytes
	if next && !truncated {
		page.NextCursor, err = s.seal(c)
	}
	if err != nil || ctx.Err() != nil {
		return Page{}, ErrUnavailable
	}
	return page, nil
}

// scan 的累计预算随加密游标传递；分页不能重置扫描预算或把截断伪装为全部结果。
func (s *Service) scan(ctx context.Context, c *cursor, limit, maxRecords int, maxBytes int64, tokens bool) ([]Item, bool, bool, bool, error) {
	items := make([]Item, 0, min(limit, 500))
	incomplete := c.Incomplete
	for {
		if err := ctx.Err(); err != nil {
			return nil, false, false, false, err
		}
		candidates, err := s.index.LogQueryBatches(ctx, Window{SubjectID: c.Subject, Fingerprint: c.Fingerprint, Filter: c.Filter, Watermark: c.Watermark, BeforeKey: c.Key, Include: c.Include, AllRetained: c.AllRetained})
		if err != nil {
			return nil, false, false, false, err
		}
		for bi, candidate := range candidates {
			records, err := s.reader.ReadIndexedBatch(ctx, candidate.TaskID, candidate.Batch)
			if err != nil {
				return nil, false, false, false, err
			}
			if len(records) == 0 {
				return nil, false, false, false, ErrUnavailable
			}
			start := 0
			if c.Include && c.Key == candidate.Key {
				start = c.Offset
			}
			if start > len(records) {
				return nil, false, false, false, ErrCursor
			}
			for offset := start; offset < len(records); offset++ {
				r := records[len(records)-1-offset]
				raw, _ := json.Marshal(r)
				if c.Scanned >= maxRecords || c.Bytes+int64(len(raw)) > maxBytes {
					return items, false, incomplete, true, nil
				}
				c.Scanned++
				c.Bytes += int64(len(raw))
				c.Key = candidate.Key
				c.Offset = offset + 1
				c.Include = c.Offset < len(records)
				if !c.Include {
					c.Offset = 0
				}
				incomplete = incomplete || r.Kind != logstream.RecordLog
				c.Incomplete = incomplete
				if !strings.Contains(r.Message, c.Filter.Keyword) {
					continue
				}
				item := project(candidate, r)
				if tokens {
					rc := *c
					rc.Kind = "record"
					rc.TaskID = candidate.TaskID
					rc.StreamID = candidate.Batch.StreamID
					rc.Sequence = r.SourceSeq
					item.Token, err = s.seal(rc)
					if err != nil {
						return nil, false, false, false, err
					}
				}
				items = append(items, item)
				if len(items) == limit {
					return items, c.Include || bi+1 < len(candidates) || len(candidates) == 64, incomplete, false, nil
				}
			}
			c.Key = candidate.Key
			c.Include = false
			c.Offset = 0
		}
		if len(candidates) < 64 {
			return items, false, incomplete, false, nil
		}
	}
}

func project(c Candidate, r logstream.Record) Item {
	return Item{TaskID: c.TaskID, Source: string(r.SourceKind), StreamID: c.Batch.StreamID, Epoch: r.SourceEpoch, Sequence: r.SourceSeq, Kind: string(r.Kind), Message: r.Message, IntegrityCode: r.IntegrityCode, ReceivedAt: r.ReceivedAt}
}

// ReadContext 只按已签发记录的同一内部 stream/epoch 取序号窗口，不聚合相近时间的其他通道。
func (s *Service) ReadContext(ctx context.Context, subject, value string) (result Context, err error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c, err := s.open(value, subject, "record")
	if err != nil {
		return Context{}, err
	}
	defer func() {
		if _, e := s.check(ctx, c); e != nil {
			result = Context{}
			err = e
		}
	}()
	if _, err = s.check(ctx, c); err != nil {
		return Context{}, err
	}
	f := c.Filter
	f.TaskID = c.TaskID
	f.Source = ""
	f.Keyword = ""
	candidates, err := s.index.LogQueryBatches(ctx, Window{SubjectID: subject, Fingerprint: c.Fingerprint, Filter: f, Watermark: c.Watermark, StreamID: c.StreamID, MinSeq: max(int64(1), c.Sequence-50), MaxSeq: c.Sequence + 50})
	if err != nil {
		return Context{}, err
	}
	result = Context{Items: []Item{}, SelectedSequence: c.Sequence, SnapshotAt: c.SnapshotAt}
	found := false
	scannedBytes := int64(0)
	scannedRecords := 0
	for _, candidate := range candidates {
		scannedBytes += candidate.Batch.SegmentOffsetEnd - candidate.Batch.SegmentOffsetStart
		scannedRecords += candidate.Batch.RecordCount
		if scannedBytes > queryBytes || scannedRecords > queryRecords {
			return Context{}, ErrUnavailable
		}
		records, err := s.reader.ReadIndexedBatch(ctx, candidate.TaskID, candidate.Batch)
		if err != nil {
			return Context{}, ErrUnavailable
		}
		for _, r := range records {
			if r.SourceSeq < c.Sequence-50 || r.SourceSeq > c.Sequence+50 {
				continue
			}
			found = found || r.SourceSeq == c.Sequence
			result.Items = append(result.Items, project(candidate, r))
			result.Incomplete = result.Incomplete || r.Kind != logstream.RecordLog
		}
	}
	if !found || len(result.Items) > 101 {
		return Context{}, ErrUnavailable
	}
	sort.Slice(result.Items, func(i, j int) bool { return result.Items[i].Sequence < result.Items[j].Sequence })
	if _, err = s.check(ctx, c); err != nil {
		return Context{}, err
	}
	return result, nil
}

// CopyContext 在释放上下文文本前记录有界批量复制；审计失败时不返回可复制内容。
func (s *Service) CopyContext(ctx context.Context, subject, token, request string) (text string, err error) {
	c, e := s.open(token, subject, "record")
	result := Context{}
	if e == nil {
		result, e = s.ReadContext(ctx, subject, token)
	}
	audit := Audit{SubjectID: subject, RequestID: request, Action: "TASK_LOG_CONTEXT_COPY", Result: "FAILED", Filter: c.Filter, At: s.now().UTC(), Fingerprint: c.Fingerprint}
	if e == nil {
		audit.Result = "SUCCEEDED"
		audit.Count = len(result.Items)
	}
	auditCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer stop()
	if auditErr := s.index.AuditLogQuery(auditCtx, audit); auditErr != nil {
		return "", ErrUnavailable
	}
	if e != nil {
		return "", e
	}
	if _, e = s.check(ctx, c); e != nil {
		return "", e
	}
	var out strings.Builder
	for _, item := range result.Items {
		out.WriteString(formatLine(item))
	}
	return out.String(), nil
}

// Download 生成固定快照的 UTF-8 脱敏文件；不完整或超限时拒绝整个下载，绝不静默导出部分结果。
func (s *Service) Download(ctx context.Context, subject, token, request string, fullTask bool) (data []byte, err error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c, err := s.open(token, subject, "snapshot")
	audit := Audit{SubjectID: subject, RequestID: request, Action: "TASK_LOG_DOWNLOAD", Result: "FAILED", Filter: c.Filter, At: s.now().UTC(), Fingerprint: c.Fingerprint}
	defer func() {
		// 查询超时或用户断开后仍以独立短上下文记录失败，审计不可用时不释放正文。
		auditCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer stop()
		if c.Version == 1 {
			if _, e := s.check(auditCtx, c); e != nil {
				data = nil
				err = e
			}
		}
		if err == nil {
			audit.Result = "SUCCEEDED"
		}
		if e := s.index.AuditLogQuery(auditCtx, audit); e != nil {
			data = nil
			err = ErrUnavailable
		}
		if err == nil {
			if _, e := s.check(auditCtx, c); e != nil {
				data = nil
				err = e
			}
		}
	}()
	if err != nil {
		return nil, err
	}
	scope, scopeErr := s.check(ctx, c)
	if scopeErr != nil {
		return nil, scopeErr
	}
	if ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	if fullTask {
		if c.Filter.TaskID == "" {
			return nil, ErrInvalid
		}
		c.Filter.From = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		c.Filter.To = c.SnapshotAt
		c.Filter.Source = ""
		c.Filter.Keyword = ""
		c.AllRetained = true
		audit.Filter = c.Filter
	}
	var out bytes.Buffer
	mode := "FILTERED"
	if fullTask {
		mode = "FULL_RETAINED_TASK"
	}
	keyword := sha256.Sum256([]byte(c.Filter.Keyword))
	fmt.Fprintf(&out, "# OB Data Orch · 已脱敏任务日志\n# generatedAt=%s timezone=UTC snapshotAt=%s\n# mode=%s taskId=%s from=%s to=%s source=%s keywordSHA256=%s\n# expiredContentExcluded=%t\n# 仅包含固定水位前已登记的保留日志；来源时间及级别未取得；不承诺采集完整。\n", s.now().UTC().Format(time.RFC3339Nano), c.SnapshotAt.Format(time.RFC3339Nano), mode, c.Filter.TaskID, c.Filter.From.Format(time.RFC3339Nano), c.Filter.To.Format(time.RFC3339Nano), c.Filter.Source, hex.EncodeToString(keyword[:]), scope.Expired)
	incomplete := false
	sources := map[string]bool{}
	for {
		var items []Item
		var next, truncated, partial bool
		items, next, partial, truncated, err = s.scan(ctx, &c, 500, downloadRecords, downloadBytes, false)
		if err != nil {
			return nil, ErrUnavailable
		}
		if truncated {
			return nil, ErrLimit
		}
		incomplete = incomplete || partial
		for _, item := range items {
			sources[item.Source] = true
			out.WriteString(formatLine(item))
			audit.Count++
			if out.Len() > int(downloadBytes) {
				return nil, ErrLimit
			}
		}
		if !next {
			break
		}
	}
	sourceNames := make([]string, 0, len(sources))
	for source := range sources {
		sourceNames = append(sourceNames, source)
	}
	sort.Strings(sourceNames)
	fmt.Fprintf(&out, "# sources=%s\n", strings.Join(sourceNames, ","))
	fmt.Fprintf(&out, "# records=%d incomplete=%t scanRecords=%d scanBytes=%d truncated=false\n", audit.Count, incomplete, c.Scanned, c.Bytes)
	if out.Len() > int(downloadBytes) {
		return nil, ErrLimit
	}
	if _, err = s.check(ctx, c); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func formatLine(item Item) string {
	return fmt.Sprintf("[%s receivedAt] [%s] task=%s stream=%s epoch=%d seq=%d kind=%s integrity=%s\n  %s\n", item.ReceivedAt.UTC().Format(time.RFC3339Nano), item.Source, item.TaskID, item.StreamID, item.Epoch, item.Sequence, item.Kind, item.IntegrityCode, strings.ReplaceAll(item.Message, "\n", "\n  "))
}
