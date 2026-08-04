import type { TaskLog } from '@/api/browser'

// taskFailureSummary 仅从已经脱敏的任务日志提取工具已明确输出的失败行。
// 不能据此推断根因；日志不存在或没有具体错误时明确保留为待人工查看的失败事实。
export function taskFailureSummary(state: string | undefined, logs: readonly TaskLog[]): string | undefined {
  if (state !== 'FAILED') return undefined
  for (const record of [...logs].reverse()) {
    const message = record.message.trim()
    if (/\[ERROR\]/.test(message) && !/\bSystem exit \d+\b/i.test(message)) return message
  }
  return 'Agent 已确认 OBDUMPER 非正常退出；暂未收到可展示的具体工具错误。'
}
