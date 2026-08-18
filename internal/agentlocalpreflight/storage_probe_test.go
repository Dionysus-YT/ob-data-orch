package agentlocalpreflight

import (
	"context"
	"errors"
	"testing"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/agentstate"
	"ob-data-orch/internal/commandgen"
)

// storageRequest 构造对象存储输出任务的预检查请求（EX-I6 存储专用预检查）。
func storageRequest(tmpPath string) agentpreflight.Request {
	binding := agentstate.PrecheckBinding{PrecheckID: "precheck-1", NodeID: "node-1", DraftRevision: 1, ConfigFingerprint: "synthetic-fingerprint", CredentialRevision: 1, NodeFactsVersion: 1}
	return agentpreflight.Request{
		Capability: agentpreflight.CapabilityExportPreflight, PrecheckID: binding.PrecheckID, NodeID: binding.NodeID,
		AgentID: "agent-1", LeaseID: "lease-1", LeaseEpoch: 1, Binding: binding,
		CompatibilityMode: "MYSQL", Database: "synthetic_db", Objects: []string{"synthetic_table"},
		ContentKind: "DATA_ONLY", TargetPlatform: commandgen.PlatformWindowsAMD64,
		OutputPath:   "oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou.aliyuncs.com",
		AllowedRoots: []string{`E:\synthetic`},
		OutputKind:   agentpreflight.OutputKindOSS,
		StorageTarget: &agentpreflight.StorageTarget{
			Provider: "OSS", URI: "oss://synthetic-bucket/exports?endpoint=oss-cn-hangzhou.aliyuncs.com",
			Endpoint: "oss-cn-hangzhou.aliyuncs.com", TmpPath: tmpPath,
		},
	}
}

type recordingConnectivityProber struct {
	reachable bool
	err       error
	calls     int
}

func (p *recordingConnectivityProber) ProbeConnectivity(context.Context, string) (bool, error) {
	p.calls++
	return p.reachable, p.err
}

type recordingAuthProber struct {
	verified bool
	err      error
	calls    int
}

func (p *recordingAuthProber) ProbeAuth(context.Context, string, []byte, []byte, agentpreflight.StorageTarget) (bool, error) {
	p.calls++
	return p.verified, p.err
}

func (*recordingAuthProber) RequiresStorageCredentials() bool { return true }

type recordingStorageCredentialResolver struct {
	credential agentpreflight.StorageCredential
	calls      int
}

func (r *recordingStorageCredentialResolver) ResolveStorageCredential(context.Context, agentstate.PrecheckBinding) (agentpreflight.StorageCredential, error) {
	r.calls++
	return agentpreflight.StorageCredential{
		Provider: r.credential.Provider, AccessKey: append([]byte(nil), r.credential.AccessKey...), SecretKey: append([]byte(nil), r.credential.SecretKey...),
	}, nil
}

func TestParseStorageEndpoint(t *testing.T) {
	t.Parallel()
	if host, port, ok := parseStorageEndpoint("oss-cn-hangzhou.aliyuncs.com"); !ok || host != "oss-cn-hangzhou.aliyuncs.com" || port != "443" {
		t.Fatalf("无端口解析 = %q %q %t", host, port, ok)
	}
	if host, port, ok := parseStorageEndpoint("s3.example.com:9000"); !ok || host != "s3.example.com" || port != "9000" {
		t.Fatalf("带端口解析 = %q %q %t", host, port, ok)
	}
	for _, invalid := range []string{"", "host:0", "host:99999", "host:not-a-port", "-host", "host-", "host name", "https://host"} {
		if _, _, ok := parseStorageEndpoint(invalid); ok {
			t.Fatalf("非法端点被接受：%q", invalid)
		}
	}
}

func TestStorageConnectivityResult(t *testing.T) {
	t.Parallel()
	// 未装配探测：固定 UNKNOWN，不发起网络连接。
	unassembled := Probe{}
	result, err := unassembled.storageConnectivityResult(context.Background(), storageRequest(""))
	if err != nil || result.Status != agentpreflight.StatusUnknown || result.EvidenceCode != "STORAGE_CONNECTIVITY_UNAVAILABLE" {
		t.Fatalf("未装配结果 = %#v, %v", result, err)
	}
	// URI 未携带 endpoint（仅 region）：无法确定探测目标，固定 UNKNOWN。
	request := storageRequest("")
	request.StorageTarget.Endpoint = ""
	result, err = unassembled.storageConnectivityResult(context.Background(), request)
	if err != nil || result.Status != agentpreflight.StatusUnknown {
		t.Fatalf("无端点结果 = %#v, %v", result, err)
	}
	// 可达 → PASSED；不可达 → FAILED；探测异常 → UNKNOWN，均不携带底层错误原文。
	for name, want := range map[string]struct {
		prober StorageConnectivityProber
		status agentpreflight.Status
		code   string
	}{
		"可达":   {&recordingConnectivityProber{reachable: true}, agentpreflight.StatusPassed, "STORAGE_ENDPOINT_REACHABLE"},
		"不可达":  {&recordingConnectivityProber{}, agentpreflight.StatusFailed, "STORAGE_ENDPOINT_UNREACHABLE"},
		"探测异常": {&recordingConnectivityProber{err: errors.New("synthetic-secret-not-allowed")}, agentpreflight.StatusUnknown, "STORAGE_CONNECTIVITY_UNAVAILABLE"},
	} {
		probe := Probe{StorageConnectivity: want.prober}
		result, err := probe.storageConnectivityResult(context.Background(), storageRequest(""))
		if err != nil || result.Status != want.status || result.EvidenceCode != want.code {
			t.Fatalf("%s 结果 = %#v, %v", name, result, err)
		}
	}
}

func TestStorageAuthResult(t *testing.T) {
	t.Parallel()
	// 未装配：固定 UNKNOWN，不解析任何凭据。
	probe := Probe{}
	result, err := probe.storageAuthResult(context.Background(), storageRequest(""))
	if err != nil || result.Status != agentpreflight.StatusUnknown || result.EvidenceCode != "STORAGE_AUTH_UNAVAILABLE" {
		t.Fatalf("未装配结果 = %#v, %v", result, err)
	}
	// EX-V1 前的固定失败关闭实现：绝不返回 PASSED/FAILED 或解析真实凭据。
	defaultResolver := &recordingStorageCredentialResolver{credential: agentpreflight.StorageCredential{Provider: "OSS", AccessKey: []byte("synthetic-access"), SecretKey: []byte("synthetic-secret")}}
	result, err = (Probe{StorageAuth: UnavailableStorageAuthProber{}, StorageCredentials: defaultResolver}).storageAuthResult(context.Background(), storageRequest(""))
	if err != nil || result.Status != agentpreflight.StatusUnknown || result.EvidenceCode != "STORAGE_AUTH_UNAVAILABLE" {
		t.Fatalf("默认凭据探测结果 = %#v, %v", result, err)
	}
	if defaultResolver.calls != 0 {
		t.Fatalf("默认失败关闭探测器解析凭据 %d 次", defaultResolver.calls)
	}
	// 探测接口本身受控投影：verified → PASSED，拒绝 → FAILED，异常 → UNKNOWN。
	for name, want := range map[string]struct {
		prober StorageAuthProber
		status agentpreflight.Status
		code   string
	}{
		"凭据有效": {&recordingAuthProber{verified: true}, agentpreflight.StatusPassed, "STORAGE_CREDENTIAL_VERIFIED"},
		"凭据被拒": {&recordingAuthProber{}, agentpreflight.StatusFailed, "STORAGE_CREDENTIAL_REJECTED"},
		"探测异常": {&recordingAuthProber{err: errors.New("synthetic-secret-not-allowed")}, agentpreflight.StatusUnknown, "STORAGE_AUTH_UNAVAILABLE"},
	} {
		resolver := &recordingStorageCredentialResolver{credential: agentpreflight.StorageCredential{Provider: "OSS", AccessKey: []byte("synthetic-access"), SecretKey: []byte("synthetic-secret")}}
		result, err := (Probe{StorageAuth: want.prober, StorageCredentials: resolver}).storageAuthResult(context.Background(), storageRequest(""))
		if err != nil || result.Status != want.status || result.EvidenceCode != want.code {
			t.Fatalf("%s 结果 = %#v, %v", name, result, err)
		}
		if resolver.calls != 1 {
			t.Fatalf("%s 凭据解析次数 = %d，期望 1", name, resolver.calls)
		}
	}
}

func TestStorageAvailableSpaceUsesTmpPathVolume(t *testing.T) {
	t.Parallel()
	// 未指定 --tmp-path：无法定位卷，固定 UNKNOWN 失败关闭。
	probe := Probe{MinimumAvailableBytes: 1 << 30, AvailableBytes: func(string) (uint64, error) { return 1 << 31, nil }}
	result := probe.availableSpaceResult(storageRequest(""))
	if result.Status != agentpreflight.StatusUnknown || result.EvidenceCode != "OUTPUT_SPACE_UNAVAILABLE" {
		t.Fatalf("无 tmp-path 空间结果 = %#v", result)
	}
	// 指定 tmp-path：空间检查必须落在该目录（或最近既有父目录），而不是 URI。
	root := t.TempDir()
	tmpDir := root + `\tmp`
	request := storageRequest(tmpDir)
	request.AllowedRoots = []string{root}
	probe = Probe{
		MinimumAvailableBytes: 1 << 30,
		AvailableBytes: func(path string) (uint64, error) {
			if path != root {
				t.Fatalf("空间检查目标 = %q，期望 %q", path, root)
			}
			return 1 << 30, nil
		},
	}
	result = probe.availableSpaceResult(request)
	if result.Status != agentpreflight.StatusPassed || result.EvidenceCode != "OUTPUT_SPACE_SUFFICIENT" {
		t.Fatalf("tmp-path 空间结果 = %#v", result)
	}
}
