package agentexecution

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ob-data-orch/internal/agentlogqueue"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/logstream"
)

func Test真实Agent队列在持续不可达后按序重放(t *testing.T) {
	fixture := new受控日志故障夹具(t)
	fixture.设置模式(日志持续不可达)
	queue := fixture.打开队列(t, agentlogqueue.DefaultMaxBytes)
	uploader := newLogUploader(context.Background(), fixture.store, "boot-fixture", 受控测试任务信封(), queue, nil)
	if err := uploader.append(受控测试日志(1)); err != nil {
		t.Fatalf("首批日志入队失败: %v", err)
	}
	if err := uploader.append(受控测试日志(2)); err != nil {
		t.Fatalf("第二批日志入队失败: %v", err)
	}
	if err := flushQueuedLogs(context.Background(), queue, fixture.store, "boot-fixture", ""); err == nil {
		t.Fatal("持续不可达不能被当作控制面确认")
	}
	待确认批次(t, queue, 2)
	if fixture.不可达请求数() < 6 {
		t.Fatalf("持续不可达未经过真实 HTTPS 重试: %d", fixture.不可达请求数())
	}

	fixture.设置模式(日志正常确认)
	重启后的协议 := fixture.重开已关联协议(t)
	if err := flushQueuedLogs(context.Background(), queue, 重启后的协议, "boot-restarted", ""); err != nil {
		t.Fatalf("控制面恢复后重放失败: %v", err)
	}
	待确认批次(t, queue, 0)
	if got := fixture.已确认序号(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("恢复后的真实 Agent 上传顺序错误: %v", got)
	}
	证据, err := queue.EvidenceSnapshot()
	if err != nil {
		t.Fatalf("读取无正文队列证据失败: %v", err)
	}
	if !含队列证据(证据, agentlogqueue.EvidenceRecoveryReplayAttempt) || !含队列证据(证据, agentlogqueue.EvidenceControlPlaneConfirmed) {
		t.Fatalf("持续不可达恢复缺少重放或确认事实: %#v", 证据)
	}
}

func Test真实Agent队列对重复确认按幂等重放且拒绝错位回执(t *testing.T) {
	fixture := new受控日志故障夹具(t)
	queue := fixture.打开队列(t, agentlogqueue.DefaultMaxBytes)
	uploader := newLogUploader(context.Background(), fixture.store, "boot-fixture", 受控测试任务信封(), queue, nil)

	fixture.设置模式(日志已持久化但响应丢失)
	if err := uploader.append(受控测试日志(1)); err != nil {
		t.Fatalf("响应丢失时不应阻断已持久化日志接收: %v", err)
	}
	待确认批次(t, queue, 1)
	fixture.设置模式(日志错位确认)
	if err := flushQueuedLogs(context.Background(), queue, fixture.store, "boot-fixture", ""); !errors.Is(err, agentwire.ErrProtocolRejected) {
		t.Fatalf("错位确认 error = %v, want ErrProtocolRejected", err)
	}
	待确认批次(t, queue, 1)
	if fixture.错位确认数() == 0 {
		t.Fatal("故障夹具未发送错位确认")
	}

	fixture.设置模式(日志正常确认)
	if err := flushQueuedLogs(context.Background(), queue, fixture.重开已关联协议(t), "boot-restarted", ""); err != nil {
		t.Fatalf("重复批次的幂等重放失败: %v", err)
	}
	待确认批次(t, queue, 0)
	if fixture.重复确认数() == 0 {
		t.Fatal("响应丢失后的重放未获得幂等重复确认")
	}
}

func Test真实Agent队列达到缩小上限时保留先前批次并失败关闭(t *testing.T) {
	fixture := new受控日志故障夹具(t)
	fixture.设置模式(日志持续不可达)
	任务信封 := 受控测试任务信封()
	首批 := 受控队列条目(t, 任务信封, 受控测试日志(1), "")
	次批 := 受控队列条目(t, 任务信封, 受控测试日志(2), 首批.Batch.Digest)
	首批编码, err := json.Marshal(首批)
	if err != nil {
		t.Fatalf("编码首批队列条目失败: %v", err)
	}
	次批编码, err := json.Marshal(次批)
	if err != nil {
		t.Fatalf("编码次批队列条目失败: %v", err)
	}
	queue := fixture.打开队列(t, int64(len(首批编码)+len(次批编码)-1))
	uploader := newLogUploader(context.Background(), fixture.store, "boot-fixture", 任务信封, queue, nil)
	if err := uploader.append(受控测试日志(1)); err != nil {
		t.Fatalf("首批日志入队失败: %v", err)
	}
	if err := uploader.append(受控测试日志(2)); err != nil {
		t.Fatalf("队列达到缩小上限后持久化无正文缺口失败: %v", err)
	}
	items, err := queue.PendingItems()
	if err != nil || len(items) != 2 || items[0].Entry == nil || items[1].Gap == nil || items[1].Gap.Gap.ReasonCode != "LOCAL_SPOOL_LIMIT" {
		t.Fatalf("容量失败后未保留批次和无正文缺口: %#v, error=%v", items, err)
	}
	if fixture.不可达请求数() != 4 {
		t.Fatalf("容量失败后不应上传未入队的次批: %d", fixture.不可达请求数())
	}
	fixture.设置模式(日志正常确认)
	if err := uploader.append(受控测试日志(3)); err != nil {
		t.Fatalf("控制面恢复后继续接收日志失败: %v", err)
	}
	待确认批次(t, queue, 0)
	if got := fixture.确认顺序(); strings.Join(got, ",") != "LOG:1,GAP:2,LOG:3" {
		t.Fatalf("恢复后未按来源序号先确认缺口: %v", got)
	}
}

func Test真实Agent队列拒绝错位缺口确认并接受幂等重放(t *testing.T) {
	fixture := new受控日志故障夹具(t)
	queue := fixture.打开队列(t, agentlogqueue.DefaultMaxBytes)
	gap := agentlogqueue.GapEntry{
		ExecutionID: "execution-fixture", LeaseID: "lease-fixture", LeaseEpoch: 1, EnvelopeDigest: strings.Repeat("a", 64),
		Gap: logstream.GapNotice{
			StreamID: "execution-fixture", SourceKind: logstream.SourceStdout, SourceEpoch: 1,
			FirstSeq: 1, LastSeq: 1, ReasonCode: "LOCAL_SPOOL_LIMIT", PolicyVersion: "fault-fixture-v1", ParserVersion: "fault-fixture-v1",
		},
	}
	if err := queue.EnqueueGap(gap); err != nil {
		t.Fatalf("持久化受控缺口失败: %v", err)
	}
	fixture.设置模式(日志错位确认)
	if err := flushQueuedLogs(context.Background(), queue, fixture.store, "boot-fixture", ""); !errors.Is(err, agentwire.ErrProtocolRejected) {
		t.Fatalf("错位缺口确认 error = %v, want ErrProtocolRejected", err)
	}
	items, err := queue.PendingItems()
	if err != nil || len(items) != 1 || items[0].Gap == nil {
		t.Fatalf("错位缺口确认后本地条目未保留: %#v, error=%v", items, err)
	}
	fixture.设置模式(日志已持久化但响应丢失)
	if err := flushQueuedLogs(context.Background(), queue, fixture.store, "boot-fixture", ""); err == nil {
		t.Fatal("缺口响应丢失不能被当作确认")
	}
	fixture.设置模式(日志正常确认)
	if err := flushQueuedLogs(context.Background(), queue, fixture.重开已关联协议(t), "boot-restarted", ""); err != nil {
		t.Fatalf("缺口幂等重放失败: %v", err)
	}
	items, err = queue.PendingItems()
	if err != nil || len(items) != 0 || fixture.重复确认数() == 0 {
		t.Fatalf("缺口幂等确认结果不正确: items=%#v error=%v duplicates=%d", items, err, fixture.重复确认数())
	}
}

type 受控日志故障模式 string

const (
	日志正常确认      受控日志故障模式 = "ACCEPT"
	日志持续不可达     受控日志故障模式 = "UNAVAILABLE"
	日志已持久化但响应丢失 受控日志故障模式 = "PERSISTED_RESPONSE_LOST"
	日志错位确认      受控日志故障模式 = "MISMATCHED_CONFIRMATION"
)

type 受控日志故障夹具 struct {
	t      *testing.T
	store  *agentwire.StateStore
	身份目录   string
	mu     sync.Mutex
	模式     受控日志故障模式
	账本     *logstream.BatchLedger
	已确认的序号 []int64
	确认的顺序  []string
	不可达请求  int
	错位确认   int
	重复确认   int
}

type 受控日志请求 struct {
	ProtocolVersion string `json:"protocolVersion"`
	AgentID         string `json:"agentId"`
	NodeID          string `json:"nodeId"`
	BootID          string `json:"bootId"`
	RequestID       string `json:"requestId"`
	SentAt          string `json:"sentAt"`
	PayloadType     string `json:"payloadType"`
	Payload         struct {
		LeaseID        string          `json:"leaseId"`
		LeaseEpoch     int64           `json:"leaseEpoch"`
		EnvelopeDigest string          `json:"envelopeDigest"`
		Batch          logstream.Batch `json:"batch"`
	} `json:"payload"`
}

type 受控日志缺口请求 struct {
	ProtocolVersion string `json:"protocolVersion"`
	AgentID         string `json:"agentId"`
	NodeID          string `json:"nodeId"`
	BootID          string `json:"bootId"`
	RequestID       string `json:"requestId"`
	SentAt          string `json:"sentAt"`
	PayloadType     string `json:"payloadType"`
	Payload         struct {
		LeaseID        string              `json:"leaseId"`
		LeaseEpoch     int64               `json:"leaseEpoch"`
		EnvelopeDigest string              `json:"envelopeDigest"`
		Gap            logstream.GapNotice `json:"gap"`
	} `json:"payload"`
}

func new受控日志故障夹具(t *testing.T) *受控日志故障夹具 {
	t.Helper()
	夹具 := &受控日志故障夹具{t: t, 模式: 日志正常确认, 账本: logstream.NewBatchLedger()}
	服务 := httptest.NewTLSServer(http.HandlerFunc(夹具.处理请求))
	t.Cleanup(服务.Close)
	身份目录 := filepath.Join(t.TempDir(), "agent-identity")
	CA路径 := 写入受控服务CA(t, 服务)
	协议, err := agentwire.OpenStateStore(身份目录)
	if err != nil {
		t.Fatalf("打开 Agent 身份状态失败: %v", err)
	}
	if err := 协议.PrepareEnrollment(agentwire.EnrollmentConfig{
		ControlPlaneURL: 服务.URL, CAFile: CA路径, EnrollmentID: "enrollment-fixture", NodeID: "node-fixture",
		EnrollmentMaterial: []byte("synthetic-enrollment-material-for-fault-fixture"),
	}); err != nil {
		t.Fatalf("准备合成 Agent 关联失败: %v", err)
	}
	if err := 协议.EnsureEnrollment(context.Background()); err != nil {
		t.Fatalf("完成合成 Agent 关联失败: %v", err)
	}
	夹具.store, 夹具.身份目录 = 协议, 身份目录
	return 夹具
}

func (夹具 *受控日志故障夹具) 处理请求(writer http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/agent/v1/enrollments:exchange":
		var enrollment struct {
			AgentID         string `json:"agentId"`
			NodeID          string `json:"nodeId"`
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.NewDecoder(request.Body).Decode(&enrollment) != nil || enrollment.AgentID == "" || enrollment.NodeID != "node-fixture" || enrollment.ProtocolVersion != agentwire.Version {
			夹具.t.Error("受控夹具收到无效的关联请求")
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		写入受控协议响应(夹具.t, writer, "ENROLLED", map[string]any{
			"agentId": enrollment.AgentID, "nodeId": enrollment.NodeID, "protocolVersion": agentwire.Version,
			"replayed": false, "realExecutionEnabled": false,
		})
	case "/agent/v1/executions/execution-fixture:logs:append":
		夹具.处理日志上传(writer, request)
	case "/agent/v1/executions/execution-fixture:logs:gap":
		夹具.处理日志缺口上传(writer, request)
	default:
		夹具.t.Errorf("受控夹具收到意外路径: %s", request.URL.Path)
		writer.WriteHeader(http.StatusNotFound)
	}
}

func (夹具 *受控日志故障夹具) 处理日志上传(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.Header.Get("Authorization") == "" {
		夹具.t.Error("日志上传未使用受认证 POST")
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}
	var 上传 受控日志请求
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&上传) != nil || 上传.PayloadType != "OBDUMPER_EXPORT_APPEND_LOG" || 上传.Payload.Batch.StreamID != "execution-fixture" || 上传.Payload.Batch.FirstSeq < 1 || 上传.Payload.Batch.LastSeq < 上传.Payload.Batch.FirstSeq || len(上传.Payload.Batch.Digest) != 64 {
		夹具.t.Error("受控夹具收到无效日志批次信封")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	夹具.mu.Lock()
	模式 := 夹具.模式
	if 模式 == 日志持续不可达 {
		夹具.不可达请求++
		夹具.mu.Unlock()
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if 模式 == 日志错位确认 {
		夹具.错位确认++
		夹具.mu.Unlock()
		写入受控协议响应(夹具.t, writer, "EXECUTION_LOG_ACCEPTED", 受控日志确认(上传.Payload.Batch, logstream.BatchAccepted, 1))
		return
	}
	result, err := 夹具.账本.Accept(上传.Payload.Batch)
	if err != nil {
		夹具.mu.Unlock()
		夹具.t.Error("受控夹具拒绝了本应有序的日志批次")
		writer.WriteHeader(http.StatusConflict)
		return
	}
	if result.Decision == logstream.BatchAccepted {
		夹具.已确认的序号 = append(夹具.已确认的序号, 上传.Payload.Batch.FirstSeq)
		夹具.确认的顺序 = append(夹具.确认的顺序, "LOG:"+strconv.FormatInt(上传.Payload.Batch.FirstSeq, 10))
	} else {
		夹具.重复确认++
	}
	if 模式 == 日志已持久化但响应丢失 {
		夹具.不可达请求++
		夹具.mu.Unlock()
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	夹具.mu.Unlock()
	写入受控协议响应(夹具.t, writer, "EXECUTION_LOG_ACCEPTED", 受控日志确认(上传.Payload.Batch, result.Decision, 0))
}

func (夹具 *受控日志故障夹具) 处理日志缺口上传(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.Header.Get("Authorization") == "" {
		夹具.t.Error("日志缺口上传未使用受认证 POST")
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}
	var 上传 受控日志缺口请求
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&上传) != nil || 上传.PayloadType != "OBDUMPER_EXPORT_APPEND_LOG_GAP" || 上传.Payload.Gap.StreamID != "execution-fixture" || 上传.Payload.Gap.ReasonCode != "LOCAL_SPOOL_LIMIT" {
		夹具.t.Error("受控夹具收到无效日志缺口信封")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	gapDigest, err := logstream.GapDigest(上传.Payload.Gap)
	if err != nil {
		夹具.t.Error("受控夹具无法计算日志缺口摘要")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	夹具.mu.Lock()
	if 夹具.模式 == 日志持续不可达 {
		夹具.不可达请求++
		夹具.mu.Unlock()
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if 夹具.模式 == 日志错位确认 {
		夹具.错位确认++
		夹具.mu.Unlock()
		写入受控协议响应(夹具.t, writer, "EXECUTION_LOG_GAP_ACCEPTED", 受控日志缺口确认(logstream.GapNotice{
			StreamID: 上传.Payload.Gap.StreamID, SourceKind: 上传.Payload.Gap.SourceKind, SourceEpoch: 上传.Payload.Gap.SourceEpoch,
			FirstSeq: 上传.Payload.Gap.FirstSeq + 1, LastSeq: 上传.Payload.Gap.LastSeq + 1, ReasonCode: 上传.Payload.Gap.ReasonCode,
			PolicyVersion: 上传.Payload.Gap.PolicyVersion, ParserVersion: 上传.Payload.Gap.ParserVersion,
		}, gapDigest, logstream.BatchAccepted))
		return
	}
	result, err := 夹具.账本.AcceptGap(上传.Payload.Gap)
	if err != nil {
		夹具.mu.Unlock()
		writer.WriteHeader(http.StatusConflict)
		return
	}
	if result.Decision == logstream.BatchAccepted {
		夹具.确认的顺序 = append(夹具.确认的顺序, "GAP:"+strconv.FormatInt(上传.Payload.Gap.FirstSeq, 10))
	} else {
		夹具.重复确认++
	}
	if 夹具.模式 == 日志已持久化但响应丢失 {
		夹具.不可达请求++
		夹具.mu.Unlock()
		writer.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	夹具.mu.Unlock()
	写入受控协议响应(夹具.t, writer, "EXECUTION_LOG_GAP_ACCEPTED", 受控日志缺口确认(上传.Payload.Gap, gapDigest, result.Decision))
}

func (夹具 *受控日志故障夹具) 设置模式(模式 受控日志故障模式) {
	夹具.mu.Lock()
	defer 夹具.mu.Unlock()
	夹具.模式 = 模式
}

func (夹具 *受控日志故障夹具) 打开队列(t *testing.T, 上限 int64) *agentlogqueue.Queue {
	t.Helper()
	目录, err := 夹具.store.LogQueueDirectory()
	if err != nil {
		t.Fatalf("取得 Agent 私有队列目录失败: %v", err)
	}
	queue, err := agentlogqueue.Open(目录, 上限)
	if err != nil {
		t.Fatalf("打开 Agent 私有队列失败: %v", err)
	}
	return queue
}

func (夹具 *受控日志故障夹具) 重开已关联协议(t *testing.T) *agentwire.StateStore {
	t.Helper()
	协议, err := agentwire.OpenStateStore(夹具.身份目录)
	if err != nil {
		t.Fatalf("模拟 Agent 重启后打开身份失败: %v", err)
	}
	return 协议
}

func (夹具 *受控日志故障夹具) 不可达请求数() int {
	夹具.mu.Lock()
	defer 夹具.mu.Unlock()
	return 夹具.不可达请求
}

func (夹具 *受控日志故障夹具) 错位确认数() int {
	夹具.mu.Lock()
	defer 夹具.mu.Unlock()
	return 夹具.错位确认
}

func (夹具 *受控日志故障夹具) 重复确认数() int {
	夹具.mu.Lock()
	defer 夹具.mu.Unlock()
	return 夹具.重复确认
}

func (夹具 *受控日志故障夹具) 已确认序号() []int64 {
	夹具.mu.Lock()
	defer 夹具.mu.Unlock()
	return append([]int64(nil), 夹具.已确认的序号...)
}

func (夹具 *受控日志故障夹具) 确认顺序() []string {
	夹具.mu.Lock()
	defer 夹具.mu.Unlock()
	return append([]string(nil), 夹具.确认的顺序...)
}

func 受控测试任务信封() agentwire.ExecutionGrant {
	return agentwire.ExecutionGrant{
		TaskID: "task-fixture", ExecutionID: "execution-fixture", LeaseID: "lease-fixture", LeaseEpoch: 1,
		EnvelopeDigest: strings.Repeat("a", 64),
	}
}

func 受控测试日志(序号 int64) logstream.Record {
	return logstream.Record{
		StreamID: "execution-fixture", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: 序号,
		Kind: logstream.RecordLog, Message: "已脱敏合成日志", PolicyVersion: "fault-fixture-v1", ParserVersion: "fault-fixture-v1",
		ReceivedAt: time.Date(2026, time.August, 3, 0, 0, int(序号), 0, time.UTC),
	}
}

func 受控队列条目(t *testing.T, 任务信封 agentwire.ExecutionGrant, 记录 logstream.Record, 前序摘要 string) agentlogqueue.Entry {
	t.Helper()
	批次, err := logstream.SealBatch(logstream.Batch{
		StreamID: 记录.StreamID, SourceEpoch: 记录.SourceEpoch, FirstSeq: 记录.SourceSeq, LastSeq: 记录.SourceSeq,
		PreviousDigest: 前序摘要, PolicyVersion: 记录.PolicyVersion, Records: []logstream.Record{记录},
	})
	if err != nil {
		t.Fatalf("密封受控日志批次失败: %v", err)
	}
	return agentlogqueue.Entry{ExecutionID: 任务信封.ExecutionID, LeaseID: 任务信封.LeaseID, LeaseEpoch: 任务信封.LeaseEpoch, EnvelopeDigest: 任务信封.EnvelopeDigest, Batch: 批次}
}

func 待确认批次(t *testing.T, queue *agentlogqueue.Queue, 数量 int) {
	t.Helper()
	entries, _, err := queue.Pending()
	if err != nil || len(entries) != 数量 {
		t.Fatalf("待确认批次数 = %d, error = %v, want %d", len(entries), err, 数量)
	}
}

func 含队列证据(证据 []agentlogqueue.Evidence, 类型 agentlogqueue.EvidenceType) bool {
	for _, item := range 证据 {
		if item.Type == 类型 {
			return true
		}
	}
	return false
}

func 受控日志确认(批次 logstream.Batch, 决策 logstream.BatchDecision, 序号偏移 int64) map[string]any {
	return map[string]any{
		"decision": 决策, "expectedSequence": 批次.LastSeq + 1,
		"streamId": 批次.StreamID, "sourceEpoch": 批次.SourceEpoch,
		"firstSequence": 批次.FirstSeq + 序号偏移, "lastSequence": 批次.LastSeq + 序号偏移,
		"batchDigest": 批次.Digest, "realExecutionEnabled": true,
	}
}

func 受控日志缺口确认(缺口 logstream.GapNotice, 摘要 string, 决策 logstream.BatchDecision) map[string]any {
	return map[string]any{
		"decision": 决策, "expectedSequence": 缺口.LastSeq + 1,
		"streamId": 缺口.StreamID, "sourceEpoch": 缺口.SourceEpoch,
		"firstSequence": 缺口.FirstSeq, "lastSequence": 缺口.LastSeq,
		"gapDigest": 摘要, "realExecutionEnabled": true,
	}
}

func 写入受控服务CA(t *testing.T, 服务 *httptest.Server) string {
	t.Helper()
	证书 := 服务.Certificate()
	if 证书 == nil {
		t.Fatal("受控 TLS 服务没有证书")
	}
	解析后的证书, err := x509.ParseCertificate(证书.Raw)
	if err != nil {
		t.Fatalf("解析受控 TLS 证书失败: %v", err)
	}
	路径 := filepath.Join(t.TempDir(), "controlled-control-plane-ca.pem")
	if err := os.WriteFile(路径, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: 解析后的证书.Raw}), 0o600); err != nil {
		t.Fatalf("写入受控 TLS CA 失败: %v", err)
	}
	return 路径
}

func 写入受控协议响应(t *testing.T, writer http.ResponseWriter, 状态 string, payload map[string]any) {
	t.Helper()
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{
		"requestId": "fixture-response", "serverTime": time.Now().UTC().Format(time.RFC3339Nano), "status": 状态, "payload": payload,
	}); err != nil {
		t.Errorf("写入受控协议响应失败: %v", err)
	}
}
