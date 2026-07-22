// Package jdbcprobe 提供 Agent 使用的固定 JDBC 连接探针适配。
// 它不接受任意 SQL、JDBC URL、驱动、Java 主类或命令文本，只能启动受控探针验证 ODP 基础连接。
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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"ob-data-orch/internal/credential"
)

const (
	probeMainClass = "com.obdataorch.jdbcprobe.ConnectionProbe"
	probeTimeout   = 10 * time.Second
	maxOutputBytes = 4 * 1024
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

// Result 是可安全写入预检查结果的基础 JDBC 元信息。
// 它不包含地址、用户名、密码、JDBC URL、异常文本或数据库对象信息。
type Result struct {
	ProductName    string
	ProductVersion string
	DriverName     string
	DriverVersion  string
}

type probeResponse struct {
	Status         string `json:"status"`
	Code           string `json:"code"`
	ProductName    string `json:"productName"`
	ProductVersion string `json:"productVersion"`
	DriverName     string `json:"driverName"`
	DriverVersion  string `json:"driverVersion"`
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
	bounded, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	arguments := []string{"-cp", runtime.ProbePath + string(os.PathListSeparator) + runtime.ConnectorPath, probeMainClass}
	command := exec.CommandContext(bounded, runtime.JavaPath, arguments...)
	command.Dir = filepath.Dir(runtime.ProbePath)
	command.Env = append([]string(nil), runtime.Environment...)
	stdin, err := command.StdinPipe()
	if err != nil {
		return Result{}, ErrProbeFailed
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return Result{}, ErrProbeFailed
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return Result{}, ErrProbeFailed
	}
	if err := command.Start(); err != nil {
		return Result{}, ErrProbeFailed
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
		output, outputErr = readLimited(stdout, maxOutputBytes)
	}()
	go func() {
		defer group.Done()
		_, _ = io.Copy(io.Discard, stderr)
	}()
	group.Wait()
	written := <-writeDone
	waitErr := command.Wait()
	if written != nil || outputErr != nil {
		zero(output)
		return Result{}, ErrProbeFailed
	}
	defer zero(output)
	if waitErr != nil {
		_, responseErr := parseResponse(output)
		if errors.Is(responseErr, ErrConnectionFailed) || errors.Is(responseErr, ErrDriverUnavailable) || errors.Is(responseErr, ErrInvalidRequest) {
			return Result{}, responseErr
		}
		return Result{}, ErrProbeFailed
	}
	return parseResponse(output)
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

func encodeRequest(request Request) []byte {
	length := 4 + 4 + len(request.Host) + 4 + 4 + len(request.Username) + 4 + len(request.Password)
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
	writeInt(1)
	writeBytes([]byte(request.Host))
	writeInt(request.Port)
	writeBytes(request.Username)
	writeBytes(request.Password)
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
	var response probeResponse
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return Result{}, ErrProbeFailed
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Result{}, ErrProbeFailed
	}
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
