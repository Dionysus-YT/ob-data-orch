import { describe, expect, it } from 'vitest'
import { createBrowserApi, type FetchLike, type GeneralizedExportConfig } from './browser'

const base: GeneralizedExportConfig = {
  objectScope: { database: 'synthetic_db', scopeKind: 'SPECIFIED', objectTypes: ['TABLE'], expressions: [{ name: 'synthetic_table', objectType: 'TABLE' }] },
  contentSelection: { contentKind: 'DDL_AND_DATA' }, dataFormat: { formatKind: 'CSV' },
  outputConfig: { outputKind: 'LOCAL', filePath: '/E:/synthetic/output', logPath: '', skipCheckDir: false, controlFilePath: '', tmpPath: '' },
}

function draftApi(config: unknown) {
  const writes: RequestInit[] = []
  const fetcher = (async (_path: string | URL | Request, init?: RequestInit) => {
    if (init?.method === 'PATCH') writes.push(init)
    return Response.json({ item: { id: 'synthetic-draft', dataSourceId: 'synthetic-source', nodeId: 'synthetic-node', revision: 1, configVersion: 'v6', configFingerprint: 'synthetic-fingerprint', config: { config } } })
  }) as FetchLike
  return { writes, api: createBrowserApi({ fetcher, csrfToken: () => 'synthetic-csrf', idempotencyKey: () => 'synthetic-idempotency' }) }
}

describe('导出草稿读取与更新往返', () => {
  it.each([
    { name: 'MySQL 值格式与读取/DDL 参数', config: { ...base, dataFormat: { formatKind: 'CSV', timestampFormats: { dateValueFormat: 'yyyy/MM/dd', timeValueFormat: 'HH-mm-ss', datetimeValueFormat: 'yyyy/MM/dd HH:mm:ss', timestampValueFormat: 'yyyy/MM/dd HH:mm:ss', preserveZeroDatetime: true } }, filterConfig: { where: 'id > 0', partition: 'p0', excludeDataTypes: ['BLOB'], enableHiddenPk: true, weakRead: true, includeColumnNames: ['id'], excludeVirtualColumns: true, snapshot: true }, ddlBehavior: { dropObject: true, retainSchema: true, compactSchema: true, addExtraMessage: true }, outputConfig: { ...base.outputConfig, compress: true, compressionAlgo: 'zstd', compressionLevel: 3 } } },
    { name: 'Oracle NLS 与时区格式', config: { ...base, dataFormat: { formatKind: 'CUT', timestampFormats: { dateValueFormat: 'yyyy-MM-dd', timestampValueFormat: 'yyyy-MM-dd HH:mm:ss', timestampTzValueFormat: 'yyyy-MM-dd HH:mm:ss TZD', timestampLtzValueFormat: 'yyyy-MM-dd HH:mm:ss', nlsDateFormat: 'YYYY-MM-DD', nlsTimestampFormat: 'YYYY-MM-DD HH24:MI:SS', nlsTimestampTzFormat: 'YYYY-MM-DD HH24:MI:SS TZR' } }, filterConfig: { flashbackTimestamp: '2026-01-01 00:00:00' }, performanceConfig: { fetchSize: 100 } } },
    { name: '序列策略', config: { objectScope: { database: 'synthetic_db', scopeKind: 'SPECIFIED', objectTypes: ['SEQUENCE'], expressions: [{ name: 'synthetic_sequence', objectType: 'SEQUENCE' }] }, contentSelection: { contentKind: 'DDL_ONLY' }, outputConfig: base.outputConfig, ddlBehavior: { sequencePolicy: 'RESTART' } } },
  ])('$name不会在读取后静默丢失', async ({ config }) => {
    const { api, writes } = draftApi(config)
    const draft = await api.getExportDraft('synthetic-draft')
    await api.updateExportDraft(draft)
    expect(JSON.parse(String(writes[0]?.body)).config).toEqual(config)
  })

  it.each([
    { ...base, filterConfig: { weakRead: 'true' } },
    { ...base, dataFormat: { formatKind: 'CSV', timestampFormats: { timeValueFormat: 12 } } },
    { ...base, ddlBehavior: { sequencePolicy: 'UNKNOWN_POLICY' } },
  ])('新增字段类型或枚举无效时失败关闭', async config => {
    await expect(draftApi(config).api.getExportDraft('synthetic-draft')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })

  it('只重建公开参数，不复制未知内部材料', async () => {
    const { api, writes } = draftApi({ ...base, ddlBehavior: { retainSchema: true, internalMaterial: 'synthetic-private-value' }, privateSlot: 'synthetic-private-value' })
    await api.updateExportDraft(await api.getExportDraft('synthetic-draft'))
    expect(String(writes[0]?.body)).not.toContain('synthetic-private-value')
  })
})
