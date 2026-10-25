package store

import (
	"encoding/json"
	"io"
	"strings"

	"ob-data-orch/internal/directload"
)

// strictDirectLoadSnapshotJSON 在解码前保留原始键，拒绝重复、大小写别名与任何空值。
// 快照只接受当前旁路固定结构，避免 encoding/json 的宽松兼容悄悄替换冻结证明。
func strictDirectLoadSnapshotJSON(raw string) bool {
	root, ok := directLoadSnapshotObject([]byte(raw), "configVersion", "taskType", "taskName", "config", "inputManifest", "precheckProof", "directLoadConfigFingerprint")
	if !ok {
		return false
	}
	if _, err := directload.Decode(string(root["config"])); err != nil {
		return false
	}
	manifest, ok := directLoadSnapshotObject(root["inputManifest"], "metadataDigest", "files", "fileCount", "totalBytes", "digest")
	if !ok {
		return false
	}
	proof, ok := directLoadSnapshotObject(root["precheckProof"], "id", "draftRevision", "configFingerprint", "results")
	if !ok {
		return false
	}
	for _, array := range []struct {
		raw  json.RawMessage
		keys []string
	}{
		{manifest["files"], []string{"path", "kind", "object", "size", "modified", "sha256"}},
		{proof["results"], []string{"check", "status", "evidenceCode"}},
	} {
		var entries []json.RawMessage
		if json.Unmarshal(array.raw, &entries) != nil || entries == nil {
			return false
		}
		for _, entry := range entries {
			if _, ok := directLoadSnapshotObject(entry, array.keys...); !ok {
				return false
			}
		}
	}
	return true
}

func directLoadSnapshotObject(raw []byte, allowed ...string) (map[string]json.RawMessage, bool) {
	d := json.NewDecoder(strings.NewReader(string(raw)))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return nil, false
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err := d.Token()
		name, ok := token.(string)
		if err != nil || !ok || !oneOf(name, allowed...) || fields[name] != nil {
			return nil, false
		}
		var value json.RawMessage
		if d.Decode(&value) != nil || string(value) == "null" {
			return nil, false
		}
		fields[name] = value
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') || d.Decode(new(any)) != io.EOF {
		return nil, false
	}
	return fields, true
}
