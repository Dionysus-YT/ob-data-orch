import { describe, expect, it } from 'vitest'
import { createBrowserApi } from './browser'
import { parseTaskProgressEvidence } from './taskProgress'

const progress = { phase: 'IMPORT_DATA', percent: 63.25, completedTasks: 7, runningTasks: 3, updatedAt: '2026-10-19T00:00:00Z', incomplete: false }
const evidence = { stageEvidence: 'RELIABLE', progressEvidence: 'PERCENT', progress }

describe('任务进度响应边界', () => {
  it('兼容旧服务未知进度并接受可靠零值及阶段完成事实', () => {
    expect(parseTaskProgressEvidence({ stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE' })).not.toHaveProperty('progress')
    expect(parseTaskProgressEvidence(evidence, 'OBLOADER_NORMAL_IMPORT').progress?.percent).toBe(63.25)
    expect(parseTaskProgressEvidence({ ...evidence, progress: { ...progress, percent: 0, completedTasks: 0, runningTasks: 0 } }).progress?.percent).toBe(0)
    expect(parseTaskProgressEvidence({ stageEvidence: 'RELIABLE', progressEvidence: 'UNAVAILABLE', progress: { phase: 'IMPORT_SCHEMA_COMPLETE', updatedAt: progress.updatedAt, incomplete: false } }).progress).not.toHaveProperty('percent')
  })
  it('拒绝跨工具、越界、部分证据、未知字段和不安全数量', () => {
    for (const value of [
      { ...evidence, stageEvidence: 'UNAVAILABLE' }, { ...evidence, progressEvidence: 'UNAVAILABLE' }, { ...evidence, progress: undefined },
      { stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE', progress: null },
      ...[-1, 100.01, 1.234, NaN, Infinity, '50'].map(percent => ({ ...evidence, progress: { ...progress, percent } })),
      { ...evidence, progress: { ...progress, phase: 'UNKNOWN' } }, { ...evidence, progress: { ...progress, updatedAt: 'invalid' } },
      { ...evidence, progress: { ...progress, completedTasks: Number.MAX_SAFE_INTEGER + 1 } }, { ...evidence, progress: { ...progress, runningTasks: undefined } },
      { ...evidence, progress: { ...progress, path: '/synthetic-path' } }, { ...evidence, progress: { ...progress, phase: 'IMPORT_SCHEMA_COMPLETE' } },
    ]) expect(() => parseTaskProgressEvidence(value, 'OBLOADER_NORMAL_IMPORT')).toThrow()
    expect(() => parseTaskProgressEvidence(evidence, 'OBDUMPER_EXPORT')).toThrow()
  })
  it('旁路仅接受未知进度，不借用导出或普通导入证据', () => {
    expect(parseTaskProgressEvidence({ stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE' }, 'OBLOADER_DIRECT_LOAD')).not.toHaveProperty('progress')
    expect(() => parseTaskProgressEvidence(evidence, 'OBLOADER_DIRECT_LOAD')).toThrow()
    expect(() => parseTaskProgressEvidence({ ...evidence, progress: { ...progress, phase: 'EXPORT_DATA' } }, 'OBLOADER_DIRECT_LOAD')).toThrow()
  })
  it('列表和执行详情解析同一摘要，100% 保留真实运行状态', async () => {
    const full = { ...evidence, progress: { ...progress, percent: 100 }, state: 'RUNNING', reconciliationRequired: false, updatedAt: progress.updatedAt }
    const item = { ...full, id: 'progress-task', type: 'OBLOADER_NORMAL_IMPORT', dataSourceId: 'source', nodeId: 'node', reconciliationRequired: false, ownedByCurrentUser: true, submittedAt: progress.updatedAt, updatedAt: progress.updatedAt, displayName: '合成进度任务', dataSourceDeleted: false, nodeDeleted: false, cancelRequested: false, availableActions: [] }
    const api = createBrowserApi({ csrfToken: () => 'synthetic-csrf', idempotencyKey: () => 'synthetic-key', fetcher: async path => Response.json(String(path).endsWith('/execution') ? { item: full } : { items: [item], totalPages: 1 }) })
    const list = await api.listTasks(undefined, 10), detail = await api.getTaskExecution('progress-task')
    expect(list.items[0]?.progress).toEqual(detail.progress)
    expect(detail.state).toBe('RUNNING'); expect(detail.progress?.percent).toBe(100)
  })
})
