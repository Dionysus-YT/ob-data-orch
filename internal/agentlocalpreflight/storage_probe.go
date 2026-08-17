package agentlocalpreflight

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"ob-data-orch/internal/agentpreflight"
)

// StorageConnectivityProber 是对象存储端点网络可达性的窄探测边界。
// 它只接收控制面从受控 URI 解析出的 endpoint 主机（不含凭据与对象路径），
// 返回受控布尔结论；任何错误都必须投影为“事实不可用”，不能把云服务错误原文传给预检查结果。
type StorageConnectivityProber interface {
	ProbeConnectivity(ctx context.Context, endpoint string) (bool, error)
}

// StorageAuthProber 是对象存储凭据有效性的窄探测边界。
// 真实凭据探测需要四类云厂商的签名协议与显式授权，归 EX-V1 排期；
// 本切片只保留接口与失败关闭默认实现，绝不提前解析或发送真实凭据。
type StorageAuthProber interface {
	ProbeAuth(ctx context.Context, provider string, accessKey, secretKey []byte, target agentpreflight.StorageTarget) (bool, error)
}

var (
	// ErrStorageConnectivityUnavailable 表示本机无法对存储端点给出可验证结论。
	ErrStorageConnectivityUnavailable = errors.New("存储端点连通性不可用")
	// ErrStorageAuthUnavailable 表示凭据有效性探测未授权或未实现。
	ErrStorageAuthUnavailable = errors.New("存储凭据探测未授权")
)

// TCPStorageConnectivityProber 对 endpoint 执行单次受控 TCP 建连（默认 443 端口）。
// 它不发送任何凭据或业务数据；真实网络探测必须由显式运行开关启用，
// 本机 MVP/Agent 包启动器开启该开关即视为对当前进程生命周期的持续授权。
type TCPStorageConnectivityProber struct {
	DialTimeout time.Duration
}

// ProbeConnectivity 只接受有限端点主机形态：字母、数字、点、连字符、冒号与可选端口。
// 建连成功即可达；连接失败投影为不可达，其余错误投影为不可用，均不携带底层错误原文。
func (p TCPStorageConnectivityProber) ProbeConnectivity(ctx context.Context, endpoint string) (bool, error) {
	if ctx == nil || ctx.Err() != nil {
		return false, ErrStorageConnectivityUnavailable
	}
	host, port, ok := parseStorageEndpoint(endpoint)
	if !ok {
		return false, ErrStorageConnectivityUnavailable
	}
	timeout := p.DialTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return false, ErrStorageConnectivityUnavailable
		}
		// DNS 解析失败、拒绝连接等统一投影为不可达；不区分具体网络原因。
		return false, nil
	}
	_ = conn.Close()
	return true, nil
}

// parseStorageEndpoint 把受控 endpoint 拆成主机与端口；未写端口时使用对象存储标准 HTTPS 端口 443。
// 主机只接受 DNS 形态的受限字符集，端口必须是 1~65535 的十进制数。
func parseStorageEndpoint(endpoint string) (host, port string, ok bool) {
	if endpoint == "" || len(endpoint) > 253 {
		return "", "", false
	}
	host, port = endpoint, "443"
	if colon := strings.LastIndex(endpoint, ":"); colon >= 0 {
		host, port = endpoint[:colon], endpoint[colon+1:]
	}
	if host == "" || host == "." || strings.HasSuffix(host, "-") || strings.HasPrefix(host, "-") {
		return "", "", false
	}
	for _, char := range host {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '.' || char == '-') {
			return "", "", false
		}
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", "", false
	}
	return host, port, true
}

// storageConnectivityResult 复核端点探测结论并投影为受控证据码。
func (p Probe) storageConnectivityResult(ctx context.Context, request agentpreflight.Request) (agentpreflight.Result, error) {
	check := agentpreflight.CheckStorageConnectivity
	if p.StorageConnectivity == nil || request.StorageTarget == nil || request.StorageTarget.Endpoint == "" {
		// 探测未装配或 URI 未携带 endpoint（仅 region）时，无法给出可验证结论。
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: "STORAGE_CONNECTIVITY_UNAVAILABLE"}, nil
	}
	reachable, err := p.StorageConnectivity.ProbeConnectivity(ctx, request.StorageTarget.Endpoint)
	switch {
	case err == nil && reachable:
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: "STORAGE_ENDPOINT_REACHABLE"}, nil
	case err == nil:
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusFailed, EvidenceCode: "STORAGE_ENDPOINT_UNREACHABLE"}, nil
	default:
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: "STORAGE_CONNECTIVITY_UNAVAILABLE"}, nil
	}
}

// storageAuthResult 复核凭据有效性探测结论；探测未授权或失败一律投影为 UNKNOWN 失败关闭。
func (p Probe) storageAuthResult(ctx context.Context, request agentpreflight.Request) (agentpreflight.Result, error) {
	check := agentpreflight.CheckStorageAuth
	if p.StorageAuth == nil || request.StorageTarget == nil {
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: "STORAGE_AUTH_UNAVAILABLE"}, nil
	}
	verified, err := p.StorageAuth.ProbeAuth(ctx, request.StorageTarget.Provider, nil, nil, *request.StorageTarget)
	if err != nil {
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusUnknown, EvidenceCode: "STORAGE_AUTH_UNAVAILABLE"}, nil
	}
	if verified {
		return agentpreflight.Result{Check: check, Status: agentpreflight.StatusPassed, EvidenceCode: "STORAGE_CREDENTIAL_VERIFIED"}, nil
	}
	return agentpreflight.Result{Check: check, Status: agentpreflight.StatusFailed, EvidenceCode: "STORAGE_CREDENTIAL_REJECTED"}, nil
}

// UnavailableStorageAuthProber 是 EX-V1 前凭据探测的固定失败关闭实现。
// 它不解析真实凭据、不发起任何网络连接。
type UnavailableStorageAuthProber struct{}

// ProbeAuth 固定返回未授权错误，让调用方投影为 STORAGE_AUTH_UNAVAILABLE。
func (UnavailableStorageAuthProber) ProbeAuth(context.Context, string, []byte, []byte, agentpreflight.StorageTarget) (bool, error) {
	return false, fmt.Errorf("%w: 真实凭据探测归 EX-V1 排期", ErrStorageAuthUnavailable)
}
