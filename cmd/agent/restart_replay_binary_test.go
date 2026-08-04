package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"ob-data-orch/internal/agentlogqueue"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/config"
	"ob-data-orch/internal/featuregate"
	"ob-data-orch/internal/logstream"
)

func Test常规Agent二进制重启后补传普通批次与本地队列缺口(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("该受控常规二进制验证只覆盖当前 Windows AMD64 本地切片")
	}

	工作目录 := t.TempDir()
	Agent二进制 := 构建受控常规Agent二进制(t, 工作目录)
	运行时 := agentwire.RuntimeConfiguration{
		Platform:     "WINDOWS_AMD64",
		ToolHome:     filepath.Join(工作目录, "tool-home"),
		JavaPath:     Agent二进制,
		AllowedRoots: []string{filepath.Join(工作目录, "allowed-output")},
		Revision:     1,
		Digest:       strings.Repeat("a", 64),
	}
	for _, 目录 := range []string{运行时.ToolHome, 运行时.AllowedRoots[0], filepath.Join(工作目录, "hadoop-home"), filepath.Join(工作目录, "program-data")} {
		if err := os.MkdirAll(目录, 0o700); err != nil {
			t.Fatalf("创建受控本地目录失败: %v", err)
		}
	}

	控制面 := 新受控重启补传控制面(t, 运行时)
	服务 := httptest.NewTLSServer(http.HandlerFunc(控制面.处理请求))
	t.Cleanup(服务.Close)
	CA文件 := 写入重启补传服务CA(t, 服务)
	身份目录 := filepath.Join(工作目录, "agent-security")
	状态, err := agentwire.OpenStateStore(身份目录)
	if err != nil {
		t.Fatalf("打开受控 Agent 身份状态失败: %v", err)
	}
	if err := 状态.PrepareEnrollment(agentwire.EnrollmentConfig{
		ControlPlaneURL:    服务.URL,
		CAFile:             CA文件,
		EnrollmentID:       "restart-replay-enrollment",
		NodeID:             "restart-replay-node",
		EnrollmentMaterial: []byte("synthetic-enrollment-material-for-agent-binary-replay"),
	}); err != nil {
		t.Fatalf("准备受控 Agent 关联状态失败: %v", err)
	}
	队列目录, err := 状态.LogQueueDirectory()
	if err != nil {
		t.Fatalf("取得受控日志队列目录失败: %v", err)
	}
	队列, err := agentlogqueue.Open(队列目录, agentlogqueue.DefaultMaxBytes)
	if err != nil {
		t.Fatalf("打开受控日志队列失败: %v", err)
	}
	普通批次 := 受控重启补传普通批次(t)
	缺口 := 受控重启补传缺口()
	if err := 队列.Enqueue(普通批次); err != nil {
		t.Fatalf("持久化受控普通批次失败: %v", err)
	}
	if err := 队列.EnqueueGap(缺口); err != nil {
		t.Fatalf("持久化受控本地队列缺口失败: %v", err)
	}
	核对受控待确认条目(t, 队列, 2)

	环境 := 受控Agent运行环境(身份目录, 工作目录)
	控制面.设置阶段(重启补传暂时不可达)
	首次进程 := 启动受控常规Agent二进制(t, Agent二进制, 环境)
	等待受控信号(t, 控制面.不可达已观测, "常规 Agent 未向假控制面重试不可达日志批次", 首次进程)
	停止受控Agent二进制(首次进程)
	核对受控待确认条目(t, 队列, 2)

	控制面.设置阶段(重启补传确认)
	二次进程 := 启动受控常规Agent二进制(t, Agent二进制, 环境)
	等待受控信号(t, 控制面.确认已完成, "重启后的常规 Agent 未完成队列补传", 二次进程)
	等待受控队列清空(t, 队列)
	停止受控Agent二进制(二次进程)

	if 顺序 := 控制面.确认顺序(); strings.Join(顺序, ",") != "LOG:1,GAP:2" {
		t.Fatalf("重启补传没有保持同源顺序: %v", 顺序)
	}
	if 路径 := 控制面.意外路径(); len(路径) != 0 {
		t.Fatalf("受控常规 Agent 访问了范围外协议路径: %v", 路径)
	}
}

type 重启补传阶段 string

const (
	重启补传暂时不可达 重启补传阶段 = "UNAVAILABLE"
	重启补传确认    重启补传阶段 = "ACCEPT"
)

type 受控重启补传控制面 struct {
	t       *testing.T
	运行时     agentwire.RuntimeConfiguration
	mu      sync.Mutex
	阶段      重启补传阶段
	账本      *logstream.BatchLedger
	顺序      []string
	意外请求路径  []string
	不可达请求数  int
	心跳数     int
	不可达已观测  chan struct{}
	确认已完成   chan struct{}
	不可达通知一次 sync.Once
	确认通知一次  sync.Once
}

func 新受控重启补传控制面(t *testing.T, 运行时 agentwire.RuntimeConfiguration) *受控重启补传控制面 {
	t.Helper()
	return &受控重启补传控制面{
		t: t, 运行时: 运行时, 阶段: 重启补传暂时不可达, 账本: logstream.NewBatchLedger(),
		不可达已观测: make(chan struct{}), 确认已完成: make(chan struct{}),
	}
}

func (控制面 *受控重启补传控制面) 处理请求(writer http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/agent/v1/enrollments:exchange":
		控制面.处理关联(writer, request)
	case "/agent/v1/heartbeats":
		控制面.处理心跳(writer, request)
	case "/agent/v1/data-source-connection-tests:claim-next", "/agent/v1/prechecks:claim-next", "/agent/v1/executions:claim-next":
		if !受控认证请求(request) {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	case "/agent/v1/executions/binary-replay-execution:logs:append":
		控制面.处理普通日志(writer, request)
	case "/agent/v1/executions/binary-replay-execution:logs:gap":
		控制面.处理日志缺口(writer, request)
	default:
		控制面.mu.Lock()
		控制面.意外请求路径 = append(控制面.意外请求路径, request.URL.Path)
		控制面.mu.Unlock()
		writer.WriteHeader(http.StatusNotFound)
	}
}

func (控制面 *受控重启补传控制面) 处理关联(writer http.ResponseWriter, request *http.Request) {
	var 关联 struct {
		AgentID         string `json:"agentId"`
		NodeID          string `json:"nodeId"`
		ProtocolVersion string `json:"protocolVersion"`
	}
	if request.Method != http.MethodPost || json.NewDecoder(request.Body).Decode(&关联) != nil || 关联.AgentID == "" || 关联.NodeID != "restart-replay-node" || 关联.ProtocolVersion != agentwire.Version {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	写入重启补传协议响应(控制面.t, writer, "ENROLLED", map[string]any{
		"agentId": 关联.AgentID, "nodeId": 关联.NodeID, "protocolVersion": agentwire.Version,
		"replayed": false, "realExecutionEnabled": false, "runtimeConfiguration": 控制面.运行时,
	})
}

func (控制面 *受控重启补传控制面) 处理心跳(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || !受控认证请求(request) {
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}
	控制面.mu.Lock()
	控制面.心跳数++
	达到持续不可达观察 := 控制面.阶段 == 重启补传暂时不可达 && 控制面.不可达请求数 >= 6 && 控制面.心跳数 >= 2
	控制面.mu.Unlock()
	if 达到持续不可达观察 {
		控制面.不可达通知一次.Do(func() { close(控制面.不可达已观测) })
	}
	写入重启补传协议响应(控制面.t, writer, "ACCEPTED", map[string]any{
		"factsRevision": 1, "environmentStatus": "NOT_CHECKED", "realExecutionEnabled": false,
	})
}

func (控制面 *受控重启补传控制面) 处理普通日志(writer http.ResponseWriter, request *http.Request) {
	var 上传 struct {
		PayloadType string `json:"payloadType"`
		Payload     struct {
			LeaseID        string          `json:"leaseId"`
			LeaseEpoch     int64           `json:"leaseEpoch"`
			EnvelopeDigest string          `json:"envelopeDigest"`
			Batch          logstream.Batch `json:"batch"`
		} `json:"payload"`
	}
	if request.Method != http.MethodPost || !受控认证请求(request) || json.NewDecoder(request.Body).Decode(&上传) != nil || 上传.PayloadType != "OBDUMPER_EXPORT_APPEND_LOG" || 上传.Payload.LeaseID != "binary-replay-lease" || 上传.Payload.LeaseEpoch != 1 || 上传.Payload.EnvelopeDigest != strings.Repeat("b", 64) {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	控制面.mu.Lock()
	阶段 := 控制面.阶段
	if 阶段 == 重启补传暂时不可达 {
		控制面.不可达请求数++
		达到持续不可达观察 := 控制面.不可达请求数 >= 6 && 控制面.心跳数 >= 2
		控制面.mu.Unlock()
		if 达到持续不可达观察 {
			控制面.不可达通知一次.Do(func() { close(控制面.不可达已观测) })
		}
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	结果, err := 控制面.账本.Accept(上传.Payload.Batch)
	if err == nil && 结果.Decision == logstream.BatchAccepted {
		控制面.顺序 = append(控制面.顺序, "LOG:"+"1")
	}
	控制面.mu.Unlock()
	if err != nil || (结果.Decision != logstream.BatchAccepted && 结果.Decision != logstream.BatchDuplicate) {
		writer.WriteHeader(http.StatusConflict)
		return
	}
	写入重启补传协议响应(控制面.t, writer, "EXECUTION_LOG_ACCEPTED", map[string]any{
		"decision": 结果.Decision, "expectedSequence": 结果.ExpectedSeq, "streamId": 上传.Payload.Batch.StreamID,
		"sourceEpoch": 上传.Payload.Batch.SourceEpoch, "firstSequence": 上传.Payload.Batch.FirstSeq, "lastSequence": 上传.Payload.Batch.LastSeq,
		"batchDigest": 上传.Payload.Batch.Digest, "realExecutionEnabled": true,
	})
}

func (控制面 *受控重启补传控制面) 处理日志缺口(writer http.ResponseWriter, request *http.Request) {
	var 上传 struct {
		PayloadType string `json:"payloadType"`
		Payload     struct {
			LeaseID        string              `json:"leaseId"`
			LeaseEpoch     int64               `json:"leaseEpoch"`
			EnvelopeDigest string              `json:"envelopeDigest"`
			Gap            logstream.GapNotice `json:"gap"`
		} `json:"payload"`
	}
	if request.Method != http.MethodPost || !受控认证请求(request) || json.NewDecoder(request.Body).Decode(&上传) != nil || 上传.PayloadType != "OBDUMPER_EXPORT_APPEND_LOG_GAP" || 上传.Payload.LeaseID != "binary-replay-lease" || 上传.Payload.LeaseEpoch != 1 || 上传.Payload.EnvelopeDigest != strings.Repeat("b", 64) || 上传.Payload.Gap.ReasonCode != "LOCAL_SPOOL_LIMIT" {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	缺口摘要, err := logstream.GapDigest(上传.Payload.Gap)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	控制面.mu.Lock()
	阶段 := 控制面.阶段
	if 阶段 == 重启补传暂时不可达 {
		控制面.mu.Unlock()
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	结果, err := 控制面.账本.AcceptGap(上传.Payload.Gap)
	if err == nil && 结果.Decision == logstream.BatchAccepted {
		控制面.顺序 = append(控制面.顺序, "GAP:"+"2")
	}
	顺序已完整 := strings.Join(控制面.顺序, ",") == "LOG:1,GAP:2"
	控制面.mu.Unlock()
	if err != nil || (结果.Decision != logstream.BatchAccepted && 结果.Decision != logstream.BatchDuplicate) {
		writer.WriteHeader(http.StatusConflict)
		return
	}
	写入重启补传协议响应(控制面.t, writer, "EXECUTION_LOG_GAP_ACCEPTED", map[string]any{
		"decision": 结果.Decision, "expectedSequence": 结果.ExpectedSeq, "streamId": 上传.Payload.Gap.StreamID,
		"sourceEpoch": 上传.Payload.Gap.SourceEpoch, "firstSequence": 上传.Payload.Gap.FirstSeq, "lastSequence": 上传.Payload.Gap.LastSeq,
		"gapDigest": 缺口摘要, "realExecutionEnabled": true,
	})
	if 顺序已完整 {
		控制面.确认通知一次.Do(func() { close(控制面.确认已完成) })
	}
}

func (控制面 *受控重启补传控制面) 设置阶段(阶段 重启补传阶段) {
	控制面.mu.Lock()
	defer 控制面.mu.Unlock()
	控制面.阶段 = 阶段
}

func (控制面 *受控重启补传控制面) 确认顺序() []string {
	控制面.mu.Lock()
	defer 控制面.mu.Unlock()
	return append([]string(nil), 控制面.顺序...)
}

func (控制面 *受控重启补传控制面) 意外路径() []string {
	控制面.mu.Lock()
	defer 控制面.mu.Unlock()
	return append([]string(nil), 控制面.意外请求路径...)
}

func 构建受控常规Agent二进制(t *testing.T, 工作目录 string) string {
	t.Helper()
	仓库根目录, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("解析仓库目录失败: %v", err)
	}
	二进制 := filepath.Join(工作目录, "ob-data-orch-agent.exe")
	Go命令 := filepath.Join(runtime.GOROOT(), "bin", "go.exe")
	命令 := exec.Command(Go命令, "build", "-o", 二进制, "./cmd/agent")
	命令.Dir = 仓库根目录
	if 输出, err := 命令.CombinedOutput(); err != nil {
		t.Fatalf("构建常规 Agent 二进制失败: %v: %s", err, strings.TrimSpace(string(输出)))
	}
	return 二进制
}

func 受控Agent运行环境(身份目录, 工作目录 string) []string {
	覆盖 := map[string]string{
		config.AgentStateDirectoryEnvironmentVariable:     身份目录,
		config.AgentHeartbeatIntervalEnvironmentVariable:  "5s",
		featuregate.RealExecutionEnvironmentVariable:      "true",
		config.AgentExportPreflightEnvironmentVariable:    "true",
		config.AgentJDBCConnectionTestEnvironmentVariable: "false",
		"HADOOP_HOME": filepath.Join(工作目录, "hadoop-home"),
		"ProgramData": filepath.Join(工作目录, "program-data"),
	}
	环境 := make([]string, 0, len(os.Environ())+len(覆盖))
	for _, 项 := range os.Environ() {
		名称, _, 找到 := strings.Cut(项, "=")
		if 找到 {
			if _, 已覆盖 := 覆盖[名称]; 已覆盖 {
				continue
			}
		}
		环境 = append(环境, 项)
	}
	for 名称, 值 := range 覆盖 {
		环境 = append(环境, 名称+"="+值)
	}
	return 环境
}

type 受控Agent二进制进程 struct {
	取消   context.CancelFunc
	命令   *exec.Cmd
	标准输出 bytes.Buffer
	标准错误 bytes.Buffer
	停止一次 sync.Once
}

func 启动受控常规Agent二进制(t *testing.T, 二进制 string, 环境 []string) *受控Agent二进制进程 {
	t.Helper()
	上下文, 取消 := context.WithCancel(context.Background())
	进程 := exec.CommandContext(上下文, 二进制)
	进程.Env = 环境
	受控进程 := &受控Agent二进制进程{取消: 取消, 命令: 进程}
	进程.Stdout = &受控进程.标准输出
	进程.Stderr = &受控进程.标准错误
	if err := 进程.Start(); err != nil {
		取消()
		t.Fatalf("启动受控常规 Agent 二进制失败: %v", err)
	}
	t.Cleanup(func() { 停止受控Agent二进制(受控进程) })
	return 受控进程
}

func 停止受控Agent二进制(进程 *受控Agent二进制进程) {
	if 进程 == nil {
		return
	}
	进程.停止一次.Do(func() {
		进程.取消()
		_ = 进程.命令.Wait()
	})
}

func 等待受控信号(t *testing.T, 信号 <-chan struct{}, 未达错误 string, 进程 *受控Agent二进制进程) {
	t.Helper()
	select {
	case <-信号:
	case <-time.After(12 * time.Second):
		t.Fatalf("%s；Agent 安全诊断：%s", 未达错误, 受控Agent安全诊断(进程))
	}
}

func 受控Agent安全诊断(进程 *受控Agent二进制进程) string {
	if 进程 == nil {
		return "进程不可用"
	}
	输出 := strings.TrimSpace(进程.标准输出.String() + " " + 进程.标准错误.String())
	if 输出 == "" {
		return "没有标准输出或标准错误"
	}
	if len(输出) > 2048 {
		return 输出[:2048]
	}
	return 输出
}

func 等待受控队列清空(t *testing.T, 队列 *agentlogqueue.Queue) {
	t.Helper()
	截止 := time.After(5 * time.Second)
	计时器 := time.NewTicker(25 * time.Millisecond)
	defer 计时器.Stop()
	for {
		条目, err := 队列.PendingItems()
		// 受控观察端与独立 Agent 进程没有共享互斥锁；Glob 后文件已被确认删除时重试即可，其他读取错误仍失败关闭。
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatalf("读取受控待确认条目失败: %v", err)
		}
		if len(条目) == 0 {
			return
		}
		select {
		case <-截止:
			t.Fatal("控制面已确认后 Agent 没有清理本地待确认条目")
		case <-计时器.C:
		}
	}
}

func 核对受控待确认条目(t *testing.T, 队列 *agentlogqueue.Queue, 期望数量 int) {
	t.Helper()
	条目, err := 队列.PendingItems()
	if err != nil || len(条目) != 期望数量 {
		t.Fatalf("待确认条目数=%d，错误=%v，期望=%d", len(条目), err, 期望数量)
	}
}

func 受控重启补传普通批次(t *testing.T) agentlogqueue.Entry {
	t.Helper()
	记录 := logstream.Record{
		StreamID: "binary-replay-execution", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: 1,
		Kind: logstream.RecordLog, Message: "已脱敏合成重启补传日志", PolicyVersion: "binary-replay-v1", ParserVersion: "binary-replay-v1", ReceivedAt: time.Now().UTC(),
	}
	批次, err := logstream.SealBatch(logstream.Batch{
		StreamID: 记录.StreamID, SourceEpoch: 记录.SourceEpoch, FirstSeq: 1, LastSeq: 1, PolicyVersion: 记录.PolicyVersion, Records: []logstream.Record{记录},
	})
	if err != nil {
		t.Fatalf("密封受控普通批次失败: %v", err)
	}
	return agentlogqueue.Entry{
		ExecutionID: "binary-replay-execution", LeaseID: "binary-replay-lease", LeaseEpoch: 1, EnvelopeDigest: strings.Repeat("b", 64), Batch: 批次,
	}
}

func 受控重启补传缺口() agentlogqueue.GapEntry {
	return agentlogqueue.GapEntry{
		ExecutionID: "binary-replay-execution", LeaseID: "binary-replay-lease", LeaseEpoch: 1, EnvelopeDigest: strings.Repeat("b", 64),
		Gap: logstream.GapNotice{
			StreamID: "binary-replay-execution", SourceKind: logstream.SourceStdout, SourceEpoch: 1, FirstSeq: 2, LastSeq: 2,
			ReasonCode: "LOCAL_SPOOL_LIMIT", PolicyVersion: "binary-replay-v1", ParserVersion: "binary-replay-v1",
		},
	}
}

func 受控认证请求(request *http.Request) bool {
	return strings.HasPrefix(request.Header.Get("Authorization"), "Bearer ")
}

func 写入重启补传服务CA(t *testing.T, 服务 *httptest.Server) string {
	t.Helper()
	证书 := 服务.Certificate()
	if 证书 == nil {
		t.Fatal("受控 TLS 服务没有证书")
	}
	解析后的证书, err := x509.ParseCertificate(证书.Raw)
	if err != nil {
		t.Fatalf("解析受控 TLS 证书失败: %v", err)
	}
	路径 := filepath.Join(t.TempDir(), "restart-replay-control-plane-ca.pem")
	if err := os.WriteFile(路径, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: 解析后的证书.Raw}), 0o600); err != nil {
		t.Fatalf("写入受控 TLS CA 失败: %v", err)
	}
	return 路径
}

func 写入重启补传协议响应(t *testing.T, writer http.ResponseWriter, 状态 string, 载荷 map[string]any) {
	t.Helper()
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{
		"requestId": "controlled-restart-replay", "serverTime": time.Now().UTC().Format(time.RFC3339Nano), "status": 状态, "payload": 载荷,
	}); err != nil {
		t.Errorf("写入受控协议响应失败: %v", err)
	}
}
