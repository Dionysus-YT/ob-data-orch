package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/config"
	"ob-data-orch/internal/credential"
)

const (
	registrationCodePrefix      = "obdo-r1."
	agentBundleConfigFileName   = "agent-config.json"
	defaultLocalControlPlaneURL = "https://127.0.0.1:8080"
)

// bundledExecutionSettings 从已安装配置统一读取固定能力开关，日常启动不再依赖多组环境变量。
func bundledExecutionSettings(lookup func(string) (string, bool)) (func(string) (string, bool), error) {
	path, err := os.Executable()
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(filepath.Join(filepath.Dir(path), agentBundleConfigFileName))
	if errors.Is(err, os.ErrNotExist) {
		return lookup, nil
	}
	if err != nil {
		return nil, err
	}
	if len(content) > 4096 {
		return nil, errors.New("Agent 安装配置过大")
	}
	var bundle bundledAgentConfig
	if json.Unmarshal(content, &bundle) != nil || bundle.FormatVersion != "agent-bundle-config-v1" {
		return nil, errors.New("Agent 安装配置无效")
	}
	return func(key string) (string, bool) {
		switch key {
		case "OB_DATA_ORCH_ENABLE_REAL_EXECUTION", "OB_DATA_ORCH_ENABLE_AGENT_JDBC_CONNECTION_TEST", "OB_DATA_ORCH_ENABLE_AGENT_EXPORT_PREFLIGHT", "OB_DATA_ORCH_ENABLE_AGENT_STORAGE_CONNECTIVITY_PROBE":
			return strconv.FormatBool(bundle.RealExecutionEnabled), true
		default:
			return lookup(key)
		}
	}, nil
}

type registrationCodePayload struct {
	FormatVersion      string `json:"formatVersion"`
	NodeID             string `json:"nodeId"`
	EnrollmentID       string `json:"enrollmentId"`
	EnrollmentMaterial string `json:"enrollmentMaterial"`
}

type bundledAgentConfig struct {
	RealExecutionEnabled bool   `json:"realExecutionEnabled,omitempty"`
	FormatVersion        string `json:"formatVersion"`
	ControlPlaneURL      string `json:"controlPlaneUrl"`
	ControlPlaneCAFile   string `json:"controlPlaneCaFile"`
	StateDirectory       string `json:"stateDirectory"`
}

func prepareSimpleRegistration(stateStore *agentwire.StateStore, controlPlane config.AgentControlPlane, stdin io.Reader) error {
	registration, err := readRegistrationCode(stdin)
	if err != nil {
		return errors.New("agent registration code is invalid")
	}
	material := []byte(registration.EnrollmentMaterial)
	defer credential.Zero(material)
	if err := stateStore.PrepareEnrollment(agentwire.EnrollmentConfig{
		ControlPlaneURL:    controlPlane.URL,
		CAFile:             controlPlane.CAFile,
		EnrollmentID:       registration.EnrollmentID,
		NodeID:             registration.NodeID,
		EnrollmentMaterial: material,
		ReplaceExisting:    true,
	}); err != nil {
		return errors.New("agent registration state cannot be prepared")
	}
	return nil
}

func readRegistrationCode(reader io.Reader) (registrationCodePayload, error) {
	if reader == nil {
		return registrationCodePayload{}, errors.New("registration input is unavailable")
	}
	// 注册码只读取一行，交互式安装时粘贴后按 Enter 即可，不要求用户再发送文件结束符。
	content, err := bufio.NewReader(io.LimitReader(reader, 8193)).ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		credential.Zero(content)
		return registrationCodePayload{}, errors.New("registration input is invalid")
	}
	if len(content) > 8193 {
		credential.Zero(content)
		return registrationCodePayload{}, errors.New("registration input is invalid")
	}
	trimmed := strings.TrimSpace(string(content))
	credential.Zero(content)
	if len(trimmed) == 0 || len(trimmed) > 8192 || !strings.HasPrefix(trimmed, registrationCodePrefix) {
		return registrationCodePayload{}, errors.New("registration code prefix is invalid")
	}
	encoded := strings.TrimPrefix(trimmed, registrationCodePrefix)
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return registrationCodePayload{}, errors.New("registration code encoding is invalid")
	}
	defer credential.Zero(decoded)
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	var payload registrationCodePayload
	if err := decoder.Decode(&payload); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return registrationCodePayload{}, errors.New("registration code payload is invalid")
	}
	if payload.FormatVersion != "obdo-r1" || !validRegistrationIdentifier(payload.NodeID) || !validRegistrationIdentifier(payload.EnrollmentID) || len(payload.EnrollmentMaterial) < 32 || len(payload.EnrollmentMaterial) > 4096 || payload.EnrollmentMaterial != strings.TrimSpace(payload.EnrollmentMaterial) || strings.ContainsAny(payload.EnrollmentMaterial, "\r\n\x00") {
		return registrationCodePayload{}, errors.New("registration code payload is invalid")
	}
	return payload, nil
}

func loadSimpleRegistrationControlPlane(lookupEnv func(string) (string, bool)) (config.AgentControlPlane, error) {
	if hasAgentWireOverride(lookupEnv) {
		return config.LoadAgentControlPlane(lookupEnv)
	}
	executable, err := os.Executable()
	if err != nil {
		return config.AgentControlPlane{}, errors.New("agent bundle location is unavailable")
	}
	return loadBundledAgentControlPlane(filepath.Dir(executable), lookupEnv)
}

func loadAgentStateDirectory(lookupEnv func(string) (string, bool)) (string, error) {
	if hasAgentWireOverride(lookupEnv) {
		return config.LoadAgentStateDirectory(lookupEnv)
	}
	executable, err := os.Executable()
	if err != nil {
		return config.LoadAgentStateDirectory(lookupEnv)
	}
	configPath := filepath.Join(filepath.Dir(executable), agentBundleConfigFileName)
	if _, err := os.Lstat(configPath); errors.Is(err, os.ErrNotExist) {
		return config.LoadAgentStateDirectory(lookupEnv)
	} else if err != nil {
		return "", errors.New("agent bundle configuration is unavailable")
	}
	controlPlane, err := loadBundledAgentControlPlane(filepath.Dir(executable), lookupEnv)
	if err != nil {
		return "", err
	}
	return controlPlane.StateDirectory, nil
}

func loadBundledAgentControlPlane(bundleDirectory string, lookupEnv func(string) (string, bool)) (config.AgentControlPlane, error) {
	configPath := filepath.Join(bundleDirectory, agentBundleConfigFileName)
	info, err := os.Lstat(configPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > 4096 {
		return config.AgentControlPlane{}, errors.New("agent bundle configuration is invalid")
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		return config.AgentControlPlane{}, errors.New("agent bundle configuration is unavailable")
	}
	defer credential.Zero(content)
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var bundle bundledAgentConfig
	if err := decoder.Decode(&bundle); err != nil || decoder.Decode(&struct{}{}) != io.EOF || bundle.FormatVersion != "agent-bundle-config-v1" {
		return config.AgentControlPlane{}, errors.New("agent bundle configuration is invalid")
	}
	caFile, err := bundledPath(bundleDirectory, bundle.ControlPlaneCAFile, true)
	if err != nil {
		return config.AgentControlPlane{}, err
	}
	stateDirectory, err := bundledPath(bundleDirectory, bundle.StateDirectory, false)
	if err != nil {
		return config.AgentControlPlane{}, err
	}
	values := map[string]string{
		config.AgentControlPlaneURLEnvironmentVariable:    bundle.ControlPlaneURL,
		config.AgentControlPlaneCAFileEnvironmentVariable: caFile,
		config.AgentStateDirectoryEnvironmentVariable:     stateDirectory,
	}
	return config.LoadAgentControlPlane(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
}

func bundledPath(bundleDirectory, value string, requireRegularFile bool) (string, error) {
	if strings.TrimSpace(value) == "" || filepath.IsAbs(value) || strings.ContainsRune(value, 0) {
		return "", errors.New("agent bundle path is invalid")
	}
	root := filepath.Clean(bundleDirectory)
	resolved := filepath.Clean(filepath.Join(root, value))
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", errors.New("agent bundle path is invalid")
	}
	if !requireRegularFile {
		return resolved, nil
	}
	info, err := os.Lstat(resolved)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("agent bundle CA file is invalid")
	}
	return resolved, nil
}

func hasAgentWireOverride(lookupEnv func(string) (string, bool)) bool {
	if lookupEnv == nil {
		return false
	}
	for _, key := range []string{
		config.AgentControlPlaneURLEnvironmentVariable,
		config.AgentControlPlaneCAFileEnvironmentVariable,
		config.AgentStateDirectoryEnvironmentVariable,
		config.AgentHeartbeatIntervalEnvironmentVariable,
	} {
		if _, exists := lookupEnv(key); exists {
			return true
		}
	}
	return false
}

func validRegistrationIdentifier(value string) bool {
	return value != "" && len(value) <= 256 && value == strings.TrimSpace(value) && !strings.ContainsAny(value, "\r\n\x00")
}
