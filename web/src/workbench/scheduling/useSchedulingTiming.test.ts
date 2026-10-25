import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import type { SchedulingTimingApi, SchedulingTimingPolicy } from '@/api/schedulingTiming'
import { useSchedulingTiming } from './useSchedulingTiming'

const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { for (const scope of scopes.splice(0)) scope.stop() })
function policy(heartbeat = 120, startup: number | null = null, polling: number | null = null, revision = 1): SchedulingTimingPolicy {
  return { heartbeatTimeoutSeconds: heartbeat, heartbeatTimeoutSource: heartbeat === 120 ? 'CONTROL_PLANE' : 'PLATFORM_SETTING', startupTimeoutSeconds: startup, pollingIntervalSeconds: polling, revision, candidateStrategy: 'EXPLICIT_SELECTION', applicationMode: 'HEARTBEAT_NOW_STARTUP_NEXT_CLAIM_POLL_NEXT_CHECK', restartRequired: false, ...(heartbeat === 120 && startup === null && polling === null ? {} : { modifiedBy: 'synthetic-admin', modifiedAt: '2026-10-24T02:00:00Z' }) }
}
function deferred<T>() { let resolve!: (value: T) => void; let reject!: (reason: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function setup(overrides: Partial<SchedulingTimingApi> = {}) {
  const api = { getSchedulingTiming: vi.fn<SchedulingTimingApi['getSchedulingTiming']>().mockResolvedValue(policy()), changeSchedulingTiming: vi.fn<SchedulingTimingApi['changeSchedulingTiming']>().mockResolvedValue({ revision: 2, changed: true, replayed: false }) }
  if (overrides.getSchedulingTiming) api.getSchedulingTiming.mockImplementation(overrides.getSchedulingTiming)
  if (overrides.changeSchedulingTiming) api.changeSchedulingTiming.mockImplementation(overrides.changeSchedulingTiming)
  const onSaved = vi.fn(), clearNotice = vi.fn(), scope = effectScope(); scopes.push(scope)
  let sequence = 0
  const model = scope.run(() => useSchedulingTiming(api, { onSaved, clearNotice, newKey: () => `synthetic-timing-${++sequence}` }))!
  return { api, model, scope, onSaved, clearNotice }
}
function edit(model: ReturnType<typeof useSchedulingTiming>) { model.edit(); model.form.heartbeatTimeoutSeconds = 90; model.form.startupTimeoutSeconds = 30; model.form.pollingIntervalSeconds = 5; model.form.reason = '合成时间策略验证'; model.reviewChange() }

describe('时间策略会话边界', () => {
  it('读取中不猜测，初次编辑保留空覆盖；无权清除全部事实', async () => {
    const pending = deferred<SchedulingTimingPolicy>(), { api, model } = setup({ getSchedulingTiming: vi.fn().mockReturnValueOnce(pending.promise).mockRejectedValueOnce({ status: 404 }) })
    const reading = model.read(); expect(model.policy.value).toBeUndefined(); expect(model.canEdit.value).toBe(false)
    pending.resolve(policy()); await reading; model.edit(); expect(model.form.startupTimeoutSeconds).toBeNull(); expect(model.form.pollingIntervalSeconds).toBeNull(); expect(model.form.heartbeatTimeoutSeconds).toBe(120)
    model.cancel(); await model.read(); expect(model.policy.value).toBeUndefined(); expect(model.forbidden.value).toBe(true); expect(api.changeSchedulingTiming).not.toHaveBeenCalled()
  })
  it('范围、清空已配置值与秘密反例不发送写入，字段修正后清错', async () => {
    const { api, model } = setup({ getSchedulingTiming: vi.fn().mockResolvedValue(policy(180, 60, 5, 2)) }); await model.read(); model.edit(); model.form.reason = '合成验证'
    for (const value of [null, 59, 3601, 1.5]) { model.form.heartbeatTimeoutSeconds = value; model.reviewChange(); expect(model.errors.heartbeatTimeoutSeconds).toBeTruthy() }
    model.form.heartbeatTimeoutSeconds = 240; expect(model.errors.heartbeatTimeoutSeconds).toBe(''); model.form.startupTimeoutSeconds = null; model.reviewChange(); expect(model.errors.startupTimeoutSeconds).toContain('不能')
    model.form.startupTimeoutSeconds = 60; model.form.pollingIntervalSeconds = 61; model.reviewChange(); expect(model.errors.pollingIntervalSeconds).toBeTruthy()
    model.form.pollingIntervalSeconds = 5; model.form.reason = 'password=synthetic-only'; model.reviewChange(); expect(model.errors.reason).toBeTruthy(); expect(api.changeSchedulingTiming).not.toHaveBeenCalled()
  })
  it('首次启动期限或改变频率需确认，提高单一心跳阈值仍可核对', async () => {
    const { api, model } = setup(); await model.read(); edit(model); expect(model.review.value?.payload.confirmed).toBe(true); await model.save(); expect(api.changeSchedulingTiming).not.toHaveBeenCalled()
    model.cancelReview(); model.form.heartbeatTimeoutSeconds = 240; model.form.startupTimeoutSeconds = null; model.form.pollingIntervalSeconds = null; model.reviewChange(); expect(model.review.value?.payload.confirmed).toBe(false)
  })
  it('冻结差异失效旧确认，重复提交方法只调用一次', async () => {
    const pending = deferred<{ revision: number; changed: boolean; replayed: boolean }>(), { api, model } = setup({ changeSchedulingTiming: vi.fn().mockReturnValue(pending.promise) }); await model.read(); edit(model)
    model.confirmed.value = true; model.form.startupTimeoutSeconds = 45; expect(model.review.value).toBeUndefined(); expect(model.confirmed.value).toBe(false)
    model.reviewChange(); model.confirmed.value = true; const saving = model.save(); await model.save(); expect(api.changeSchedulingTiming).toHaveBeenCalledTimes(1)
    pending.resolve({ revision: 2, changed: true, replayed: false }); await saving
  })
  it('未知保存结果同内容复用原键，取消仍必须重新读取', async () => {
    const { api, model } = setup(); api.changeSchedulingTiming.mockRejectedValue({ status: 503 }); await model.read(); edit(model); model.confirmed.value = true; await model.save()
    expect(model.unconfirmed.value).toBe(true); const firstKey = api.changeSchedulingTiming.mock.calls[0]?.[2]
    model.reviewChange(); model.confirmed.value = true; await model.save(); expect(api.changeSchedulingTiming.mock.calls[1]?.[2]).toBe(firstKey)
    model.cancel(); expect(model.stale.value).toBe(true); expect(model.canEdit.value).toBe(false); await model.read(); expect(model.canEdit.value).toBe(true)
  })
  it('版本冲突与基线确认变化不覆盖，撤权后不保留输入', async () => {
    for (const error of [{ code: 'REVISION_CONFLICT' }, { code: 'SCHEDULING_TIMING_CONFIRMATION_REQUIRED' }, { status: 403 }]) {
      const { api, model } = setup(); api.changeSchedulingTiming.mockRejectedValue(error); await model.read(); edit(model); model.confirmed.value = true; await model.save()
      expect(model.editing.value).toBe(false); expect(model.form.reason).toBe(''); expect(model.canEdit.value).toBe(false)
      if ('status' in error) expect(model.policy.value).toBeUndefined()
    }
  })
  it('历史回执后只显示当前复读结果，复读失败不保留旧事实', async () => {
    const { api, model, onSaved } = setup(); api.getSchedulingTiming.mockResolvedValueOnce(policy()).mockResolvedValueOnce(policy(240, 120, 10, 3)); api.changeSchedulingTiming.mockResolvedValueOnce({ revision: 2, changed: true, replayed: true })
    await model.read(); edit(model); model.confirmed.value = true; await model.save(); expect(model.policy.value?.revision).toBe(3); expect(onSaved).toHaveBeenCalledWith(expect.stringContaining('原时间设置回执'))
    model.edit(); model.form.heartbeatTimeoutSeconds = 300; model.form.reason = '合成再次设置'; model.reviewChange(); api.getSchedulingTiming.mockRejectedValueOnce({ status: 503 }); await model.save()
    expect(model.policy.value).toBeUndefined(); expect(model.failure.value).toContain('回执已确认')
  })
  it('A 到 B 再到 A 的迟到读取和卸载后的失败均不恢复旧会话', async () => {
    const old = deferred<SchedulingTimingPolicy>(), { api, model, scope } = setup(); api.getSchedulingTiming.mockReturnValueOnce(old.promise)
    const reading = model.read(); model.reset(); await model.read(); old.resolve(policy(900, 30, 1, 9)); await reading; expect(model.policy.value?.revision).toBe(1)
    const pending = deferred<SchedulingTimingPolicy>(); api.getSchedulingTiming.mockReturnValueOnce(pending.promise); const next = model.read(); scope.stop(); pending.reject({ status: 503 }); await next; expect(model.failure.value).toBe(''); expect(model.policy.value).toBeUndefined()
  })
  it('卸载后的写回执不触发复读、成功提示或新会话 finally', async () => {
    const pending = deferred<{ revision: number; changed: boolean; replayed: boolean }>(), { api, model, scope, onSaved } = setup({ changeSchedulingTiming: vi.fn().mockReturnValue(pending.promise) }); await model.read(); edit(model); model.confirmed.value = true; const saving = model.save(); scope.stop(); pending.resolve({ revision: 2, changed: true, replayed: false }); await saving
    expect(api.getSchedulingTiming).toHaveBeenCalledTimes(1); expect(onSaved).not.toHaveBeenCalled(); expect(model.saving.value).toBe(false)
  })
})
