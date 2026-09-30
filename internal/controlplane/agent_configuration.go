package controlplane

import (
	"context"
	"net/http"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/store"
	"time"
)

type agentConfigurationReader interface {
	AgentRuntimeConfiguration(context.Context, []byte) (store.AgentEnrollmentRuntimeConfiguration, error)
}

func (s *Server) syncAgentConfiguration(w http.ResponseWriter, r *http.Request) {
	reader, ok := s.agentProtocol.(agentConfigurationReader)
	if !ok {
		writeError(w, 503, "AGENT_CONFIGURATION_UNAVAILABLE", "节点配置暂时不可用", true)
		return
	}
	digest, ok := agentCredentialDigest(r)
	if !ok {
		writeError(w, 401, "AGENT_AUTHENTICATION_FAILED", "Agent 机器认证失败", false)
		return
	}
	var request struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	if request.ProtocolVersion != agentwire.Version {
		writeError(w, 400, "AGENT_REQUEST_INVALID", "Agent 请求无效", false)
		return
	}
	configuration, err := reader.AgentRuntimeConfiguration(r.Context(), digest)
	if err != nil {
		writeError(w, 401, "AGENT_AUTHENTICATION_FAILED", "Agent 身份或节点配置不可用", false)
		return
	}
	payload := agentwire.RuntimeConfiguration{Platform: configuration.Platform, ToolHome: configuration.ToolHome, JavaPath: configuration.JavaPath, AllowedRoots: configuration.AllowedRoots, Revision: configuration.Revision, Digest: configuration.Digest}
	writeJSON(w, 200, map[string]any{"requestId": requestID(w), "serverTime": time.Now().UTC().Format(time.RFC3339Nano), "status": "CONFIGURED", "payload": payload})
}
