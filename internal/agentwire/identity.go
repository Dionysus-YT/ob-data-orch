package agentwire

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/identifier"
	"ob-data-orch/internal/outputpath"
)

const (
	identityStateFormatVersion = "agent-identity-state-v1"
	identityRootKeyID          = "agent-identity-root-v1"
	identityStateFileName      = "agent-identity.json"
	identityRootKeyFileName    = "agent-identity-root-key.json"
)

var (
	ErrIdentityUnavailable       = errors.New("Agent 机器身份不可用")
	ErrEnrollmentPending         = errors.New("Agent 关联尚未完成")
	ErrEnrollmentRejected        = errors.New("Agent 关联被拒绝")
	ErrAgentAuthenticationDenied = errors.New("Agent 机器认证被拒绝")
	ErrProtocolRejected          = errors.New("Agent 协议响应无效")
	ErrControlPlaneUnavailable   = errors.New("控制面暂时不可用")
)

// EnrollmentConfig 是首次关联时来自本机管理员的最小输入。
// EnrollmentMaterial 只能从短时内存传入，StateStore 会在发送网络请求前加密持久化它。
type EnrollmentConfig struct {
	ControlPlaneURL    string
	CAFile             string
	EnrollmentID       string
	NodeID             string
	EnrollmentMaterial []byte
	// ReplaceExisting 仅允许本机显式重新注册时替换已完成关联的机器身份。
	// 待关联材料不能被替换，避免丢失可安全重试的一次性关联请求。
	ReplaceExisting bool
}

// EnvironmentFacts 是严格心跳允许上报的最小机器事实。
// 它不承载工具路径、磁盘路径、数据库连接、任务内容或任意远程操作参数。
type EnvironmentFacts struct {
	OperatingSystem            string
	Architecture               string
	AgentVersion               string
	ObservedAt                 time.Time
	CapacityTotal              int
	CapacityUsed               int
	CPUUsagePercent            *int
	MemoryUsagePercent         *int
	RuntimeConfigurationDigest string
	DataRootUsages             []DataRootUsage
}

// DataRootUsage 是固定本机数据目录的空间采样；只上报目录摘要和字节数，不上报任意探测路径。
type DataRootUsage struct {
	RootDigest     string
	TotalBytes     uint64
	AvailableBytes uint64
}

// Heartbeat 是一轮受认证心跳的本地输入。
// RequestID 由 StateStore 生成并在一次网络重发中保持不变，调用方不能伪造机器身份字段。
type Heartbeat struct {
	BootID string
	SentAt time.Time
	Facts  EnvironmentFacts
}

// HeartbeatResult 是控制面确认的非敏感心跳结果。
// FactsRevision 只用于后续事实绑定，不代表环境检查通过或节点可以执行任务。
type HeartbeatResult struct {
	FactsRevision      int64
	EnvironmentCheckID string
}

// AgentIdentity 是已关联 Agent 的非敏感稳定标识。
// 它只用于将本地固定预检查投影与已加密的机器身份状态绑定，绝不返回机器凭据或关联材料。
type AgentIdentity struct {
	AgentID string
	NodeID  string
}

// StateStore 管理仅属于本机 Agent 的受保护身份状态。
// 机器凭据和一次性关联材料始终位于 AES-GCM 密文中，根密钥由平台受保护载体保存。
type StateStore struct {
	directory   string
	statePath   string
	rootKeyPath string
}

type identityState struct {
	FormatVersion        string               `json:"formatVersion"`
	ControlPlaneURL      string               `json:"controlPlaneUrl"`
	CAFile               string               `json:"caFile,omitempty"`
	ProtocolVersion      string               `json:"protocolVersion"`
	AgentID              string               `json:"agentId"`
	NodeID               string               `json:"nodeId"`
	MachineCredential    []byte               `json:"machineCredential"`
	EnrollmentID         string               `json:"enrollmentId,omitempty"`
	EnrollmentMaterial   []byte               `json:"enrollmentMaterial,omitempty"`
	ExchangeRequestID    string               `json:"exchangeRequestId,omitempty"`
	RuntimeConfiguration RuntimeConfiguration `json:"runtimeConfiguration,omitempty"`
}

type encryptedIdentityState struct {
	FormatVersion string `json:"formatVersion"`
	Nonce         []byte `json:"nonce"`
	Ciphertext    []byte `json:"ciphertext"`
}

// OpenStateStore 打开指定的本机身份目录。
// 它只校验路径，不在调用时创建密钥或状态文件，避免普通启动意外改变机器身份。
func OpenStateStore(directory string) (*StateStore, error) {
	if strings.TrimSpace(directory) == "" || strings.ContainsRune(directory, 0) {
		return nil, ErrIdentityUnavailable
	}
	absDirectory, err := filepath.Abs(directory)
	if err != nil {
		return nil, ErrIdentityUnavailable
	}
	return &StateStore{
		directory:   filepath.Clean(absDirectory),
		statePath:   filepath.Join(filepath.Clean(absDirectory), identityStateFileName),
		rootKeyPath: filepath.Join(filepath.Clean(absDirectory), identityRootKeyFileName),
	}, nil
}

// LogQueueDirectory 返回身份目录内供已脱敏日志队列使用的固定私有位置。
// 队列不能位于 execution 工作目录，否则正常清理会删除尚未获得控制面确认的批次。
func (s *StateStore) LogQueueDirectory() (string, error) {
	if s == nil || s.directory == "" {
		return "", ErrIdentityUnavailable
	}
	return filepath.Join(s.directory, "log-queue"), nil
}

// PrepareEnrollment 先生成并加密写入机器凭据、关联材料与稳定请求标识，再允许调用方发起 HTTPS 关联。
// 仅本机明确请求重新注册时才替换已完成关联；待关联材料始终不可覆盖，防止丢失安全重试能力。
func (s *StateStore) PrepareEnrollment(input EnrollmentConfig) error {
	if s == nil || !validEnrollmentConfig(input) {
		return ErrIdentityUnavailable
	}
	if err := ensurePrivateDirectory(s.directory); err != nil {
		return ErrIdentityUnavailable
	}
	if existing, found, err := s.loadState(); err != nil {
		return err
	} else if found {
		defer existing.destroy()
		if !existing.pendingEnrollment() {
			if !input.ReplaceExisting {
				return ErrIdentityUnavailable
			}
		} else {
			if existing.EnrollmentID == input.EnrollmentID && existing.NodeID == input.NodeID && bytes.Equal(existing.EnrollmentMaterial, input.EnrollmentMaterial) {
				return nil
			}
			return ErrEnrollmentPending
		}
	}

	agentID, err := newOpaqueID()
	if err != nil {
		return ErrIdentityUnavailable
	}
	machineCredential, err := newOpaqueSecret()
	if err != nil {
		return ErrIdentityUnavailable
	}
	requestID, err := newOpaqueID()
	if err != nil {
		credential.Zero(machineCredential)
		return ErrIdentityUnavailable
	}
	state := identityState{
		FormatVersion:      identityStateFormatVersion,
		ControlPlaneURL:    input.ControlPlaneURL,
		CAFile:             input.CAFile,
		ProtocolVersion:    Version,
		AgentID:            agentID,
		NodeID:             input.NodeID,
		MachineCredential:  append([]byte(nil), machineCredential...),
		EnrollmentID:       input.EnrollmentID,
		EnrollmentMaterial: append([]byte(nil), input.EnrollmentMaterial...),
		ExchangeRequestID:  requestID,
	}
	credential.Zero(machineCredential)
	defer state.destroy()
	return s.saveState(&state)
}

// EnsureEnrollment 完成受保护状态中尚未完成的关联。
// 控制面响应丢失或短时不可用时不会清除关联材料和请求标识，下一次调用会以同一 requestId 安全重试。
func (s *StateStore) EnsureEnrollment(ctx context.Context) error {
	state, found, err := s.loadState()
	if err != nil {
		return err
	}
	if !found {
		return ErrIdentityUnavailable
	}
	defer state.destroy()
	if !state.pendingEnrollment() {
		return nil
	}
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return ErrIdentityUnavailable
	}
	runtimeConfiguration, err := client.exchangeEnrollment(ctx, &state)
	if err != nil {
		return err
	}
	state.RuntimeConfiguration = runtimeConfiguration
	credential.Zero(state.EnrollmentMaterial)
	state.EnrollmentMaterial = nil
	state.EnrollmentID = ""
	state.ExchangeRequestID = ""
	if err := s.saveState(&state); err != nil {
		return err
	}
	return nil
}

// SendHeartbeat 读取已完成关联的机器身份并发送严格心跳。
// 待关联状态、无效机器身份或未校验证书都会失败关闭，绝不降级到匿名请求。
func (s *StateStore) SendHeartbeat(ctx context.Context, input Heartbeat) (HeartbeatResult, error) {
	state, found, err := s.loadState()
	if err != nil {
		return HeartbeatResult{}, err
	}
	if !found {
		return HeartbeatResult{}, ErrIdentityUnavailable
	}
	defer state.destroy()
	if state.pendingEnrollment() {
		return HeartbeatResult{}, ErrEnrollmentPending
	}
	if !validHeartbeat(input) {
		return HeartbeatResult{}, ErrProtocolRejected
	}
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return HeartbeatResult{}, ErrIdentityUnavailable
	}
	requestID, err := newOpaqueID()
	if err != nil {
		return HeartbeatResult{}, ErrIdentityUnavailable
	}
	return client.sendHeartbeat(ctx, &state, requestID, input)
}

// AgentIdentity 读取已完成关联的本机 Agent 与节点标识。
// 待关联、缺失或无法验证的身份状态一律拒绝，避免预检查把本地输入误当成机器身份。
func (s *StateStore) AgentIdentity() (AgentIdentity, error) {
	state, found, err := s.loadState()
	if err != nil {
		return AgentIdentity{}, err
	}
	if !found {
		return AgentIdentity{}, ErrIdentityUnavailable
	}
	defer state.destroy()
	if state.pendingEnrollment() {
		return AgentIdentity{}, ErrEnrollmentPending
	}
	return AgentIdentity{AgentID: state.AgentID, NodeID: state.NodeID}, nil
}

// RuntimeConfiguration 返回首次关联时固化的工具、Java 和导出数据目录配置。
// 空配置仅为旧状态兼容而保留；调用方必须将它视为不可用，不能回退到环境变量或系统 PATH。
func (s *StateStore) RuntimeConfiguration() (RuntimeConfiguration, error) {
	state, found, err := s.loadState()
	if err != nil {
		return RuntimeConfiguration{}, err
	}
	if !found {
		return RuntimeConfiguration{}, ErrIdentityUnavailable
	}
	defer state.destroy()
	if state.pendingEnrollment() || !validRuntimeConfiguration(state.RuntimeConfiguration) {
		return RuntimeConfiguration{}, ErrIdentityUnavailable
	}
	configuration := state.RuntimeConfiguration
	configuration.AllowedRoots = append([]string(nil), state.RuntimeConfiguration.AllowedRoots...)
	return configuration, nil
}

func (s *StateStore) loadState() (identityState, bool, error) {
	if s == nil {
		return identityState{}, false, ErrIdentityUnavailable
	}
	if info, err := os.Lstat(s.statePath); errors.Is(err, os.ErrNotExist) {
		return identityState{}, false, nil
	} else if err != nil || info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
		return identityState{}, false, ErrIdentityUnavailable
	}
	if err := securePrivatePath(s.statePath, false); err != nil {
		return identityState{}, false, ErrIdentityUnavailable
	}
	content, err := os.ReadFile(s.statePath)
	if err != nil {
		return identityState{}, false, ErrIdentityUnavailable
	}
	defer credential.Zero(content)
	if info, err := os.Lstat(s.rootKeyPath); err != nil || info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
		return identityState{}, false, ErrIdentityUnavailable
	}
	if err := securePrivatePath(s.rootKeyPath, false); err != nil {
		return identityState{}, false, ErrIdentityUnavailable
	}
	rootKey, err := s.loadOrCreateRootKey()
	if err != nil {
		return identityState{}, false, ErrIdentityUnavailable
	}
	defer credential.Zero(rootKey)
	state, err := decryptIdentityState(content, rootKey, identityAAD(s.statePath))
	if err != nil {
		return identityState{}, false, ErrIdentityUnavailable
	}
	if !validIdentityState(state) {
		state.destroy()
		return identityState{}, false, ErrIdentityUnavailable
	}
	return state, true, nil
}

func (s *StateStore) saveState(state *identityState) error {
	if s == nil || !validIdentityState(*state) {
		return ErrIdentityUnavailable
	}
	if err := ensurePrivateDirectory(s.directory); err != nil {
		return ErrIdentityUnavailable
	}
	rootKey, err := s.loadOrCreateRootKey()
	if err != nil {
		return ErrIdentityUnavailable
	}
	defer credential.Zero(rootKey)
	content, err := encryptIdentityState(*state, rootKey, identityAAD(s.statePath))
	if err != nil {
		return ErrIdentityUnavailable
	}
	defer credential.Zero(content)
	if err := writePrivateFileAtomically(s.statePath, content); err != nil {
		return ErrIdentityUnavailable
	}
	return nil
}

// loadOrCreateRootKey 在平台保护载体读取前先拒绝符号链接和不安全路径。
// 身份目录已收紧给服务账户，但此检查仍必须先于根密钥读取，避免首个关联写入被路径替换引向外部文件。
func (s *StateStore) loadOrCreateRootKey() ([]byte, error) {
	if s == nil {
		return nil, ErrIdentityUnavailable
	}
	if info, err := os.Lstat(s.rootKeyPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || info.IsDir() || securePrivatePath(s.rootKeyPath, false) != nil {
			return nil, ErrIdentityUnavailable
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, ErrIdentityUnavailable
	}
	rootKey, err := credential.LoadOrCreateRootKey(s.rootKeyPath, identityRootKeyID)
	if err != nil {
		return nil, ErrIdentityUnavailable
	}
	if err := securePrivatePath(s.rootKeyPath, false); err != nil {
		credential.Zero(rootKey)
		return nil, ErrIdentityUnavailable
	}
	return rootKey, nil
}

func encryptIdentityState(state identityState, rootKey []byte, aad []byte) ([]byte, error) {
	plaintext, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	defer credential.Zero(plaintext)
	block, err := aes.NewCipher(rootKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)
	record, err := json.Marshal(encryptedIdentityState{FormatVersion: identityStateFormatVersion, Nonce: nonce, Ciphertext: ciphertext})
	credential.Zero(nonce)
	credential.Zero(ciphertext)
	if err != nil {
		return nil, err
	}
	return record, nil
}

func decryptIdentityState(content, rootKey, aad []byte) (identityState, error) {
	var record encryptedIdentityState
	if err := decodeStrictJSON(content, &record); err != nil || record.FormatVersion != identityStateFormatVersion || len(record.Nonce) == 0 || len(record.Ciphertext) == 0 {
		return identityState{}, errors.New("identity state record is invalid")
	}
	defer credential.Zero(record.Nonce)
	defer credential.Zero(record.Ciphertext)
	block, err := aes.NewCipher(rootKey)
	if err != nil {
		return identityState{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(record.Nonce) != gcm.NonceSize() {
		return identityState{}, errors.New("identity state cipher is invalid")
	}
	plaintext, err := gcm.Open(nil, record.Nonce, record.Ciphertext, aad)
	if err != nil {
		return identityState{}, err
	}
	defer credential.Zero(plaintext)
	var state identityState
	if err := decodeStrictJSON(plaintext, &state); err != nil {
		return identityState{}, err
	}
	return state, nil
}

func validEnrollmentConfig(input EnrollmentConfig) bool {
	return validControlPlaneURL(input.ControlPlaneURL) && validOptionalAbsolutePath(input.CAFile) && validOpaqueValue(input.EnrollmentID, 256) && validOpaqueValue(input.NodeID, 256) && len(input.EnrollmentMaterial) >= 32 && len(input.EnrollmentMaterial) <= 4096
}

func validIdentityState(state identityState) bool {
	if state.FormatVersion != identityStateFormatVersion || !validControlPlaneURL(state.ControlPlaneURL) || !validOptionalAbsolutePath(state.CAFile) || state.ProtocolVersion != Version || !validOpaqueValue(state.AgentID, 256) || !validOpaqueValue(state.NodeID, 256) || len(state.MachineCredential) < 32 || len(state.MachineCredential) > 4096 {
		return false
	}
	if state.pendingEnrollment() {
		return validOpaqueValue(state.EnrollmentID, 256) && len(state.EnrollmentMaterial) >= 32 && len(state.EnrollmentMaterial) <= 4096 && validOpaqueValue(state.ExchangeRequestID, 256)
	}
	return state.EnrollmentID == "" && len(state.EnrollmentMaterial) == 0 && state.ExchangeRequestID == ""
}

func validRuntimeConfiguration(configuration RuntimeConfiguration) bool {
	if configuration.Revision < 1 || !validOpaqueValue(configuration.Platform, 32) || !validSHA256Digest(configuration.Digest) || !validOptionalAbsolutePath(configuration.ToolHome) || configuration.ToolHome == "" ||
		!validOptionalAbsolutePath(configuration.JavaPath) || configuration.JavaPath == "" || len(configuration.AllowedRoots) == 0 || len(configuration.AllowedRoots) > 32 {
		return false
	}
	for _, root := range configuration.AllowedRoots {
		if !outputpath.IsAllowedRootPath(configuration.Platform, root) {
			return false
		}
	}
	return true
}

func validHeartbeat(input Heartbeat) bool {
	facts := input.Facts
	if !validOpaqueValue(input.BootID, 256) || input.SentAt.IsZero() || facts.ObservedAt.IsZero() || facts.CapacityTotal != 1 || facts.CapacityUsed < 0 || facts.CapacityUsed > facts.CapacityTotal || !validOpaqueValue(facts.AgentVersion, 128) {
		return false
	}
	if facts.OperatingSystem != "WINDOWS" && facts.OperatingSystem != "LINUX" {
		return false
	}
	if facts.Architecture != "AMD64" && facts.Architecture != "ARM64" {
		return false
	}
	for _, value := range []*int{facts.CPUUsagePercent, facts.MemoryUsagePercent} {
		if value != nil && (*value < 0 || *value > 100) {
			return false
		}
	}
	if facts.RuntimeConfigurationDigest != "" && !validSHA256Digest(facts.RuntimeConfigurationDigest) {
		return false
	}
	if len(facts.DataRootUsages) > 32 {
		return false
	}
	seenRoots := make(map[string]struct{}, len(facts.DataRootUsages))
	for _, usage := range facts.DataRootUsages {
		if !validSHA256Digest(usage.RootDigest) || usage.AvailableBytes > usage.TotalBytes {
			return false
		}
		if _, exists := seenRoots[usage.RootDigest]; exists {
			return false
		}
		seenRoots[usage.RootDigest] = struct{}{}
	}
	return true
}

func validSHA256Digest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

func validOpaqueValue(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && value == strings.TrimSpace(value) && !strings.ContainsRune(value, 0) && !strings.ContainsAny(value, "\r\n")
}

func validControlPlaneURL(value string) bool {
	_, err := parseHTTPSURL(value)
	return err == nil
}

func validOptionalAbsolutePath(value string) bool {
	return value == "" || (!strings.ContainsRune(value, 0) && filepath.IsAbs(value) && filepath.Clean(value) == value)
}

func identityAAD(path string) []byte {
	return []byte(identityStateFormatVersion + "|" + filepath.Clean(path))
}

func (s *identityState) pendingEnrollment() bool {
	return s.EnrollmentID != "" || len(s.EnrollmentMaterial) != 0 || s.ExchangeRequestID != ""
}

func (s *identityState) destroy() {
	if s == nil {
		return
	}
	credential.Zero(s.MachineCredential)
	credential.Zero(s.EnrollmentMaterial)
	s.MachineCredential = nil
	s.EnrollmentMaterial = nil
}

func newOpaqueID() (string, error) {
	return identifier.NewUUIDV4()
}

func newOpaqueSecret() ([]byte, error) {
	raw := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return nil, err
	}
	defer credential.Zero(raw)
	encoded := make([]byte, base64.RawURLEncoding.EncodedLen(len(raw)))
	base64.RawURLEncoding.Encode(encoded, raw)
	return encoded, nil
}
