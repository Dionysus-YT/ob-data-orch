package logstream

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func Test旁路持久缺口重读保持物理来源和独立解析器(t *testing.T) {
	for _, kind := range []SourceKind{SourceLoaderStdout, SourceLoaderStderr} {
		t.Run(string(kind), func(t *testing.T) {
			root := t.TempDir()
			segment := strings.Repeat("a", 64)
			gap := GapNotice{StreamID: "synthetic-direct", SourceKind: kind, SourceEpoch: 1, FirstSeq: 2, LastSeq: 3, ReasonCode: "LOCAL_SPOOL_LIMIT", PolicyVersion: "synthetic-v1", ParserVersion: "obloader-direct-4.3.5-v1"}
			now := time.Now().UTC()
			record := Record{StreamID: gap.StreamID, SourceKind: gap.SourceKind, SourceEpoch: gap.SourceEpoch, SourceSeq: gap.FirstSeq, Kind: RecordGap, Message: "日志序号存在缺口", IntegrityCode: gap.ReasonCode, PolicyVersion: gap.PolicyVersion, ParserVersion: gap.ParserVersion, ReceivedAt: now}
			writer, err := OpenSegment(root, segment, Policy{Version: gap.PolicyVersion})
			if err != nil {
				t.Fatal(err)
			}
			end, appendErr := writer.AppendGap(record)
			closeErr := writer.Close()
			if appendErr != nil || closeErr != nil {
				t.Fatal("合成缺口未落盘", appendErr, closeErr)
			}
			physical, _ := persistedSourceKindFromRecord(kind)
			batch := IndexedBatch{SourceKind: physical, SourceRef: gap.StreamID, SourceEpoch: gap.SourceEpoch, FirstSequence: gap.FirstSeq, LastSequence: gap.LastSeq, Digest: gapDigest(gap), PolicyVersion: gap.PolicyVersion, ParserVersion: gap.ParserVersion, SegmentID: segment, SegmentStorageKey: segment + ".open", SegmentState: "OPEN", SegmentOffsetEnd: end, RecordCount: 1, GapReasonCode: gap.ReasonCode, ReceivedAt: now}
			store := &PersistentStore{root: root}
			rows, err := store.readBatch(batch)
			if err != nil || len(rows) != 1 || rows[0].SourceKind != kind || rows[0].ParserVersion != gap.ParserVersion || rows[0].Kind != RecordGap {
				t.Fatal("旁路缺口重读丢失来源或误报损坏", err)
			}
			// 篡改索引的解析器不应让持久正文自动获得相似工具身份。
			batch.ParserVersion = "obloader-direct-unknown"
			if _, err := store.readBatch(batch); !errors.Is(err, ErrStorageCorrupt) {
				t.Fatal("未知解析器被提升为旁路来源")
			}
		})
	}
}

func Test未知解析器与错误物理来源不能恢复OBLOADER身份(t *testing.T) {
	for _, parser := range []string{"", "direct-java-v1", "obloader-direct-4.3.5-v2", "prefix-obloader-direct-4.3.5-v1", "obloader-direct-4.3.5-v1-extra"} {
		for _, physical := range []string{"STDOUT", "STDERR"} {
			kind, err := storedSourceKind(physical, parser)
			if err != nil || kind == SourceLoaderStdout || kind == SourceLoaderStderr {
				t.Fatal("未知解析器伪造了工具来源")
			}
		}
	}
	if kind, err := storedSourceKind("TOOL_FILE", "obloader-direct-4.3.5-v1"); err != nil || kind != SourceFile {
		t.Fatal("工具文件被提升为旁路管道来源")
	}
	if _, err := storedSourceKind("OBLOADER_STDOUT", "obloader-direct-4.3.5-v1"); err == nil {
		t.Fatal("非物理来源被持久层接受")
	}
}
