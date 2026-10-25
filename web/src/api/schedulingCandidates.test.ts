import { describe, expect, it, vi } from 'vitest'
import { createSchedulingCandidatesApi, parseSchedulingCandidates } from './schedulingCandidates'
const initial = () => ({ order: 'EXISTING', revision: 1, selectionMode: 'EXPLICIT_SELECTION', applicationMode: 'NEXT_CANDIDATE_READ', restartRequired: false })
describe('候选策略 API', () => {
  it('严格枚举、元信息和显式选择，改回原顺序仍保留修改记录', () => {
    expect(parseSchedulingCandidates(initial()).order).toBe('EXISTING')
    const modified = { ...initial(), revision: 3, modifiedBy: 'synthetic-admin', modifiedAt: '2026-10-24T03:00:00Z' }
    expect(parseSchedulingCandidates(modified).modifiedBy).toBe('synthetic-admin')
    for (const value of [{ ...initial(), order: 'AUTO_SELECT' }, { ...initial(), selectionMode: 'AUTO' }, { ...initial(), extra: 'synthetic' }, { ...modified, modifiedAt: undefined }, { ...modified, modifiedBy: 'bad/actor' }, { ...modified, revision: 1 }, { ...initial(), revision: 2 }]) expect(() => parseSchedulingCandidates(value)).toThrow()
  })
  it('仅写白名单并校验回执版本，携带信号与原请求标识', async () => {
    const read = vi.fn(async () => ({ item: initial() })), write = vi.fn(async () => ({ item: { revision: 2, changed: true, replayed: false } })), controller = new AbortController()
    const api = createSchedulingCandidatesApi({ read, write }), payload = { order: 'ONLINE_FIRST' as const, reason: '合成原因', confirmed: true, extra: 'ignored' }
    await api.getSchedulingCandidates(controller.signal); await api.changeSchedulingCandidates(1, payload, 'synthetic-key', controller.signal)
    expect(write).toHaveBeenCalledWith('/api/v1/settings/scheduling/candidates', { order: 'ONLINE_FIRST', reason: '合成原因', confirmed: true }, 1, 'synthetic-key', controller.signal)
    write.mockResolvedValueOnce({ item: { revision: 3, changed: true, replayed: false } })
    await expect(api.changeSchedulingCandidates(1, payload, 'synthetic-key')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })
})
