import type { TaskListAction, TaskListItem } from '@/api/browser'
import { isKnownTaskState, taskStateBadge, taskStateText, taskTypeText } from './taskStatePresentation'

export function taskStateLabel(task: Pick<TaskListItem, 'state'>): string {
  return taskStateText(task.state)
}

// 未知状态或服务端明确要求核对时，只附加核对标识，不把最后可信状态改写成失败。
export function taskNeedsReconciliation(task: Pick<TaskListItem, 'state' | 'reconciliationRequired'>): boolean {
  return task.reconciliationRequired || !isKnownTaskState(task.state)
}

export function taskStateBadgeStatus(task: Pick<TaskListItem, 'state' | 'reconciliationRequired'>) {
  return taskStateBadge(task.state, taskNeedsReconciliation(task))
}

// 契约当前把阶段证据固定为 UNAVAILABLE；没有可靠阶段映射时不写“暂无可靠阶段”这类占位，页面留空。
// 将来接入可靠阶段时在此处映射，调用方无需改动。
export function taskStageLabel(): string {
  return ''
}

export function taskTypeLabel(type: TaskListItem['type']): string {
  return taskTypeText(type)
}

/** 历史名称不可用时退回标识；对象已删除时附加说明，不反查当前配置。 */
export function taskHistoricalName(name: string | undefined, id: string, deleted: boolean): string {
  const base = name || id
  return deleted ? `${base}（已删除）` : base
}

export type TaskRowPrimary = 'VIEW_DETAIL' | 'VIEW_ERROR'

/** 行主操作只做导航：失败任务直达失败摘要，其余进入详情。 */
export function taskRowPrimary(task: Pick<TaskListItem, 'state'>): TaskRowPrimary {
  return task.state === 'FAILED' ? 'VIEW_ERROR' : 'VIEW_DETAIL'
}

const moreMenuOrder: readonly TaskListAction[] = ['VIEW_LOGS', 'COPY_COMMAND', 'REBUILD_FROM_CONFIG', 'RERUN_FROM_SCRATCH', 'CHECKPOINT_RESUME', 'SAVE_TEMPLATE', 'CANCEL']

export const taskActionLabels: Readonly<Record<TaskListAction, string>> = {
  VIEW_LOGS: '查看日志',
  COPY_COMMAND: '复制脱敏命令',
  REBUILD_FROM_CONFIG: '基于原配置新建',
  RERUN_FROM_SCRATCH: '从头重新执行',
  CHECKPOINT_RESUME: '从检查点继续',
  SAVE_TEMPLATE: '保存为模板',
  CANCEL: '取消任务',
}

/** More 菜单只渲染服务端返回的资格，顺序固定，危险操作置底。 */
export function taskMoreActions(task: Pick<TaskListItem, 'availableActions'>): readonly TaskListAction[] {
  return moreMenuOrder.filter(action => task.availableActions.includes(action))
}
