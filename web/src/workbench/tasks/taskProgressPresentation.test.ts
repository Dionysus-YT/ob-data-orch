import { describe, expect, it } from 'vitest'
import type { TaskProgress } from '@/api/browser'
import { taskProgressPresentation } from './taskProgressPresentation'

const progress: TaskProgress = { phase: 'IMPORT_DATA', percent: 63.25, completedTasks: 7, runningTasks: 3, updatedAt: '2026-10-19T00:00:00Z', incomplete: false }

describe('列表详情共用的任务进度表达', () => {
  it('未知不造数值，成功按真实终态表达完成而非凭空补 100%', () => {
    expect(taskProgressPresentation(undefined, 'RUNNING')).toMatchObject({ text: '—', percent: undefined, phase: '' })
    expect(taskProgressPresentation(undefined, 'SUCCEEDED')).toMatchObject({ text: '已完成', percent: undefined })
    expect(taskProgressPresentation(progress, 'SUCCEEDED')).toMatchObject({ text: '已完成', percent: undefined })
    expect(taskProgressPresentation({ ...progress, percent: 0 }, 'RUNNING')).toMatchObject({ text: '0%', percent: 0 })
    expect(taskProgressPresentation(progress, 'RUNNING', false, 'OBDUMPER_EXPORT')).toMatchObject({ text: '—', phase: '' })
    expect(taskProgressPresentation(progress, 'RUNNING', false, 'OBLOADER_DIRECT_LOAD')).toMatchObject({ text: '—', phase: '', percent: undefined })
    expect(taskProgressPresentation({ ...progress, phase: 'EXPORT_DATA' }, 'RUNNING', false, 'OBLOADER_DIRECT_LOAD')).toMatchObject({ text: '—', phase: '', percent: undefined })
  })
  it('数据达到 100% 仍表达等待核验，失败取消保留最后值并说明历史属性', () => {
    const complete = taskProgressPresentation({ ...progress, percent: 100 }, 'RUNNING')
    expect(complete.percent).toBe(100); expect(complete.text).not.toBe('已完成'); expect(complete.description).toContain('等待收尾与结果核验')
    expect(complete.description).toContain('不表示记录数、字节数或数据库结果')
    for (const state of ['FAILED', 'CANCELLED']) expect(taskProgressPresentation(progress, state)).toMatchObject({ percent: 63.25, historical: true, text: '63.25%' })
    expect(taskProgressPresentation({ ...progress, percent: 100 }, 'CANCELLED')).toMatchObject({ percent: 100, status: 'normal', historical: true, text: '100%' })
    expect(taskProgressPresentation({ ...progress, incomplete: true }, 'RUNNING').historical).toBe(true)
    expect(taskProgressPresentation(progress, 'RUNNING', true).historical).toBe(true)
    expect(taskProgressPresentation(progress, 'UNKNOWN_STATE')).toMatchObject({ percent: 63.25, historical: true, status: 'normal' })
    expect(taskProgressPresentation(progress, undefined)).toMatchObject({ percent: 63.25, historical: false })
    for (const state of ['constructor', '__proto__']) expect(taskProgressPresentation(progress, state)).toMatchObject({ percent: 63.25, historical: true, status: 'normal' })
  })
  it('阶段证据不按阶段数均分百分比，工具子任务与数据库结果分别表达', () => {
    const stage = taskProgressPresentation({ phase: 'EXPORT_SCHEMA_COMPLETE', updatedAt: progress.updatedAt, incomplete: false }, 'RUNNING')
    expect(stage).toMatchObject({ text: '结构导出完成', percent: undefined })
    expect(stage.description).toContain('工具仅报告阶段，尚无可靠百分比')
    expect(stage.description).not.toContain('。。')
    expect(taskProgressPresentation(progress, 'RUNNING').description).toContain('不表示记录数、字节数或数据库结果')
  })
})
