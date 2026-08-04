package controlplane

import (
	"testing"

	"ob-data-orch/internal/logstream"
)

func Test执行日志确认回显同一密封批次的位置与摘要(t *testing.T) {
	batch, err := logstream.SealBatch(logstream.Batch{
		StreamID: "execution-fixture", SourceEpoch: 1, FirstSeq: 3, LastSeq: 3, PolicyVersion: "fixture-v1",
		Records: []logstream.Record{{
			StreamID: "execution-fixture", SourceKind: logstream.SourceStdout, SourceEpoch: 1, SourceSeq: 3,
			Kind: logstream.RecordLog, Message: "已脱敏合成日志", PolicyVersion: "fixture-v1", ParserVersion: "fixture-v1",
		}},
	})
	if err != nil {
		t.Fatalf("密封日志批次失败: %v", err)
	}
	payload := executionLogConfirmationPayload(batch, logstream.BatchResult{Decision: logstream.BatchDuplicate, ExpectedSeq: 4})
	if payload["decision"] != logstream.BatchDuplicate || payload["expectedSequence"] != int64(4) || payload["streamId"] != batch.StreamID || payload["sourceEpoch"] != batch.SourceEpoch || payload["firstSequence"] != batch.FirstSeq || payload["lastSequence"] != batch.LastSeq || payload["batchDigest"] != batch.Digest || payload["realExecutionEnabled"] != true {
		t.Fatalf("日志确认回执未回显当前批次: %#v", payload)
	}
}

func Test执行日志缺口确认回显同一来源范围与摘要(t *testing.T) {
	gap := logstream.GapNotice{
		StreamID: "execution-fixture", SourceKind: logstream.SourceStderr, SourceEpoch: 2,
		FirstSeq: 5, LastSeq: 7, ReasonCode: "LOCAL_SPOOL_LIMIT", PolicyVersion: "fixture-v1", ParserVersion: "fixture-v1",
	}
	digest, err := logstream.GapDigest(gap)
	if err != nil {
		t.Fatalf("计算日志缺口摘要失败: %v", err)
	}
	payload := executionLogGapConfirmationPayload(gap, digest, logstream.BatchResult{Decision: logstream.BatchAccepted, ExpectedSeq: 8})
	if payload["decision"] != logstream.BatchAccepted || payload["expectedSequence"] != int64(8) || payload["streamId"] != gap.StreamID || payload["sourceEpoch"] != gap.SourceEpoch || payload["firstSequence"] != gap.FirstSeq || payload["lastSequence"] != gap.LastSeq || payload["gapDigest"] != digest || payload["realExecutionEnabled"] != true {
		t.Fatalf("日志缺口确认回执未回显当前缺口: %#v", payload)
	}
}
