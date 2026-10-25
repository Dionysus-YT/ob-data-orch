package logstream

import (
	"encoding/json"
	"strings"
	"testing"
)

func Test进度只接受已脱敏完整管道且摘要覆盖固定事实(t *testing.T) {
	record := Record{StreamID: "synthetic-progress", SourceKind: SourceLoaderStdout, SourceEpoch: 1, SourceSeq: 1, Kind: RecordLog, IntegrityCode: "COMPLETE", PolicyVersion: "synthetic-v1", ParserVersion: "obloader-4.3.5-v1", Message: "2026-10-19 10:20:30 [INFO] ---------- Finished Tasks: 1 Running Tasks: 1 Progress: 50.00% ----------"}
	old, _ := json.Marshal(record)
	if strings.Contains(string(old), `"Progress":`) {
		t.Fatal("旧记录被新增空字段改变摘要")
	}
	batch := Batch{StreamID: record.StreamID, SourceEpoch: 1, FirstSeq: 1, LastSeq: 1, PolicyVersion: record.PolicyVersion, Records: []Record{record}}
	legacy, err := SealBatch(batch)
	if err != nil {
		t.Fatal(err)
	}
	record.Progress = ParseProgress(record)
	if record.Progress == nil {
		t.Fatal("未提取固定事实")
	}
	batch.Records[0] = record
	current, err := SealBatch(batch)
	if err != nil || current.Digest == legacy.Digest {
		t.Fatal("进度事实未进入批次摘要", err)
	}
	*record.Progress.BPS = 9000
	if verified, err := SealBatch(current); err != nil || verified.Digest != current.Digest {
		t.Fatal("调用方修改指针改变了密封进度", err)
	}
	*record.Progress.BPS = 5000
	for _, alter := range []func(*Record){
		func(r *Record) { r.SourceKind = SourceLoaderStderr }, func(r *Record) { r.IntegrityCode = "UNKNOWN" }, func(r *Record) { r.Kind = RecordTruncated },
		func(r *Record) { r.ParserVersion = "unknown" }, func(r *Record) { r.Message = "unrelated synthetic output" },
	} {
		changed := record
		alter(&changed)
		batch.Records[0] = changed
		if _, err := SealBatch(batch); err == nil {
			t.Fatal("来源或正文不符仍接纳进度")
		}
	}
}

func Test进度结构化值不能脱离正文或通过重算摘要伪造(t *testing.T) {
	for _, field := range []string{"phase", "percent", "completed", "running"} {
		t.Run(field, func(t *testing.T) {
			record := Record{StreamID: "synthetic-progress", SourceKind: SourceStdout, SourceEpoch: 1, SourceSeq: 1, Kind: RecordLog, IntegrityCode: "COMPLETE", PolicyVersion: "synthetic-v1", ParserVersion: "direct-java-v1", Message: "2026-10-19 10:20:30 [INFO] ---------- Finished Tasks: 1 Running Tasks: 1 Progress: 50.00% ----------"}
			record.Progress = ParseProgress(record)
			switch field {
			case "phase":
				record.Progress.Phase = "IMPORT_DATA"
			case "percent":
				*record.Progress.BPS = 10000
			case "completed":
				record.Progress.Completed = 2
			case "running":
				record.Progress.Running = 0
			}
			if _, err := SealBatch(Batch{StreamID: record.StreamID, SourceEpoch: 1, FirstSeq: 1, LastSeq: 1, PolicyVersion: record.PolicyVersion, Records: []Record{record}}); err != ErrInvalidInput {
				t.Fatal("未拒绝与正文不一致的进度", err)
			}
		})
	}
}
