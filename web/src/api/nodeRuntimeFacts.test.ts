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
  it('近期任务与操作保留有界授权投影，不接受其它业务或原始差异', () => {
    const time = '2026-10-12T00:00:00Z'
    const activity = { events: [], operations: [{ id: 'synthetic-audit', action: 'EXECUTION_NODE_CREATED', source: 'SUBJECT', result: 'SUCCEEDED', occurredAt: time, safeDiff: 'synthetic-private-diff' }], checkSource: 'SUBJECT' }
    expect(JSON.stringify(parseNodeActivity(activity))).not.toContain('synthetic-private-diff')
    expect(() => parseNodeActivity({ ...activity, operations: [{ ...activity.operations[0], action: 'UNKNOWN_OPERATION' }] })).toThrow()
    const item = { id: 'synthetic-task', type: 'OBDUMPER_EXPORT', state: 'SUCCEEDED', reconciliationRequired: false, submittedAt: time }
    expect(parseNodeRecentTasks({ scope: 'READABLE', items: [item] })?.items[0]).toEqual(item)
    expect(() => parseNodeRecentTasks({ scope: 'READABLE', items: Array.from({ length: 6 }, () => item) })).toThrow()
  })
})
