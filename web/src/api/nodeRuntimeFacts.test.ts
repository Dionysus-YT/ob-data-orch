import { describe, expect, it } from 'vitest'
import { nodeInspectionKinds, parseNodeHostInventory, parseNodeRuntimeInspection, parseNodeActivity, parseNodeRecentTasks } from './nodeRuntimeFacts'

export function runtimeInspectionFixture() {
  return {
    checkId: 'synthetic-check', runtimeConfigurationDigest: 'a'.repeat(64), observedAt: '2026-10-12T00:00:00Z', versionProbeAllowed: true,
    items: nodeInspectionKinds.map(kind => ({ kind, status: 'PASSED', code: 'READY', required: ['JAVA_RUNTIME', 'OBDUMPER_RUNTIME', 'CONNECTOR', 'WORKSPACE', 'LOG_QUEUE', 'RESOURCES', 'RUNTIME_LAYOUT'].includes(kind), path: kind === 'RESOURCES' || kind === 'RUNTIME_LAYOUT' ? undefined : 'C:\\synthetic-fixture', version: kind === 'JAVA_RUNTIME' ? '1.8.0_491' : kind === 'OBDUMPER_RUNTIME' || kind === 'OBLOADER_RUNTIME' ? '4.3.5-RELEASE' : undefined, exists: kind === 'RESOURCES' ? undefined : true })),
    directLoad: { status: 'READ', taskTimeoutMs: 0, heartbeatTimeoutMs: 60000000, heartbeatIntervalMs: 30000000, hasUnrecognizedKeys: false, fingerprint: 'b'.repeat(64) },
  }
}

describe('节点完整采集事实解析', () => {
  it('保留真实零配置与全部固定项目，未知字段不进入模型', () => {
    const report = runtimeInspectionFixture()
    const result = parseNodeRuntimeInspection({ ...report, rawOutput: 'synthetic-secret' })
    expect(result?.items).toHaveLength(10)
    expect(result?.directLoad.taskTimeoutMs).toBe(0)
    expect(JSON.stringify(result)).not.toContain('synthetic-secret')
  })
  it('拒绝自报版本许可、缺项、重复、任意路径和矛盾通过', () => {
    const report = runtimeInspectionFixture()
    for (const invalid of [
      { ...report, versionProbeAllowed: false },
      { ...report, items: report.items.slice(1) },
      { ...report, items: [report.items[0], ...report.items.slice(0, 9)] },
      { ...report, items: report.items.map(item => ({ ...item, path: 'https://untrusted.invalid' })) },
      { ...report, items: report.items.map(item => ({ ...item, exists: false })) },
      { ...report, items: report.items.map(item => item.kind === 'JAVA_RUNTIME' ? { ...item, version: '17.0.1' } : item) },
      { ...report, items: report.items.map(item => item.kind === 'OBDUMPER_RUNTIME' ? { ...item, version: '4.3.6' } : item) },
      { ...report, directLoad: { ...report.directLoad, status: 'INVALID' } },
      { ...report, directLoad: { ...report.directLoad, taskTimeoutMs: -1 } },
    ]) expect(() => parseNodeRuntimeInspection(invalid)).toThrow(expect.objectContaining({ code: 'RESPONSE_INVALID' }))
  })
  it('主机仅接受受控网段摘要，原始地址不能展示', () => {
    const value = { hostname: 'synthetic-host', osName: 'Synthetic OS', osVersion: '1', cpuModel: 'Synthetic CPU', logicalCpus: 8, physicalCores: 4, addressPrefixes: ['192.0.2.0/24', '2001:db8::/64'] }
    expect(parseNodeHostInventory(value)).toEqual(value)
    for (const invalid of [{ ...value, logicalCpus: 0 }, { ...value, physicalCores: 0 }, { ...value, hostname: 'line\nline' }, { ...value, addressPrefixes: ['192.0.2.12'] }, { ...value, addressPrefixes: ['192.0.2.12/32'] }]) expect(() => parseNodeHostInventory(invalid)).toThrow()
  })
  it('近期任务与操作保留有界授权投影，不接受未知业务或原始差异', () => {
    const time = '2026-10-12T00:00:00Z'
    const activity = { events: [], operations: [{ id: 'synthetic-audit', action: 'EXECUTION_NODE_CREATED', source: 'SUBJECT', result: 'SUCCEEDED', occurredAt: time, safeDiff: 'synthetic-private-diff' }], checkSource: 'SUBJECT' }
    expect(JSON.stringify(parseNodeActivity(activity))).not.toContain('synthetic-private-diff')
    expect(() => parseNodeActivity({ ...activity, operations: [{ ...activity.operations[0], action: 'UNKNOWN_OPERATION' }] })).toThrow()
    const item = { id: 'synthetic-task', type: 'OBDUMPER_EXPORT', state: 'SUCCEEDED', reconciliationRequired: false, submittedAt: time }
    expect(parseNodeRecentTasks({ scope: 'READABLE', items: [item] })?.items[0]).toEqual(item)
    expect(() => parseNodeRecentTasks({ scope: 'READABLE', items: Array.from({ length: 6 }, () => item) })).toThrow()
  })

  it.each(['READABLE', 'ALL'])('近期任务支持 %s 范围内混合导出和普通导入，不改写任务类型', scope => {
    const items = ['OBDUMPER_EXPORT', 'OBLOADER_NORMAL_IMPORT'].map((type, index) => ({ id: `synthetic-task-${index}`, type, state: 'SUCCEEDED', reconciliationRequired: false, submittedAt: '2026-10-16T00:00:00Z' }))
    expect(parseNodeRecentTasks({ scope, items })).toEqual({ scope, items })
    expect(parseNodeRecentTasks(undefined)).toBeUndefined()
  })

  it('未知任务类型、状态、范围及过量记录分别说明原因，原值不进入错误', () => {
    const item = { id: 'synthetic-task', type: 'OBDUMPER_EXPORT', state: 'SUCCEEDED', reconciliationRequired: false, submittedAt: '2026-10-16T00:00:00Z' }
    for (const [value, reason] of [
      [{ scope: 'ALL', items: [{ ...item, type: 'synthetic-private-value' }] }, '尚未支持的任务类型'],
      [{ scope: 'ALL', items: [{ ...item, type: 'OBLOADER_DIRECT_LOAD' }] }, '尚未支持的任务类型'],
      [{ scope: 'ALL', items: [{ ...item, state: 'synthetic-private-value' }] }, '尚未支持的任务状态'],
      [{ scope: 'synthetic-private-value', items: [item] }, '任务读取范围'],
      [{ scope: 'ALL', items: Array.from({ length: 6 }, () => item) }, '5 条记录'],
      [{ scope: 'ALL', items: [{ ...item, submittedAt: 'synthetic-private-value' }] }, '任务提交时间'],
    ] as const) {
      let caught: unknown
      try { parseNodeRecentTasks(value) } catch (error) { caught = error }
      expect(caught).toMatchObject({ code: 'RESPONSE_INVALID', message: expect.stringContaining(reason) })
      expect((caught as Error).message).toContain('节点近期任务无法校验')
      expect((caught as Error).message).toContain('核对网页与控制面版本')
      expect(JSON.stringify(caught)).not.toContain('synthetic-private-value')
    }
  })

  it('主机、检查缺项、未知结果码与操作异常具有各自的安全解释', () => {
    const inspection = runtimeInspectionFixture()
    const failures: readonly [() => unknown, string, string][] = [
      [() => parseNodeHostInventory({ logicalCpus: 0, addressPrefixes: [] }), '主机信息', '逻辑 CPU 数'],
      [() => parseNodeRuntimeInspection({ ...inspection, items: inspection.items.slice(1) }), '环境检查明细', '十项固定检查未全部返回'],
      [() => parseNodeRuntimeInspection({ ...inspection, items: inspection.items.map(item => ({ ...item, code: 'synthetic-private-value' })) }), '环境检查明细', '尚未支持的检查结果码'],
      [() => parseNodeRuntimeInspection({ ...inspection, items: inspection.items.map(item => ({ ...item, path: 'https://synthetic-private-value.invalid' })) }), '环境检查明细', '本机绝对路径'],
      [() => parseNodeActivity({ checkSource: 'UNKNOWN', events: [{ id: 'synthetic-event', action: 'synthetic-private-value', source: 'SYSTEM', result: 'SUCCEEDED' }], operations: [] }), '节点操作记录', '尚未支持的节点操作类型'],
    ]
    for (const [read, section, reason] of failures) {
      let caught: unknown
      try { read() } catch (error) { caught = error }
      expect(caught).toMatchObject({ code: 'RESPONSE_INVALID' })
      expect((caught as Error).message).toContain(section + '无法校验')
      expect((caught as Error).message).toContain(reason)
      expect((caught as Error).message).toContain('请刷新页面')
      expect(JSON.stringify(caught)).not.toContain('synthetic-private-value')
    }
  })
})
