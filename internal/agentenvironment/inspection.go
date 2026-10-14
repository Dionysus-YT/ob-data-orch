package agentenvironment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"ob-data-orch/internal/agentexec"
	"ob-data-orch/internal/agentlocalpreflight"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/nodefacts"
)

// VersionProber 限定为三种固定版本查询，不接收自由命令、数据库参数或秘密。
type VersionProber interface {
	Probe(context.Context, string) (string, error)
}

// Inspector 是固化配置的本机只读检查端口；授权标志只能来自当前控制面检查请求。
type Inspector interface {
	Inspect(context.Context, bool) (*nodefacts.RuntimeInspection, error)
}

// InspectorFunc 供装配层绑定固化配置，合成测试可以注入固定结果。
type InspectorFunc func(context.Context, bool) (*nodefacts.RuntimeInspection, error)

// Inspect 调用已装配的固定检查，不增加检查类型。
func (f InspectorFunc) Inspect(ctx context.Context, allowed bool) (*nodefacts.RuntimeInspection, error) {
	if f == nil {
		return nil, ErrInvalidConfiguration
	}
	return f(ctx, allowed)
}

// LocalInspector 核对固定工具、私有工作/日志根目录和安全配置投影。
type LocalInspector struct {
	Platform            string
	JavaPath            string
	ToolHome            string
	WorkspaceRoot       string
	LogQueueRoot        string
	ConfigurationDigest string
	Versions            VersionProber
	ResourceReady       func() bool
	Clock               func() time.Time
}

// Inspect 遍历完整固定清单；版本查询仅在明确请求时执行，全部原始输出与未知配置丢弃。
func (i LocalInspector) Inspect(ctx context.Context, allowed bool) (*nodefacts.RuntimeInspection, error) {
	if ctx == nil || i.Clock == nil {
		return nil, ErrInvalidConfiguration
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	report := &nodefacts.RuntimeInspection{RuntimeConfigurationDigest: i.ConfigurationDigest, VersionProbeAllowed: allowed, Items: make([]nodefacts.InspectionItem, 0, len(nodefacts.InspectionKinds()))}
	dumper, dumperOK := agentlocalpreflight.ToolLauncherPath(i.ToolHome, commandgen.Platform(i.Platform), "obdumper")
	loader, loaderOK := agentlocalpreflight.ToolLauncherPath(i.ToolHome, commandgen.Platform(i.Platform), "obloader")
	if !dumperOK || !loaderOK {
		return nil, ErrInvalidConfiguration
	}
	paths := map[string]string{"JAVA_RUNTIME": i.JavaPath, "OBDUMPER_RUNTIME": dumper, "OBLOADER_RUNTIME": loader, "CONNECTOR": filepath.Join(i.ToolHome, "lib", "oceanbase-client-2.4.14.jar"), "WORKSPACE": i.WorkspaceRoot, "LOG_QUEUE": i.LogQueueRoot, "TOOL_LOG": filepath.Join(i.WorkspaceRoot, "<execution>", "tool-raw-log"), "DIRECT_LOAD_CONFIG": filepath.Join(i.ToolHome, "conf", "session.config.json")}
	for _, kind := range nodefacts.InspectionKinds() {
		item := nodefacts.InspectionItem{Kind: kind, Status: "PASSED", Code: "READY", Path: paths[kind], Required: kind == "JAVA_RUNTIME" || kind == "OBDUMPER_RUNTIME" || kind == "CONNECTOR" || kind == "WORKSPACE" || kind == "LOG_QUEUE" || kind == "RESOURCES" || kind == "RUNTIME_LAYOUT"}
		switch kind {
		case "JAVA_RUNTIME", "OBDUMPER_RUNTIME", "OBLOADER_RUNTIME":
			inspectFile(&item)
			if item.Status == "PASSED" {
				executable := i.Platform == "WINDOWS_AMD64"
				if info, err := os.Stat(item.Path); err == nil && i.Platform != "WINDOWS_AMD64" {
					executable = info.Mode().Perm()&0o111 != 0
				}
				item.Executable = &executable
				if !executable {
					item.Status, item.Code = "FAILED", "NOT_EXECUTABLE"
				} else if !allowed {
					item.Status, item.Code = "UNKNOWN", "VERSION_PROBE_NOT_REQUESTED"
				} else if i.Versions == nil {
					item.Status, item.Code = "UNKNOWN", "VERSION_UNAVAILABLE"
				} else {
					version, err := i.Versions.Probe(ctx, kind)
					if err != nil {
						item.Status, item.Code = "UNKNOWN", "VERSION_UNAVAILABLE"
						if errors.Is(err, agentexec.ErrVersionProbeTimeout) {
							item.Code = "VERSION_PROBE_TIMEOUT"
						}
						if errors.Is(err, agentexec.ErrVersionOutputInvalid) {
							item.Code = "VERSION_OUTPUT_INVALID"
						}
					} else {
						item.Version = version
						if !nodefacts.SupportedRuntimeVersion(kind, version) {
							item.Status, item.Code = "FAILED", "VERSION_UNSUPPORTED"
						}
					}
				}
			}
		case "CONNECTOR":
			inspectFile(&item)
		case "WORKSPACE", "LOG_QUEUE":
			inspectDirectory(&item)
		case "TOOL_LOG":
			// 工具日志由私有 execution 目录隔离，只核对其公共基础目录，不扫描任务子目录。
			item.Path = i.WorkspaceRoot
			inspectDirectory(&item)
		case "DIRECT_LOAD_CONFIG":
			configuration := readDirectLoadConfiguration(item.Path)
			report.DirectLoad = configuration
			item.Status, item.Code = "PASSED", "READY"
			if configuration.Status == "MISSING" {
				item.Status, item.Code = "NOT_APPLICABLE", "NOT_CONFIGURED"
			}
			if configuration.Status == "UNAVAILABLE" {
				item.Status, item.Code = "UNKNOWN", "NOT_READABLE"
			}
			if configuration.Status == "INVALID" {
				item.Status, item.Code = "FAILED", "CONFIG_INVALID"
			}
		case "RESOURCES":
			if i.ResourceReady == nil || !i.ResourceReady() {
				item.Status, item.Code = "UNKNOWN", "SAMPLING_UNAVAILABLE"
			}
		}
		report.Items = append(report.Items, item)
	}
	report.ObservedAt = i.Clock().UTC()
	return report, nil
}

func inspectFile(item *nodefacts.InspectionItem) {
	info, err := os.Stat(item.Path)
	exists := err == nil && info.Mode().IsRegular()
	item.Exists = &exists
	if errors.Is(err, os.ErrNotExist) || err == nil && !exists {
		item.Status, item.Code = "FAILED", "MISSING"
		return
	}
	if err != nil {
		item.Exists = nil
		item.Status, item.Code = "UNKNOWN", "NOT_READABLE"
		return
	}
	file, err := os.Open(item.Path)
	readable := err == nil
	item.Readable = &readable
	if file != nil {
		_ = file.Close()
	}
	if !readable {
		item.Status, item.Code = "FAILED", "NOT_READABLE"
	}
}

func inspectDirectory(item *nodefacts.InspectionItem) {
	info, err := os.Stat(item.Path)
	exists := err == nil && info.IsDir()
	item.Exists = &exists
	if errors.Is(err, os.ErrNotExist) || err == nil && !exists {
		item.Status, item.Code = "FAILED", "MISSING"
		return
	}
	if err != nil {
		item.Exists = nil
		item.Status, item.Code = "UNKNOWN", "NOT_READABLE"
		return
	}
	file, err := os.Open(item.Path)
	readable := err == nil
	item.Readable = &readable
	if file != nil {
		_, err = file.Readdirnames(1)
		_ = file.Close()
		readable = err == nil || errors.Is(err, io.EOF)
		item.Readable = &readable
	}
	if !readable {
		item.Status, item.Code = "FAILED", "NOT_READABLE"
		return
	}
	probe, err := os.CreateTemp(item.Path, ".node-access-")
	writable := err == nil
	item.Writable = &writable
	if probe != nil {
		name := probe.Name()
		err = probe.Close()
		if removeErr := os.Remove(name); err == nil {
			err = removeErr
		}
		if err != nil {
			writable = false
		}
	}
	if !writable {
		item.Status, item.Code = "FAILED", "NOT_WRITABLE"
	}
}

func readDirectLoadConfiguration(path string) *nodefacts.DirectLoadConfiguration {
	result := &nodefacts.DirectLoadConfiguration{Status: "UNAVAILABLE"}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		result.Status = "MISSING"
		return result
	}
	if err != nil {
		return result
	}
	if !info.Mode().IsRegular() {
		result.Status = "INVALID"
		return result
	}
	file, err := os.Open(path)
	if err != nil {
		return result
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 64<<10+1))
	defer credential.Zero(content)
	if err != nil || len(content) > 64<<10 {
		result.Status = "INVALID"
		return result
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	root, err := uniqueRawObject(decoder)
	defer zeroRawValues(root)
	if err != nil {
		result.Status = "INVALID"
		return result
	}
	section, found := root["direct_path_load"]
	if !found {
		result.Status = "MISSING"
		return result
	}
	values, err := uniqueRawObject(json.NewDecoder(bytes.NewReader(section)))
	defer zeroRawValues(values)
	if err != nil {
		result.Status = "INVALID"
		return result
	}
	fields := map[string]**int64{"task_timeout_ms": &result.TaskTimeoutMS, "heartbeat_timeout_ms": &result.HeartbeatTimeoutMS, "heartbeat_interval_ms": &result.HeartbeatIntervalMS}
	for key, value := range values {
		target, known := fields[key]
		if !known {
			result.HasUnrecognizedKeys = true
			continue
		}
		value = bytes.TrimSpace(value)
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}
		valid := len(value) > 0 && len(value) <= 16
		for _, character := range value {
			valid = valid && character >= '0' && character <= '9'
		}
		if !valid {
			return &nodefacts.DirectLoadConfiguration{Status: "INVALID"}
		}
		integer, err := strconv.ParseInt(string(value), 10, 64)
		if err != nil || integer < 0 || integer > 1<<53-1 {
			return &nodefacts.DirectLoadConfiguration{Status: "INVALID"}
		}
		*target = &integer
	}
	result.Status = "READ"
	result.Fingerprint = nodefacts.DirectLoadFingerprint(*result)
	return result
}

// uniqueRawObject 不把未知配置值解码成不可擦除字符串；所有原始片段由调用方清零。
func uniqueRawObject(decoder *json.Decoder) (map[string]json.RawMessage, error) {
	result := make(map[string]json.RawMessage)
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return result, errors.New("configuration object is invalid")
	}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return result, err
		}
		name, ok := key.(string)
		if !ok {
			return result, errors.New("configuration key is invalid")
		}
		if _, found := result[name]; found {
			return result, errors.New("configuration key is duplicated")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			credential.Zero(value)
			return result, err
		}
		result[name] = value
	}
	if _, err = decoder.Token(); err != nil {
		return result, err
	}
	if _, err = decoder.Token(); !errors.Is(err, io.EOF) {
		return result, errors.New("configuration trailing content is invalid")
	}
	return result, nil
}
func zeroRawValues(values map[string]json.RawMessage) {
	for key, value := range values {
		credential.Zero(value)
		delete(values, key)
	}
}
