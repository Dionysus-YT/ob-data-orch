import { describe, expect, it, vi } from 'vitest'
import { createNormalImportApi, type NormalImportConfig } from './normalImport'

function config(): NormalImportConfig { return { dataSourceId: 'source', nodeId: 'node', database: 'synthetic_db', filePath: 'E:\\input', content: 'DATA_ONLY', format: 'CSV', scope: 'ALL', parser: {}, execution: {}, cleanup: 'NONE' } }
function draft() { return { id: 'draft', revision: 1, configVersion: 'normal-import-v1' as const, config: config(), configFingerprint: 'a'.repeat(64), metadataVersion: 'obloader-4.3.5-normal-v1', capabilityVersion: 'import-odp-local-v1', executionEnabled: false } }
function check() { return { id: 'check', draftId: 'draft', draftRevision: 1, configFingerprint: 'a'.repeat(64), status: 'SUCCEEDED', integrityStatus: 'COMPLETE', checkKind: 'NORMAL_IMPORT_FILES', validUntil: '2099-01-01T00:00:00Z', executionEnabled: false, results: [{ check: 'INPUT_FILES', status: 'PASSED', evidenceCode: 'IMPORT_VERIFIED' }], inputManifest: { fileCount: 1, totalBytes: 2, digest: 'b'.repeat(64), files: [{ path: 'synthetic.csv', kind: 'DATA', object: 'synthetic', size: 2, modified: '1790000000000000000', sha256: 'c'.repeat(64) }] } } }
function apiWith(value: Record<string, unknown>) { const read = vi.fn(async () => value), write = vi.fn(async () => value); return { api: createNormalImportApi({ read, write }), read, write } }

describe('普通导入独立 API 解码与请求', () => {
  it('控制文件摘要保留冻结绑定且拒绝无效摘要', async () => {
    const value = check()
    const withControl = { ...value, inputManifest: { ...value.inputManifest, controlDigest: 'd'.repeat(64) } }
    expect((await apiWith(withControl).api.getNormalImportCheck('check')).inputManifest?.controlDigest).toBe('d'.repeat(64))
    await expect(apiWith({ ...withControl, inputManifest: { ...withControl.inputManifest, controlDigest: 'invalid' } }).api.getNormalImportCheck('check')).rejects.toThrow('普通导入响应无效')
  })
  it('逐表权限与映射细化原因均不能提升为通过', async () => {
    const cases = [
      ['NODE_RESOURCE', 'FAILED', ['IMPORT_JVM_HEAP_TOO_SMALL']],
      ['WRITE_ACCESS', 'FAILED', ['IMPORT_WRITE_INSERT_REQUIRED', 'IMPORT_WRITE_UPDATE_REQUIRED', 'IMPORT_WRITE_DELETE_REQUIRED', 'IMPORT_WRITE_CREATE_REQUIRED', 'IMPORT_WRITE_ALTER_REQUIRED', 'IMPORT_WRITE_DROP_REQUIRED']],
      ['WRITE_ACCESS', 'UNKNOWN', ['IMPORT_WRITE_GRANTS_UNAVAILABLE', 'IMPORT_WRITE_GRANTS_UNSUPPORTED', 'IMPORT_WRITE_DDL_UNSUPPORTED', 'IMPORT_WRITE_INPUT_UNAVAILABLE', 'IMPORT_WRITE_SCHEMA_UNAVAILABLE', 'IMPORT_WRITE_CLEANUP_SCOPE_UNAVAILABLE']],
      ['INPUT_MAPPING', 'FAILED', ['IMPORT_COLUMN_COUNT_MISMATCH', 'IMPORT_COLUMN_NAME_MISMATCH', 'IMPORT_REQUIRED_COLUMN_MISSING', 'IMPORT_CSV_WHITESPACE_UNQUOTED']],
      ['INPUT_MAPPING', 'UNKNOWN', ['IMPORT_TARGET_METADATA_UNAVAILABLE', 'IMPORT_TARGET_STRUCTURE_MISSING', 'IMPORT_INPUT_DEFINITION_UNSUPPORTED', 'IMPORT_AUTO_COLUMNS_UNSUPPORTED', 'IMPORT_INPUT_FILES_CHANGED']],
    ] as const
    for (const [name, status, codes] of cases) for (const evidenceCode of codes) {
      const results = ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'INPUT_FILES', 'INPUT_MAPPING'].map(check => ({ check, status: check === name ? status : 'PASSED', evidenceCode: check === name ? evidenceCode : 'SYNTHETIC_OK' }))
      const report = { ...check(), checkKind: 'NORMAL_IMPORT_PREFLIGHT', status: 'FAILED', results }
      expect((await apiWith(report).api.getNormalImportCheck('check')).results.find(item => item.check === name)?.evidenceCode).toBe(evidenceCode)
      await expect(apiWith({ ...report, results: results.map(item => item.check === name ? { ...item, status: 'PASSED' } : item) }).api.getNormalImportCheck('check')).rejects.toThrow('普通导入响应无效')
    }
  })
  it('只读权限与映射证据严格区分拒绝和无法确认，不能伪造通过', async () => {
    for (const [name, status, evidenceCode] of [['WRITE_ACCESS', 'FAILED', 'IMPORT_WRITE_PERMISSION_REJECTED'], ['WRITE_ACCESS', 'UNKNOWN', 'IMPORT_WRITE_PERMISSION_UNAVAILABLE'], ['INPUT_MAPPING', 'FAILED', 'IMPORT_INPUT_MAPPING_MISMATCH'], ['INPUT_MAPPING', 'UNKNOWN', 'IMPORT_INPUT_MAPPING_UNAVAILABLE'], ['INPUT_MAPPING', 'UNKNOWN', 'IMPORT_INPUT_MAPPING_UNSUPPORTED']]) {
      const results = ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'INPUT_FILES', 'INPUT_MAPPING'].map(check => ({ check, status: check === name ? status : 'PASSED', evidenceCode: check === name ? evidenceCode : 'SYNTHETIC_OK' }))
      const report = { ...check(), checkKind: 'NORMAL_IMPORT_PREFLIGHT', status: 'FAILED', results }
      expect((await apiWith(report).api.getNormalImportCheck('check')).results.find(item => item.check === name)?.status).toBe(status)
      await expect(apiWith({ ...report, results: results.map(item => item.check === name ? { ...item, status: 'PASSED' } : item) }).api.getNormalImportCheck('check')).rejects.toThrow('普通导入响应无效')
    }
  })
  it('提交名称是可选平台字段，空值省略且不改写冻结配置', async () => {
    const saved = draft(), s = apiWith({ id: 'task-normal' })
    await s.api.submitNormalImport(saved, 'check', undefined, '  合成名称  ')
    expect(s.write.mock.calls[0]).toEqual(['/api/v1/normal-import-drafts/draft:submit', { precheckId: 'check', taskName: '合成名称' }, 1])
    await s.api.submitNormalImport(saved, 'check', undefined, '  ')
    expect(s.write.mock.calls[1]).toEqual(['/api/v1/normal-import-drafts/draft:submit', { precheckId: 'check' }, 1])
    expect(saved.config.taskName).toBeUndefined()
  })
  it('未装配证据只允许用于独立检查的未知状态', async () => {
    const base = check()
    const results = ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'INPUT_FILES', 'INPUT_MAPPING'].map(name => ({ check: name, status: ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'INPUT_MAPPING'].includes(name) ? 'UNKNOWN' : 'PASSED', evidenceCode: ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'INPUT_MAPPING'].includes(name) ? 'NORMAL_IMPORT_PROBE_NOT_CONFIGURED' : 'SYNTHETIC_OK' }))
    const pending = { ...base, checkKind: 'NORMAL_IMPORT_PREFLIGHT', status: 'FAILED', results }
    expect((await apiWith(pending).api.getNormalImportCheck('check')).results.filter(item => item.evidenceCode === 'NORMAL_IMPORT_PROBE_NOT_CONFIGURED')).toHaveLength(3)
    for (const name of ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'INPUT_MAPPING']) {
      await expect(apiWith({ ...pending, results: results.map(item => item.check === name ? { ...item, status: 'PASSED' } : item) }).api.getNormalImportCheck('check')).rejects.toThrow('普通导入响应无效')
    }
  })
  it('只发送独立配置与草稿修订，保留纳秒时间精度', async () => {
    const saved = apiWith(draft()); await saved.api.createNormalImportDraft(config()); await saved.api.updateNormalImportDraft(draft(), config())
    expect(saved.write.mock.calls[0]).toEqual(['/api/v1/normal-import-drafts', { configVersion: 'normal-import-v1', config: config() }])
    expect(saved.write.mock.calls[1]).toEqual(['/api/v1/normal-import-drafts/draft', { configVersion: 'normal-import-v1', config: config() }, 1, 'PATCH'])
    const files = apiWith(check()); expect((await files.api.getNormalImportCheck('check')).inputManifest?.files[0]?.modified).toBe('1790000000000000000')
  })
  it('兼容版本响应只接纳官方枚举并保留未选择的旧草稿', async () => {
    for (const compatMysqlVersion of ['mysql56', 'mysql57', 'mysql80'] as const) {
      const value = { ...draft(), config: { ...config(), content: 'DDL_ONLY', compatMysql: true, compatMysqlVersion } }
      expect((await apiWith(value).api.getNormalImportDraft('draft')).config.compatMysqlVersion).toBe(compatMysqlVersion)
    }
    const legacy = { ...draft(), config: { ...config(), content: 'DDL_ONLY', compatMysql: true } }
    expect((await apiWith(legacy).api.getNormalImportDraft('draft')).config.compatMysqlVersion).toBeUndefined()
    await expect(apiWith({ ...legacy, config: { ...legacy.config, compatMysqlVersion: 'mysql' } }).api.getNormalImportDraft('draft')).rejects.toThrow('普通导入响应无效')
  })
  it('响应中的未知字段、旁路参数、任务连接覆盖和不支持格式失败关闭', async () => {
    for (const foreign of [{ ...config(), direct: true }, { ...config(), sessionConfig: 'synthetic' }, { ...config(), format: 'MIX' }, { ...config(), execution: { parallel: '8' } }, { ...config(), parser: { compress: 'yes' } }]) {
      await expect(apiWith({ ...draft(), config: foreign }).api.getNormalImportDraft('draft')).rejects.toThrow('普通导入响应无效')
    }
  })
  it('拒绝缺失、重复、越界、损坏和不完整输入证明，未知证据码不能显示已通过', async () => {
    const good = check()
    for (const bad of [
      { ...good, inputManifest: null }, { ...good, integrityStatus: 'UNKNOWN' },
      { ...good, inputManifest: { ...good.inputManifest, fileCount: 2 } },
      { ...good, inputManifest: { ...good.inputManifest, files: [{ ...good.inputManifest.files[0]!, path: '../outside.csv' }] } },
      { ...good, inputManifest: { ...good.inputManifest, files: [{ ...good.inputManifest.files[0]!, path: 'E:outside.csv' }] } },
      { ...good, inputManifest: { ...good.inputManifest, files: [{ ...good.inputManifest.files[0]!, modified: 1790000000000000000 }] } },
      { ...good, results: [{ ...good.results[0]!, evidenceCode: 'UNKNOWN_SUCCESS' }] },
      { ...good, results: [{ ...good.results[0]!, status: 'UNKNOWN' }] },
      { ...good, checkKind: 'NORMAL_IMPORT_PREFLIGHT' },
    ]) await expect(apiWith(bad).api.getNormalImportCheck('check')).rejects.toThrow('普通导入响应无效')
  })
  it('真实密码或错误工具命令拒绝展示，合法预览保持只读', async () => {
    for (const command of ['obloader --password synthetic-value', 'obloader -p synthetic-value', 'obdumper -p ******', 'obloader password=synthetic-value']) {
      await expect(apiWith({ redactedCommand: command, configFingerprint: 'a'.repeat(64), executionEnabled: false }).api.previewNormalImportCommand(draft())).rejects.toThrow('普通导入响应无效')
    }
    const preview = apiWith({ redactedCommand: 'obloader --csv -p ******', configFingerprint: 'a'.repeat(64), executionEnabled: false })
    expect((await preview.api.previewNormalImportCommand(draft())).redactedCommand).toBe('obloader --csv -p ******')
  })
})
