import { describe, expect, it } from 'vitest'
import { createBrowserApi } from './browser'

const time = '2026-10-12T00:00:00Z'
const detail = {
  id: 'synthetic-node', displayName: '合成节点', platform: 'WINDOWS_AMD64', managementState: 'DISABLED',
  agentAssociationStatus: 'ASSOCIATED', heartbeatStatus: 'ONLINE', lastHeartbeatAt: time, environmentStatus: 'NOT_CHECKED', capacityStatus: 'AVAILABLE',
  acceptsNewTasks: false, unavailableReasons: ['NODE_DISABLED'], revision: 1, updatedAt: time, createdAt: time,
  toolHome: 'C:\\synthetic-tools', javaPath: 'C:\\synthetic-java\\bin\\java.exe', allowedRoots: ['C:\\synthetic-output'],
  agentFacts: { os: 'WINDOWS', arch: 'AMD64', agentVersion: 'synthetic', bootId: 'synthetic-boot', observedAt: time, capacityTotal: 1, capacityUsed: 0 },
}
const diagnostics = {
  agentId: 'synthetic-agent', protocolVersion: 'agent-v1', factsRevision: 3, startedAt: '2026-10-11T23:00:00Z',
  expectedRuntimeConfigurationDigest: 'a'.repeat(64), runtimeConfigurationDigest: 'a'.repeat(64),
  runtimeVersions: { java: { version: '1.8.0_491', source: 'JAVA_RELEASE' } },
  logQueue: { pendingBatches: 0, pendingGaps: 0, pendingBytes: 0, lastUploadResult: 'UNKNOWN' },
}

function read(overrides: object) {
  return createBrowserApi({ fetcher: async () => Response.json({ item: { ...detail, ...overrides } }), csrfToken: () => 'synthetic-csrf', idempotencyKey: () => 'synthetic-key' }).getExecutionNode(detail.id)
}

describe('执行节点详情诊断边界', () => {
  it('旧响应省略诊断保持未知，新响应保留真实零值并忽略无关字段', async () => {
    expect((await read({})).agentDiagnostics).toBeUndefined()
    const result = await read({ agentDiagnostics: { ...diagnostics, machineCredential: 'synthetic-unread-secret' }, activeExecutions: { scope: 'READABLE', items: [] }, environmentCheck: { status: 'PENDING', factsRevision: 0, requestedAt: time } })
    expect(result.agentDiagnostics?.logQueue?.pendingBytes).toBe(0)
    expect(result.activeExecutions?.items).toEqual([])
    expect(JSON.stringify(result)).not.toContain('synthetic-unread-secret')
  })
  it.each([
    { ...diagnostics, factsRevision: 2 ** 53 },
    { ...diagnostics, startedAt: '2026-10-13T00:00:00Z' },
    { ...diagnostics, runtimeConfigurationDigest: 'not-a-digest' },
    { ...diagnostics, runtimeVersions: { java: { version: 'C:\\private\\java', source: 'JAVA_RELEASE' } } },
    { ...diagnostics, runtimeVersions: { java: { version: '1.8.0_491', source: 'JAR_MANIFEST' } } },
    { ...diagnostics, logQueue: { ...diagnostics.logQueue, pendingBytes: -1 } },
    { ...diagnostics, logQueue: { ...diagnostics.logQueue, pendingBytes: 2 ** 31 } },
    { ...diagnostics, logQueue: { ...diagnostics.logQueue, lastUploadResult: 'SUCCEEDED' } },
    { ...diagnostics, logQueue: { ...diagnostics.logQueue, lastUploadResult: 'SUCCEEDED', lastUploadAt: '2026-10-11T00:00:00Z' } },
    { ...diagnostics, logQueue: { ...diagnostics.logQueue, lastUploadAt: time } },
  ])('拒绝矛盾版本、时间或队列 %j', async agentDiagnostics => {
    await expect(read({ agentDiagnostics })).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })
  it('拒绝伪造检查结果及活动租约，不用详情字段改变资格', async () => {
    for (const environmentCheck of [{ status: 'PASSED', factsRevision: 0 }, { status: 'FAILED', factsRevision: 1, code: 'raw-output' }, { status: 'PENDING', factsRevision: 1, requestedAt: time, completedAt: '2026-10-11T00:00:00Z' }]) {
      await expect(read({ environmentCheck })).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
    }
    const item = { taskId: 'synthetic-task', executionId: 'synthetic-execution', leaseId: 'synthetic-lease', leaseEpoch: 1, lastEventSeq: 3 }
    for (const items of [[{ ...item, leaseId: '/private/path' }], [item, item], [{ ...item, leaseEpoch: 0 }]]) {
      await expect(read({ activeExecutions: { scope: 'READABLE', items } })).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
    }
  })
})
