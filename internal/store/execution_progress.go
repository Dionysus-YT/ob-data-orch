package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"ob-data-orch/internal/logstream"
	"ob-data-orch/internal/toolprogress"
)

// TaskProgress 是列表和详情共用的固定安全投影，不含正文、路径、表名或任意阶段文本。
// Percent 只描述工具数据子任务；即使达到 100 也不能触发产品成功或结果核验。
type TaskProgress struct {
	Phase          string    `json:"phase"`
	Percent        *float64  `json:"percent,omitempty"`
	CompletedTasks *int64    `json:"completedTasks,omitempty"`
	RunningTasks   *int64    `json:"runningTasks,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Incomplete     bool      `json:"incomplete"`
}

type executionProgress struct {
	Version     string            `json:"version"`
	StreamID    string            `json:"streamId"`
	SourceEpoch int64             `json:"sourceEpoch"`
	Sequence    int64             `json:"sequence"`
	Fact        toolprogress.Fact `json:"fact"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	Incomplete  bool              `json:"incomplete"`
}

func progressToolForTask(kind string) string {
	if kind == "OBDUMPER_EXPORT" {
		return "OBDUMPER"
	}
	if kind == "OBLOADER_NORMAL_IMPORT" {
		return "OBLOADER"
	}
	return ""
}

func decodeExecutionProgress(raw, kind string) *executionProgress {
	var item executionProgress
	if json.Unmarshal([]byte(raw), &item) != nil || item.Version != "tool-progress-v1" || item.StreamID == "" || item.SourceEpoch < 1 || item.Sequence < 1 || item.UpdatedAt.IsZero() || !item.Fact.ValidFor(progressToolForTask(kind)) {
		return nil
	}
	return &item
}

func projectTaskProgress(raw, kind string, reconciliation bool) *TaskProgress {
	item := decodeExecutionProgress(raw, kind)
	if item == nil {
		return nil
	}
	result := &TaskProgress{Phase: item.Fact.Phase, UpdatedAt: item.UpdatedAt, Incomplete: item.Incomplete || reconciliation}
	if item.Fact.BPS != nil {
		percent := float64(*item.Fact.BPS) / 100
		result.Percent, result.CompletedTasks, result.RunningTasks = &percent, &item.Fact.Completed, &item.Fact.Running
	}
	return result
}

// commitExecutionProgress 与批次索引在同一短事务提交。重复、序号缺口和落盘失败不能推进摘要。
// 只消费版本匹配的单一 stdout 来源；终态补传仅补日志，不改写已冻结的末次进度或业务状态。
func commitExecutionProgress(ctx context.Context, tx *sql.Tx, input logstream.IndexedBatch) error {
	if len(input.Progress) == 0 || input.SourceKind != "STDOUT" {
		return nil
	}
	var state, kind, version, raw string
	if err := tx.QueryRowContext(ctx, `SELECT e.state,t.task_type,t.tool_version,COALESCE(e.progress_json,'') FROM task_executions e JOIN tasks t ON t.task_id=e.task_id WHERE e.execution_id=?`, input.ExecutionID).Scan(&state, &kind, &version, &raw); err != nil {
		return err
	}
	if version != "4.3.5-RELEASE" || state != "STARTING" && state != "RUNNING" && state != "CANCELLING" {
		return nil
	}
	tool := progressToolForTask(kind)
	if tool == "" || tool == "OBDUMPER" && input.ParserVersion != "direct-java-v1" || tool == "OBLOADER" && input.ParserVersion != "obloader-4.3.5-v1" {
		return nil
	}
	current := decodeExecutionProgress(raw, kind)
	changed := false
	for _, record := range input.Progress {
		if record.Sequence < input.FirstSequence || record.Sequence > input.LastSequence {
			return logstream.ErrInvalidInput
		}
		if current != nil && (current.StreamID != input.StreamID || current.SourceEpoch != input.SourceEpoch || record.Sequence <= current.Sequence) {
			continue
		}
		if record.Fact == nil {
			if current != nil {
				current.Incomplete, current.Sequence = true, record.Sequence
				changed = true
			}
			continue
		}
		if !record.Fact.ValidFor(tool) {
			return logstream.ErrInvalidInput
		}
		current = &executionProgress{Version: "tool-progress-v1", StreamID: input.StreamID, SourceEpoch: input.SourceEpoch, Sequence: record.Sequence, Fact: *record.Fact, UpdatedAt: input.ReceivedAt}
		changed = true
	}
	if !changed {
		return nil
	}
	encoded, err := json.Marshal(current)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE task_executions SET progress_json=? WHERE execution_id=?`, string(encoded), input.ExecutionID)
	return err
}
