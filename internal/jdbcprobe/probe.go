// Package jdbcprobe 提供 Agent 使用的固定 JDBC 连接探针适配。
// 它不接受任意 SQL、JDBC URL、驱动、Java 主类或命令文本，只能启动受控探针验证 ODP 连接及固定预检查。
package jdbcprobe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"ob-data-orch/internal/catalogresult"
	"ob-data-orch/internal/credential"
)

const (
	probeMainClass = "com.obdataorch.jdbcprobe.ConnectionProbe"
	probeTimeout   = 10 * time.Second
	maxOutputBytes = 4 * 1024

	connectionProtocolVersion     = 1
	preflightProtocolVersion      = 3
	catalogProtocolVersion        = 4
	batchPreflightProtocolVersion = 5
)

var (
	ErrInvalidRuntime    = errors.New("JDBC 探针运行时无效")
	ErrInvalidRequest    = errors.New("JDBC 探针请求无效")
	ErrProbeFailed       = errors.New("JDBC 探针执行失败")
	ErrConnectionFailed  = errors.New("JDBC 数据库连接失败")
	ErrDriverUnavailable = errors.New("JDBC 驱动不可用")
)

var (
	digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
	hostPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)
)

// Runtime 是固定 JDBC 探针运行所需的已核验文件与显式环境。
// 三个文件都必须使用绝对路径和 SHA-256 摘要，防止调用方替换 Java、探针或 Connector/J。
type Runtime struct {
	JavaPath        string
	JavaSHA256      string
	ProbePath       string
	ProbeSHA256     string
	ConnectorPath   string
	ConnectorSHA256 string
	Environment     []string
}

// Request 是一次数据源基础连接测试所需的短时输入。
// Username 和 Password 只在编码到子进程标准输入期间驻留；调用方负责在适当边界清零其原始缓冲区。
type Request struct {
	Host     string
	Port     int
	Username []byte
	Password []byte
}

// PreflightRequest 是固定 EXPORT_PREFLIGHT 的 JDBC 输入。
// CompatibilityMode、Database 和 Table 只用于受控 JDBC 元数据与探针内部固定零行读取，不出现在 argv、环境变量或错误正文。
type PreflightRequest struct {
	Connection        Request
	CompatibilityMode CompatibilityMode
	Database          string
	Table             string
}

// PreflightObject 是冻结的单个导出对象，类型限定为五类已开放对象。
type PreflightObject struct {
	Type string
	Name string
}

// BatchPreflightRequest 在一次固定 JDBC 连接中核对全部冻结对象。
type BatchPreflightRequest struct {
	Connection        Request
	CompatibilityMode CompatibilityMode
	Database          string
	Objects           []PreflightObject
}

// CatalogRequest 只允许列举有界数据库名，或指定数据库内的五类已开放对象名。
type CatalogRequest struct {
	Connection        Request
	CompatibilityMode CompatibilityMode
	Database          string
	ObjectType        string
	Keyword           string
}

// CatalogResult 是单类最多 100 项或五类合计最多 500 项的固定目录结果。
type CatalogResult struct {
	Objects   []string
	Truncated bool
	Groups    []catalogresult.Group
}

// CompatibilityMode 是已冻结数据源的 OceanBase 兼容模式。
// 它只决定固定元数据 API 的命名空间参数和零行读取中的标识符引用方式，不能由 Agent 本地配置覆盖。
type CompatibilityMode string

const (
	CompatibilityModeMySQL  CompatibilityMode = "MYSQL"
	CompatibilityModeOracle CompatibilityMode = "ORACLE"
)

// ObjectAccess 是固定对象元数据与零行读取的安全投影。
// 它不包含对象名称、SQL、驱动异常或权限细节。
type ObjectAccess string

const (
	ObjectAccessible    ObjectAccess = "ACCESSIBLE"
	ObjectUnavailable   ObjectAccess = "UNAVAILABLE"
	ObjectNotAccessible ObjectAccess = "NOT_ACCESSIBLE"
)

// PreflightResult 同时保存一条 JDBC 连接和固定对象读取检查的安全结论。
type PreflightResult struct {
	Connection   Result
	ObjectAccess ObjectAccess
}

// Result 是可安全写入预检查结果的基础 JDBC 元信息。
// 它不包含地址、用户名、密码、JDBC URL、异常文本或数据库对象信息。
type Result struct {
	ProductName    string
	ProductVersion string
	DriverName     string
	DriverVersion  string
}

type probeResponse struct {
	Status         string                `json:"status"`
	Code           string                `json:"code"`
	ProductName    string                `json:"productName"`
	ProductVersion string                `json:"productVersion"`
	DriverName     string                `json:"driverName"`
	DriverVersion  string                `json:"driverVersion"`
	ObjectAccess   string                `json:"objectAccess"`
	Objects        []string              `json:"objects"`
	Truncated      *bool                 `json:"truncated"`
	Groups         []catalogresult.Group `json:"groups"`
}

// ListObjectsInWorkspace 安装已核验探针后执行一次固定元数据查询。
func ListObjectsInWorkspace(ctx context.Context, workspace credential.Workspace, runtime Runtime, request CatalogRequest) (CatalogResult, error) {
	probePath, probeDigest, err := Install(workspace)
	if err != nil {
		return CatalogResult{}, err
	}
	runtime.ProbePath, runtime.ProbeSHA256 = probePath, probeDigest
	return ListObjects(ctx, runtime, request)
}

// ListObjects 使用探针协议 v4；请求和响应均受长度约束，不接受 SQL。
func ListObjects(ctx context.Context, runtime Runtime, request CatalogRequest) (CatalogResult, error) {
	if !validRuntime(runtime) {
		return CatalogResult{}, ErrInvalidRuntime
	}
	if !validRequest(request.Connection) || !validCompatibilityMode(request.CompatibilityMode) ||
		!validCatalogRequestScope(request.Database, request.ObjectType) ||
		len(request.Keyword) > 100 || !utf8.ValidString(request.Keyword) || request.Keyword != strings.TrimSpace(request.Keyword) {
		return CatalogResult{}, ErrInvalidRequest
	}
	for _, character := range request.Keyword {
		if unicode.IsControl(character) {
			return CatalogResult{}, ErrInvalidRequest
		}
	}
	input := encodeProbeRequest(catalogProtocolVersion, request.Connection, [][]byte{[]byte(request.CompatibilityMode), []byte(request.Database), []byte(request.ObjectType), []byte(request.Keyword)})
	defer zero(input)
	maximum := 64 * 1024
	if request.ObjectType == "ALL" {
		maximum = 512 * 1024
	}
	timeout := probeTimeout
	if request.ObjectType == "ALL" {
		timeout = 35 * time.Second
	}
	output, err := runProbeLimitedWithTimeout(ctx, runtime, input, maximum, timeout)
	if err != nil {
		return CatalogResult{}, err
	}
	defer zero(output)
	return parseCatalogResponseForType(output, request.ObjectType)
}

// validCatalogRequestScope 与控制面冻结条件保持一致，防止探针接收任意元数据范围。
func validCatalogRequestScope(database, objectType string) bool {
	if objectType == "DATABASE" {
		return database == ""
	}
	return (objectType == "ALL" || objectType == "TABLE" || objectType == "VIEW" || objectType == "FUNCTION" || objectType == "PROCEDURE" || objectType == "SEQUENCE") && validObjectIdentifier(database)
}

func parseCatalogResponse(output []byte) (CatalogResult, error) {
	return parseCatalogResponseForType(output, "TABLE")
}

func parseCatalogResponseForType(output []byte, objectType string) (CatalogResult, error) {
	response, err := decodeResponse(output)
	if err != nil {
		return CatalogResult{}, ErrProbeFailed
	}
	if response.Status != "SUCCESS" || response.Code != "" || response.ObjectAccess != "" ||
		response.ProductName != "" || response.ProductVersion != "" || response.DriverName != "" || response.DriverVersion != "" ||
		response.Truncated == nil || len(response.Objects) > 100 {
		return CatalogResult{}, ErrProbeFailed
	}
	if objectType == "ALL" {
		if response.Objects == nil || len(response.Objects) != 0 || *response.Truncated || !catalogresult.ValidGroups(response.Groups) {
			return CatalogResult{}, ErrProbeFailed
		}
		return CatalogResult{Groups: response.Groups}, nil
	}
	if response.Objects == nil || len(response.Groups) != 0 {
		return CatalogResult{}, ErrProbeFailed
	}
	seen := map[string]bool{}
	for _, name := range response.Objects {
		if !validObjectIdentifier(name) || strings.ContainsAny(name, "*,") || seen[name] {
			return CatalogResult{}, ErrProbeFailed
		}
		seen[name] = true
	}
	return CatalogResult{Objects: response.Objects, Truncated: *response.Truncated}, nil
}

// TestConnectionInWorkspace 在预检查专属工作区中释放固定探针后执行基础连接测试。
// 调用方仍须提供已核验的 Java 与 OBDUMPER 包内 Connector/J；该函数不接受其他本地资产路径。
func TestConnectionInWorkspace(ctx context.Context, workspace credential.Workspace, runtime Runtime, request Request) (Result, error) {
	probePath, probeDigest, err := Install(workspace)
	if err != nil {
		return Result{}, err
	}
	runtime.ProbePath = probePath
	runtime.ProbeSHA256 = probeDigest
	return TestConnection(ctx, runtime, request)
}

// TestPreflightInWorkspace 在一个预检查私有工作区中执行连接、固定对象元数据和零行读取。
// 同一子进程只接受一次短时秘密输入，避免把连接与对象检查拆成多次秘密解析。
func TestPreflightInWorkspace(ctx context.Context, workspace credential.Workspace, runtime Runtime, request PreflightRequest) (PreflightResult, error) {
	probePath, probeDigest, err := Install(workspace)
	if err != nil {
		slog.Warn("JDBC 预检查探针未完成", "phase", "asset_install")
		return PreflightResult{}, err
	}
	runtime.ProbePath = probePath
	runtime.ProbeSHA256 = probeDigest
	return TestPreflight(ctx, runtime, request)
}

// TestConnection 直接启动固定 Java 主类，完成 ODP 基础 JDBC 连接测试。
// 它不向 argv 或环境变量写入连接输入；标准错误只被排空以防止子进程阻塞，绝不作为错误正文返回。
func TestConnection(ctx context.Context, runtime Runtime, request Request) (Result, error) {
	if !validRuntime(runtime) {
		return Result{}, ErrInvalidRuntime
	}
	if !validRequest(request) {
		return Result{}, ErrInvalidRequest
	}
	input := encodeRequest(request)
	defer zero(input)
	output, err := runProbe(ctx, runtime, input)
	if err != nil {
		return Result{}, err
	}
	defer zero(output)
	return parseResponse(output)
}

// TestPreflight 直接启动固定 Java 主类，并在一个连接中完成基础元信息和对象访问检查。
// 它拒绝非冻结对象输入，且不会把对象或秘密写入命令行、环境变量、日志或返回错误。
func TestPreflight(ctx context.Context, runtime Runtime, request PreflightRequest) (PreflightResult, error) {
	if !validRuntime(runtime) {
		slog.Warn("JDBC 预检查探针未完成", "phase", "runtime_validation")
		return PreflightResult{}, ErrInvalidRuntime
	}
	if !validPreflightRequest(request) {
		slog.Warn("JDBC 预检查探针未完成", "phase", "request_validation")
		return PreflightResult{}, ErrInvalidRequest
	}
	input := encodePreflightRequest(request)
	defer zero(input)
	output, err := runProbe(ctx, runtime, input)
	if err != nil {
		return PreflightResult{}, err
	}
	defer zero(output)
	result, err := parsePreflightResponse(output)
	if err != nil {
		slog.Warn("JDBC 预检查探针未完成", "phase", "response_validation", "category", probeResponseErrorCategory(err))
	}
	return result, err
}

// TestBatchPreflight 使用一次受控子进程和 JDBC 连接逐项核对冻结对象，仅返回汇总三态。
func TestBatchPreflight(ctx context.Context, runtime Runtime, request BatchPreflightRequest) (PreflightResult, error) {
	if !validRuntime(runtime) {
		return PreflightResult{}, ErrInvalidRuntime
	}
	if !validBatchPreflightRequest(request) {
		return PreflightResult{}, ErrInvalidRequest
	}
	input := encodeBatchPreflightRequest(request)
	defer zero(input)
	output, err := runProbeLimitedWithTimeout(ctx, runtime, input, maxOutputBytes, 90*time.Second)
	if err != nil {
		return PreflightResult{}, err
	}
	defer zero(output)
	return parsePreflightResponse(output)
}

func validBatchPreflightRequest(request BatchPreflightRequest) bool {
	if !validRequest(request.Connection) || !validCompatibilityMode(request.CompatibilityMode) || !validObjectIdentifier(request.Database) || len(request.Objects) == 0 {
		return false
	}
	for _, object := range request.Objects {
		if !validObjectIdentifier(object.Name) || strings.ContainsAny(object.Name, "*,") {
			return false
		}
		switch object.Type {
		case "TABLE", "VIEW", "FUNCTION", "PROCEDURE", "SEQUENCE":
		default:
			return false
		}
	}
	return true
}

func encodeBatchPreflightRequest(request BatchPreflightRequest) []byte {
	length := 4 + 4 + len(request.Connection.Host) + 4 + 4 + len(request.Connection.Username) + 4 + len(request.Connection.Password) + 4 + len(request.CompatibilityMode) + 4 + len(request.Database) + 4
	for _, object := range request.Objects {
		length += 4 + len(object.Type) + 4 + len(object.Name)
	}
	result := make([]byte, length)
	offset := 0
	writeInt := func(value int) {
		binary.BigEndian.PutUint32(result[offset:offset+4], uint32(value))
		offset += 4
	}
	writeBytes := func(value []byte) {
		writeInt(len(value))
		copy(result[offset:], value)
		offset += len(value)
	}
	writeInt(batchPreflightProtocolVersion)
	writeBytes([]byte(request.Connection.Host))
	writeInt(request.Connection.Port)
	writeBytes(request.Connection.Username)
	writeBytes(request.Connection.Password)
	writeBytes([]byte(request.CompatibilityMode))
	writeBytes([]byte(request.Database))
	writeInt(len(request.Objects))
	for _, object := range request.Objects {
		writeBytes([]byte(object.Type))
		writeBytes([]byte(object.Name))
	}
	return result
}

// probeResponseErrorCategory 仅保留受控错误类别，不记录 JDBC 输出或连接输入。
func probeResponseErrorCategory(err error) string {
	switch {
	case errors.Is(err, ErrConnectionFailed):
		return "connection_failed"
	case errors.Is(err, ErrDriverUnavailable):
		return "driver_unavailable"
	case errors.Is(err, ErrInvalidRequest):
		return "invalid_request"
	default:
		return "invalid_response"
	}
}

func runProbe(ctx context.Context, runtime Runtime, input []byte) ([]byte, error) {
	return runProbeLimited(ctx, runtime, input, maxOutputBytes)
}

func runProbeLimited(ctx context.Context, runtime Runtime, input []byte, maximum int) ([]byte, error) {
	return runProbeLimitedWithTimeout(ctx, runtime, input, maximum, probeTimeout)
}

// runProbeLimitedWithTimeout 只为五类批量目录延长单次固定探针时限。
func runProbeLimitedWithTimeout(ctx context.Context, runtime Runtime, input []byte, maximum int, timeout time.Duration) ([]byte, error) {
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	arguments := []string{"-cp", runtime.ProbePath + string(os.PathListSeparator) + runtime.ConnectorPath, probeMainClass}
	command := exec.CommandContext(bounded, runtime.JavaPath, arguments...)
	command.Dir = filepath.Dir(runtime.ProbePath)
	command.Env = append([]string(nil), runtime.Environment...)
	stdin, err := command.StdinPipe()
	if err != nil {
		slog.Warn("JDBC 固定子进程未完成", "phase", "stdin_pipe")
		return nil, ErrProbeFailed
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		slog.Warn("JDBC 固定子进程未完成", "phase", "stdout_pipe")
		return nil, ErrProbeFailed
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		slog.Warn("JDBC 固定子进程未完成", "phase", "stderr_pipe")
		return nil, ErrProbeFailed
	}
	if err := command.Start(); err != nil {
		slog.Warn("JDBC 固定子进程未完成", "phase", "process_start")
		return nil, ErrProbeFailed
	}
	writeDone := make(chan error, 1)
	go func() {
		_, writeErr := stdin.Write(input)
		closeErr := stdin.Close()
		if writeErr != nil || closeErr != nil {
			writeDone <- ErrProbeFailed
			return
		}
		writeDone <- nil
	}()
	var group sync.WaitGroup
	group.Add(2)
	var output []byte
	var outputErr error
	go func() {
		defer group.Done()
		output, outputErr = readLimited(stdout, maximum)
	}()
	go func() {
		defer group.Done()
		_, _ = io.Copy(io.Discard, stderr)
	}()
	group.Wait()
	written := <-writeDone
	waitErr := command.Wait()
	if written != nil || outputErr != nil {
		phase := "stdin_write"
		if outputErr != nil {
			phase = "stdout_read"
		}
		slog.Warn("JDBC 固定子进程未完成", "phase", phase)
		zero(output)
		return nil, ErrProbeFailed
	}
	if waitErr != nil {
		_, responseErr := parseResponse(output)
		if errors.Is(responseErr, ErrConnectionFailed) || errors.Is(responseErr, ErrDriverUnavailable) || errors.Is(responseErr, ErrInvalidRequest) {
			zero(output)
			return nil, responseErr
		}
		phase := "process_exit"
		if bounded.Err() != nil {
			phase = "process_timeout"
		} else if response, decodeErr := decodeResponse(output); decodeErr == nil && response.Status == "SUCCESS" {
			phase = "process_exit_after_success"
		}
		slog.Warn("JDBC 固定子进程未完成", "phase", phase)
		zero(output)
		return nil, ErrProbeFailed
	}
	return output, nil
}

func validRuntime(runtime Runtime) bool {
	if len(runtime.Environment) == 0 || forbiddenEnvironment(runtime.Environment) {
		return false
	}
	for _, file := range []struct {
		path   string
		digest string
	}{
		{runtime.JavaPath, runtime.JavaSHA256},
		{runtime.ProbePath, runtime.ProbeSHA256},
		{runtime.ConnectorPath, runtime.ConnectorSHA256},
	} {
		if !absoluteRegularFile(file.path) || !digestMatches(file.path, file.digest) {
			return false
		}
	}
	return true
}

func validRequest(request Request) bool {
	return hostPattern.MatchString(request.Host) && request.Port >= 1 && request.Port <= 65535 && len(request.Username) > 0 && len(request.Username) <= 256 && len(request.Password) > 0 && len(request.Password) <= 4096 && !bytes.ContainsAny(request.Username, "\r\n") && !bytes.ContainsAny(request.Password, "\r\n")
}

func validPreflightRequest(request PreflightRequest) bool {
	return validRequest(request.Connection) && validCompatibilityMode(request.CompatibilityMode) && validObjectIdentifier(request.Database) && validObjectIdentifier(request.Table)
}

func validCompatibilityMode(mode CompatibilityMode) bool {
	return mode == CompatibilityModeMySQL || mode == CompatibilityModeOracle
}

func validObjectIdentifier(value string) bool {
	if value == "" || len(value) > 256 || value != strings.TrimSpace(value) || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func encodeRequest(request Request) []byte {
	return encodeProbeRequest(connectionProtocolVersion, request, nil)
}

func encodePreflightRequest(request PreflightRequest) []byte {
	return encodeProbeRequest(preflightProtocolVersion, request.Connection, [][]byte{[]byte(request.CompatibilityMode), []byte(request.Database), []byte(request.Table)})
}

func encodeProbeRequest(version int, request Request, fields [][]byte) []byte {
	length := 4 + 4 + len(request.Host) + 4 + 4 + len(request.Username) + 4 + len(request.Password)
	for _, field := range fields {
		length += 4 + len(field)
	}
	result := make([]byte, length)
	offset := 0
	writeInt := func(value int) {
		binary.BigEndian.PutUint32(result[offset:offset+4], uint32(value))
		offset += 4
	}
	writeBytes := func(value []byte) {
		writeInt(len(value))
		copy(result[offset:], value)
		offset += len(value)
	}
	writeInt(version)
	writeBytes([]byte(request.Host))
	writeInt(request.Port)
	writeBytes(request.Username)
	writeBytes(request.Password)
	for _, field := range fields {
		writeBytes(field)
	}
	return result
}

func readLimited(reader io.Reader, maximum int) ([]byte, error) {
	if maximum < 1 {
		return nil, ErrProbeFailed
	}
	buffer := make([]byte, 1024)
	defer zero(buffer)
	result := make([]byte, 0, maximum)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			if len(result)+count > maximum {
				zero(result)
				for err == nil {
					_, err = reader.Read(buffer)
				}
				return nil, ErrProbeFailed
			}
			result = append(result, buffer[:count]...)
			zero(buffer[:count])
		}
		if errors.Is(err, io.EOF) {
			return result, nil
		}
		if err != nil {
			zero(result)
			return nil, ErrProbeFailed
		}
	}
}

func parseResponse(output []byte) (Result, error) {
	response, err := decodeResponse(output)
	if err != nil || response.ObjectAccess != "" || response.Objects != nil || response.Truncated != nil || response.Groups != nil {
		return Result{}, ErrProbeFailed
	}
	return parseConnectionResponse(response)
}

func parsePreflightResponse(output []byte) (PreflightResult, error) {
	response, err := decodeResponse(output)
	if err != nil {
		return PreflightResult{}, ErrProbeFailed
	}
	if response.Objects != nil || response.Truncated != nil || response.Groups != nil {
		return PreflightResult{}, ErrProbeFailed
	}
	connection, err := parseConnectionResponse(response)
	if err != nil {
		return PreflightResult{}, err
	}
	objectAccess := ObjectAccess(response.ObjectAccess)
	if objectAccess != ObjectAccessible && objectAccess != ObjectUnavailable && objectAccess != ObjectNotAccessible {
		return PreflightResult{}, ErrProbeFailed
	}
	return PreflightResult{Connection: connection, ObjectAccess: objectAccess}, nil
}

func decodeResponse(output []byte) (probeResponse, error) {
	var response probeResponse
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return probeResponse{}, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return probeResponse{}, errors.New("JDBC 探针响应包含额外内容")
	}
	return response, nil
}

func parseConnectionResponse(response probeResponse) (Result, error) {
	if response.Status == "FAILED" && response.ProductName == "" && response.ProductVersion == "" && response.DriverName == "" && response.DriverVersion == "" {
		switch response.Code {
		case "CONNECTION_FAILED":
			return Result{}, ErrConnectionFailed
		case "DRIVER_UNAVAILABLE":
			return Result{}, ErrDriverUnavailable
		case "INVALID_INPUT":
			return Result{}, ErrInvalidRequest
		default:
			return Result{}, ErrProbeFailed
		}
	}
	if response.Status != "SUCCESS" || response.Code != "" || !safeMetadata(response.ProductName) || !safeMetadata(response.ProductVersion) || !safeMetadata(response.DriverName) || !safeMetadata(response.DriverVersion) {
		return Result{}, ErrProbeFailed
	}
	return Result{ProductName: response.ProductName, ProductVersion: response.ProductVersion, DriverName: response.DriverName, DriverVersion: response.DriverVersion}, nil
}

func safeMetadata(value string) bool {
	return len(value) <= 256 && !strings.ContainsAny(value, "\r\n\x00")
}

func absoluteRegularFile(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func digestMatches(path, expected string) bool {
	if !digestPattern.MatchString(expected) {
		return false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	defer zero(content)
	digest := sha256.Sum256(content)
	return strings.EqualFold(hex.EncodeToString(digest[:]), expected)
}

func forbiddenEnvironment(environment []string) bool {
	seen := make(map[string]struct{}, len(environment))
	for _, item := range environment {
		key, _, found := strings.Cut(item, "=")
		if !found || key == "" || strings.ContainsRune(item, 0) {
			return true
		}
		key = strings.ToUpper(key)
		if _, exists := seen[key]; exists || key == "JAVA_OPTS" || key == "JAVA_TOOL_OPTIONS" || key == "JDK_JAVA_OPTIONS" || key == "_JAVA_OPTIONS" || key == "CLASSPATH" || key == "LD_PRELOAD" {
			return true
		}
		seen[key] = struct{}{}
	}
	return false
}

func zero(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
