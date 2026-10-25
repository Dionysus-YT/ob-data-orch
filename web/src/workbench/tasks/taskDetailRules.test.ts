import { describe, expect, it } from 'vitest'
import type { TaskSnapshot } from '@/api/browser'
import { taskDetailLocation, taskResultText, taskSnapshotGroups } from './taskDetailRules'

const snapshot: TaskSnapshot = {
  type: 'OBLOADER_NORMAL_IMPORT', snapshotVersion: 'normal-import-v1', dataSourceId: 'synthetic-source', nodeId: 'synthetic-node',
  precheckId: 'synthetic-precheck', objectSummary: 'synthetic.orders', format: 'CSV', configFingerprint: 'synthetic',
  toolVersion: '4.3.5', metadataVersion: 'synthetic', capabilityVersion: 'synthetic',
}

describe('导出与普通导入共用详情的展示边界', () => {
  it('普通导入最小快照沿用自身五步，不回退为导出配置', () => {
    const groups = taskSnapshotGroups(snapshot)
    expect(groups.map(group => group.key)).toEqual(['source', 'input', 'objects', 'execution', 'review'])
    expect(groups.map(group => group.label).join(' ')).not.toContain('导出')
    expect(groups[1]?.fields.some(field => field.key === 'node')).toBe(true)
    const projected = { ...snapshot, groups: [{ key: 'source' as const, label: '冻结标题', fields: [] }] }
    expect(taskSnapshotGroups(projected)).toBe(projected.groups)
  })

  it('输入位置只消费服务端裁剪字段，缺失时不借用输出位置', () => {
    const output = { kind: 'LOCAL' as const, redaction: 'FULL' as const, value: 'synthetic-output' }
    expect(taskDetailLocation(snapshot.type, snapshot, output)).toMatchObject({ kind: 'CSV', value: '—' })
    const hidden: TaskSnapshot = { ...snapshot, groups: [{ key: 'input', label: '输入', fields: [{ key: 'filePath', label: '输入路径', value: '已隐藏', source: 'UNKNOWN' }] }] }
    expect(taskDetailLocation(snapshot.type, hidden, output).value).toBe('已隐藏')
    expect(taskDetailLocation('OBDUMPER_EXPORT', undefined, { kind: 'S3', redaction: 'KIND_ONLY' })).toMatchObject({ kind: 'S3', value: '仅创建者可见' })
    expect(taskDetailLocation('OBDUMPER_EXPORT', undefined, { kind: 'UNAVAILABLE', redaction: 'UNAVAILABLE' }).kind).toBe('—')
  })

  it('普通导入成功仅说明完成证据，缺失和失败证据不能变成成功', () => {
    expect(taskResultText({ result: 'VERIFIED' }, false, snapshot.type)).toBe('导入完成证据已核验')
    expect(taskResultText(undefined, false, snapshot.type)).toBe('尚无结果证据')
    expect(taskResultText({ result: 'FAILED' }, true, snapshot.type)).toBe('结果未通过核验')
    expect(taskResultText({ result: 'VERIFIED' }, true, 'OBDUMPER_EXPORT')).toBe('远端对象未逐项核验')
  })
})
