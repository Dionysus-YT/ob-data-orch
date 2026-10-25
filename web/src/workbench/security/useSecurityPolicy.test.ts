import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import type { SecurityPolicy, SecurityPolicyApi } from '@/api/securityPolicy'
import { useSecurityPolicy } from './useSecurityPolicy'

const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()) })
function policy(id = 'synthetic-key-v1'): SecurityPolicy { return { capturedAt: '2026-10-24T01:00:00Z', loadedAt: '2026-10-24T00:00:00Z', source: 'STARTUP_CONFIGURATION', builtInRedaction: true, credentialFormat: 'credential-aes-gcm-v1', credentialKeyId: id, rootKeyStatus: 'LOADED', rootKeySource: 'LOCAL_PROTECTED_FILE', sensitiveCommandView: 'NOT_AVAILABLE', productionConfirmation: 'TASK_SUBMISSION_RULES', auditRetentionDays: null, auditAutomaticCleanup: false, realExecutionEnabled: false, normalImportExecutionEnabled: false, directLoadProbeEnabled: false, directLoadExecutionEnabled: false } }
function deferred<T>() { let resolve!: (value: T) => void, reject!: (reason: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function setup() { const scope = effectScope(); scopes.push(scope); const api = { getSecurityPolicy: vi.fn<SecurityPolicyApi['getSecurityPolicy']>().mockResolvedValue(policy()) }; const state = scope.run(() => useSecurityPolicy(api))!; return { scope, api, state } }

describe('安全状态唯一读取会话', () => {
  it('方法防重且读取期间传递取消信号', async () => {
    const { api, state } = setup(), pending = deferred<SecurityPolicy>(); api.getSecurityPolicy.mockReturnValueOnce(pending.promise)
    const first = state.read(); await state.read(); expect(api.getSecurityPolicy).toHaveBeenCalledTimes(1); expect(api.getSecurityPolicy.mock.calls[0]?.[0]).toBeInstanceOf(AbortSignal)
    pending.resolve(policy()); await first; expect(state.policy.value?.rootKeyStatus).toBe('LOADED'); expect(state.loading.value).toBe(false)
  })
  it('权限与读取失败清除旧状态，安全提示后允许恢复', async () => {
    const { api, state } = setup(); await state.read(); api.getSecurityPolicy.mockRejectedValueOnce({ status: 404, message: 'synthetic-private-only' }); const read = state.read(); expect(state.policy.value).toBeUndefined(); await read
    expect(state.failure.value).toContain('系统管理员'); expect(state.failure.value).not.toContain('synthetic-private-only'); await state.read(); expect(state.policy.value).toEqual(policy()); expect(state.failure.value).toBe('')
    api.getSecurityPolicy.mockRejectedValueOnce(new Error('synthetic-private-only')); await state.read(); expect(state.policy.value).toBeUndefined(); expect(state.failure.value).toContain('保持未知')
  })
  it.each(['成功', '失败'] as const)('A→B→A 的旧%s及 finally 不改变当前读取', async mode => {
    const { api, state } = setup(), old = deferred<SecurityPolicy>(), current = deferred<SecurityPolicy>()
    api.getSecurityPolicy.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const a = state.read(), signal = api.getSecurityPolicy.mock.calls[0]?.[0]; state.reset(); state.reset(); const b = state.read(); expect(signal?.aborted).toBe(true)
    if (mode === '成功') old.resolve(policy('synthetic-old')); else old.reject({ status: 404 })
    await a; expect(state.policy.value).toBeUndefined(); expect(state.loading.value).toBe(true); expect(state.failure.value).toBe(''); current.resolve(policy('synthetic-current')); await b; expect(state.policy.value?.credentialKeyId).toBe('synthetic-current')
  })
  it('卸载清引用并阻止迟到成功和重新读取', async () => {
    const { api, state, scope } = setup(), pending = deferred<SecurityPolicy>(); api.getSecurityPolicy.mockReturnValueOnce(pending.promise); const read = state.read(); const signal = api.getSecurityPolicy.mock.calls[0]?.[0]
    scope.stop(); expect(signal?.aborted).toBe(true); pending.resolve(policy()); await read; await state.read(); expect(state.policy.value).toBeUndefined(); expect(state.loading.value).toBe(false); expect(api.getSecurityPolicy).toHaveBeenCalledTimes(1)
  })
})
