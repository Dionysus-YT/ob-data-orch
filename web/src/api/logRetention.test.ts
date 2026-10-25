import { describe, expect, it, vi } from 'vitest'
import { createLogRetentionApi, parseLogRetention } from './logRetention'
const policy = { retentionDays: 30, revision: 1, source: 'BUILT_IN_DEFAULT', applicationMode: 'NEXT_BATCH', restartRequired: false }
describe('普通日志策略严格解码与传输', () => {
  it('缺省来源不伪造修改记录，平台覆盖要求完整元信息', () => {
    expect(parseLogRetention(policy).modifiedAt).toBeUndefined()
    expect(parseLogRetention({ ...policy, retentionDays: 90, source: 'PLATFORM_SETTING', modifiedBy: 'synthetic-admin', modifiedAt: '2026-10-23T00:00:00Z' }).retentionDays).toBe(90)
    for (const bad of [{ ...policy, retentionDays: 7.5 }, { ...policy, retentionDays: 366 }, { ...policy, retentionDays: 7 }, { ...policy, revision: 0 }, { ...policy, source: 'UNKNOWN' }, { ...policy, source: 'PLATFORM_SETTING' }, { ...policy, passwordHash: 'synthetic-only' }, { ...policy, modifiedAt: '2026-10-23T00:00:00Z' }, { ...policy, restartRequired: true }]) expect(() => parseLogRetention(bad)).toThrow()
  })
  it('复用同一版本、幂等键和取消信号；只接受原事务回执', async () => {
    const read = vi.fn(async () => ({ item: policy })), write = vi.fn(async () => ({ item: { revision: 2, changed: true, replayed: true } }))
    const api = createLogRetentionApi({ read, write }), controller = new AbortController(), body = { retentionDays: 7, reason: '合成验证', confirmed: true }
    await api.getLogRetention(controller.signal)
    await expect(api.changeLogRetention(1, body, 'synthetic-key', controller.signal)).resolves.toEqual({ revision: 2, changed: true, replayed: true })
    expect(write).toHaveBeenCalledWith('/api/v1/settings/log-retention', body, 1, 'synthetic-key', controller.signal)
    write.mockResolvedValueOnce({ item: { revision: 3, changed: true, replayed: false } })
    await expect(api.changeLogRetention(1, body, 'synthetic-key')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })
})
