package agentwire

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"ob-data-orch/internal/credential"
)

const (
	maximumProtocolResponseBytes = 1 << 20
	protocolRequestTimeout       = 20 * time.Second
)

type httpsClient struct {
	baseURL    *url.URL
	httpClient *http.Client
}

type enrollmentExchangeRequest struct {
	RequestID          string `json:"requestId"`
	EnrollmentID       string `json:"enrollmentId"`
	EnrollmentMaterial string `json:"enrollmentMaterial"`
	AgentID            string `json:"agentId"`
	NodeID             string `json:"nodeId"`
	MachineCredential  string `json:"machineCredential"`
	ProtocolVersion    string `json:"protocolVersion"`
}

type heartbeatRequest struct {
	ProtocolVersion string           `json:"protocolVersion"`
	AgentID         string           `json:"agentId"`
	NodeID          string           `json:"nodeId"`
	BootID          string           `json:"bootId"`
	RequestID       string           `json:"requestId"`
	SentAt          string           `json:"sentAt"`
	PayloadType     string           `json:"payloadType"`
	Payload         heartbeatPayload `json:"payload"`
}

type heartbeatPayload struct {
	OperatingSystem            string                 `json:"operatingSystem"`
	Architecture               string                 `json:"architecture"`
	AgentVersion               string                 `json:"agentVersion"`
	ObservedAt                 string                 `json:"observedAt"`
	CapacityTotal              int                    `json:"capacityTotal"`
	CapacityUsed               int                    `json:"capacityUsed"`
	CPUUsagePercent            *int                   `json:"cpuUsagePercent,omitempty"`
	MemoryUsagePercent         *int                   `json:"memoryUsagePercent,omitempty"`
	RuntimeConfigurationDigest string                 `json:"runtimeConfigurationDigest,omitempty"`
	DataRootUsages             []dataRootUsagePayload `json:"dataRootUsages,omitempty"`
}

type dataRootUsagePayload struct {
	RootDigest     string `json:"rootDigest"`
	TotalBytes     uint64 `json:"totalBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
}

type responseEnvelope struct {
	RequestID  string          `json:"requestId"`
	ServerTime string          `json:"serverTime"`
	Status     string          `json:"status"`
	Payload    json.RawMessage `json:"payload"`
}

type enrollmentPayload struct {
	AgentID              string               `json:"agentId"`
	NodeID               string               `json:"nodeId"`
	ProtocolVersion      string               `json:"protocolVersion"`
	Replayed             *bool                `json:"replayed"`
	RealExecutionEnabled *bool                `json:"realExecutionEnabled"`
	RuntimeConfiguration RuntimeConfiguration `json:"runtimeConfiguration"`
}

// RuntimeConfiguration 是首次关联时由控制面返回、随后仅由 Agent 本机状态持有的固定工具配置。
// 它没有秘密，但任何空值或非法值都不能让 Agent 退化为读取环境变量或系统 PATH。
type RuntimeConfiguration struct {
	Platform     string   `json:"platform"`
	ToolHome     string   `json:"toolHome"`
	JavaPath     string   `json:"javaPath"`
	AllowedRoots []string `json:"allowedRoots"`
	Revision     int64    `json:"revision"`
	Digest       string   `json:"digest"`
}

type heartbeatResponsePayload struct {
	FactsRevision        int64  `json:"factsRevision"`
	EnvironmentStatus    string `json:"environmentStatus"`
	EnvironmentCheckID   string `json:"environmentCheckId,omitempty"`
	RealExecutionEnabled *bool  `json:"realExecutionEnabled"`
}

// executionNodeEnvironmentCheckCompletionRequest 是 Agent 对唯一固定运行时检查的回执。
// 载荷不包含路径、命令、工具输出、连接参数或任意可扩展检查项。
type executionNodeEnvironmentCheckCompletionRequest struct {
	ProtocolVersion string                                  `json:"protocolVersion"`
	AgentID         string                                  `json:"agentId"`
	NodeID          string                                  `json:"nodeId"`
	BootID          string                                  `json:"bootId"`
	RequestID       string                                  `json:"requestId"`
	SentAt          string                                  `json:"sentAt"`
	PayloadType     string                                  `json:"payloadType"`
	Payload         executionNodeEnvironmentCheckCompletion `json:"payload"`
}

type executionNodeEnvironmentCheckCompletion struct {
	FactsRevision int64  `json:"factsRevision"`
	Status        string `json:"status"`
	Code          string `json:"code"`
}

// newHTTPSClient 构造仅允许 HTTPS 的控制面客户端。
// 显式设置 ServerName、根证书池和禁止重定向，确保机器凭据不会因错误端点或 TLS 降级泄露。
func newHTTPSClient(endpoint, caFile string) (*httpsClient, error) {
	baseURL, err := parseHTTPSURL(endpoint)
	if err != nil {
		return nil, ErrIdentityUnavailable
	}
	rootCAs, err := loadRootCAs(caFile)
	if err != nil {
		return nil, ErrIdentityUnavailable
	}
	transport := &http.Transport{
		Proxy:                 nil,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    rootCAs,
			ServerName: baseURL.Hostname(),
		},
	}
	return &httpsClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   protocolRequestTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *httpsClient) exchangeEnrollment(ctx context.Context, state *identityState) (RuntimeConfiguration, error) {
	request := enrollmentExchangeRequest{
		RequestID:          state.ExchangeRequestID,
		EnrollmentID:       state.EnrollmentID,
		EnrollmentMaterial: string(state.EnrollmentMaterial),
		AgentID:            state.AgentID,
		NodeID:             state.NodeID,
		MachineCredential:  string(state.MachineCredential),
		ProtocolVersion:    state.ProtocolVersion,
	}
	requestBody, err := json.Marshal(request)
	request.EnrollmentMaterial = ""
	request.MachineCredential = ""
	if err != nil {
		return RuntimeConfiguration{}, ErrIdentityUnavailable
	}
	defer credential.Zero(requestBody)
	responseBody, status, err := c.postWithRetry(ctx, "/agent/v1/enrollments:exchange", requestBody, nil)
	if err != nil {
		return RuntimeConfiguration{}, err
	}
	defer credential.Zero(responseBody)
	if status != http.StatusOK {
		if status >= http.StatusInternalServerError {
			return RuntimeConfiguration{}, ErrControlPlaneUnavailable
		}
		return RuntimeConfiguration{}, ErrEnrollmentRejected
	}
	envelope, err := decodeResponseEnvelope(responseBody, "ENROLLED")
	if err != nil {
		return RuntimeConfiguration{}, ErrProtocolRejected
	}
	var payload enrollmentPayload
	if err := decodeStrictJSON(envelope.Payload, &payload); err != nil || payload.AgentID != state.AgentID || payload.NodeID != state.NodeID || payload.ProtocolVersion != Version || payload.Replayed == nil || payload.RealExecutionEnabled == nil || *payload.RealExecutionEnabled {
		return RuntimeConfiguration{}, ErrProtocolRejected
	}
	return payload.RuntimeConfiguration, nil
}

func (c *httpsClient) sendHeartbeat(ctx context.Context, state *identityState, requestID string, input Heartbeat) (HeartbeatResult, error) {
	request := heartbeatRequest{
		ProtocolVersion: state.ProtocolVersion,
		AgentID:         state.AgentID,
		NodeID:          state.NodeID,
		BootID:          input.BootID,
		RequestID:       requestID,
		SentAt:          input.SentAt.UTC().Format(time.RFC3339Nano),
		PayloadType:     "HEARTBEAT",
		Payload: heartbeatPayload{
			OperatingSystem:    input.Facts.OperatingSystem,
			Architecture:       input.Facts.Architecture,
			AgentVersion:       input.Facts.AgentVersion,
			ObservedAt:         input.Facts.ObservedAt.UTC().Format(time.RFC3339Nano),
			CapacityTotal:      input.Facts.CapacityTotal,
			CapacityUsed:       input.Facts.CapacityUsed,
			CPUUsagePercent:    input.Facts.CPUUsagePercent,
			MemoryUsagePercent: input.Facts.MemoryUsagePercent, RuntimeConfigurationDigest: input.Facts.RuntimeConfigurationDigest,
			DataRootUsages: dataRootUsagesPayload(input.Facts.DataRootUsages),
		},
	}
	requestBody, err := json.Marshal(request)
	if err != nil {
		return HeartbeatResult{}, ErrProtocolRejected
	}
	defer credential.Zero(requestBody)
	responseBody, status, err := c.postWithRetry(ctx, "/agent/v1/heartbeats", requestBody, state.MachineCredential)
	if err != nil {
		return HeartbeatResult{}, err
	}
	defer credential.Zero(responseBody)
	if status != http.StatusOK {
		if status >= http.StatusInternalServerError {
			return HeartbeatResult{}, ErrControlPlaneUnavailable
		}
		return HeartbeatResult{}, ErrAgentAuthenticationDenied
	}
	envelope, err := decodeResponseEnvelope(responseBody, "ACCEPTED")
	if err != nil {
		return HeartbeatResult{}, ErrProtocolRejected
	}
	var payload heartbeatResponsePayload
	if err := decodeStrictJSON(envelope.Payload, &payload); err != nil || payload.FactsRevision < 1 || payload.EnvironmentStatus != "NOT_CHECKED" || !validOptionalEnvironmentCheckID(payload.EnvironmentCheckID) || payload.RealExecutionEnabled == nil || *payload.RealExecutionEnabled {
		return HeartbeatResult{}, ErrProtocolRejected
	}
	return HeartbeatResult{FactsRevision: payload.FactsRevision, EnvironmentCheckID: payload.EnvironmentCheckID}, nil
}

func dataRootUsagesPayload(usages []DataRootUsage) []dataRootUsagePayload {
	if len(usages) == 0 {
		return nil
	}
	result := make([]dataRootUsagePayload, 0, len(usages))
	for _, usage := range usages {
		result = append(result, dataRootUsagePayload{RootDigest: usage.RootDigest, TotalBytes: usage.TotalBytes, AvailableBytes: usage.AvailableBytes})
	}
	return result
}

func (c *httpsClient) completeExecutionNodeEnvironmentCheck(ctx context.Context, state *identityState, requestID, bootID, checkID string, factsRevision int64, status, code string, sentAt time.Time) error {
	if state == nil || !validOpaqueValue(bootID, 256) || !validEnvironmentCheckID(checkID) || factsRevision < 1 || !validEnvironmentCheckResult(status, code) || sentAt.IsZero() {
		return ErrProtocolRejected
	}
	request := executionNodeEnvironmentCheckCompletionRequest{
		ProtocolVersion: state.ProtocolVersion, AgentID: state.AgentID, NodeID: state.NodeID, BootID: bootID,
		RequestID: requestID, SentAt: sentAt.UTC().Format(time.RFC3339Nano), PayloadType: "EXECUTION_NODE_ENVIRONMENT_CHECK_COMPLETE",
		Payload: executionNodeEnvironmentCheckCompletion{FactsRevision: factsRevision, Status: status, Code: code},
	}
	content, err := json.Marshal(request)
	if err != nil {
		return ErrProtocolRejected
	}
	defer credential.Zero(content)
	responseBody, httpStatus, err := c.postWithRetry(ctx, "/agent/v1/execution-node-environment-checks/"+checkID+":complete", content, state.MachineCredential)
	if err != nil {
		return err
	}
	defer credential.Zero(responseBody)
	if httpStatus != http.StatusOK {
		if httpStatus >= http.StatusInternalServerError {
			return ErrControlPlaneUnavailable
		}
		if httpStatus == http.StatusUnauthorized || httpStatus == http.StatusForbidden {
			return ErrAgentAuthenticationDenied
		}
		// HTTP 状态本身是安全协议诊断，不包含控制面错误正文、路径、秘密或本机信息。
		return fmt.Errorf("%w: HTTP %d", ErrProtocolRejected, httpStatus)
	}
	if _, err := decodeResponseEnvelope(responseBody, "ENVIRONMENT_CHECK_COMPLETED"); err != nil {
		return ErrProtocolRejected
	}
	return nil
}

func validEnvironmentCheckID(value string) bool {
	return validOpaqueValue(value, 256) && !strings.ContainsAny(value, "/\\?#:")
}

func validOptionalEnvironmentCheckID(value string) bool {
	return value == "" || validEnvironmentCheckID(value)
}

func validEnvironmentCheckResult(status, code string) bool {
	return (status == "PASSED" && code == "TOOL_RUNTIME_READY") ||
		(status == "FAILED" && (code == "TOOL_RUNTIME_INVALID" || code == "TOOL_RUNTIME_UNAVAILABLE"))
}

func (c *httpsClient) postWithRetry(ctx context.Context, endpoint string, body, credentialValue []byte) ([]byte, int, error) {
	if c == nil || c.baseURL == nil || c.httpClient == nil || !strings.HasPrefix(endpoint, "/agent/v1/") {
		return nil, 0, ErrIdentityUnavailable
	}
	for attempt := 0; attempt < 2; attempt++ {
		responseBody, status, retry, err := c.postOnce(ctx, endpoint, body, credentialValue)
		if err == nil {
			return responseBody, status, nil
		}
		if responseBody != nil {
			credential.Zero(responseBody)
		}
		if !retry || attempt == 1 || ctx.Err() != nil {
			return nil, 0, err
		}
	}
	return nil, 0, ErrControlPlaneUnavailable
}

func (c *httpsClient) postOnce(ctx context.Context, endpoint string, body, credentialValue []byte) ([]byte, int, bool, error) {
	requestURL := c.requestURL(endpoint)
	if requestURL == nil {
		return nil, 0, false, ErrIdentityUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, 0, false, ErrIdentityUnavailable
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Cache-Control", "no-store")
	request.Header.Set("Content-Type", "application/json")
	if len(credentialValue) > 0 {
		request.Header.Set("Authorization", "Bearer "+string(credentialValue))
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, 0, true, ErrControlPlaneUnavailable
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maximumProtocolResponseBytes+1))
	if err != nil || len(responseBody) > maximumProtocolResponseBytes {
		credential.Zero(responseBody)
		return nil, 0, response.StatusCode >= http.StatusInternalServerError, ErrControlPlaneUnavailable
	}
	if response.StatusCode >= http.StatusInternalServerError {
		return responseBody, response.StatusCode, true, ErrControlPlaneUnavailable
	}
	return responseBody, response.StatusCode, false, nil
}

func (c *httpsClient) requestURL(endpoint string) *url.URL {
	if c == nil || c.baseURL == nil || !strings.HasPrefix(endpoint, "/agent/v1/") {
		return nil
	}
	result := *c.baseURL
	result.RawQuery = ""
	result.Fragment = ""
	result.RawPath = ""
	result.Path = strings.TrimSuffix(c.baseURL.Path, "/") + endpoint
	if !strings.HasPrefix(result.Path, "/") {
		result.Path = "/" + result.Path
	}
	return &result
}

func parseHTTPSURL(endpoint string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || strings.ContainsRune(parsed.Host, 0) || strings.ContainsRune(parsed.Path, 0) {
		return nil, errors.New("invalid HTTPS endpoint")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	return parsed, nil
}

func loadRootCAs(caFile string) (*x509.CertPool, error) {
	if caFile == "" {
		return nil, nil
	}
	if !validOptionalAbsolutePath(caFile) {
		return nil, errors.New("invalid CA file path")
	}
	content, err := os.ReadFile(caFile)
	if err != nil {
		return nil, errors.New("read CA file failed")
	}
	defer credential.Zero(content)
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(content) {
		return nil, errors.New("CA file is invalid")
	}
	return pool, nil
}

func decodeResponseEnvelope(content []byte, expectedStatus string) (responseEnvelope, error) {
	var envelope responseEnvelope
	if err := decodeStrictJSON(content, &envelope); err != nil || !validOpaqueValue(envelope.RequestID, 256) || envelope.Status != expectedStatus || len(envelope.Payload) == 0 {
		return responseEnvelope{}, errors.New("response envelope is invalid")
	}
	if _, err := time.Parse(time.RFC3339Nano, envelope.ServerTime); err != nil {
		return responseEnvelope{}, errors.New("response server time is invalid")
	}
	return envelope, nil
}

func decodeStrictJSON(content []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("unexpected JSON content")
	}
	return nil
}
