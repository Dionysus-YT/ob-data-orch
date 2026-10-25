package agentjdbc

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"ob-data-orch/internal/agentpreflight"
	"ob-data-orch/internal/commandgen"
	"ob-data-orch/internal/credential"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/directloadfiles"
	"ob-data-orch/internal/jdbcprobe"
	"ob-data-orch/internal/nodefacts"
)

func directLoadFixture(t *testing.T) agentpreflight.Request {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "synthetic_table.csv"), []byte("1,synthetic\n"), 0600); err != nil {
		t.Fatal(err)
	}
	platform := commandgen.PlatformLinuxAMD64
	if runtime.GOOS == "windows" {
		platform = commandgen.PlatformWindowsAMD64
	}
	c, issues := directload.Normalize(directload.Config{DataSourceID: "source-1", NodeID: "node-1", Database: "synthetic_db", Scenario: "OBSERVER_DIRECT", RPCPort: 2882, Table: "synthetic_table", FilePath: root, Format: "CSV", Applicability: directload.Applicability{SingleTable: true, WholeTableCommit: true, NoCheckpoint: true, StructureLimits: true}}, "MYSQL", string(platform), true)
	if len(issues) > 0 {
		t.Fatal("合成配置无效", issues)
	}
	m, err := directloadfiles.Scan(context.Background(), c, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	r := validRequest()
	r.Capability = "DIRECT_LOAD_PREFLIGHT"
	r.CheckKind = "DIRECT_LOAD_PREFLIGHT"
	r.DirectLoad = &c
	r.InputManifest = &m
	r.TargetPlatform = platform
	r.AllowedRoots = []string{root}
	r.Objects = nil
	r.OutputPath = ""
	return r
}

func directLoadSafeFacts() jdbcprobe.DirectLoadResult {
	yes, no := true, false
	return jdbcprobe.DirectLoadResult{DatabaseAccess: jdbcprobe.ObjectAccessible, WriteAccess: jdbcprobe.ObjectAccessible, ObserverVersion: "4.3.5.2", TableFacts: directload.TableFacts{Exists: &yes, Empty: &yes, HasBIT: &no, HasBinary: &no, HasVirtualGenerated: &no, HasIndex: &no, HasLOB: &no, HasUniqueIndex: &no}}
}

func Test旁路结构失败返回固定可操作原因(t *testing.T) {
	for _, name := range []string{"passed", "missing", "unknown", "bit", "binary", "virtual", "index", "lob", "full-replace", "incremental-replace"} {
		t.Run(name, func(t *testing.T) {
			c := directload.Config{ReplaceData: true}
			f := directLoadSafeFacts().TableFacts
			yes, no := true, false
			status, code := agentpreflight.StatusFailed, ""
			switch name {
			case "passed", "full-replace", "incremental-replace":
				status, code = agentpreflight.StatusPassed, "DIRECT_LOAD_VERIFIED"
				if name == "full-replace" {
					f.HasIndex, f.HasLOB = &yes, &yes
				}
				if name == "incremental-replace" {
					f.Empty = &no
				}
			case "missing":
				f.Exists, code = &no, "DIRECT_LOAD_TABLE_NOT_FOUND"
			case "unknown":
				f.HasBinary, status, code = nil, agentpreflight.StatusUnknown, "DIRECT_LOAD_TABLE_FACT_UNAVAILABLE"
			case "bit":
				f.HasBIT, code = &yes, "DIRECT_LOAD_BIT_BLOCKED"
			case "binary":
				f.HasBinary, code = &yes, "DIRECT_LOAD_BINARY_BLOCKED"
			case "virtual":
				f.HasVirtualGenerated, code = &yes, "DIRECT_LOAD_VIRTUAL_BLOCKED"
			case "index":
				f.Empty, f.HasIndex, code = &no, &yes, "DIRECT_LOAD_REPLACE_INDEX_BLOCKED"
			case "lob":
				f.Empty, f.HasLOB, code = &no, &yes, "DIRECT_LOAD_REPLACE_LOB_BLOCKED"
			}
			if got := directLoadStructureResult(c, f); got.Status != status || got.EvidenceCode != code {
				t.Fatal("结构原因不符", got)
			}
		})
	}
}

func directLoadSafeRuntime(context.Context, agentpreflight.Request) (jdbcprobe.DirectLoadResult, error) {
	a, b, c := int64(120000), int64(60000), int64(10000)
	config := &nodefacts.DirectLoadConfiguration{Status: "READ", TaskTimeoutMS: &a, HeartbeatTimeoutMS: &b, HeartbeatIntervalMS: &c}
	config.Fingerprint = nodefacts.DirectLoadFingerprint(*config)
	return jdbcprobe.DirectLoadResult{ToolVersion: directload.ToolVersion, DirectLoadConfig: config, ObservedAt: time.Now().UTC()}, nil
}

func directLoadSyntheticDial(_ context.Context, network, address string) (net.Conn, error) {
	left, right := net.Pipe()
	_ = right.Close()
	return left, nil
}

func Test旁路独立检查一次解析固定租约并清零秘密(t *testing.T) {
	r := directLoadFixture(t)
	root := t.TempDir()
	calls := 0
	resolver := &countingResolver{connection: Connection{Host: "synthetic.invalid", Port: 2881, Username: []byte("synthetic-user"), Password: []byte("synthetic-password")}}
	p := &DirectLoadProbe{WorkspaceRoot: root, Resolver: resolver, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != "synthetic.invalid:2882" {
			t.Fatal("TCP未绑定手工RPC端口和固定业务Host")
		}
		return directLoadSyntheticDial(ctx, network, address)
	}, ObserveRuntime: directLoadSafeRuntime, Run: func(_ context.Context, w credential.Workspace, _ jdbcprobe.Runtime, input jdbcprobe.DirectLoadRequest) (jdbcprobe.DirectLoadResult, error) {
		calls++
		if input.Table != "synthetic_table" || input.Database != "synthetic_db" || input.Scenario != "OBSERVER_DIRECT" || string(input.Connection.Password) != "synthetic-password" {
			t.Fatal("未使用旁路独立输入")
		}
		if _, err := os.Stat(w.RuntimeDirectory()); err != nil {
			t.Fatal(err)
		}
		return directLoadSafeFacts(), nil
	}}
	if p.DirectLoadConfigFingerprint(context.Background(), r) != "" {
		t.Fatal("未观测配置提前生成指纹")
	}
	for _, check := range directLoadProbeChecks[1:] {
		if result, _ := p.Probe(context.Background(), check, r); result.Status != agentpreflight.StatusUnknown || resolver.calls != 0 {
			t.Fatal("非数据库入口提前解析了秘密")
		}
	}
	for _, check := range directLoadProbeChecks {
		if result, err := p.Probe(context.Background(), check, r); err != nil || result.Status != agentpreflight.StatusPassed {
			t.Fatal("合成旁路事实未通过", check, result, err)
		}
	}
	if calls != 1 || resolver.calls != 1 || !allZero(resolver.connection.Username) || !allZero(resolver.connection.Password) {
		t.Fatal("重复解析或保留秘密")
	}
	local, _ := directLoadSafeRuntime(context.Background(), r)
	if p.DirectLoadConfigFingerprint(context.Background(), r) != local.DirectLoadConfig.Fingerprint {
		t.Fatal("已完成安全投影未输出同绑定指纹")
	}
	if _, err := os.Stat(filepath.Join(root, "executions", r.PrecheckID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("旁路短时工作区未清理")
	}
}

func Test旁路缓存拒绝配置租约主体和输入绑定漂移(t *testing.T) {
	for _, kind := range []string{"config", "credential", "facts", "lease", "epoch", "agent", "manifest", "mode", "roots"} {
		t.Run(kind, func(t *testing.T) {
			r := directLoadFixture(t)
			resolver := &countingResolver{connection: Connection{Host: "synthetic.invalid", Port: 2881, Username: []byte("synthetic-user"), Password: []byte("synthetic-password")}}
			p := &DirectLoadProbe{WorkspaceRoot: t.TempDir(), Resolver: resolver, DialContext: directLoadSyntheticDial, ObserveRuntime: directLoadSafeRuntime, Run: func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.DirectLoadRequest) (jdbcprobe.DirectLoadResult, error) {
				return directLoadSafeFacts(), nil
			}}
			if result, _ := p.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, r); result.Status != agentpreflight.StatusPassed {
				t.Fatal("初始合成事实无效")
			}
			switch kind {
			case "config":
				r.DirectLoad.Thread = "2"
			case "credential":
				r.Binding.CredentialRevision++
			case "facts":
				r.Binding.NodeFactsVersion++
			case "lease":
				r.LeaseID = "lease-2"
			case "epoch":
				r.LeaseEpoch++
			case "agent":
				r.AgentID = "agent-2"
			case "manifest":
				r.InputManifest.Digest = "unavailable"
			case "mode":
				r.CompatibilityMode = "ORACLE"
			case "roots":
				r.AllowedRoots = append(r.AllowedRoots, r.DirectLoad.FilePath+"/other")
			}
			if result, _ := p.Probe(context.Background(), "WRITE_ACCESS", r); result.Status != agentpreflight.StatusUnknown || resolver.calls != 1 {
				t.Fatal("跨绑定复用了安全事实")
			}
			if p.DirectLoadConfigFingerprint(context.Background(), r) != "" {
				t.Fatal("跨绑定读取了配置安全指纹")
			}
		})
	}
}

func Test旁路权限结构版本与本机配置未知均失败关闭(t *testing.T) {
	for _, kind := range []string{"permission", "bit", "binary", "virtual", "incremental-replace", "unknown-structure", "unknown-version", "odp-version", "unknown-config", "invalid-fingerprint", "java-local-spoof"} {
		t.Run(kind, func(t *testing.T) {
			r := directLoadFixture(t)
			facts := directLoadSafeFacts()
			wantCheck := agentpreflight.CheckID("DIRECT_LOAD_STRUCTURE")
			want := agentpreflight.StatusFailed
			observer := DirectLoadRuntimeObserver(directLoadSafeRuntime)
			yes, no := true, false
			switch kind {
			case "permission":
				facts.WriteAccess = jdbcprobe.ObjectNotAccessible
				wantCheck = "WRITE_ACCESS"
			case "bit":
				facts.TableFacts.HasBIT = &yes
			case "binary":
				facts.TableFacts.HasBinary = &yes
			case "virtual":
				facts.TableFacts.HasVirtualGenerated = &yes
			case "incremental-replace":
				r.DirectLoad.ReplaceData = true
				facts.TableFacts.Empty = &no
				facts.TableFacts.HasIndex = &yes
			case "unknown-structure":
				facts.TableFacts.HasBinary = nil
				want = agentpreflight.StatusUnknown
			case "unknown-version":
				facts.ObserverVersion = ""
				wantCheck = "DIRECT_LOAD_VERSION"
				want = agentpreflight.StatusUnknown
			case "odp-version":
				r.DirectLoad.Scenario = "CLOUD_ODP"
				wantCheck = "DIRECT_LOAD_VERSION"
				want = agentpreflight.StatusUnknown
			case "unknown-config":
				observer = nil
				wantCheck = "DIRECT_LOAD_CONFIG"
				want = agentpreflight.StatusUnknown
			case "invalid-fingerprint":
				observer = func(ctx context.Context, r agentpreflight.Request) (jdbcprobe.DirectLoadResult, error) {
					v, _ := directLoadSafeRuntime(ctx, r)
					v.DirectLoadConfig.Fingerprint = "untrusted"
					return v, nil
				}
				wantCheck = "DIRECT_LOAD_CONFIG"
				want = agentpreflight.StatusUnknown
			case "java-local-spoof":
				local, _ := directLoadSafeRuntime(context.Background(), r)
				facts.ToolVersion = local.ToolVersion
				facts.DirectLoadConfig = local.DirectLoadConfig
				facts.ObservedAt = local.ObservedAt
				observer = nil
				wantCheck = "DIRECT_LOAD_VERSION"
				want = agentpreflight.StatusUnknown
			}
			p := &DirectLoadProbe{WorkspaceRoot: t.TempDir(), Resolver: &countingResolver{connection: Connection{Host: "synthetic.invalid", Port: 2881, Username: []byte("synthetic-user"), Password: []byte("synthetic-password")}}, DialContext: directLoadSyntheticDial, ObserveRuntime: observer, Run: func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.DirectLoadRequest) (jdbcprobe.DirectLoadResult, error) {
				return facts, nil
			}}
			p.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, r)
			if result, _ := p.Probe(context.Background(), wantCheck, r); result.Status != want {
				t.Fatal("不可信旁路事实被放行", kind, result)
			}
			if (kind == "unknown-config" || kind == "invalid-fingerprint" || kind == "java-local-spoof") && p.DirectLoadConfigFingerprint(context.Background(), r) != "" {
				t.Fatal("无本机有效投影仍输出指纹")
			}
		})
	}
}

func Test旁路取消连接失败和意外系统秘密不保留成功(t *testing.T) {
	for _, kind := range []string{"cancelled", "connection-rejected", "unexpected-sys"} {
		t.Run(kind, func(t *testing.T) {
			r := directLoadFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			resolver := &countingResolver{connection: Connection{Host: "synthetic.invalid", Port: 2881, Username: []byte("synthetic-user"), Password: []byte("synthetic-password")}}
			if kind == "unexpected-sys" {
				resolver.connection.SysPassword = []byte("synthetic-system-password")
			}
			p := &DirectLoadProbe{WorkspaceRoot: t.TempDir(), Resolver: resolver, DialContext: directLoadSyntheticDial, ObserveRuntime: directLoadSafeRuntime, Run: func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.DirectLoadRequest) (jdbcprobe.DirectLoadResult, error) {
				if kind == "cancelled" {
					cancel()
				}
				if kind == "connection-rejected" {
					return jdbcprobe.DirectLoadResult{}, jdbcprobe.ErrConnectionFailed
				}
				return directLoadSafeFacts(), nil
			}}
			result, _ := p.Probe(ctx, agentpreflight.CheckDatabaseConnectivity, r)
			want := agentpreflight.StatusUnknown
			if kind == "connection-rejected" {
				want = agentpreflight.StatusFailed
			}
			if result.Status != want {
				t.Fatal("失败或取消被投影成功", result)
			}
			if result, _ := p.Probe(context.Background(), "WRITE_ACCESS", r); result.Status != agentpreflight.StatusUnknown {
				t.Fatal("失败后复用了权限成功")
			}
			if !allZero(resolver.connection.Password) || !allZero(resolver.connection.SysPassword) {
				t.Fatal("异常路径保留秘密")
			}
			if kind == "cancelled" && p.DirectLoadConfigFingerprint(context.Background(), r) != "" {
				t.Fatal("取消观测仍缓存安全指纹")
			}
		})
	}
}

func Test旁路TCP仅使用隔离回环端口且取消不算通过(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	done := make(chan struct{})
	go func() {
		defer close(done)
		connection, e := listener.Accept()
		if e == nil {
			connection.Close()
		}
	}()
	p := &DirectLoadProbe{}
	if result := p.directLoadTCP(context.Background(), "127.0.0.1", port); result.Status != agentpreflight.StatusPassed {
		t.Fatal("隔离TCP连接未取得事实", result)
	}
	<-done
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := p.directLoadTCP(ctx, "127.0.0.1", port); result.Status != agentpreflight.StatusUnknown {
		t.Fatal("取消的TCP被提升通过")
	}
}

func Test旁路本机安全观测不依赖数据库且不伪造连接(t *testing.T) {
	for _, kind := range []string{"connection-error", "no-resolver", "java-unknown"} {
		t.Run(kind, func(t *testing.T) {
			r := directLoadFixture(t)
			observations := 0
			p := &DirectLoadProbe{WorkspaceRoot: t.TempDir(), Resolver: &countingResolver{connection: Connection{Host: "synthetic.invalid", Port: 2881, Username: []byte("synthetic-user"), Password: []byte("synthetic-password")}}, ObserveRuntime: func(ctx context.Context, r agentpreflight.Request) (jdbcprobe.DirectLoadResult, error) {
				observations++
				v, _ := directLoadSafeRuntime(ctx, r)
				v.DatabaseAccess = jdbcprobe.ObjectAccessible
				v.ObserverVersion = "4.3.5.2"
				return v, nil
			}, Run: func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.DirectLoadRequest) (jdbcprobe.DirectLoadResult, error) {
				if kind == "connection-error" {
					return jdbcprobe.DirectLoadResult{}, jdbcprobe.ErrConnectionFailed
				}
				return jdbcprobe.DirectLoadResult{DatabaseAccess: jdbcprobe.ObjectUnavailable}, nil
			}}
			if kind == "no-resolver" {
				p.Resolver = nil
			}
			result, _ := p.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, r)
			want := agentpreflight.StatusUnknown
			if kind == "connection-error" {
				want = agentpreflight.StatusFailed
			}
			if result.Status != want {
				t.Fatal("本机端口伪造了数据库事实", result)
			}
			if result, _ := p.Probe(context.Background(), "DIRECT_LOAD_CONFIG", r); result.Status != agentpreflight.StatusPassed || observations != 1 {
				t.Fatal("数据库失败吞掉独立本机安全投影", result)
			}
			if len(p.DirectLoadConfigFingerprint(context.Background(), r)) != 64 {
				t.Fatal("数据库失败吞掉独立本机指纹")
			}
			if result, _ := p.Probe(context.Background(), "DIRECT_LOAD_VERSION", r); result.Status != agentpreflight.StatusUnknown {
				t.Fatal("没有数据库版本仍放行版本检查")
			}
		})
	}
}

func Test旁路同租约并发只启动一次并拒绝未完成缓存(t *testing.T) {
	r := directLoadFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	resolver := &countingResolver{connection: Connection{Host: "synthetic.invalid", Port: 2881, Username: []byte("synthetic-user"), Password: []byte("synthetic-password")}}
	p := &DirectLoadProbe{WorkspaceRoot: t.TempDir(), Resolver: resolver, DialContext: directLoadSyntheticDial, ObserveRuntime: directLoadSafeRuntime, Run: func(context.Context, credential.Workspace, jdbcprobe.Runtime, jdbcprobe.DirectLoadRequest) (jdbcprobe.DirectLoadResult, error) {
		close(started)
		<-release
		return directLoadSafeFacts(), nil
	}}
	done := make(chan agentpreflight.Result, 1)
	go func() { v, _ := p.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, r); done <- v }()
	<-started
	if result, _ := p.Probe(context.Background(), agentpreflight.CheckDatabaseConnectivity, r); result.Status != agentpreflight.StatusUnknown {
		t.Fatal("进行中数据库检查重复启动")
	}
	if result, _ := p.Probe(context.Background(), "WRITE_ACCESS", r); result.Status != agentpreflight.StatusUnknown {
		t.Fatal("未完成缓存被视为权限通过")
	}
	if p.DirectLoadConfigFingerprint(context.Background(), r) != "" {
		t.Fatal("未完成观测泄露配置指纹")
	}
	close(release)
	if result := <-done; result.Status != agentpreflight.StatusPassed || resolver.calls != 1 {
		t.Fatal("单次租约探测未完成")
	}
	if result, _ := p.Probe(context.Background(), "WRITE_ACCESS", r); result.Status != agentpreflight.StatusPassed {
		t.Fatal("完成的同绑定事实不可读取")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if p.DirectLoadConfigFingerprint(ctx, r) != "" || len(p.DirectLoadConfigFingerprint(context.Background(), r)) != 64 {
		t.Fatal("取消读取或完成观测指纹边界失效")
	}
}
