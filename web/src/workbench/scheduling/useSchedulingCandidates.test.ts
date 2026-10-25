import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import type { CandidateOrder, SchedulingCandidatesApi, SchedulingCandidatesPolicy } from '@/api/schedulingCandidates'
import { useSchedulingCandidates } from './useSchedulingCandidates'
const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { for (const scope of scopes.splice(0)) scope.stop() })
function policy(order: CandidateOrder = 'EXISTING', revision = 1): SchedulingCandidatesPolicy { return { order, revision, selectionMode: 'EXPLICIT_SELECTION', applicationMode: 'NEXT_CANDIDATE_READ', restartRequired: false, ...(revision === 1 ? {} : { modifiedBy: 'synthetic-admin', modifiedAt: '2026-10-24T03:00:00Z' }) } }
function deferred<T>() { let resolve!: (value: T) => void, reject!: (reason: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function setup() {
  const scope = effectScope(); scopes.push(scope)
  const api = { getSchedulingCandidates: vi.fn<SchedulingCandidatesApi['getSchedulingCandidates']>().mockResolvedValue(policy()), changeSchedulingCandidates: vi.fn<SchedulingCandidatesApi['changeSchedulingCandidates']>().mockResolvedValue({ revision: 2, changed: true, replayed: false }) }, onSaved = vi.fn()
  const model = scope.run(() => useSchedulingCandidates(api, { onSaved, clearNotice: vi.fn(), newKey: () => 'synthetic-candidate-key' }))!
  return { scope, api, model, onSaved }
}
function edit(model: ReturnType<typeof useSchedulingCandidates>) { model.edit(); model.form.order = 'ONLINE_FIRST'; model.form.reason = '合成候选变化'; model.reviewChange() }
describe('候选策略独立会话', () => {
  it('加载后编辑，原因校验和确认后保存并复读', async () => {
    const { model, api, onSaved } = setup(); await model.read(); model.edit(); model.form.order = 'ONLINE_FIRST'; model.reviewChange(); expect(model.fieldError.value).toBeTruthy()
    model.form.reason = '合成候选变化'; expect(model.fieldError.value).toBe(''); model.reviewChange(); await model.save(); expect(api.changeSchedulingCandidates).not.toHaveBeenCalled()
    model.confirmed.value = true; api.getSchedulingCandidates.mockResolvedValueOnce(policy('ONLINE_FIRST', 2)); await model.save()
    expect(model.policy.value?.order).toBe('ONLINE_FIRST'); expect(model.dirty.value).toBe(false); expect(onSaved).toHaveBeenCalledWith(expect.stringContaining('下次读取'))
  })
  it('方法入口防重，未知失败重试沿用请求标识', async () => {
    const { model, api } = setup(); await model.read(); edit(model); model.confirmed.value = true
    const pending = deferred<{ revision: number; changed: boolean; replayed: boolean }>(); api.changeSchedulingCandidates.mockReturnValueOnce(pending.promise)
    const first = model.save(); await model.save(); expect(api.changeSchedulingCandidates).toHaveBeenCalledTimes(1); pending.reject({ status: 503 }); await first
    expect(model.unconfirmed.value).toBe(true); expect(model.form.reason).toBe('合成候选变化'); model.reviewChange(); model.confirmed.value = true; await model.save()
    expect(api.changeSchedulingCandidates.mock.calls[1]?.[2]).toBe(api.changeSchedulingCandidates.mock.calls[0]?.[2])
  })
  it('版本冲突、撤权及确认变化均阻断旧输入重交', async () => {
    for (const error of [{ code: 'REVISION_CONFLICT' }, { status: 404 }, { code: 'CANDIDATE_ORDER_CONFIRMATION_REQUIRED' }]) {
      const { model, api } = setup(); await model.read(); edit(model); model.confirmed.value = true; api.changeSchedulingCandidates.mockRejectedValueOnce(error); await model.save()
      expect(model.editing.value).toBe(false); expect(model.canEdit.value).toBe(false); expect(model.form.reason).toBe('')
    }
  })
  it('历史回执仅确认原事务，显示重新读取的较新策略', async () => {
    const { model, api, onSaved } = setup(); api.getSchedulingCandidates.mockResolvedValueOnce(policy()).mockResolvedValueOnce(policy('EXISTING', 3)); await model.read(); edit(model); model.confirmed.value = true
    api.changeSchedulingCandidates.mockResolvedValueOnce({ revision: 2, changed: true, replayed: true }); await model.save()
    expect(model.policy.value?.revision).toBe(3); expect(onSaved).toHaveBeenCalledWith(expect.stringContaining('原候选顺序回执'))
  })
  it('回执确认后读取失败不把旧事实称为当前', async () => {
    const { model, api } = setup(); await model.read(); edit(model); model.confirmed.value = true; api.getSchedulingCandidates.mockRejectedValueOnce({ status: 503 }); await model.save()
    expect(model.policy.value).toBeUndefined(); expect(model.failure.value).toContain('回执已确认'); expect(model.canEdit.value).toBe(false)
  })
  it('会话 A 到 B 再回 A 隔离旧成功、失败与 finally', async () => {
    const { model, api } = setup(), old = deferred<SchedulingCandidatesPolicy>(), middle = deferred<SchedulingCandidatesPolicy>()
    api.getSchedulingCandidates.mockReturnValueOnce(old.promise).mockReturnValueOnce(middle.promise).mockResolvedValueOnce(policy('ONLINE_FIRST', 2))
    const first = model.read(); model.reset(); const second = model.read(); model.reset(); await model.read(); old.resolve(policy()); middle.reject({ status: 404 }); await Promise.all([first, second])
    expect(model.policy.value?.revision).toBe(2); expect(model.failure.value).toBe(''); expect(model.loading.value).toBe(false)
  })
  it('卸载取消写等待，迟到响应不得恢复策略或成功提示', async () => {
    const { model, api, scope, onSaved } = setup(); await model.read(); edit(model); model.confirmed.value = true
    const pending = deferred<{ revision: number; changed: boolean; replayed: boolean }>(); api.changeSchedulingCandidates.mockReturnValueOnce(pending.promise); const save = model.save(); const signal = api.changeSchedulingCandidates.mock.calls[0]?.[3]
    scope.stop(); expect(signal?.aborted).toBe(true); pending.resolve({ revision: 2, changed: true, replayed: false }); await save
    expect(model.policy.value).toBeUndefined(); expect(onSaved).not.toHaveBeenCalled()
  })
  it('核对后改字段使旧确认失效，未知保存取消后须重新读取', async () => {
    const { model, api } = setup(); await model.read(); edit(model); model.confirmed.value = true; model.form.reason = '合成另一原因'; expect(model.review.value).toBeUndefined(); await model.save(); expect(api.changeSchedulingCandidates).not.toHaveBeenCalled()
    model.reviewChange(); model.confirmed.value = true; api.changeSchedulingCandidates.mockRejectedValueOnce({ status: 503 }); await model.save(); model.cancel(); expect(model.canEdit.value).toBe(false); expect(model.failure.value).toContain('尚未核对')
  })
})
