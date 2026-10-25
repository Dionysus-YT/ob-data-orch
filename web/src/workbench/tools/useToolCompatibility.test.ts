import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import type { ToolCompatibility, ToolCompatibilityApi } from '@/api/toolCompatibility'
import { useToolCompatibility } from './useToolCompatibility'

const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()) })
function baseline(at = '2026-10-24T01:00:00Z'): ToolCompatibility {
  return { capturedAt: at, loadedAt: '2026-10-24T00:00:00Z', source: 'BUILT_IN_CATALOGS', tools: [
    { tool: 'OBDUMPER', supportedVersions: ['4.3.5-RELEASE'], defaultTargetVersion: '4.3.5-RELEASE', modules: [{ taskType: 'OBDUMPER_EXPORT', metadataSelection: 'TASK_CONFIGURATION', catalogs: [{ metadataVersion: 'obdumper-4.3.5-slice-v11-expert-parameters', usage: 'TASK_CONFIGURATION' }] }] },
    { tool: 'OBLOADER', supportedVersions: ['4.3.5-RELEASE'], defaultTargetVersion: '4.3.5-RELEASE', modules: [{ taskType: 'OBLOADER_NORMAL_IMPORT', metadataSelection: 'FIXED', catalogs: [{ metadataVersion: 'obloader-4.3.5-normal-v1', usage: 'TASK_CONFIGURATION' }] }, { taskType: 'OBLOADER_DIRECT_LOAD', metadataSelection: 'FIXED', catalogs: [{ metadataVersion: 'obloader-4.3.5-direct-v1', usage: 'TASK_CONFIGURATION' }] }] },
  ] }
}
function deferred<T>() { let resolve!: (value: T) => void, reject!: (reason: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function setup() { const scope = effectScope(); scopes.push(scope); const api = { getToolCompatibility: vi.fn<ToolCompatibilityApi['getToolCompatibility']>().mockResolvedValue(baseline()) }; const state = scope.run(() => useToolCompatibility(api))!; return { scope, api, state } }

describe('工具状态唯一读取会话', () => {
  it('方法入口防重并传递可取消信号', async () => {
    const { api, state } = setup(), pending = deferred<ToolCompatibility>(); api.getToolCompatibility.mockReturnValueOnce(pending.promise)
    const first = state.read(); await state.read(); expect(api.getToolCompatibility).toHaveBeenCalledTimes(1); expect(api.getToolCompatibility.mock.calls[0]?.[0]).toBeInstanceOf(AbortSignal)
    pending.resolve(baseline()); await first; expect(state.baseline.value).toEqual(baseline()); expect(state.loading.value).toBe(false)
  })
  it('撤权或失败撤下旧事实，安全反馈后可复读恢复', async () => {
    const { api, state } = setup(); await state.read(); api.getToolCompatibility.mockRejectedValueOnce({ status: 404, message: 'synthetic-private-only' }); const read = state.read(); expect(state.baseline.value).toBeUndefined(); await read
    expect(state.failure.value).toContain('系统管理员'); expect(state.failure.value).not.toContain('synthetic-private-only'); await state.read(); expect(state.baseline.value).toEqual(baseline()); expect(state.failure.value).toBe('')
    api.getToolCompatibility.mockRejectedValueOnce(new Error('synthetic-private-only')); await state.read(); expect(state.baseline.value).toBeUndefined(); expect(state.failure.value).toContain('保持未知')
  })
  it.each(['成功', '失败'] as const)('A→B→A 旧%s及 finally 不影响当前会话', async mode => {
    const { api, state } = setup(), old = deferred<ToolCompatibility>(), current = deferred<ToolCompatibility>()
    api.getToolCompatibility.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const a = state.read(), signal = api.getToolCompatibility.mock.calls[0]?.[0]; state.reset(); state.reset(); const b = state.read(); expect(signal?.aborted).toBe(true)
    if (mode === '成功') old.resolve(baseline('2026-10-24T01:00:00Z')); else old.reject({ status: 404 })
    await a; expect(state.baseline.value).toBeUndefined(); expect(state.loading.value).toBe(true); expect(state.failure.value).toBe(''); current.resolve(baseline('2026-10-24T02:00:00Z')); await b; expect(state.baseline.value?.capturedAt).toBe('2026-10-24T02:00:00Z')
  })
  it.each(['成功', '失败'] as const)('卸载后旧%s不恢复状态且不能重读', async mode => {
    const { api, state, scope } = setup(), pending = deferred<ToolCompatibility>(); api.getToolCompatibility.mockReturnValueOnce(pending.promise); const read = state.read(), signal = api.getToolCompatibility.mock.calls[0]?.[0]
    scope.stop(); expect(signal?.aborted).toBe(true); if (mode === '成功') pending.resolve(baseline()); else pending.reject({ status: 503 }); await read; await state.read(); expect(state.baseline.value).toBeUndefined(); expect(state.loading.value).toBe(false); expect(state.failure.value).toBe(''); expect(api.getToolCompatibility).toHaveBeenCalledTimes(1)
  })
})
