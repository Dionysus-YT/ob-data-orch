import { describe, expect, it } from 'vitest'
import { overviewFixture } from '../../../tests/fixtures/overview'
import { parseTaskListRoute, taskListApiFilter, taskListConditionChips, taskListDateRangeError } from '@/workbench/tasks/taskListQuery'
import { taskProgressPresentation } from '@/workbench/tasks/taskProgressPresentation'
import { overviewBucketLabel, overviewMetricLink, overviewTime } from './overviewPresentation'

describe('首页下钻与时间口径', () => {
  it('成功失败指标按终态时间跳转，筛选保持 UTC 精度并可以单独清除', () => {
    const snapshot = overviewFixture(new Date('2026-10-23T08:30:00Z'))
    expect(overviewMetricLink('todayFailed', snapshot)).toEqual({ name: 'tasks', query: { state: 'FAILED', finishedFrom: snapshot.todayFrom, finishedTo: snapshot.todayTo } })
    const state = parseTaskListRoute({ finishedFrom: '2026-10-23T08:30:00+08:00', finishedTo: '2026-10-23T09:30:00+08:00' })
    expect(taskListApiFilter(state)).toMatchObject({ finishedFrom: '2026-10-23T00:30:00.000Z', finishedTo: '2026-10-23T01:30:00.000Z' })
    expect(taskListConditionChips(state)[0]).toMatchObject({ key: 'finished', clear: { finishedFrom: '', finishedTo: '' } })
    expect(taskListConditionChips(state)[0]?.label).toContain('UTC')
    expect(taskListDateRangeError({ ...state, finishedTo: state.finishedFrom })).not.toBe('')
  })
  it('趋势明细按真实分桶起止展示，部分桶不补齐整点或整日', () => {
    expect(overviewBucketLabel({ from: '2026-10-22T08:30:00Z', to: '2026-10-22T09:00:00Z', total: 1, succeeded: 0, failed: 0 }, '24h')).toBe('10-22 08:30 – 09:00')
    expect(overviewBucketLabel({ from: '2026-10-22T00:00:00Z', to: '2026-10-23T00:00:00Z', total: 1, succeeded: 0, failed: 0 }, '7d')).toBe('10-22')
    expect(overviewBucketLabel({ from: '2026-10-23T00:00:00Z', to: '2026-10-23T08:30:00Z', total: 1, succeeded: 0, failed: 0 }, '7d')).toBe('10-23 至 08:30')
  })
  it('首页时间与进度说明使用同一声明时区，不把百分比转换为成功', () => {
    expect(overviewTime('2026-10-23T08:30:00+08:00')).toBe('2026-10-23 00:30:00')
    const progress = taskProgressPresentation({ phase: 'IMPORT_DATA', percent: 100, completedTasks: 2, runningTasks: 0, updatedAt: '2026-10-23T08:30:00Z', incomplete: false }, 'RUNNING', false, 'OBLOADER_NORMAL_IMPORT', 'UTC')
    expect(progress.description).toContain('2026-10-23 08:30:00 UTC')
    expect(progress.description).toContain('等待收尾与结果核验')
  })
})
