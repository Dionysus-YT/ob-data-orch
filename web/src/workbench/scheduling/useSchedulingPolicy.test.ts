import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import type { SchedulingPolicyApi, SchedulingPolicy, SchedulingPolicyReceipt } from '@/api/schedulingPolicy'
import { useSchedulingPolicy } from './useSchedulingPolicy'
const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => scopes.splice(0).forEach(scope => scope.stop()))
const policy = (maximum: number | null = 30, revision = 1): SchedulingPolicy => ({ maxConcurrentExecutions: maximum, revision, source: maximum === null ? 'UNCONFIGURED' : 'PLATFORM_SETTING', applicationMode: 'NEXT_CLAIM', restartRequired: false, modifiedBy: 'synthetic-admin', modifiedAt: '2026-10-24T00:00:00Z', usage: { visibility: 'AVAILABLE', heldExecutions: 0, runningExecutions: 0, observedAt: '2026-10-24T00:00:00Z' } })
function deferred<T>() { let resolve!: (v: T) => void; let reject!: (e: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function setup() {
  const api: SchedulingPolicyApi = { getSchedulingPolicy: vi.fn(async () => policy()), changeSchedulingPolicy: vi.fn(async () => ({ revision: 2, changed: true, replayed: false })) }
  const saved = vi.fn(), clear = vi.fn(), scope = effectScope(); scopes.push(scope)
  const model = scope.run(() => useSchedulingPolicy(api, { onSaved: saved, clearNotice: clear, newKey: vi.fn(() => crypto.randomUUID()) }))!
  return { api, saved, model, scope }
}
function edit(model: ReturnType<typeof setup>['model'], maximum = 7) { model.edit(); model.maximum.value = maximum; model.reason.value = '合成上限调整'; model.reviewChange() }
describe('平台并发设置的会话与事务', () => {
  it('未配置不预填并发值，首次设置必须确认收紧影响', async () => {
    const { model, api } = setup(); vi.mocked(api.getSchedulingPolicy).mockResolvedValueOnce(policy(null)); await model.read(); model.edit()
    expect(model.maximum.value).toBeNull(); model.maximum.value=8; model.reason.value='合成首次上限';model.reviewChange()
    expect(model.review.value?.payload.confirmed).toBe(true); await model.save();expect(api.changeSchedulingPolicy).not.toHaveBeenCalled()
    model.confirmed.value=true; await model.save();expect(api.changeSchedulingPolicy).toHaveBeenCalledTimes(1)
  })
  it('首次编辑不报错，修正已有错误立即更新字段反馈', async () => {
    const { model } = setup(); await model.read(); model.edit(); expect(model.reasonError.value).toBe(''); expect(model.maximumError.value).toBe('')
    model.maximum.value = 0; model.reviewChange(); expect(model.maximumError.value).toBeTruthy(); expect(model.reasonError.value).toBeTruthy()
    model.maximum.value = 7; model.reason.value = '修正后的合成原因'; expect(model.maximumError.value).toBe(''); expect(model.reasonError.value).toBe('')
  })
  it('加载失败或无权时没有猜测值及编辑资格', async () => {
    const { api, model } = setup(); vi.mocked(api.getSchedulingPolicy).mockRejectedValueOnce({ status: 503 }).mockRejectedValueOnce({ status: 404 })
    await model.read(); model.edit(); expect(model.policy.value).toBeUndefined(); expect(model.editing.value).toBe(false)
    await model.read(); expect(model.forbidden.value).toBe(true); expect(model.canEdit.value).toBe(false)
  })
  it('范围、整数、原因、同值和秘密反例不进入确认', async () => {
    const { model, api } = setup(); await model.read(); model.edit(); model.reason.value = '合成验证'; model.reviewChange(); expect(model.review.value).toBeUndefined()
    for (const maximum of [null, 0, 1025, 1.5]) { model.maximum.value = maximum; model.reviewChange(); expect(model.maximumError.value).toBeTruthy(); expect(model.review.value).toBeUndefined() }
    model.maximum.value = 7
    for (const reason of ['', '字'.repeat(201), 'password=synthetic-only']) { model.reason.value = reason; model.reviewChange(); expect(model.review.value).toBeUndefined() }
    expect(api.changeSchedulingPolicy).not.toHaveBeenCalled()
  })
  it('降低必须确认、内容改变失效旧确认，重复提交只发一次', async () => {
    const { model, api } = setup(); await model.read(); edit(model); await model.save(); expect(api.changeSchedulingPolicy).not.toHaveBeenCalled()
    model.confirmed.value = true; model.maximum.value = 8; expect(model.review.value).toBeUndefined(); expect(model.confirmed.value).toBe(false)
    model.reviewChange(); model.confirmed.value = true
    const pending = deferred<SchedulingPolicyReceipt>(); vi.mocked(api.changeSchedulingPolicy).mockReturnValueOnce(pending.promise)
    const write = model.save(); await model.save(); expect(api.changeSchedulingPolicy).toHaveBeenCalledTimes(1)
    pending.resolve({ revision: 2, changed: true, replayed: false }); await write; expect(model.editing.value).toBe(false)
  })
  it('未知失败复用同键同载荷，冲突后阻断再次编辑', async () => {
    const { model, api } = setup(); await model.read(); edit(model, 90)
    const key = model.review.value?.key
    vi.mocked(api.changeSchedulingPolicy).mockRejectedValueOnce({ status: 503 }).mockRejectedValueOnce({ code: 'REVISION_CONFLICT', status: 412 })
    await model.save(); expect(model.reason.value).toBe('合成上限调整'); expect(model.unconfirmed.value).toBe(true); model.reviewChange(); expect(model.review.value?.key).toBe(key)
    await model.save(); expect(model.stale.value).toBe(true); expect(model.canEdit.value).toBe(false); expect(model.editing.value).toBe(false)
  })
  it('结果不确定后取消只结束编辑，重新读取才能确认当前策略', async () => {
    const { model, api } = setup(); await model.read(); edit(model, 90)
    vi.mocked(api.changeSchedulingPolicy).mockRejectedValueOnce({ status: 503 }); await model.save(); model.cancel()
    expect(model.unconfirmed.value).toBe(true); expect(model.stale.value).toBe(true); expect(model.canEdit.value).toBe(false); expect(model.failure.value).toContain('尚未核对')
    vi.mocked(api.getSchedulingPolicy).mockResolvedValueOnce(policy(90, 2)); await model.read()
    expect(model.unconfirmed.value).toBe(false); expect(model.policy.value?.maxConcurrentExecutions).toBe(90); expect(model.canEdit.value).toBe(true)
  })
  it('写回执后重新读取当前策略，历史回执不覆盖新版本', async () => {
    const { model, api, saved } = setup(); await model.read(); edit(model, 90)
    vi.mocked(api.changeSchedulingPolicy).mockResolvedValueOnce({ revision: 2, changed: true, replayed: true }); vi.mocked(api.getSchedulingPolicy).mockResolvedValueOnce(policy(120, 3))
    await model.save(); expect(model.policy.value?.maxConcurrentExecutions).toBe(120); expect(saved).toHaveBeenCalledWith(expect.stringContaining('原请求'))
  })
  it('已确认保存但重新读取失败时清除旧值并保留明确回执说明', async () => {
    const { model, api, saved } = setup(); await model.read(); edit(model, 90); vi.mocked(api.getSchedulingPolicy).mockRejectedValueOnce({ status: 503 }); await model.save()
    expect(model.policy.value).toBeUndefined(); expect(model.failure.value).toContain('保存回执已确认'); expect(saved).not.toHaveBeenCalled()
  })
  it('A → B → A 读取、错误和 finally 不覆盖当前读取', async () => {
    const { api, model } = setup(), a = deferred<SchedulingPolicy>(), b = deferred<SchedulingPolicy>(), c = deferred<SchedulingPolicy>()
    vi.mocked(api.getSchedulingPolicy).mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise).mockReturnValueOnce(c.promise)
    const first = model.read(); model.reset(); const second = model.read(); model.reset(); const third = model.read()
    a.resolve(policy(365, 8)); b.reject({ status: 404 }); await first; await second; expect(model.loading.value).toBe(true); expect(model.forbidden.value).toBe(false)
    c.resolve(policy()); await third; expect(model.policy.value?.maxConcurrentExecutions).toBe(30)
  })
  it('卸载和会话重置中止请求，迟到成功不发布提示或恢复编辑', async () => {
    const { api, model, saved, scope } = setup(); await model.read(); edit(model, 90)
    const pending = deferred<SchedulingPolicyReceipt>(); vi.mocked(api.changeSchedulingPolicy).mockReturnValueOnce(pending.promise)
    const write = model.save(), signal = vi.mocked(api.changeSchedulingPolicy).mock.calls[0]?.[3]
    scope.stop(); expect(signal?.aborted).toBe(true); pending.resolve({ revision: 2, changed: true, replayed: false }); await write
    expect(saved).not.toHaveBeenCalled(); expect(model.policy.value).toBeUndefined(); expect(model.reason.value).toBe('')
  })
})
