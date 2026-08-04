//go:build queuevalidation

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ob-data-orch/internal/agentexecution"
)

func Test本地队列演练屏障仅写安全位置并等待三次释放(t *testing.T) {
	observer := queueValidationObserver(t.TempDir())
	local, ok := observer.(*localQueueValidationObserver)
	if !ok {
		t.Fatal("测试构建必须安装本地演练屏障")
	}
	position := agentexecution.LogQueuePosition{StreamID: "stream-test", SourceEpoch: 1, FirstSeq: 1, LastSeq: 1, BatchDigest: "digest-test"}
	beforeDone := make(chan error, 1)
	go func() { beforeDone <- local.BeforeEnqueue(context.Background(), position) }()
	beforeReady := filepath.Join(local.directory, "before-enqueue-ready.json")
	waitForQueueValidationFile(t, beforeReady)
	assertQueueValidationEvidence(t, beforeReady, "BEFORE_ENQUEUE", position)
	writeQueueValidationRelease(t, filepath.Join(local.directory, "release-before-enqueue"))
	if err := <-beforeDone; err != nil {
		t.Fatalf("入队前屏障未释放: %v", err)
	}

	afterDone := make(chan error, 1)
	go func() { afterDone <- local.AfterEnqueue(context.Background(), position) }()
	afterReady := filepath.Join(local.directory, "after-enqueue-ready.json")
	waitForQueueValidationFile(t, afterReady)
	assertQueueValidationEvidence(t, afterReady, "AFTER_ENQUEUE", position)
	writeQueueValidationRelease(t, filepath.Join(local.directory, "release-after-enqueue"))
	if err := <-afterDone; err != nil {
		t.Fatalf("入队后屏障未释放: %v", err)
	}

	confirmedDone := make(chan error, 1)
	go func() { confirmedDone <- local.AfterConfirmation(context.Background(), position) }()
	confirmedReady := filepath.Join(local.directory, "after-confirmation-ready.json")
	waitForQueueValidationFile(t, confirmedReady)
	assertQueueValidationEvidence(t, confirmedReady, "AFTER_CONFIRMATION", position)
	writeQueueValidationRelease(t, filepath.Join(local.directory, "release-after-confirmation"))
	if err := <-confirmedDone; err != nil {
		t.Fatalf("确认后屏障未释放: %v", err)
	}
}

func waitForQueueValidationFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("未等到演练标记: %s", path)
}

func assertQueueValidationEvidence(t *testing.T, path, phase string, position agentexecution.LogQueuePosition) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取演练标记失败: %v", err)
	}
	var evidence queueValidationEvidence
	if err := json.Unmarshal(content, &evidence); err != nil {
		t.Fatalf("演练标记不是受控 JSON: %v", err)
	}
	if evidence.Phase != phase || evidence.StreamID != position.StreamID || evidence.SourceEpoch != position.SourceEpoch || evidence.FirstSeq != position.FirstSeq || evidence.LastSeq != position.LastSeq || evidence.BatchDigest != position.BatchDigest {
		t.Fatalf("演练标记不是预期安全位置: %+v", evidence)
	}
	if string(content) == "" || filepath.Base(path) == "" {
		t.Fatal("演练标记不能为空")
	}
}

func writeQueueValidationRelease(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("release"), 0o600); err != nil {
		t.Fatalf("写入演练释放标记失败: %v", err)
	}
}
