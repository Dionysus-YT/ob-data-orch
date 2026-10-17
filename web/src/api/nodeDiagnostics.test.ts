import { describe, expect, it } from 'vitest'
import { createBrowserApi, executionNodeErrorMessage } from './browser'
import { NodeResponseError, readNodeResponse } from './nodeResponseValidation'

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
  it.each(['ENVIRONMENT_CHECK_AUTHORIZATION_REQUIRED', 'TOOL_RUNTIME_UNAVAILABLE'])('接受待核验状态及 %s 原因且保持阻断', async reason => {
    expect(await read({ environmentStatus: 'UNCONFIRMED', acceptsNewTasks: false, unavailableReasons: [reason] })).toMatchObject({ environmentStatus: 'UNCONFIRMED', acceptsNewTasks: false, unavailableReasons: [reason] })
    await expect(read({ environmentStatus: 'UNCONFIRMED', acceptsNewTasks: true, unavailableReasons: [] })).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })
  it('详情接受混合导出与普通导入的近期任务，未知任务类型仍拒绝整份响应', async () => {
    const items = ['OBDUMPER_EXPORT', 'OBLOADER_NORMAL_IMPORT'].map((type, index) => ({ id: `synthetic-task-${index}`, type, state: 'SUCCEEDED', reconciliationRequired: false, submittedAt: time }))
    expect((await read({ recentTasks: { scope: 'ALL', items } })).recentTasks).toEqual({ scope: 'ALL', items })
    await expect(read({ recentTasks: { scope: 'ALL', items: [{ ...items[0], type: 'OBLOADER_DIRECT_LOAD' }] } })).rejects.toMatchObject({ code: 'RESPONSE_INVALID', message: expect.stringContaining('节点近期任务无法校验：存在当前页面尚未支持的任务类型。') })
  })

  it.each<readonly [string, object, string]>([
    ['节点基本状态', { managementState: 'synthetic-private-value' }, '管理状态不在支持范围'],
    ['Agent 资源采样', { agentFacts: { ...detail.agentFacts, capacityTotal: 1, capacityUsed: 2 } }, '已用任务槽位超过了总槽位数'],
    ['执行内存额度', { executionQuota: { totalGiB: 1, reservedGiB: 1, usedGiB: 1 } }, '预留与已用量之和不能超过总量'],
    ['节点任务统计', { taskSummary: { running: 0, waiting: 0, reconciling: 0, scope: 'synthetic-private-value' } }, '任务统计范围'],
    ['节点环境配置', { toolHome: undefined }, '缺少必需字段'],
    ['环境检查记录', { environmentCheck: { status: 'PENDING', factsRevision: 1, requestedAt: time, completedAt: '2026-10-11T00:00:00Z' } }, '检查完成时间早于请求时间'],
    ['环境检查明细', { environmentCheck: { status: 'PENDING', factsRevision: 1, inspection: { versionProbeAllowed: true, items: [] } } }, '十项固定检查未全部返回'],
    ['主机信息', { agentDiagnostics: { ...diagnostics, host: { logicalCpus: 0, addressPrefixes: [] } } }, '逻辑 CPU 数'],
    ['运行时版本', { agentDiagnostics: { ...diagnostics, runtimeVersions: { java: { version: 'synthetic-private-value', source: 'JAVA_RELEASE' } } } }, '安装版本格式'],
    ['日志上传队列', { agentDiagnostics: { ...diagnostics, logQueue: { ...diagnostics.logQueue, lastUploadResult: 'synthetic-private-value' } } }, '尚未支持的上传结果'],
    ['日志上传队列', { agentDiagnostics: { ...diagnostics, logQueue: { ...diagnostics.logQueue, pendingBytes: -1 } } }, '待上传字节数'],
    ['日志上传队列', { agentDiagnostics: { ...diagnostics, logQueue: { ...diagnostics.logQueue, lastUploadResult: 'SUCCEEDED', lastUploadAt: '2026-10-11T00:00:00Z' } } }, '上传时间早于当前 Agent'],
    ['活动任务', { activeExecutions: { scope: 'synthetic-private-value', items: [] } }, '活动任务的读取范围'],
    ['导出目录空间采样', { dataRootUsages: [{ root: 'C:\\synthetic-private-value', totalBytes: 1, availableBytes: 2 }] }, '可用空间大于总量'],
    ['节点近期任务', { recentTasks: { scope: 'ALL', items: [{ id: 'synthetic-task', type: 'synthetic-private-value', state: 'SUCCEEDED' }] } }, '尚未支持的任务类型'],
  ])('%s 异常保留内层区域、具体原因与处理建议，不回显原始值', async (section, overrides, reason) => {
    const error = await read(overrides).then(() => undefined, (value: unknown) => value)
    expect(error).toBeInstanceOf(NodeResponseError)
    expect(error).toMatchObject({ code: 'RESPONSE_INVALID' })
    const message = executionNodeErrorMessage(error, '执行节点加载失败。')
    expect(message).toContain(section + '无法校验')
    expect(message).toContain(reason)
    expect(message).toContain('请刷新页面')
    expect(message).not.toContain('synthetic-private-value')
    expect(JSON.stringify(error)).not.toContain('synthetic-private-value')
  })

  it('启用响应校验失败保留具体区域，并提示先核对可能已经生效的操作', async () => {
    const api = createBrowserApi({ fetcher: async () => Response.json({ item: { ...detail, recentTasks: { scope: 'ALL', items: [{ id: 'synthetic-task', type: 'OBLOADER_DIRECT_LOAD', state: 'SUCCEEDED' }] } } }), csrfToken: () => 'synthetic-csrf', idempotencyKey: () => 'synthetic-key' })
    await expect(api.enableExecutionNode(detail.id, 1)).rejects.toMatchObject({ code: 'RESPONSE_INVALID', message: expect.stringMatching(/节点近期任务无法校验.*操作可能已经生效.*先刷新节点核对结果/) })
  })

  it.each([
    ['EXECUTION_NODE_QUERY_UNAVAILABLE', '控制面服务与本地数据库状态'],
    ['EXECUTION_NODE_UPDATE_UNAVAILABLE', '刷新并核对节点配置'],
    ['EXECUTION_NODE_DELETE_UNAVAILABLE', '刷新节点列表核对'],
    ['ENVIRONMENT_CHECK_REQUEST_UNAVAILABLE', '是否已进入检查中'],
    ['NODE_ENABLE_PRECONDITION_FAILED', '确认 Agent 在线'],
    ['AGENT_ENROLLMENT_UNAVAILABLE', '关闭弹层并刷新节点'],
    ['RESPONSE_INVALID', '核对网页、控制面和 Agent 版本'],
    ['__proto__', '检查控制面服务'],
  ])('节点错误 %s 使用固定原因与处理建议，不回显服务端原始消息', (code, suggestion) => {
    const message = executionNodeErrorMessage({ status: 503, code, message: 'synthetic-private-value' }, '节点操作未完成。')
    expect(message).toContain(suggestion)
    expect(message).not.toContain('synthetic-private-value')
  })

  it('未知异常、空错误和兼容层校验异常使用安全说明，授权失效不泄露对象', () => {
    for (const error of [null, new Error('synthetic-private-value'), { code: 'RESPONSE_INVALID', message: 'synthetic-private-value' }]) {
      expect(executionNodeErrorMessage(error, '节点操作未完成。')).not.toContain('synthetic-private-value')
    }
    expect(executionNodeErrorMessage({ status: 404, message: 'synthetic-private-value' }, '')).toBe('执行节点不存在或当前身份无权访问。')
    expect(executionNodeErrorMessage({ status: 403, message: 'synthetic-private-value' }, '')).toContain('核对权限')
    expect(() => readNodeResponse('host', () => { throw { code: 'RESPONSE_INVALID', message: 'synthetic-private-value' } })).toThrow('主机信息无法校验：返回的数据缺少必需字段')
    const bug = new Error('synthetic-private-value')
    expect(() => readNodeResponse('host', () => { throw bug })).toThrow(bug)
  })

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
