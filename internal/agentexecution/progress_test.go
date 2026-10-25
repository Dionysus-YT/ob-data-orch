package agentexecution

import (
	"context"
	"errors"
	"testing"

	"ob-data-orch/internal/agentlogqueue"
	"ob-data-orch/internal/logstream"
)

func Test进度随脱敏批次持久化且重放同一摘要(t *testing.T) {
	queue, err := agentlogqueue.Open(t.TempDir(), agentlogqueue.DefaultMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	protocol := &queueTestProtocol{appendErrors: []error{errors.New("合成网络中断")}}
	uploader := newLogUploader(context.Background(), protocol, "synthetic-boot", queueTestGrant(), queue, nil)
	record := logstream.Record{StreamID: "execution-test", SourceKind: logstream.SourceLoaderStdout, SourceEpoch: 1, SourceSeq: 1, Kind: logstream.RecordLog, IntegrityCode: "COMPLETE", PolicyVersion: "synthetic-v1", ParserVersion: "obloader-4.3.5-v1", Message: "2026-10-19 10:20:30 [INFO] ---------- Finished Tasks: 1 Running Tasks: 3 Progress: 25.00% ----------"}
	if err := uploader.append(record); err != nil {
		t.Fatal("已持久日志不应因一次上传失败丢弃", err)
	}
	items, _, err := queue.Pending()
	if err != nil || len(items) != 1 || items[0].Batch.Records[0].Progress == nil || *items[0].Batch.Records[0].Progress.BPS != 2500 {
		t.Fatal("进度未随批次持久化", err)
	}
	sealed := items[0].Batch.Digest
	if err := flushQueuedLogs(context.Background(), queue, protocol, "synthetic-restart", ""); err != nil {
		t.Fatal(err)
	}
	if len(protocol.batches) != 2 || protocol.batches[0].Batch.Digest != sealed || protocol.batches[1].Batch.Digest != sealed || protocol.batches[1].Batch.Records[0].Progress == nil {
		t.Fatal("重放改变了进度事实或摘要")
	}
	if items, _, err = queue.Pending(); err != nil || len(items) != 0 {
		t.Fatal("确认后没有释放进度批次", err)
	}
}
