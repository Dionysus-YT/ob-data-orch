import { describe, expect, it } from 'vitest'

import type { TaskListItem } from '@/api/browser'
import { taskNeedsReconciliation, taskProgressLabel, taskStageLabel, taskStateLabel } from './taskListPresentation'

const baseTask: TaskListItem = {
  id: 'task-1', type: 'OBDUMPER_EXPORT', dataSourceId: 'source-1', objectSummary: 'synthetic_db.synthetic_table',
  state: 'RUNNING', stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE', reconciliationRequired: false,
  nodeId: 'node-1', ownedByCurrentUser: true, submittedAt: '2026-07-31T03:00:00Z', updatedAt: '2026-07-31T03:01:00Z',
}

describe('任务列表可靠证据降级', () => {
  it('没有阶段和百分比证据时明确显示不可用', () => {
    expect(taskStateLabel(baseTask)).toBe('运行中')
    expect(taskStageLabel(baseTask)).toBe('暂无可靠阶段')
    expect(taskProgressLabel()).toBe('暂无可靠进度')
  })

  it('未知状态与核对标识都显示状态核对中且不伪造失败', () => {
    const unknown = { ...baseTask, state: 'UNEXPECTED_STATE' }
    expect(taskStateLabel(unknown)).toBe('状态核对中')
    expect(taskNeedsReconciliation(unknown)).toBe(true)
    expect(taskStageLabel({ ...baseTask, reconciliationRequired: true })).toBe('等待状态核对')
  })
})
