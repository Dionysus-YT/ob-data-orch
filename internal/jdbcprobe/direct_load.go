package jdbcprobe

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"regexp"
	"time"
	"unicode"
	"unicode/utf8"

	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/nodefacts"
)

const directLoadMainClass = "com.obdataorch.jdbcprobe.DirectLoadProbe"

// DirectLoadRequest 是固定 v11 旁路只读探针输入，不接受 SQL、RPC 地址、文件或系统租户凭据。
type DirectLoadRequest struct {
	Connection                Request
	CompatibilityMode         CompatibilityMode
	Database, Table, Scenario string
	TruncateTable             bool
}

// DirectLoadResult 只表达当前绑定的观测，不能证明旁路导入或服务端提交已经完成。
type DirectLoadResult struct {
	DatabaseAccess, WriteAccess ObjectAccess
	ObserverVersion, ODPVersion string
	TableFacts                  directload.TableFacts
	// 本机安全观测由 Agent 单独注入，Java 输出不能提供这些事实。
	ToolVersion      string
	DirectLoadConfig *nodefacts.DirectLoadConfiguration
	ObservedAt       time.Time
}

// TestDirectLoad 只运行旁路固定只读入口；真实装配需要独立开关，不调用普通导入探针。
func TestDirectLoad(ctx context.Context, runtime Runtime, request DirectLoadRequest) (DirectLoadResult, error) {
	if ctx == nil || !validRuntime(runtime) {
		return DirectLoadResult{}, ErrInvalidRuntime
	}
	if !validDirectLoadRequest(request) {
		return DirectLoadResult{}, ErrInvalidRequest
	}
	input := encodeDirectLoadRequest(request)
	defer zero(input)
	output, err := runFixedProbe(ctx, runtime, input, maxOutputBytes, 90*time.Second, directLoadMainClass)
	if err != nil {
		return DirectLoadResult{}, err
	}
	defer zero(output)
	return parseDirectLoadResponse(output)
}

func validDirectLoadRequest(r DirectLoadRequest) bool {
	return validRequest(r.Connection) && validCompatibilityMode(r.CompatibilityMode) && directLoadIdentifier(r.Database) && directLoadIdentifier(r.Table) && (r.Scenario == "PRIVATE_ODP" || r.Scenario == "OBSERVER_DIRECT")
}

func directLoadIdentifier(s string) bool {
	if s == "" || len(s) > 256 || !utf8.ValidString(s) || s[0] == '-' {
		return false
	}
	for _, c := range s {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' && c != '$' && c != '-' {
			return false
		}
	}
	return true
}

func encodeDirectLoadRequest(r DirectLoadRequest) []byte {
	base := encodeProbeRequest(11, r.Connection, [][]byte{[]byte(r.CompatibilityMode), []byte(r.Database), []byte(r.Table), []byte(r.Scenario)})
	defer zero(base)
	input := make([]byte, len(base)+4)
	copy(input, base)
	if r.TruncateTable {
		binary.BigEndian.PutUint32(input[len(base):], 1)
	}
	return input
}

var directLoadVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(\.[0-9]+)?$`)

func parseDirectLoadResponse(raw []byte) (DirectLoadResult, error) {
	// 精确键名及重复键先行校验，避免 encoding/json 的大小写别名与后值覆盖制造可信事实。
	d := json.NewDecoder(bytes.NewReader(raw))
	if !directLoadJSONKeys(d, false) || d.Decode(new(any)) != io.EOF {
		return DirectLoadResult{}, ErrProbeFailed
	}
	var v struct {
		Status          string                `json:"status"`
		DatabaseAccess  ObjectAccess          `json:"databaseAccess"`
		WriteAccess     ObjectAccess          `json:"writeAccess"`
		ObserverVersion string                `json:"observerVersion"`
		ODPVersion      string                `json:"odpVersion"`
		TableFacts      directload.TableFacts `json:"tableFacts"`
	}
	if json.Unmarshal(raw, &v) != nil || v.Status != "SUCCESS" || !validDirectLoadAccess(v.DatabaseAccess) || !validDirectLoadAccess(v.WriteAccess) {
		return DirectLoadResult{}, ErrProbeFailed
	}
	for _, version := range []string{v.ObserverVersion, v.ODPVersion} {
		if version != "" && (len(version) > 64 || !directLoadVersionPattern.MatchString(version)) {
			return DirectLoadResult{}, ErrProbeFailed
		}
	}
	if v.DatabaseAccess != ObjectAccessible && (v.WriteAccess != ObjectUnavailable || v.ObserverVersion != "" || v.ODPVersion != "" || v.TableFacts != (directload.TableFacts{})) {
		return DirectLoadResult{}, ErrProbeFailed
	}
	return DirectLoadResult{DatabaseAccess: v.DatabaseAccess, WriteAccess: v.WriteAccess, ObserverVersion: v.ObserverVersion, ODPVersion: v.ODPVersion, TableFacts: v.TableFacts}, nil
}

func validDirectLoadAccess(s ObjectAccess) bool {
	return s == ObjectAccessible || s == ObjectNotAccessible || s == ObjectUnavailable
}

func directLoadJSONKeys(d *json.Decoder, facts bool) bool {
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return false
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return false
		}
		seen[key] = true
		if !facts && key == "tableFacts" {
			if !directLoadJSONKeys(d, true) {
				return false
			}
			continue
		}
		allowed := false
		if facts {
			switch key {
			case "exists", "empty", "hasBIT", "hasBinary", "hasVirtualGenerated", "hasIndex", "hasLOB", "hasUniqueIndex":
				allowed = true
			}
		} else {
			switch key {
			case "status", "databaseAccess", "writeAccess", "observerVersion", "odpVersion":
				allowed = true
			}
		}
		if !allowed {
			return false
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return false
		}
	}
	token, err = d.Token()
	return err == nil && token == json.Delim('}') && (facts || len(seen) == 6)
}
