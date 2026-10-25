import { describe, expect, it, vi } from 'vitest'
import { createSecurityPolicyApi, parseSecurityPolicy, type SecurityPolicy } from './securityPolicy'

function policy(): SecurityPolicy { return { capturedAt: '2026-10-24T01:00:00Z', loadedAt: '2026-10-24T00:00:00Z', source: 'STARTUP_CONFIGURATION', builtInRedaction: true, credentialFormat: 'credential-aes-gcm-v1', credentialKeyId: 'synthetic-key-v1', rootKeyStatus: 'LOADED', rootKeySource: 'LOCAL_PROTECTED_FILE', sensitiveCommandView: 'NOT_AVAILABLE', productionConfirmation: 'TASK_SUBMISSION_RULES', auditRetentionDays: null, auditAutomaticCleanup: false, realExecutionEnabled: true, normalImportExecutionEnabled: true, directLoadProbeEnabled: true, directLoadExecutionEnabled: true } }

describe('安全状态严格投影', () => {
  it('只读取固定端点并传递取消信号，没有写方法', async () => {
    const read = vi.fn().mockResolvedValue({ requestId: 'synthetic-request', item: policy() }), api = createSecurityPolicyApi({ read }), controller = new AbortController()
    expect(await api.getSecurityPolicy(controller.signal)).toEqual(policy())
    expect(read).toHaveBeenCalledWith('/api/v1/settings/security', controller.signal)
    expect(Object.keys(api)).toEqual(['getSecurityPolicy'])
  })
  it('未知字段、缺失、秘密载荷和坏时间失败关闭且不回显', () => {
    const missing = { ...policy() } as Record<string, unknown>; delete missing.rootKeyStatus
    for (const value of [{ ...policy(), privateKey: 'synthetic-private-only' }, { ...policy(), credentialKeyId: '../synthetic' }, { ...policy(), capturedAt: 'unknown' }, { ...policy(), loadedAt: '2026-02-31T00:00:00Z' }, { ...policy(), loadedAt: '2027-01-01T00:00:00Z' }, missing, Object.create(policy())]) {
      try { parseSecurityPolicy(value); expect.fail('非法响应未拒绝') } catch (error) { expect(error).toMatchObject({ code: 'RESPONSE_INVALID' }); expect(JSON.stringify(error)).not.toContain('synthetic-private-only') }
    }
  })
  it('拒绝保护降级、审计猜测期限和矛盾执行开关', () => {
    for (const value of [{ ...policy(), builtInRedaction: false }, { ...policy(), rootKeyStatus: 'UNKNOWN' }, { ...policy(), sensitiveCommandView: 'ENABLED' }, { ...policy(), auditRetentionDays: 30 }, { ...policy(), auditAutomaticCleanup: true }, { ...policy(), realExecutionEnabled: false }, { ...policy(), directLoadProbeEnabled: false }, { ...policy(), normalImportExecutionEnabled: 'true' }]) expect(() => parseSecurityPolicy(value)).toThrow()
    expect(parseSecurityPolicy(policy())).toEqual(policy())
  })
})
