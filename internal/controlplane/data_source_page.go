package controlplane

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
)

// dataSourcePage 仅处理已逐项授权的安全投影，禁止将未授权记录传入筛选和数量计算。
func dataSourcePage(items []dataSourceListResponse, query url.Values, subject string) ([]dataSourceListResponse, string, int, error) {
	invalid := errors.New("invalid data source query")
	allowed := map[string][]string{"limit": {"10"}, "keyword": nil, "cursor": nil, "environment": {"DEVELOPMENT", "TEST", "STAGING", "PRODUCTION"}, "state": {"ENABLED", "DISABLED"}, "compatibilityMode": {"MYSQL", "ORACLE"}, "connectionStatus": {"UNTESTED", "TESTING", "SUCCEEDED", "FAILED", "INVALIDATED", "EXPIRED", "UNKNOWN"}}
	for key, values := range query {
		choices, ok := allowed[key]
		if !ok || len(values) != 1 || len(values[0]) > 2048 {
			return nil, "", 0, invalid
		}
		if values[0] != "" && choices != nil {
			found := false
			for _, choice := range choices {
				if choice == values[0] {
					found = true
				}
			}
			if !found {
				return nil, "", 0, invalid
			}
		}
	}
	if query.Get("limit") != "10" {
		return nil, "", 0, invalid
	}
	// 游标绑定调用者和筛选条件；每次请求仍重新授权，不把游标作为权限凭证。
	scope := url.Values{}
	for key, value := range query {
		if key != "cursor" {
			scope[key] = value
		}
	}
	digest := sha256.Sum256([]byte(subject + "\x00" + scope.Encode()))
	binding := hex.EncodeToString(digest[:])
	type cursor struct {
		Scope string
		After string
	}
	after := ""
	if raw := query.Get("cursor"); raw != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		var position cursor
		if err != nil || json.Unmarshal(decoded, &position) != nil || position.Scope != binding || position.After == "" {
			return nil, "", 0, invalid
		}
		after = position.After
	}
	matches := make([]dataSourceListResponse, 0, len(items))
	keyword := strings.ToLower(strings.TrimSpace(query.Get("keyword")))
	for _, item := range items {
		if keyword != "" && !strings.Contains(strings.ToLower(item.DisplayName), keyword) && !strings.Contains(strings.ToLower(item.Host), keyword) && !strings.Contains(strings.ToLower(item.ClusterName), keyword) && !strings.Contains(strings.ToLower(item.TenantName), keyword) {
			continue
		}
		if value := query.Get("environment"); value != "" && item.Environment != value {
			continue
		}
		if value := query.Get("state"); value != "" && item.State != value {
			continue
		}
		if value := query.Get("compatibilityMode"); value != "" && item.CompatibilityMode != value {
			continue
		}
		status := item.LastTestStatus
		if status == "" {
			status = "UNTESTED"
		}
		if status == "PENDING" || status == "LEASED" {
			status = "TESTING"
		}
		if status == "EXPIRED" && query.Get("connectionStatus") == "INVALIDATED" {
			status = "INVALIDATED"
		}
		if value := query.Get("connectionStatus"); value != "" && value != status {
			continue
		}
		matches = append(matches, item)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].ID < matches[j].ID })
	total := len(matches)
	start := sort.Search(len(matches), func(i int) bool { return matches[i].ID > after })
	end := start + 10
	next := ""
	if end < len(matches) {
		raw, _ := json.Marshal(cursor{Scope: binding, After: matches[end-1].ID})
		next = base64.RawURLEncoding.EncodeToString(raw)
	} else {
		end = len(matches)
	}
	return matches[start:end], next, total, nil
}
