package agentexecution

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ob-data-orch/internal/agentwire"
)

type observingExecutionProtocol struct {
	executionWorkerTestProtocol
	worker *Worker
	seen   bool
}

func (p *observingExecutionProtocol) AppendExecutionEvent(_ context.Context, event agentwire.ExecutionEvent) error {
	active, found := p.worker.ActiveExecution()
	p.seen = found && active.ExecutionID == event.ExecutionID && active.LeaseID == event.LeaseID && active.LastEventSeq == event.EventSeq
	return errors.New("synthetic unconfirmed event")
}

func TestWorker观测本地序号但上报失败返回后清除活动引用(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	protocol := &observingExecutionProtocol{executionWorkerTestProtocol: executionWorkerTestProtocol{grant: agentwire.ExecutionGrant{TaskID: "synthetic-task", ExecutionID: "synthetic-execution", LeaseID: "synthetic-lease", LeaseEpoch: 1, ExpiresAt: now.Add(time.Minute), EnvelopeDigest: strings.Repeat("a", 64), Argv: []string{"--file-path", "/E:/synthetic-output"}}, control: agentwire.ExecutionControl{CancelRequested: true}}}
	worker := &Worker{Protocol: protocol, Runtime: Runtime{TargetPlatform: testNativePlatform(root), JavaPath: filepath.Join(root, "java.exe"), ToolHome: root, WorkspaceRoot: filepath.Join(root, "workspace"), Environment: []string{"PATH=C:\\Windows\\System32"}, AllowedRoots: []string{root}}, Clock: func() time.Time { return now }, BootID: "synthetic-boot"}
	protocol.worker = worker
	if _, found, err := worker.RunNext(context.Background()); !found || err == nil || !protocol.seen {
		t.Fatal("失败事件前没有观测真实 Worker 位置")
	}
	if _, found := worker.ActiveExecution(); found {
		t.Fatal("返回后遗留旧 Worker 引用")
	}
}
