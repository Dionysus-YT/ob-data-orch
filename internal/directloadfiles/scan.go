// Package directloadfiles 验证旁路单表文件证据，不使用普通导入对象过滤或配置转换。
package directloadfiles

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/importfiles"
)

// Scan 完整扫描当前格式的文件；其他表的同格式文件也会进入集合并阻断，不能先过滤掉再宣称单表。
func Scan(ctx context.Context, c directload.Config, roots []string) (importfiles.Manifest, error) {
	m, err := importfiles.ScanMatched(ctx, c.FilePath, roots, func(name string) (string, string, bool) { return classify(name, c.Format) })
	if err != nil || !Valid(m, c) {
		return importfiles.Manifest{}, errors.New("旁路输入集合无法证明为同一目标表")
	}
	return m, nil
}

// Valid 重新核对清单摘要、唯一表、顺序、相对路径和总量，不信任 Agent 的单独成功标志。
func Valid(m importfiles.Manifest, c directload.Config) bool {
	if c.Table == "" || m.FileCount < 1 || m.FileCount > importfiles.MaxFiles || m.FileCount != len(m.Files) {
		return false
	}
	previous := ""
	total := int64(0)
	for _, f := range m.Files {
		if f.Path == "" || len(f.Path) > 512 || f.Path <= previous || strings.ContainsAny(f.Path, `\:`) || strings.HasPrefix(f.Path, "/") || strings.ContainsFunc(f.Path, unicode.IsControl) || filepath.ToSlash(filepath.Clean(f.Path)) != f.Path || f.Path == ".." || strings.HasPrefix(f.Path, "../") || f.Size < 0 || total > int64(^uint64(0)>>1)-f.Size {
			return false
		}
		kind, object, ok := classify(filepath.Base(f.Path), c.Format)
		if !ok || f.Kind != kind || f.Object != object || object != c.Table {
			return false
		}
		if value, err := hex.DecodeString(f.SHA256); err != nil || len(value) != sha256.Size {
			return false
		}
		previous = f.Path
		total += f.Size
	}
	if m.MetadataDigest != "" {
		if value, err := hex.DecodeString(m.MetadataDigest); err != nil || len(value) != sha256.Size {
			return false
		}
	}
	raw, _ := json.Marshal(m.Files)
	if m.MetadataDigest != "" {
		raw = append(append(raw, '\n'), []byte(m.MetadataDigest)...)
	}
	digest := sha256.Sum256(raw)
	return total == m.TotalBytes && m.Digest == hex.EncodeToString(digest[:])
}

func classify(name, format string) (string, string, bool) {
	suffix := map[string]string{"CSV": ".csv", "SQL": ".sql", "PARQUET": ".parquet", "ORC": ".orc"}[format]
	if suffix == "" || !strings.HasSuffix(name, suffix) || strings.HasSuffix(name, "-schema.sql") {
		return "", "", false
	}
	object := strings.TrimSuffix(name, suffix)
	for {
		index := strings.LastIndex(object, ".")
		if index < 0 {
			break
		}
		if _, err := strconv.ParseUint(object[index+1:], 10, 64); err != nil {
			break
		}
		object = object[:index]
	}
	return "DATA", object, object != "" && len(object) <= 256 && !strings.ContainsFunc(object, unicode.IsControl)
}
