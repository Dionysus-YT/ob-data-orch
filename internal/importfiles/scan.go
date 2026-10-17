// Package importfiles 只检查受租约绑定的普通导入本地输入，不提供任意文件浏览或业务内容读取接口。
package importfiles

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"ob-data-orch/internal/importdomain"
)

// MaxFiles 限制一次检查与协议证据大小；超限失败关闭，不能截断后宣称完整。
const MaxFiles = 1024

// File 是受限相对路径及可复核输入事实，不包含文件内容或机器绝对路径。
type File struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Object string `json:"object"`
	Size   int64  `json:"size"`
	// 纳秒时间以字符串传输，避免浏览器浮点数破坏冻结证据。
	Modified int64  `json:"modified,string"`
	SHA256   string `json:"sha256"`
}

// Manifest 绑定完整匹配集合的稳定摘要；不是导入结果或数据库行数。
type Manifest struct {
	// MetadataDigest 绑定官方辅助清单；不反序列化 Kryo，也不让其变化逃过执行前复核。
	MetadataDigest string `json:"metadataDigest,omitempty"`
	Files          []File `json:"files"`
	FileCount      int    `json:"fileCount"`
	TotalBytes     int64  `json:"totalBytes"`
	Digest         string `json:"digest"`
}

// ExecutionInput 是普通导入冻结的配置和输入证据；不包含连接秘密或自由命令。
// 执行前必须重新扫描并核对整个清单，不能只校验路径或文件数量。
type ExecutionInput struct {
	Config   importdomain.Config `json:"config"`
	Manifest Manifest            `json:"manifest"`
}

// Scan 从允许根目录内读取匹配文件的摘要；任何链接、越界、变化、超限或取消都不能产生成功清单。
func Scan(ctx context.Context, c importdomain.Config, allowedRoots []string) (Manifest, error) {
	if ctx == nil {
		return Manifest{}, errors.New("导入检查上下文不可用")
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	inputRoot, relative, err := openBounded(c.FilePath, allowedRoots)
	if err != nil {
		return Manifest{}, errors.New("导入输入路径无法安全访问")
	}
	defer inputRoot.Close()
	var filter *regexp.Regexp
	if c.FileRegex != "" {
		filter, err = regexp.Compile(c.FileRegex)
		if err != nil {
			return Manifest{}, errors.New("文件正则不属于受控检查子集")
		}
	}
	var files []File
	entries := 0
	var walk func(string, int) error
	walk = func(name string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries++
		if entries > 8192 || depth > 16 {
			return errors.New("导入文件扫描超限")
		}
		info, err := inputRoot.Lstat(name)
		if err != nil {
			return errors.New("导入文件事实不可用")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("导入路径不允许符号链接")
		}
		if info.IsDir() {
			dir, err := inputRoot.Open(name)
			if err != nil {
				return errors.New("导入目录不可读")
			}
			children, err := dir.ReadDir(8193)
			dir.Close()
			if err != nil && !errors.Is(err, io.EOF) {
				return errors.New("导入目录无法完整枚举")
			}
			if len(children) > 8192 {
				return errors.New("导入目录扫描超限")
			}
			for _, child := range children {
				if err := walk(filepath.Join(name, child.Name()), depth+1); err != nil {
					return err
				}
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return errors.New("导入输入包含非常规文件")
		}
		kind, object, match := classify(info.Name(), c, filter)
		if !match {
			return nil
		}
		if len(files) >= MaxFiles {
			return errors.New("导入文件数量超限")
		}
		recordPath, err := filepath.Rel(relative, name)
		if err != nil {
			return errors.New("导入相对路径无效")
		}
		if recordPath == "." {
			recordPath = info.Name()
		}
		recordPath = filepath.ToSlash(recordPath)
		if !safeRelative(recordPath) {
			return errors.New("导入相对路径无效")
		}
		file, err := inputRoot.Open(name)
		if err != nil {
			return errors.New("导入文件不可读")
		}
		hash := sha256.New()
		buffer := make([]byte, 64<<10)
		for {
			if err := ctx.Err(); err != nil {
				file.Close()
				return err
			}
			n, readErr := file.Read(buffer)
			if n > 0 {
				_, _ = hash.Write(buffer[:n])
			}
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				file.Close()
				return errors.New("导入文件读取失败")
			}
		}
		after, err := file.Stat()
		file.Close()
		if err != nil || !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
			return errors.New("导入文件在检查期间变化")
		}
		files = append(files, File{Path: recordPath, Kind: kind, Object: object, Size: info.Size(), Modified: info.ModTime().UnixNano(), SHA256: hex.EncodeToString(hash.Sum(nil))})
		return nil
	}
	if err := walk(relative, 0); err != nil {
		return Manifest{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	manifest := Manifest{Files: files, FileCount: len(files)}
	manifest.MetadataDigest, err = inputMetadataDigest(ctx, inputRoot, relative)
	if err != nil {
		return Manifest{}, err
	}
	for _, file := range files {
		if file.Size < 0 || manifest.TotalBytes > int64(^uint64(0)>>1)-file.Size {
			return Manifest{}, errors.New("导入文件总量越界")
		}
		manifest.TotalBytes += file.Size
	}
	manifest.Digest = manifestDigest(files, manifest.MetadataDigest)
	if !manifest.Valid(c) {
		return Manifest{}, errors.New("没有完整匹配当前内容与对象的文件集合")
	}
	return manifest, nil
}

// Valid 在协议与控制面复核受限清单，拒绝截断、重复、错误摘要、缺失内容或任意路径。
func (m Manifest) Valid(c importdomain.Config) bool {
	if m.FileCount < 1 || m.FileCount > MaxFiles || m.FileCount != len(m.Files) {
		return false
	}
	data, ddl := 0, 0
	total := int64(0)
	previous := ""
	var filter *regexp.Regexp
	if c.FileRegex != "" {
		var err error
		filter, err = regexp.Compile(c.FileRegex)
		if err != nil {
			return false
		}
	}
	for _, file := range m.Files {
		if !safeRelative(file.Path) || file.Path <= previous || file.Size < 0 || len(file.SHA256) != 64 || len(file.Object) == 0 || strings.ContainsFunc(file.Object, unicode.IsControl) {
			return false
		}
		if _, err := hex.DecodeString(file.SHA256); err != nil {
			return false
		}
		kind, object, matches := classify(filepath.Base(file.Path), c, filter)
		if !matches || kind != file.Kind || object != file.Object {
			return false
		}
		previous = file.Path
		if total > int64(^uint64(0)>>1)-file.Size {
			return false
		}
		total += file.Size
		switch file.Kind {
		case "DATA":
			data++
		case "DDL":
			ddl++
		default:
			return false
		}
	}
	if m.MetadataDigest != "" {
		decoded, err := hex.DecodeString(m.MetadataDigest)
		if err != nil || len(decoded) != sha256.Size {
			return false
		}
	}
	return total == m.TotalBytes && manifestDigest(m.Files, m.MetadataDigest) == m.Digest && (!c.HasData() || data > 0) && (!c.HasDDL() || ddl > 0)
}

func manifestDigest(files []File, metadata string) string {
	raw, _ := json.Marshal(files)
	if metadata != "" {
		raw = append(append(raw, '\n'), []byte(metadata)...)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func inputMetadataDigest(ctx context.Context, root *os.Root, input string) (string, error) {
	info, err := root.Lstat(input)
	if err != nil {
		return "", errors.New("导入辅助清单不可用")
	}
	directory := input
	if !info.IsDir() {
		directory = filepath.Dir(input)
	}
	var facts []File
	for _, name := range []string{".MANIFEST.bin", ".MANIFEST"} {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		path := filepath.Join(directory, name)
		before, err := root.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !before.Mode().IsRegular() || before.Size() > 1<<20 {
			return "", errors.New("导入辅助清单无效或超限")
		}
		file, err := root.Open(path)
		if err != nil {
			return "", errors.New("导入辅助清单不可读")
		}
		hash := sha256.New()
		_, readErr := io.Copy(hash, io.LimitReader(file, (1<<20)+1))
		after, statErr := file.Stat()
		file.Close()
		if readErr != nil || statErr != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			return "", errors.New("导入辅助清单在检查期间变化")
		}
		facts = append(facts, File{Path: name, Size: after.Size(), Modified: after.ModTime().UnixNano(), SHA256: hex.EncodeToString(hash.Sum(nil))})
	}
	if len(facts) == 0 {
		return "", nil
	}
	return manifestDigest(facts, ""), nil
}

func openBounded(path string, roots []string) (*os.Root, string, error) {
	if !filepath.IsAbs(path) {
		return nil, "", fs.ErrPermission
	}
	for _, allowed := range roots {
		if !filepath.IsAbs(allowed) {
			continue
		}
		relative, err := filepath.Rel(allowed, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		info, err := os.Lstat(allowed)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, "", fs.ErrPermission
		}
		root, err := os.OpenRoot(allowed)
		if err != nil {
			return nil, "", err
		}
		opened, err := root.Stat(".")
		if err != nil || !os.SameFile(info, opened) {
			root.Close()
			return nil, "", fs.ErrPermission
		}
		// 逐级拒绝链接；后续实际打开仍使用 os.Root，避免检查后的路径替换越出根目录。
		current := "."
		for _, part := range strings.Split(relative, string(filepath.Separator)) {
			if part == "." {
				continue
			}
			current = filepath.Join(current, part)
			info, err = root.Lstat(current)
			if err != nil || info.Mode()&os.ModeSymlink != 0 {
				root.Close()
				return nil, "", fs.ErrPermission
			}
		}
		return root, relative, nil
	}
	return nil, "", fs.ErrPermission
}

func classify(name string, c importdomain.Config, filter *regexp.Regexp) (string, string, bool) {
	original := name
	if c.Parser.Compress {
		for _, suffix := range []string{".zstd", ".zst", ".zlib", ".gz", ".gzip", ".snappy"} {
			if strings.HasSuffix(name, suffix) {
				name = strings.TrimSuffix(name, suffix)
				break
			}
		}
	}
	kind, object := "", ""
	if strings.HasSuffix(name, "-schema.sql") {
		if !c.HasDDL() {
			return "", "", false
		}
		kind = "DDL"
		object = strings.TrimSuffix(name, "-schema.sql")
	} else {
		if !c.HasData() {
			return "", "", false
		}
		suffix := c.FileSuffix
		if suffix == "" {
			suffix = map[string]string{"CSV": ".csv", "CUT": ".dat", "SQL": ".sql"}[c.Format]
		}
		if suffix == "" || !strings.HasSuffix(name, suffix) {
			return "", "", false
		}
		kind = "DATA"
		object = strings.TrimSuffix(name, suffix)
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
	}
	if object == "" || strings.ContainsFunc(object, unicode.IsControl) || len(object) > 256 {
		return "", "", false
	}
	if filter != nil && !filter.MatchString(original) {
		return "", "", false
	}
	if c.Scope == "SPECIFIC" {
		matches := false
		for _, expression := range c.Objects {
			if matchesExpression(object, expression) {
				matches = true
			}
		}
		if !matches {
			return "", "", false
		}
	}
	if c.ExcludeTables != "" && matchesExpression(object, c.ExcludeTables) {
		return "", "", false
	}
	return kind, object, true
}
func matchesExpression(name, expressions string) bool {
	for _, expression := range strings.Split(expressions, ",") {
		pattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(strings.TrimSpace(expression)), `\*`, ".*") + "$"
		if ok, _ := regexp.MatchString(pattern, name); ok {
			return true
		}
	}
	return false
}
func safeRelative(value string) bool {
	return value != "" && len(value) <= 512 && !strings.ContainsFunc(value, unicode.IsControl) && !strings.ContainsAny(value, `\:`) && !strings.HasPrefix(value, "/") && value != ".." && !strings.HasPrefix(value, "../") && !strings.Contains(value, "/../") && filepath.ToSlash(filepath.Clean(value)) == value
}
