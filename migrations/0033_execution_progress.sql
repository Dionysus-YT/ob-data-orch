-- 只保存已确认日志批次投影的固定进度摘要；不改变任务状态或历史结果。
ALTER TABLE task_executions ADD COLUMN progress_json TEXT CHECK (progress_json IS NULL OR json_valid(progress_json));
