package logstream

import "ob-data-orch/internal/toolprogress"

// ProgressRecord 是已双层脱敏批次中的固定进度事实或完整性中断，不包含日志正文。
type ProgressRecord struct {
	Sequence int64
	Fact     *toolprogress.Fact
}

// ParseProgress 在 Agent 第一层脱敏之后调用，只从固定工具 stdout 及版本化解析器提取事实。
// stderr 与文件流没有和 stdout 的全局因果顺序，不参与同一条进度投影。
func ParseProgress(record Record) *toolprogress.Fact {
	tool := progressTool(record)
	if tool == "" || record.Kind != RecordLog || record.IntegrityCode != "COMPLETE" {
		return nil
	}
	return toolprogress.Parse(tool, record.Message)
}

func progressTool(record Record) string {
	if record.SourceKind == SourceStdout && record.ParserVersion == "direct-java-v1" {
		return "OBDUMPER"
	}
	if record.SourceKind == SourceLoaderStdout && record.ParserVersion == "obloader-4.3.5-v1" {
		return "OBLOADER"
	}
	return ""
}

func validProgress(record Record) bool {
	if record.Progress == nil {
		return true
	}
	expected := ParseProgress(record)
	return expected != nil && expected.Equal(*record.Progress)
}

func batchProgress(batch Batch) []ProgressRecord {
	var facts []ProgressRecord
	for _, record := range batch.Records {
		if record.Progress != nil {
			facts = append(facts, ProgressRecord{Sequence: record.SourceSeq, Fact: record.Progress})
		} else if record.Kind != RecordLog || record.IntegrityCode != "COMPLETE" {
			facts = append(facts, ProgressRecord{Sequence: record.SourceSeq})
		}
	}
	return facts
}
