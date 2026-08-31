-- 0020: 任务取消请求的有界状态与控制面期限
--
-- 取消意图只保存请求标识、控制面时间和固定原因码，不保存命令、路径、秘密或用户自由文本。

ALTER TABLE task_executions ADD COLUMN cancellation_request_id TEXT;
ALTER TABLE task_executions ADD COLUMN cancellation_requested_at TEXT;
ALTER TABLE task_executions ADD COLUMN cancellation_deadline TEXT;
ALTER TABLE task_executions ADD COLUMN cancellation_reason TEXT CHECK (
    cancellation_reason IS NULL OR cancellation_reason IN ('USER_REQUEST', 'TIMEOUT')
);
