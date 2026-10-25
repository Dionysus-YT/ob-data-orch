package auditquery

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// objectTypes 仅登记已有消费者能按当前对象权限裁剪的事件域，不为未来模块建空壳。
var objectTypes = []string{"TASK", "TASK_EXECUTION", "DATA_SOURCE", "EXECUTION_NODE", "AGENT", "EXPORT_DRAFT", "SUBJECT", "PLATFORM_SETTING", "STORAGE_CREDENTIAL", "LOG_QUERY"}

func safeID(value string, empty bool) bool {
	if value == "" {
		return empty
	}
	if len(value) > 200 || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("-_.:@", r) {
			return false
		}
	}
	return true
}

func code(value string, empty bool) bool {
	if value == "" {
		return empty
	}
	if len(value) > 100 {
		return false
	}
	for _, r := range value {
		if r != '_' && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// project 只公开登记的整数/布尔事实；即使历史 safe_diff_json 误含秘密也不返回未知字段。
// 对象名、原因、角色差异和其他任意字符串没有可证明的通用脱敏语义，首批明确省略。
func project(e Event) (Item, error) {
	if !safeID(e.ID, false) || !safeID(e.ActorID, false) || !safeID(e.ObjectID, true) || !safeID(e.RequestID, false) || !code(e.Action, false) || !code(e.ObjectType, false) || !oneOf(e.ActorType, "SUBJECT", "AGENT", "SYSTEM") || !oneOf(e.Result, "SUCCEEDED", "FAILED", "DENIED") || e.At.IsZero() {
		return Item{}, ErrUnavailable
	}
	out := Item{ID: e.ID, ActorType: e.ActorType, ActorID: e.ActorID, Action: e.Action, ObjectType: e.ObjectType, ObjectID: e.ObjectID, Result: e.Result, RequestID: e.RequestID, OccurredAt: e.At.UTC(), Fields: []Field{}}
	if len(e.Diff) == 0 || len(e.Diff) > 16384 {
		out.DetailsOmitted = true
		return out, nil
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(e.Diff, &raw) != nil || raw == nil {
		return Item{}, ErrUnavailable
	}
	// 固定顺序避免 map 顺序改变搜索和下载形态，拒绝负数、小数及超出 JS 精度的数字。
	names := []string{"changed", "confirmed", "redacted", "environmentInspectionAuthorized", "storageConnectivityProbeAllowed", "revision", "records", "count"}
	for _, name := range names {
		v, ok := raw[name]
		if !ok {
			continue
		}
		delete(raw, name)
		if oneOf(name, "changed", "confirmed", "redacted", "environmentInspectionAuthorized", "storageConnectivityProbeAllowed") {
			if !bytes.Equal(v, []byte("true")) && !bytes.Equal(v, []byte("false")) {
				return Item{}, ErrUnavailable
			}
		} else {
			n, err := strconv.ParseInt(string(v), 10, 64)
			if err != nil || n < 0 || n > 9007199254740991 {
				return Item{}, ErrUnavailable
			}
		}
		out.Fields = append(out.Fields, Field{name, string(v)})
	}
	// 这些固定关联字段已有独立列或只包含内部范围信息，也不作为详情字段返回。
	delete(raw, "objectId")
	out.DetailsOmitted = len(raw) > 0
	return out, nil
}

func oneOf(v string, values ...string) bool {
	for _, x := range values {
		if v == x {
			return true
		}
	}
	return false
}
func text(i Item) string {
	var b strings.Builder
	b.WriteString(i.OccurredAt.Format("2006-01-02T15:04:05.999999999Z") + " · " + i.ActorType + " " + i.ActorID + " · " + i.Action + " · " + i.ObjectType + " " + i.ObjectID + " · " + i.Result + " · " + i.ID + " · " + i.RequestID)
	for _, f := range i.Fields {
		b.WriteString("\n  " + f.Name + "=" + f.Value)
	}
	if i.DetailsOmitted {
		b.WriteString("\n  部分差异未纳入安全投影")
	}
	return b.String()
}
