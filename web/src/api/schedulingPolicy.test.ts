import { describe, expect, it, vi } from 'vitest'
import { createSchedulingPolicyApi, parseSchedulingPolicy } from './schedulingPolicy'
const base = () => ({ maxConcurrentExecutions: null, revision: 1, source: 'UNCONFIGURED', applicationMode: 'NEXT_CLAIM', restartRequired: false, usage: { visibility: 'RESTRICTED', heldExecutions: null, runningExecutions: null, observedAt: '2026-10-24T00:00:00Z' } })
describe('调度策略安全投影', () => {
  it('初次未配置不是猜测值，受限数量不能返回 0 或局部数量', () => {
    expect(parseSchedulingPolicy(base()).maxConcurrentExecutions).toBeNull()
    for (const bad of [{ ...base(), maxConcurrentExecutions: 1 }, { ...base(), revision: 0 }, { ...base(), secret: 'synthetic-only' }, { ...base(), source: 'UNKNOWN' }, { ...base(), usage: { ...base().usage, heldExecutions: 0 } }, { ...base(), usage: { ...base().usage, visibility: 'AVAILABLE' } }]) expect(() => parseSchedulingPolicy(bad)).toThrow()
  })
  it('配置元信息和全局采样完整，字段越界、运行数超过占用均拒绝', () => {
    const configured = { ...base(), maxConcurrentExecutions: 8, source: 'PLATFORM_SETTING', modifiedBy: 'synthetic-admin', modifiedAt: '2026-10-24T00:00:00Z', usage: { visibility: 'AVAILABLE', heldExecutions: 3, runningExecutions: 2, observedAt: '2026-10-24T00:00:00Z' } }
    expect(parseSchedulingPolicy(configured).usage.heldExecutions).toBe(3)
    for (const bad of [{ ...configured, maxConcurrentExecutions: 0 }, { ...configured, maxConcurrentExecutions: 1025 }, { ...configured, maxConcurrentExecutions: 1.5 }, { ...configured, modifiedAt: undefined }, { ...configured, usage: { ...configured.usage, runningExecutions: 4 } }]) expect(() => parseSchedulingPolicy(bad)).toThrow()
  })
  it('写请求只传业务字段、版本与幂等键，回执后保留单独复读边界', async () => {
    const read = vi.fn(async () => ({ item: base() })), write = vi.fn(async () => ({ item: { revision: 2, changed: true, replayed: false } }))
    const api = createSchedulingPolicyApi({ read, write }), controller = new AbortController(), body = { maxConcurrentExecutions: 8, reason: '合成并发上限', confirmed: true }
    await api.getSchedulingPolicy(controller.signal); await api.changeSchedulingPolicy(1, body, 'synthetic-key', controller.signal)
    expect(write).toHaveBeenCalledWith('/api/v1/settings/scheduling', body, 1, 'synthetic-key', controller.signal)
    write.mockResolvedValueOnce({ item: { revision: 3, changed: true, replayed: false } }); await expect(api.changeSchedulingPolicy(1, body, 'synthetic-key')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })
})
