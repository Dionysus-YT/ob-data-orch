//go:build queuevalidation

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ob-data-orch/internal/agentexecution"
)

const queueValidationWait = 2 * time.Minute

// queueValidationObserver 只在显式测试构建中安装本机三阶段屏障。
// 三个释放标记均位于 Agent 身份目录下的私有队列根，正式构建没有对应代码路径。
func queueValidationObserver(queueRoot string) agentexecution.LogQueueObserver {
	if queueRoot == "" {
		return nil
	}
	return &localQueueValidationObserver{directory: filepath.Join(queueRoot, "queue-validation-sse")}
}

type localQueueValidationObserver struct {
	directory string
	before    queueValidationPhase
	after     queueValidationPhase
	confirmed queueValidationPhase
}

type queueValidationPhase struct {
	once sync.Once
	err  error
}

type queueValidationEvidence struct {
	Phase       string    `json:"phase"`
	StreamID    string    `json:"streamId"`
	SourceEpoch int64     `json:"sourceEpoch"`
	FirstSeq    int64     `json:"firstSeq"`
	LastSeq     int64     `json:"lastSeq"`
	BatchDigest string    `json:"batchDigest"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (o *localQueueValidationObserver) BeforeEnqueue(ctx context.Context, position agentexecution.LogQueuePosition) error {
	if o == nil {
		return errors.New("本地队列演练屏障不可用")
	}
	o.before.once.Do(func() {
		o.before.err = o.waitForRelease(ctx, "BEFORE_ENQUEUE", "before-enqueue-ready.json", "release-before-enqueue", position)
	})
	return o.before.err
}

func (o *localQueueValidationObserver) AfterEnqueue(ctx context.Context, position agentexecution.LogQueuePosition) error {
	if o == nil {
		return errors.New("本地队列演练屏障不可用")
	}
	o.after.once.Do(func() {
		o.after.err = o.waitForRelease(ctx, "AFTER_ENQUEUE", "after-enqueue-ready.json", "release-after-enqueue", position)
	})
	return o.after.err
}

func (o *localQueueValidationObserver) AfterConfirmation(ctx context.Context, position agentexecution.LogQueuePosition) error {
	if o == nil {
		return errors.New("本地队列演练屏障不可用")
	}
	o.confirmed.once.Do(func() {
		o.confirmed.err = o.waitForRelease(ctx, "AFTER_CONFIRMATION", "after-confirmation-ready.json", "release-after-confirmation", position)
	})
	return o.confirmed.err
}

func (o *localQueueValidationObserver) waitForRelease(ctx context.Context, phase, readyName, releaseName string, position agentexecution.LogQueuePosition) error {
	if ctx == nil || o.directory == "" {
		return errors.New("本地队列演练屏障参数无效")
	}
	if err := os.MkdirAll(o.directory, 0o700); err != nil {
		return err
	}
	if err := writeQueueValidationEvidence(filepath.Join(o.directory, readyName), queueValidationEvidence{
		Phase: phase, StreamID: position.StreamID, SourceEpoch: position.SourceEpoch, FirstSeq: position.FirstSeq, LastSeq: position.LastSeq, BatchDigest: position.BatchDigest, CreatedAt: time.Now().UTC(),
	}); err != nil {
		return err
	}
	timer := time.NewTimer(queueValidationWait)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if releaseMarkerReady(filepath.Join(o.directory, releaseName)) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return errors.New("本地队列演练屏障等待释放超时")
		case <-ticker.C:
		}
	}
}

func writeQueueValidationEvidence(path string, evidence queueValidationEvidence) error {
	if info, err := os.Lstat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
		if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
			return errors.New("本地队列演练就绪标记不安全")
		}
		return errors.New("本地队列演练就绪标记已存在")
	}
	content, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".queue-validation-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func releaseMarkerReady(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular()
}
