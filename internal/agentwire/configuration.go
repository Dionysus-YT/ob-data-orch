package agentwire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"ob-data-orch/internal/credential"
)

// HoldRuntimeConfiguration 冻结本进程内工具执行所依赖的配置，返回必须释放的读锁。
func (s *StateStore) HoldRuntimeConfiguration() func() {
	s.runtimeMu.RLock()
	return s.runtimeMu.RUnlock
}

// HasIdentity 区分首次启动和身份故障；已有损坏状态不能被自动注册覆盖。
func (s *StateStore) HasIdentity() (bool, error) {
	state, found, err := s.loadState()
	defer state.destroy()
	return found, err
}

// SyncRuntimeConfiguration 仅在没有运行中工具时拉取配置并原子更新加密状态，保留机器身份。
// 配置摘要、平台、版本和路径均须通过校验；环境可用性仍由随后的本机固定检查报告。
func (s *StateStore) SyncRuntimeConfiguration(ctx context.Context) error {
	if !s.runtimeMu.TryLock() {
		return nil
	}
	defer s.runtimeMu.Unlock()
	state, found, err := s.loadState()
	if err != nil {
		return err
	}
	defer state.destroy()
	if !found || state.pendingEnrollment() {
		return ErrIdentityUnavailable
	}
	client, err := newHTTPSClient(state.ControlPlaneURL, state.CAFile)
	if err != nil {
		return err
	}
	content, status, err := client.postWithRetry(ctx, "/agent/v1/runtime-configuration:sync", []byte(`{"protocolVersion":"`+Version+`"}`), state.MachineCredential)
	if err != nil {
		return err
	}
	defer credential.Zero(content)
	// 升级期间旧控制面尚无此端点时，保留本机配置，仍由既有心跳验证机器身份。
	if status == http.StatusNotFound {
		return nil
	}
	if status != http.StatusOK {
		return ErrProtocolRejected
	}
	envelope, err := decodeResponseEnvelope(content, "CONFIGURED")
	if err != nil {
		return ErrProtocolRejected
	}
	var configuration RuntimeConfiguration
	if decodeStrictJSON(envelope.Payload, &configuration) != nil || !validRuntimeConfiguration(configuration) || configuration.Platform != state.RuntimeConfiguration.Platform || configuration.Revision < state.RuntimeConfiguration.Revision || configuration.Digest != runtimeConfigurationDigest(configuration) {
		return ErrProtocolRejected
	}
	if configuration.Digest == state.RuntimeConfiguration.Digest && configuration.Revision == state.RuntimeConfiguration.Revision {
		return nil
	}
	if configuration.Revision == state.RuntimeConfiguration.Revision {
		return ErrProtocolRejected
	}
	state.RuntimeConfiguration = configuration
	return s.saveState(&state)
}

func runtimeConfigurationDigest(configuration RuntimeConfiguration) string {
	content, _ := json.Marshal(struct {
		Platform     string   `json:"platform"`
		ToolHome     string   `json:"toolHome"`
		JavaPath     string   `json:"javaPath"`
		AllowedRoots []string `json:"allowedRoots"`
	}{configuration.Platform, configuration.ToolHome, configuration.JavaPath, configuration.AllowedRoots})
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}
