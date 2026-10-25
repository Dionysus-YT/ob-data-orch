package auditquery

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const scanRecords = 50000
const scanBytes int64 = 64 << 20

// Service 持有进程级快照密钥；重启、到期、主体或授权变化均使旧令牌失效。
type Service struct {
	store  Store
	cipher cipher.AEAD
	now    func() time.Time
}

// New 不依赖 Agent、文件或工具；随机源不可用时拒绝启用审计查询。
func New(store Store) (*Service, error) {
	if store == nil {
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
	return &Service{store: store, cipher: aead, now: time.Now}, nil
}

type cursor struct {
	Kind        string
	Subject     string
	Fingerprint string
	Filter      Filter
	Watermark   int64
	At          time.Time
	Before      string
	Scanned     int
	Bytes       int64
	EventID     string
}

func valid(f Filter) bool {
	if f.From.IsZero() || !f.From.Before(f.To) || f.From.Year() < 1970 || f.To.Year() > 9999 || f.To.Sub(f.From) > 24*time.Hour || f.Limit < 1 || f.Limit > 500 {
		return false
	}
	if !safeID(f.ActorID, true) || !safeID(f.ObjectID, true) || !code(f.Action, true) || f.ObjectType != "" && !slices.Contains(objectTypes, f.ObjectType) || f.Result != "" && !oneOf(f.Result, "SUCCEEDED", "FAILED", "DENIED") || len(f.Keyword) > 256 || !utf8.ValidString(f.Keyword) {
		return false
	}
	for _, r := range f.Keyword {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
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
	return base64.RawURLEncoding.EncodeToString(s.cipher.Seal(nonce, nonce, raw, []byte("audit-query-v1"))), nil
}

func (s *Service) open(token, subject, kind string) (cursor, error) {
	var c cursor
	if len(token) > 8192 {
		return c, ErrCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	n := s.cipher.NonceSize()
	if err != nil || len(raw) < n+s.cipher.Overhead() {
		return c, ErrCursor
	}
	raw, err = s.cipher.Open(nil, raw[:n], raw[n:], []byte("audit-query-v1"))
	if err != nil || json.Unmarshal(raw, &c) != nil || c.Kind != kind || c.Subject != subject || !valid(c.Filter) || c.At.After(s.now()) || s.now().Sub(c.At) > time.Hour || c.Scanned < 0 || c.Scanned > scanRecords || c.Bytes < 0 || c.Bytes > scanBytes {
		return cursor{}, ErrCursor
	}
	return c, nil
}

// Access 只返回当前主体是否具有显式能力，不读取事件候选或数量。
func (s *Service) Access(ctx context.Context, subject string) (bool, error) {
	return s.store.AuditQueryAccess(ctx, subject)
}

func (s *Service) current(ctx context.Context, c cursor) error {
	scope, err := s.store.AuditQueryScope(ctx, c.Subject)
	if err != nil {
		return err
	}
	if scope.Fingerprint != c.Fingerprint {
		return ErrCursor
	}
	return nil
}

// finish 对成功和失败都重新校验；撤权不得通过底层错误差异泄露旧对象。
// 访问审计持久化失败时撤下全部响应，不能先发送正文再补记审计。
func (s *Service) finish(ctx context.Context, c cursor, request, action string, count int, original error) error {
	if err := s.current(ctx, c); err != nil {
		return err
	}
	result := "SUCCEEDED"
	if original != nil {
		result = "FAILED"
	}
	if err := s.store.AuditQueryAccessEvent(ctx, Access{Subject: c.Subject, Fingerprint: c.Fingerprint, RequestID: request, Action: action, Result: result, Filter: c.Filter, Count: count, At: s.now().UTC()}); err != nil {
		return err
	}
	if err := s.current(ctx, c); err != nil {
		return err
	}
	return original
}

// Query 按固定追加水位读取安全投影；查询自身的审计事件在水位之后，不污染本次分页。
func (s *Service) Query(ctx context.Context, subject, request string, f Filter, token string) (Page, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	empty := Page{}
	scope, err := s.store.AuditQueryScope(ctx, subject)
	if err != nil {
		return empty, err
	}
	f.From = f.From.UTC()
	f.To = f.To.UTC()
	if f.Limit == 0 {
		f.Limit = 200
	}
	if !valid(f) || !safeID(request, false) {
		return empty, ErrInvalid
	}
	c := cursor{Kind: "page", Subject: subject, Fingerprint: scope.Fingerprint, Filter: f, Watermark: scope.Watermark, At: s.now().UTC()}
	if token != "" {
		c, err = s.open(token, subject, "page")
		if err != nil {
			return empty, err
		}
		if c.Fingerprint != scope.Fingerprint || c.Filter != f {
			return empty, ErrCursor
		}
	}
	page := Page{Items: []Item{}, SnapshotAt: c.At}
	page.PageCursor, err = s.seal(c)
	if err != nil {
		return empty, err
	}
	snapshot := c
	snapshot.Kind = "snapshot"
	snapshot.Before = ""
	snapshot.Scanned = 0
	snapshot.Bytes = 0
	page.SnapshotToken, err = s.seal(snapshot)
	if err != nil {
		return empty, err
	}
	var sealErr error
	err = s.walk(ctx, &c, func(i Item) bool {
		record := c
		record.Kind = "record"
		record.EventID = i.ID
		i.Token, sealErr = s.seal(record)
		if sealErr != nil {
			return false
		}
		page.Items = append(page.Items, i)
		return len(page.Items) < f.Limit
	})
	if err == nil {
		err = sealErr
	}
	if err == nil && c.Scanned >= scanRecords || err == nil && c.Bytes >= scanBytes {
		page.Truncated = true
	}
	if err == nil && len(page.Items) == f.Limit && !page.Truncated {
		page.NextCursor, err = s.seal(c)
	}
	page.ScannedRecords = c.Scanned
	page.ScannedBytes = c.Bytes
	if e := s.finish(ctx, c, request, "AUDIT_LOG_QUERY", len(page.Items), err); e != nil {
		return empty, e
	}
	return page, nil
}

// walk 每批至多 128 行，授权筛选在 SQLite 中先于差异读取；匹配只针对 project 的结果。
func (s *Service) walk(ctx context.Context, c *cursor, consume func(Item) bool) error {
	for c.Scanned < scanRecords && c.Bytes < scanBytes {
		if err := ctx.Err(); err != nil {
			return ErrUnavailable
		}
		rows, err := s.store.AuditQueryRows(ctx, Window{Subject: c.Subject, Fingerprint: c.Fingerprint, Filter: c.Filter, Watermark: c.Watermark, Before: c.Before, Limit: 128})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, e := range rows {
			if e.Key == "" || e.Key >= c.Before && c.Before != "" || e.Bytes < 0 {
				return ErrUnavailable
			}
			// 不能跨越字节预算后继续匹配；预算耗尽只给截断事实，不给半条记录。
			if c.Bytes+e.Bytes > scanBytes {
				c.Bytes = scanBytes
				return nil
			}
			c.Before = e.Key
			c.Scanned++
			c.Bytes += e.Bytes
			i, err := project(e)
			if err != nil {
				return err
			}
			if c.Filter.Keyword == "" || strings.Contains(text(i), c.Filter.Keyword) {
				if !consume(i) {
					return nil
				}
			}
			if c.Scanned >= scanRecords || c.Bytes >= scanBytes {
				return nil
			}
		}
	}
	return nil
}

// Detail 只读取经当前对象权限再次裁剪的单事件，拒绝把记录令牌当业务详情权限。
func (s *Service) Detail(ctx context.Context, subject, request, token string) (Item, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// 先检查当前能力，再解令牌；已撤权者不能通过令牌格式或用途错误区分旧会话。
	scope, err := s.store.AuditQueryScope(ctx, subject)
	if err != nil {
		return Item{}, err
	}
	c, err := s.open(token, subject, "record")
	if err != nil {
		return Item{}, err
	}
	if scope.Fingerprint != c.Fingerprint {
		return Item{}, ErrCursor
	}
	rows, err := s.store.AuditQueryRows(ctx, Window{Subject: subject, Fingerprint: c.Fingerprint, Filter: c.Filter, Watermark: c.Watermark, EventID: c.EventID, Limit: 1})
	var item Item
	if err == nil {
		if len(rows) != 1 {
			err = ErrDenied
		} else {
			item, err = project(rows[0])
			item.Token = token
		}
	}
	if e := s.finish(ctx, c, request, "AUDIT_LOG_DETAIL", 0, err); e != nil {
		return Item{}, e
	}
	return item, nil
}

// Download 生成同一快照的 UTF-8 安全投影；预算耗尽或审计不可用时不返回部分文件。
func (s *Service) Download(ctx context.Context, subject, request, token string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// 与详情一致，能力拒绝优先于令牌校验，避免已无权请求得到会话有效性差异。
	scope, err := s.store.AuditQueryScope(ctx, subject)
	if err != nil {
		return nil, err
	}
	c, err := s.open(token, subject, "snapshot")
	if err != nil {
		return nil, err
	}
	if scope.Fingerprint != c.Fingerprint {
		return nil, ErrCursor
	}
	f := c.Filter
	var b strings.Builder
	fmt.Fprintf(&b, "OB Data Orch 审计日志 · 已脱敏\n来源: 平台审计事件\n快照 UTC: %s\n范围 UTC: %s 至 %s\n操作人: %s\n对象: %s %s\n操作: %s\n结果: %s\n", c.At.Format(time.RFC3339Nano), f.From.Format(time.RFC3339Nano), f.To.Format(time.RFC3339Nano), f.ActorID, f.ObjectType, f.ObjectID, f.Action, f.Result)
	// 关键字可能是用户误填秘密，不复制到文件头、访问审计或错误中。
	fmt.Fprintf(&b, "关键字筛选: %t\n差异仅包含安全投影；未接入的事件域、已无权或已清理记录不在结果中。\n\n", f.Keyword != "")
	count := 0
	over := false
	err = s.walk(ctx, &c, func(i Item) bool {
		line := text(i) + "\n"
		if count >= 100000 || b.Len()+len(line) > 100<<20 {
			over = true
			return false
		}
		b.WriteString(line)
		count++
		return true
	})
	if err == nil && (over || c.Scanned >= scanRecords || c.Bytes >= scanBytes) {
		err = ErrLimit
	}
	if e := s.finish(ctx, c, request, "AUDIT_LOG_DOWNLOAD", count, err); e != nil {
		return nil, e
	}
	return []byte(b.String()), nil
}
