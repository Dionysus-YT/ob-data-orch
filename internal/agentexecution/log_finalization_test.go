package agentexecution

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"ob-data-orch/internal/agentlogqueue"
	"ob-data-orch/internal/agentwire"
	"ob-data-orch/internal/directload"
	"ob-data-orch/internal/importdomain"
	"ob-data-orch/internal/logstream"
)

type tailConfirmationProtocol struct {
	executionWorkerTestProtocol
	delay                     time.Duration
	failStderr, waitForCancel bool
	entered                   chan struct{}
	confirmed                 int
	cancelAfterConfirmation   context.CancelFunc
}

func (p *tailConfirmationProtocol) AppendExecutionLog(ctx context.Context, batch agentwire.ExecutionLogBatch) error {
	if batch.ExecutionID != p.grant.ExecutionID {
		return errors.New("本次收尾不能发送其他执行")
	}
	if p.entered != nil {
		select {
		case p.entered <- struct{}{}:
		default:
		}
	}
	if p.waitForCancel {
		<-ctx.Done()
		return ctx.Err()
	}
	if p.failStderr && batch.Batch.SourceEpoch == 2 {
		return errors.New("合成stderr未确认")
	}
	timer := time.NewTimer(p.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	p.confirmed++
	if p.confirmed == 3 && p.cancelAfterConfirmation != nil {
		p.cancelAfterConfirmation()
	}
	return nil
}

func Test普通和旁路尾部超过两秒仍需完整确认且失败取消保留(t *testing.T) {
	for _, taskType := range []string{importdomain.TaskType, directload.TaskType} {
		for _, scenario := range []string{"slow-success", "stderr-failure", "cancel", "cancel-after-confirmation"} {
			t.Run(taskType+"/"+scenario, func(t *testing.T) {
				input, _ := normalImportTestInput(t)
				grant := queueTestGrant()
				grant.TaskID = "synthetic-task"
				grant.TaskType = taskType
				if taskType == importdomain.TaskType {
					grant.NormalImport = input
				}
				protocol := &tailConfirmationProtocol{executionWorkerTestProtocol: executionWorkerTestProtocol{grant: grant}, failStderr: scenario == "stderr-failure", waitForCancel: scenario == "cancel", entered: make(chan struct{}, 1)}
				if scenario == "slow-success" {
					protocol.delay = 800 * time.Millisecond
				}
				queue, err := agentlogqueue.Open(t.TempDir(), agentlogqueue.DefaultMaxBytes)
				if err != nil {
					t.Fatal(err)
				}
				u := newLogUploader(context.Background(), protocol, "synthetic-boot", grant, queue, nil)
				for index := 0; index < 3; index++ {
					r := queueTestRecord()
					r.StreamID = grant.ExecutionID
					r.SourceKind = logstream.SourceLoaderStdout
					r.SourceSeq = int64(index + 1)
					if index == 2 {
						r.SourceKind = logstream.SourceLoaderStderr
						r.SourceEpoch = 2
						r.SourceSeq = 1
					}
					if err := u.append(r); err != nil {
						t.Fatal(err)
					}
				}
				otherGrant := grant
				otherGrant.ExecutionID = "synthetic-other"
				other := newLogUploader(context.Background(), protocol, "synthetic-boot", otherGrant, queue, nil)
				r := queueTestRecord()
				r.StreamID = otherGrant.ExecutionID
				if err := other.append(r); err != nil {
					t.Fatal(err)
				}
				worker := &Worker{Protocol: protocol, Clock: time.Now, BootID: "synthetic-boot"}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if scenario == "cancel-after-confirmation" {
					protocol.cancelAfterConfirmation = cancel
				}
				type result struct {
					out Outcome
					err error
				}
				done := make(chan result, 1)
				started := time.Now()
				go func() {
					var out Outcome
					var err error
					if taskType == importdomain.TaskType {
						out, _, err = worker.finishNormalImport(ctx, grant, 5, 0, nil, false, false, &normalImportTerminal{data: true, exited: true}, u)
					} else {
						out, _, err = worker.finishDirectLoad(ctx, grant, 5, 0, nil, false, &directLoadTerminal{table: "synthetic", committed: true, exited: true}, u)
					}
					done <- result{out, err}
				}()
				if scenario == "cancel" {
					select {
					case <-protocol.entered:
						cancel()
					case <-ctx.Done():
						t.Fatal("没有进入可取消上传")
					}
				}
				var got result
				select {
				case got = <-done:
				case <-time.After(11 * time.Second):
					t.Fatal("收尾未在失败或取消后返回")
				}
				want := scenario == "slow-success"
				if got.out.Succeeded != want || (got.err == nil) != want {
					t.Fatal("尾部确认结果错误", got.out, got.err)
				}
				if want && (time.Since(started) <= 2*time.Second || protocol.confirmed != 3) {
					t.Fatal("未覆盖超过两秒且完整确认的正常尾部")
				}
				items, err := queue.PendingItems()
				if err != nil {
					t.Fatal(err)
				}
				own, foreign := 0, 0
				for _, item := range items {
					if item.Entry.ExecutionID == grant.ExecutionID {
						own++
					} else {
						foreign++
					}
				}
				if foreign != 1 || want && own != 0 || !want && scenario != "cancel-after-confirmation" && own == 0 {
					t.Fatal("收尾丢失未确认条目或消费了其他执行", own, foreign)
				}
				if scenario == "cancel-after-confirmation" && (own != 0 || protocol.confirmed != 3) {
					t.Fatal("未覆盖最后回执后取消与空快照竞态")
				}
				if scenario == "stderr-failure" && (own != 1 || protocol.confirmed != 2) {
					t.Fatal("stderr未确认被stdout成功覆盖")
				}
				if len(protocol.events) != 2 || protocol.events[1].Evidence["result"] != map[bool]string{true: "VERIFIED", false: "FAILED"}[want] {
					t.Fatal("终态证据被降低")
				}
				if !want && protocol.events[0].Evidence["errorCode"] != "EVIDENCE_UNAVAILABLE" {
					t.Fatal("未确认尾部没有显式失败关闭")
				}
			})
		}
	}
}

// 合成时钟逐次驱动续租，保留生产 30 秒周期；阶段计数跨发送和续租协程使用原子值。
// 阶段 1 是进程退出后的日志确认，阶段 2 是最终结果事件仍在等待网络回复。
type finalizationLeaseProtocol struct {
	*safetyExecutionProtocol
	exited                                 chan struct{}
	ticks                                  chan time.Time
	renewed                                chan int32
	stage                                  atomic.Int32
	tailRenewals, finalRenewals, confirmed atomic.Int32
	securityDir                            string
}

func (p *finalizationLeaseProtocol) requireRenewal(ctx context.Context, stage int32) error {
	p.stage.Store(stage)
	select {
	case p.ticks <- time.Now():
	case <-ctx.Done():
		return ctx.Err()
	}
	for {
		select {
		case received := <-p.renewed:
			if received == stage {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (p *finalizationLeaseProtocol) RenewExecutionLease(ctx context.Context, _ agentwire.ExecutionLeaseRenewal) error {
	stage := p.stage.Load()
	if stage == 1 {
		p.tailRenewals.Add(1)
	} else if stage == 2 {
		p.finalRenewals.Add(1)
	}
	select {
	case p.renewed <- stage:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *finalizationLeaseProtocol) AppendExecutionEvent(ctx context.Context, event agentwire.ExecutionEvent) error {
	if event.EventType == "RESULT_FACTS_OBSERVED" {
		if err := p.requireRenewal(ctx, 2); err != nil {
			return err
		}
	}
	if err := p.safetyExecutionProtocol.AppendExecutionEvent(ctx, event); err != nil {
		return err
	}
	if event.EventType == "PROCESS_EXITED" {
		close(p.exited)
	}
	return nil
}

func (p *finalizationLeaseProtocol) AppendExecutionLog(ctx context.Context, _ agentwire.ExecutionLogBatch) error {
	select {
	case <-p.exited:
	case <-ctx.Done():
		return ctx.Err()
	}
	for _, name := range []string{"security.properties", "key.pem", "secure.rsa"} {
		if _, err := os.Stat(filepath.Join(p.securityDir, name)); !errors.Is(err, os.ErrNotExist) {
			return errors.New("尾部收尾延长了秘密生命周期")
		}
	}
	if err := p.requireRenewal(ctx, 1); err != nil {
		return err
	}
	timer := time.NewTimer(600 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	p.confirmed.Add(1)
	return nil
}

func TestWorker长尾确认与最终事件持续续租且秘密先清除(t *testing.T) {
	worker, base, _ := safetyWorker(t, "normal-success")
	input, argv := normalImportTestInput(t)
	base.grant.TaskType = importdomain.TaskType
	base.grant.NormalImport = input
	base.grant.Argv = argv
	worker.TaskType = importdomain.TaskType
	worker.Runtime.AllowedRoots = []string{input.Config.FilePath}
	p := &finalizationLeaseProtocol{safetyExecutionProtocol: base, exited: make(chan struct{}), ticks: make(chan time.Time, 4), renewed: make(chan int32, 4), securityDir: filepath.Join(worker.Runtime.WorkspaceRoot, "executions", base.grant.ExecutionID, "security")}
	worker.Protocol = p
	worker.leaseRenewalTicks = p.ticks
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	started := time.Now()
	out, found, err := worker.RunNext(ctx)
	if err != nil || !found || !out.Succeeded {
		t.Fatal("正常长尾不能完成", out, err)
	}
	if p.confirmed.Load() != 4 || p.tailRenewals.Load() < 4 || p.finalRenewals.Load() < 1 || time.Since(started) <= 2*time.Second {
		t.Fatal("未覆盖长尾与最后事件的续租", p.confirmed.Load(), p.tailRenewals.Load(), p.finalRenewals.Load())
	}
	last := p.tailRenewals.Load() + p.finalRenewals.Load()
	p.ticks <- time.Now()
	timer := time.NewTimer(50 * time.Millisecond)
	defer timer.Stop()
	<-timer.C
	if p.tailRenewals.Load()+p.finalRenewals.Load() != last {
		t.Fatal("Worker返回后仍续租")
	}
}
