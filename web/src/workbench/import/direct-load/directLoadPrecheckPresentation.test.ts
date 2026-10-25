import { describe, expect, it } from 'vitest'
import type { DirectLoadCheck } from '@/api/directLoad'
import { directLoadCheckItems, directLoadPrecheckOverview, directLoadPrecheckProgress } from './directLoadPrecheckPresentation'

function report(): DirectLoadCheck {
  return {
    id: 'synthetic-check', draftId: 'synthetic-draft', draftRevision: 1, configFingerprint: 'a'.repeat(64),
    checkKind: 'DIRECT_LOAD_PREFLIGHT', status: 'SUCCEEDED', integrityStatus: 'COMPLETE',
    validUntil: '2099-01-01T00:00:00Z', executionEnabled: false,
    results: directLoadCheckItems.map(item => ({ check: item.check, status: 'PASSED', evidenceCode: 'DIRECT_LOAD_VERIFIED' })),
  }
}

describe('旁路预检查总览与步骤进度', () => {
  it('未开始与等待保持信息提示，不继承旧成功或虚构检查进度', () => {
    expect(directLoadPrecheckOverview(undefined, false, false).type).toBe('info')
    expect(directLoadPrecheckProgress(undefined, false)).toBe(0)
    for (const status of ['PENDING', 'LEASED'] as const) {
      const check = { ...report(), status }
      expect(directLoadPrecheckOverview(check, false, false).type).toBe('info')
      expect(directLoadPrecheckProgress(check, false)).toBe(0)
    }
  })
  it('失败和无法确认均为错误总览，百分比只计固定九项中的通过证据', () => {
    for (const status of ['FAILED', 'UNKNOWN'] as const) {
      const check = report()
      check.status = 'FAILED'
      check.results[0] = { ...check.results[0]!, status }
      expect(directLoadPrecheckOverview(check, false, false)).toMatchObject({ type: 'error' })
      expect(directLoadPrecheckProgress(check, false)).toBe(89)
    }
  })
  it('失效与时间过期优先于旧通过结果，并撤销检查进度', () => {
    expect(directLoadPrecheckOverview(report(), true, false).type).toBe('warning')
    expect(directLoadPrecheckProgress(report(), true)).toBe(0)
    for (const status of ['INVALIDATED', 'EXPIRED'] as const) {
      const check = { ...report(), status }
      expect(directLoadPrecheckOverview(check, false, false).type).toBe('warning')
      expect(directLoadPrecheckProgress(check, false)).toBe(0)
    }
  })
  it('缺项、重复项、额外项和不完整证据不能补足九项通过结论', () => {
    const check = report()
    check.results[0] = { ...check.results[1]! }
    check.results.push({ check: 'SYNTHETIC_OTHER', status: 'PASSED', evidenceCode: 'DIRECT_LOAD_VERIFIED' })
    expect(directLoadPrecheckProgress(check, false)).toBe(89)
    expect(directLoadPrecheckOverview(check, false, false).type).toBe('error')
    expect(directLoadPrecheckOverview({ ...report(), integrityStatus: 'INCOMPLETE' }, false, false).type).toBe('error')
  })
  it('全部证据通过只表示预检成功，不要求或授予独立执行许可', () => {
    const check = report()
    expect(check.executionEnabled).toBe(false)
    expect(directLoadPrecheckOverview(check, false, false)).toEqual({ type: 'success', message: '当前预检查通过' })
    expect(directLoadPrecheckProgress(check, false)).toBe(100)
  })
})
