import { describe, expect, it } from 'vitest'

import type { TaskListItem } from '@/api/browser'
import { taskHistoricalName, taskMoreActions, taskNeedsReconciliation, taskRowPrimary, taskStageLabel, taskStateBadgeStatus, taskStateLabel } from './taskListPresentation'
import { taskDurationText, taskStateText } from './taskStatePresentation'

function syntheticTask(overrides: Partial<TaskListItem> = {}): TaskListItem {
  return {
    id: 'task-1', type: 'OBDUMPER_EXPORT', dataSourceId: 'source-1', objectSummary: 'synthetic_db.synthetic_table',
    state: 'RUNNING', stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE', reconciliationRequired: false,
    nodeId: 'node-1', ownedByCurrentUser: true, submittedAt: '2026-07-31T03:00:00Z', updatedAt: '2026-07-31T03:01:00Z',
    displayName: 'synthetic_db.synthetic_table', dataSourceDeleted: false, nodeDeleted: false, cancelRequested: false, availableActions: [],
    ...overrides,
  }
}

describe('任务列表可靠证据降级', () => {
  it('没有阶段证据时不写占位文案', () => {
    expect(taskStateLabel(syntheticTask())).toBe('运行中')
    expect(taskStageLabel()).toBe('')
  })

  it('未知状态与核对标识都显示状态核对中且不伪造失败', () => {
    const unknown = syntheticTask({ state: 'UNEXPECTED_STATE' })
    expect(taskStateLabel(unknown)).toBe('状态核对中')
    expect(taskNeedsReconciliation(unknown)).toBe(true)
    expect(taskStateBadgeStatus(syntheticTask({ state: 'FAILED', reconciliationRequired: true }))).toBe('warning')
  })

  it('八种状态共用一份文案', () => {
    expect(['DRAFT', 'WAITING_SCHEDULE', 'STARTING', 'RUNNING', 'CANCELLING', 'SUCCEEDED', 'FAILED', 'CANCELLED'].map(taskStateText))
      .toEqual(['草稿', '等待调度', '启动中', '运行中', '取消中', '成功', '失败', '已取消'])
  })
})

describe('任务列表行操作与历史名称', () => {
  it('More 菜单只渲染服务端资格，危险操作置底', () => {
    expect(taskMoreActions(syntheticTask({ availableActions: ['CANCEL', 'COPY_COMMAND', 'VIEW_LOGS'] }))).toEqual(['VIEW_LOGS', 'COPY_COMMAND', 'CANCEL'])
    expect(taskMoreActions(syntheticTask())).toEqual([])
  })

  it('失败任务主操作直达错误，其余进入详情', () => {
    expect(taskRowPrimary(syntheticTask({ state: 'FAILED' }))).toBe('VIEW_ERROR')
    expect(taskRowPrimary(syntheticTask({ state: 'SUCCEEDED' }))).toBe('VIEW_DETAIL')
  })

  it('历史名称缺失退回标识，已删除附加说明', () => {
    expect(taskHistoricalName('源', 'source-1', false)).toBe('源')
    expect(taskHistoricalName(undefined, 'source-1', true)).toBe('source-1（已删除）')
  })

  it('耗时只在已开始且时间有效时显示', () => {
    expect(taskDurationText(undefined)).toBe('')
    expect(taskDurationText('2026-01-01T00:00:00Z', '2026-01-01T01:02:03Z')).toBe('1 小时 2 分')
    expect(taskDurationText('2026-01-01T00:00:00Z', '2025-12-31T00:00:00Z')).toBe('')
  })
})
