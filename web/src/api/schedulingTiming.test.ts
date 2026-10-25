import { describe, expect, it, vi } from 'vitest'
import { createSchedulingTimingApi, parseSchedulingTiming } from './schedulingTiming'
const base = () => ({ heartbeatTimeoutSeconds: 120, heartbeatTimeoutSource: 'CONTROL_PLANE', startupTimeoutSeconds: null, pollingIntervalSeconds: null, candidateStrategy: 'EXPLICIT_SELECTION', revision: 1, applicationMode: 'HEARTBEAT_NOW_STARTUP_NEXT_CLAIM_POLL_NEXT_CHECK', restartRequired: false })
describe('调度时间 API', () => {
  it('未配置不是默认启动期限或 Agent 周期，损坏与未知策略失败关闭', () => {
    expect(parseSchedulingTiming(base()).startupTimeoutSeconds).toBeNull()
    for (const bad of [{ ...base(), startupTimeoutSeconds: 0 }, { ...base(), pollingIntervalSeconds: undefined }, { ...base(), candidateStrategy: 'AUTO' }, { ...base(), heartbeatTimeoutSource: 'UNKNOWN' }, { ...base(), heartbeatTimeoutSeconds: 0 }, { ...base(), revision: 0 }, { ...base(), secret: 'synthetic-only' }]) expect(() => parseSchedulingTiming(bad)).toThrow()
  })
  it('覆盖与元信息成组，数值、单位和来源不能混淆', () => {
    const configured = { ...base(), heartbeatTimeoutSource: 'PLATFORM_SETTING', heartbeatTimeoutSeconds: 60, startupTimeoutSeconds: 30, pollingIntervalSeconds: 5, modifiedBy: 'synthetic-admin', modifiedAt: '2026-10-24T02:00:00Z', revision: 2 }
    expect(parseSchedulingTiming(configured).pollingIntervalSeconds).toBe(5)
    for (const bad of [{ ...configured, modifiedAt: undefined }, { ...configured, heartbeatTimeoutSeconds: 59 }, { ...configured, startupTimeoutSeconds: 3601 }, { ...configured, pollingIntervalSeconds: 61 }, { ...configured, modifiedBy: 'unsafe/actor' }, { ...configured, modifiedAt: 'unknown' }]) expect(() => parseSchedulingTiming(bad)).toThrow()
    expect(parseSchedulingTiming({ ...configured, heartbeatTimeoutSource: 'CONTROL_PLANE', heartbeatTimeoutSeconds: 45 }).heartbeatTimeoutSeconds).toBe(45)
  })
  it('仅传明确的三项时间与原因确认，携带版本、幂等和取消信号', async () => {
    const read = vi.fn(async () => ({ item: base() })), write = vi.fn(async () => ({ item: { revision: 2, changed: true, replayed: false } })), controller = new AbortController()
    const api = createSchedulingTimingApi({ read, write }), payload = { heartbeatTimeoutSeconds: 120, startupTimeoutSeconds: 30, pollingIntervalSeconds: null, reason: '合成时间调整', confirmed: true }
    await api.getSchedulingTiming(controller.signal); await api.changeSchedulingTiming(1, payload, 'synthetic-key', controller.signal)
    expect(write).toHaveBeenCalledWith('/api/v1/settings/scheduling/timing', payload, 1, 'synthetic-key', controller.signal)
    write.mockResolvedValueOnce({ item: { revision: 3, changed: true, replayed: false } }); await expect(api.changeSchedulingTiming(1, payload, 'synthetic-key')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })
})
