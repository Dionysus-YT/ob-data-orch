import type { TaskListItem } from '@/api/browser'

const taskStateLabels: Readonly<Record<string, string>> = {
  WAITING_SCHEDULE: '等待调度',
  STARTING: '启动中',
  RUNNING: '运行中',
  CANCELLING: '取消中',
  SUCCEEDED: '成功',
  FAILED: '失败',
  CANCELLED: '已取消',
}

export function taskStateLabel(task: TaskListItem): string {
  return taskStateLabels[task.state] ?? '状态核对中'
}

// 未知状态或服务端明确要求核对时，只附加核对标识，不把最后可信状态改写成失败。
export function taskNeedsReconciliation(task: TaskListItem): boolean {
  return task.reconciliationRequired || taskStateLabels[task.state] === undefined
}

export function taskStateClass(task: TaskListItem): 'success' | 'danger' | 'neutral' {
  if (task.state === 'SUCCEEDED' && !taskNeedsReconciliation(task)) return 'success'
  if (task.state === 'FAILED' && !taskNeedsReconciliation(task)) return 'danger'
  return 'neutral'
}

export function taskStageLabel(task: TaskListItem): string {
  return taskNeedsReconciliation(task) ? '等待状态核对' : '暂无可靠阶段'
}

export function taskProgressLabel(): string {
  return '暂无可靠进度'
}

export function taskTypeLabel(type: TaskListItem['type']): string {
  return type === 'OBDUMPER_EXPORT' ? '导出' : '未知类型'
}
