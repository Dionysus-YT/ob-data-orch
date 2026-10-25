import parameterBaseline from '../../../../../docs/02-design/normal-import-parameter-mapping.md?raw'
import commandMetadata from '../../../../../internal/parammeta/normal/obloader-4.3.5-normal-v1.json'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { normalImportNode, normalImportSources } from '../../../../tests/fixtures/normal-import'
import { createNormalImportForm, importFormats } from './normalImportForm'
import { isImportAbsolutePath, isNormalImportSource, projectNormalImport, validateNormalImport } from './normalImportRules'
import { normalImportParameterCatalog, normalImportParameterRows } from './normalImportParameterCatalog'
import { useNormalImportParameters } from './useNormalImportParameters'
import { useNormalImportReferences } from './useNormalImportReferences'
import type { DataSourceSummary, ExecutionNodeSummary } from '@/api/browser'

const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()) })
const source = normalImportSources[0]!
const facts = { source, node: normalImportNode }

function configuredForm() {
  const form = createNormalImportForm()
  Object.assign(form, { dataSourceId: source.id, database: 'synthetic_database', nodeId: normalImportNode.id, filePath: 'E:\\synthetic-import', content: 'DATA_ONLY', format: 'CSV', scope: 'SPECIFIC', taskName: 'synthetic-import' })
  form.objects.table = 'synthetic_table'
  return form
}

function parameters() {
  const scope = effectScope()
  scopes.push(scope)
  const form = configuredForm()
  const selectedSource = ref(source)
  const selectedNode = ref(normalImportNode)
  const state = scope.run(() => useNormalImportParameters(form, { selectedSource, selectedNode, eligibleSources: ref(normalImportSources.filter(isNormalImportSource)) }))!
  return { form, selectedSource, selectedNode, ...state }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

describe('普通导入唯一表单与活动参数', () => {
  it('MySQL兼容必须明确源版本，版本变化进入配置并清除非活动版本', () => {
    const state = parameters()
    state.form.content = 'DDL_ONLY'
    state.form.compatMysql = true
    expect(state.issues.value.some(issue => issue.field === 'compatMysqlVersion')).toBe(true)
    const signatures = new Set<string>()
    for (const version of ['mysql56', 'mysql57', 'mysql80'] as const) {
      state.form.compatMysqlVersion = version
      expect(state.configuration.value.compatMysqlVersion).toBe(version)
      expect(state.issues.value.some(issue => issue.field === 'compatMysqlVersion')).toBe(false)
      signatures.add(state.configurationSignature.value)
    }
    expect(signatures.size).toBe(3)
    state.form.compatMysql = false
    expect(state.form.compatMysqlVersion).toBe('')
    expect(state.configuration.value.compatMysqlVersion).toBe('')
  })
  it('前端目录与独立命令定义一致，四项受管连接说明不作为任务覆盖', () => {
    const managed = ['sys-user', 'sys-password', 'character-set', 'session-config']
    expect(commandMetadata.definitions.map(item => item.longName.slice(2)).sort()).toEqual(normalImportParameterCatalog.map(item => item.name).filter(name => !managed.includes(name)).sort())
    expect(commandMetadata.definitions).toHaveLength(72)
  })
  it('配置目录仅覆盖基线中本轮适用的 76 项，移除旁路、废弃、元参数与未开放能力', () => {
    const names = [...parameterBaseline.matchAll(/^\| `--([a-z-]+)` \|/gm)].map(match => match[1]!)
    const excludedNames = ['direct', 'rpc-port', 'parallel', 'storage-uri', 'column-delimiter', 'buffer-size', 'server', 'public-synonym', 'enable-hidden-pk', 'help', 'version', 'mix', 'pos', 'orc', 'par', 'avro', 'source-type', 'external-data', 'tmp-path', 'ignore-unhex', 'slow', 'pause', 'retry', 'public-cloud', 'logical-database', 'tenant', 'cluster']
    expect(normalImportParameterCatalog.map(item => item.name).sort()).toEqual([...new Set(names)].filter(name => !excludedNames.includes(name)).sort())
    expect(normalImportParameterCatalog).toHaveLength(76)
    for (const name of excludedNames) expect(normalImportParameterCatalog.find(item => item.name === name)).toBeUndefined()
    for (const item of normalImportParameterCatalog) {
      expect(item.label).toBeTruthy()
      expect(item.description).toBeTruthy()
    }
    const rows = normalImportParameterRows(configuredForm(), 'MYSQL')
    expect(rows.find(row => row.name === 'nls-date-format')?.reason).toContain('Oracle')
    expect(rows.find(row => row.name === 'column-splitter')?.reason).toContain('当前非活动')
    expect(rows.find(row => row.name === 'compression-algo')?.label).toBe('压缩算法')
    expect(rows.find(row => row.name === 'character-set')).toMatchObject({ placement: 'derived', step: 1 })
    expect(rows.find(row => row.name === 'session-config')).toMatchObject({ placement: 'derived', step: 1 })
    expect(rows.find(row => row.name === 'block-size')?.description).toContain('逻辑')
  })

  it('初始不替用户选择内容、格式、范围与节点，未设置参数不写入活动配置', () => {
    const form = createNormalImportForm()
    expect([form.content, form.format, form.scope, form.nodeId]).toEqual(['', '', '', ''])
    expect(projectNormalImport(configuredForm(), facts).execution).toEqual({})
    expect(validateNormalImport(configuredForm(), facts)).toEqual([])
    const issues = validateNormalImport(form, {})
    expect(issues.map(issue => issue.field)).toEqual(expect.arrayContaining(['dataSourceId', 'database', 'nodeId', 'filePath', 'content', 'scope']))
    expect(issues.some(issue => issue.field === 'taskName')).toBe(false)
  })

  it('文件与格式问题共同定位第二步，后续修复目标仍对应各自页面', () => {
    const form = configuredForm()
    form.filePath = 'relative.csv'
    form.formats.CSV.columnQuote = 'xx'
    form.scope = ''
    form.execution.maxErrors = '-2'
    form.taskName = ''
    const issues = validateNormalImport(form, facts)
    expect(issues.filter(issue => issue.step === 2).map(issue => issue.field)).toEqual(['filePath', 'columnQuote'])
    expect(issues.find(issue => issue.field === 'scope')?.step).toBe(3)
    expect(issues.find(issue => issue.field === 'maxErrors')?.step).toBe(4)
    expect(issues.find(issue => issue.field === 'taskName')).toBeUndefined()
    expect(normalImportParameterCatalog.find(item => item.name === 'file-path')?.step).toBe(2)
    expect(normalImportParameterCatalog.find(item => item.name === 'column-quote')?.step).toBe(2)
  })

  it('外来旁路、导出专属与任务级连接覆盖不会进入普通导入活动配置', () => {
    const clean = configuredForm()
    const contaminated = Object.assign(configuredForm(), { direct: true, rpcPort: '2882', parallel: '8' })
    Object.assign(contaminated.execution, { direct: true, rpcPort: '2882', parallel: '8' })
    Object.assign(contaminated, { querySql: 'synthetic', where: 'synthetic', flashbackTimestamp: 'synthetic', fileSize: '1G', characterSet: 'synthetic', sessionConfig: 'synthetic' })
    Object.assign(contaminated.execution, { characterSet: 'synthetic', sessionConfig: 'synthetic', limit: '1', pageSize: '1' })
    expect(clean.execution).not.toHaveProperty('characterSet')
    expect(clean.execution).not.toHaveProperty('sessionConfig')
    expect(projectNormalImport(contaminated, facts)).toEqual(projectNormalImport(clean, facts))
    expect(validateNormalImport(contaminated, facts)).toEqual([])
  })

  it('三种格式分别暂存，隐藏值不污染活动配置，仅对象定义不带独立数据格式', () => {
    const state = parameters()
    const csv = state.form.formats.CSV
    csv.columnSeparator = '|~|'
    csv.skipHeader = true
    state.form.formats.CUT.columnSplitter = '^|'
    state.form.formats.CUT.skipFooter = true
    for (const format of importFormats) {
      state.form.format = format
      const config = state.configuration.value
      expect(config.format).toBe(format)
      expect(config.parser.columnSeparator).toBe(format === 'CSV' ? '|~|' : undefined)
      expect(config.parser.columnSplitter).toBe(format === 'CUT' ? '^|' : undefined)
      expect(config.parser.skipFooter).toBe(format === 'CUT' ? true : undefined)
    }
    state.form.format = 'CSV'
    expect(state.currentFormat.value).toBe(csv)
    expect(csv.columnSeparator).toBe('|~|')
    state.selectContent('DDL_ONLY')
    expect(state.configuration.value).toMatchObject({ format: '', parser: {} })
    expect(state.configuration.value.mapping).toBeUndefined()
  })

  it('gzip/snappy 立即清除等级，仅对象定义与关闭开关不携带压缩值', () => {
    const state = parameters()
    const csv = state.form.formats.CSV
    csv.compress = true
    csv.compressionLevel = '5'
    expect(state.configuration.value.parser.compressionLevel).toBe('5')
    for (const algorithm of ['gzip', 'snappy']) {
      csv.compressionAlgo = 'zstd'
      csv.compressionLevel = '5'
      csv.compressionAlgo = algorithm
      expect(csv.compressionLevel).toBe('')
      expect(state.configuration.value.parser.compressionLevel).toBeUndefined()
    }
    state.selectContent('DDL_ONLY')
    expect(state.configuration.value.parser.compress).toBeUndefined()
    state.selectContent('DATA_ONLY')
    csv.compress = false
    expect(state.configuration.value.parser).not.toHaveProperty('compressionAlgo')
    csv.compressionAlgo = 'unsupported'
    expect(validateNormalImport(state.form, facts).some(issue => issue.field === 'compressionAlgo')).toBe(false)
    csv.compress = true
    expect(validateNormalImport(state.form, facts).some(issue => issue.field === 'compressionAlgo')).toBe(true)
  })

  it('显式包含列与排除列择一，冲突值被校验阻断且不进入活动列名单', () => {
    const form = configuredForm()
    const csv = form.formats.CSV
    Object.assign(csv, { mapping: 'EXPLICIT', includeColumns: 'synthetic_id', excludeColumns: 'synthetic_other' })
    expect(validateNormalImport(form, facts).find(issue => issue.field === 'includeColumns')?.step).toBe(3)
    expect(projectNormalImport(form, facts).mapping).not.toHaveProperty('includeColumns')
    expect(projectNormalImport(form, facts).mapping).not.toHaveProperty('excludeColumns')
    csv.excludeColumns = ''
    expect(validateNormalImport(form, facts)).toEqual([])
    expect(projectNormalImport(form, facts).mapping).toMatchObject({ includeColumns: 'synthetic_id' })
    csv.includeColumns = ''
    csv.excludeColumns = 'synthetic_other'
    expect(validateNormalImport(form, facts)).toEqual([])
    expect(projectNormalImport(form, facts).mapping).toMatchObject({ excludeColumns: 'synthetic_other' })
  })

  it('仅对象定义不携带数据写入选项，线程和 JVM 资源仍可配置，切回数据后恢复校验', () => {
    const state = parameters()
    Object.assign(state.form.execution, { maxErrors: '-2', maxDiscards: '-2', strict: 'invalid', rw: '2', batch: '1.5', blockSize: '0', maxTps: '0', maxWaitTimeout: '0', thread: '2', memory: '2G' })
    expect(state.executionAdvancedCount.value).toBe(7)
    state.selectContent('DDL_ONLY')
    expect(state.configuration.value.execution).toEqual({ thread: '2', memory: '2G' })
    expect(state.executionAdvancedCount.value).toBe(2)
    expect(validateNormalImport(state.form, facts)).toEqual([])
    expect(normalImportParameterRows(state.form, 'MYSQL').find(row => row.name === 'batch')?.reason).toContain('需要包含数据')
    state.selectContent('DATA_ONLY')
    expect(validateNormalImport(state.form, facts).map(issue => issue.field)).toEqual(expect.arrayContaining(['maxErrors', 'maxDiscards', 'strict', 'rw', 'batch', 'blockSize', 'maxTps', 'maxWaitTimeout']))
  })

  it('自动映射、控制文件和显式名单从不同时投影，CUT 与 SQL 不接受自动映射', () => {
    const form = configuredForm()
    form.formats.CSV.mapping = 'CONTROL'
    expect(projectNormalImport(form, facts).mapping).toMatchObject({ kind: 'CONTROL', ctlPath: '' })
    expect(validateNormalImport(form, facts).some(issue => issue.field === 'ctlPath')).toBe(true)
    form.formats.CSV.ctlPath = 'E:\\synthetic-controls'
    expect(validateNormalImport(form, facts)).toEqual([])
    form.format = 'CSV'
    Object.assign(form.formats.CSV, { ctlPath: 'E:\\synthetic-controls', includeColumns: 'synthetic_id', excludeColumns: 'synthetic_other', mapping: 'CONTROL' })
    expect(projectNormalImport(form, facts).mapping).not.toHaveProperty('excludeColumns')
    form.formats.CSV.mapping = 'EXPLICIT'
    expect(projectNormalImport(form, facts).mapping).not.toHaveProperty('ctlPath')
    form.formats.CSV.mapping = 'AUTO'
    expect(projectNormalImport(form, facts).mapping).toEqual({ kind: 'AUTO' })
    for (const format of ['CUT', 'SQL'] as const) {
      form.format = format
      form.formats[format].mapping = 'AUTO'
      expect(validateNormalImport(form, facts).some(issue => issue.field === 'mapping')).toBe(true)
    }
  })

  it('未开放格式和 MIX 的外来值被阻断，不生成活动解析或映射配置', () => {
    for (const format of ['POS', 'ORC', 'PARQUET', 'AVRO', 'UNKNOWN']) {
      const form = Object.assign(configuredForm(), { format })
      expect(validateNormalImport(form as ReturnType<typeof createNormalImportForm>, facts).some(issue => issue.field === 'format')).toBe(true)
      expect(projectNormalImport(form as ReturnType<typeof createNormalImportForm>, facts)).toMatchObject({ format: '', parser: {} })
      expect(projectNormalImport(form as ReturnType<typeof createNormalImportForm>, facts).mapping).toBeUndefined()
    }
    const mixed = Object.assign(configuredForm(), { content: 'MIX' })
    expect(validateNormalImport(mixed as ReturnType<typeof createNormalImportForm>, facts).some(issue => issue.field === 'content')).toBe(true)
    expect(projectNormalImport(mixed as ReturnType<typeof createNormalImportForm>, facts)).toMatchObject({ content: '', format: '', parser: {} })
    for (const name of ['pos', 'orc', 'par', 'avro', 'mix', 'source-type']) {
      expect(normalImportParameterCatalog.find(item => item.name === name)).toBeUndefined()
    }
  })

  it('对象与兼容模式只投影适用值，全部范围不携带具体对象', () => {
    const form = configuredForm()
    form.content = 'DDL_AND_DATA'
    form.objects.sequence = 'synthetic_sequence'
    form.objects.view = 'synthetic_view'
    form.execution.nlsDateFormat = 'YYYY-MM-DD'
    expect(projectNormalImport(form, facts).objects).not.toHaveProperty('sequence')
    expect(projectNormalImport(form, facts).execution).not.toHaveProperty('nlsDateFormat')
    const oracle = { ...facts, source: normalImportSources[1]! }
    expect(projectNormalImport(form, oracle).objects).toHaveProperty('sequence', 'synthetic_sequence')
    form.scope = 'ALL'
    expect(projectNormalImport(form, oracle).objects).toEqual({})
    form.content = 'DATA_ONLY'
    form.scope = 'SPECIFIC'
    expect(projectNormalImport(form, oracle).objects).toEqual({ table: 'synthetic_table' })
  })

  it('路径不转换，错误平台、相对路径、URI 与控制字符被阻断', () => {
    for (const path of ['E:\\synthetic-input', 'E:/合成输入/文件.csv']) expect(isImportAbsolutePath(path, 'WINDOWS_AMD64')).toBe(true)
    for (const path of ['/var/synthetic-input', 'relative.csv', 's3://synthetic-bucket/prefix', '\\\\synthetic\\share', 'E:\\input\n']) expect(isImportAbsolutePath(path, 'WINDOWS_AMD64')).toBe(false)
    const form = configuredForm()
    form.filePath = 'E:\\合成输入\\文件.csv'
    expect(projectNormalImport(form, facts).filePath).toBe(form.filePath)
    expect(isImportAbsolutePath('/var/合成输入', 'LINUX_ARM64')).toBe(true)
    expect(isImportAbsolutePath('E:\\synthetic-input', 'LINUX_AMD64')).toBe(false)
    expect(isImportAbsolutePath('/var/synthetic-input')).toBe(false)
  })

  it('校验活动的解析、等级、错误与资源输入，非活动值不阻断', () => {
    const form = configuredForm()
    Object.assign(form.formats.CSV, { columnQuote: 'xx', nullString: 'same', emptyString: 'same', compress: true, compressionLevel: '23' })
    Object.assign(form.execution, { maxErrors: '-2', memory: '0G', batch: '1.5', rw: '2' })
    expect(validateNormalImport(form, facts).map(issue => issue.field)).toEqual(expect.arrayContaining(['columnQuote', 'emptyString', 'compressionLevel', 'maxErrors', 'memory', 'batch', 'rw']))
    form.content = 'DDL_ONLY'
    Object.assign(form.execution, { memory: '', batch: '', rw: '' })
    expect(validateNormalImport(form, facts)).toEqual([])
    expect(projectNormalImport(form, facts).execution).not.toHaveProperty('maxErrors')
  })

  it('风险说明绑定配置及引用修订，A → B → A 不能复活旧确认', () => {
    const state = parameters()
    state.form.cleanup = 'DELETE'
    state.form.objects.table = '*'
    expect(state.riskDescriptions.value.join('')).toContain('没有对应数据文件')
    state.acknowledgeRisk(true)
    expect(state.riskAcknowledged.value).toBe(true)
    state.form.filePath = 'E:\\synthetic-other'
    expect(state.riskAcknowledged.value).toBe(false)
    state.form.filePath = 'E:\\synthetic-import'
    expect(state.riskAcknowledged.value).toBe(false)
    state.acknowledgeRisk(true)
    state.selectedSource.value = { ...source, revision: 3 }
    expect(state.riskAcknowledged.value).toBe(false)
    state.acknowledgeRisk(true)
    state.selectedNode.value = { ...normalImportNode, revision: 2 }
    expect(state.riskAcknowledged.value).toBe(false)
    state.acknowledgeRisk(true)
    state.selectedSource.value = { ...state.selectedSource.value, credentialRevision: 2 }
    expect(state.riskAcknowledged.value).toBe(false)
  })

  it('账号角色定义需要风险确认，切换定义立即撤销旧确认', () => {
    const state = parameters()
    state.form.content = 'DDL_ONLY'
    state.form.objects['obj-user'] = 'synthetic_dormant_user'
    expect(state.riskDescriptions.value.join('')).toContain('账号或角色')
    state.acknowledgeRisk(true)
    expect(state.riskAcknowledged.value).toBe(true)
    state.form.objects['obj-user'] = ''
    state.selectedSource.value = { ...source, compatibilityMode: 'ORACLE' }
    state.form.objects.role = 'synthetic_role'
    expect(state.riskAcknowledged.value).toBe(false)
    expect(state.riskDescriptions.value.join('')).toContain('账号或角色')
    state.form.content = 'DATA_ONLY'
    expect(state.riskDescriptions.value.join('')).not.toContain('账号或角色')
  })

  it('内容、范围和数据源的破坏性清值必须确认，取消不改表单', () => {
    const state = parameters()
    state.form.content = 'DDL_AND_DATA'
    state.form.objects.view = 'synthetic_view'
    state.form.replaceObject = true
    state.selectContent('DATA_ONLY')
    expect(state.form.content).toBe('DDL_AND_DATA')
    state.cancelContentChange()
    expect(state.form.objects.view).toBe('synthetic_view')
    state.selectContent('DATA_ONLY')
    state.confirmContentChange()
    expect(state.form.objects.view).toBe('')
    expect(state.form.replaceObject).toBe(false)
    state.selectScope('ALL')
    expect(state.form.scope).toBe('SPECIFIC')
    state.cancelScopeChange()
    state.selectScope('ALL')
    state.confirmScopeChange()
    expect(state.form.objects.table).toBe('')
    state.selectSource(normalImportSources[1]!.id)
    expect(state.form.dataSourceId).toBe(source.id)
    state.cancelSourceChange()
    state.selectSource(normalImportSources[1]!.id)
    state.confirmSourceChange()
    expect(state.form.database).toBe('SYNTHETIC_SCHEMA')
    expect(state.form.dataSourceId).toBe(normalImportSources[1]!.id)
  })
})

describe('普通导入授权引用读取', () => {
  it('失败关闭，并过滤失效测试、禁用、未知兼容模式和非 ODP 数据源', () => {
    expect(normalImportSources.filter(isNormalImportSource)).toHaveLength(2)
    for (const patch of [{ state: 'DISABLED' }, { lastTestStatus: 'UNKNOWN' }, { connectionKind: 'DIRECT' }, { compatibilityMode: 'UNKNOWN' }]) expect(isNormalImportSource({ ...source, ...patch })).toBe(false)
  })

  it('较新结果不被旧成功、错误或 finally 覆盖', async () => {
    const scope = effectScope(); scopes.push(scope)
    const first = deferred<DataSourceSummary[]>()
    const second = deferred<DataSourceSummary[]>()
    const api = { listDataSources: vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise), listExecutionNodes: vi.fn() }
    const form = configuredForm()
    const refs = scope.run(() => useNormalImportReferences(form, api))!
    const old = refs.loadSources()
    const latest = refs.loadSources()
    second.resolve([source]); await latest
    first.reject(new Error('synthetic-old-failure')); await old
    expect(refs.sourceFailure.value).toBe('')
    expect(refs.selectedSource.value?.id).toBe(source.id)
    expect(refs.loadingSources.value).toBe(false)
    const pending = deferred<ExecutionNodeSummary[]>()
    api.listExecutionNodes.mockReturnValue(pending.promise)
    const read = refs.loadNodes()
    refs.resetReferences()
    pending.resolve([normalImportNode]); await read
    expect(refs.nodes.value).toEqual([])
    expect(refs.loadingNodes.value).toBe(false)
  })

  it('同 ID 改名保留绑定，失效与读取失败拒绝使用旧资格，卸载后不写回', async () => {
    const scope = effectScope(); scopes.push(scope)
    const form = configuredForm()
    const api = { listDataSources: vi.fn().mockResolvedValue([source]), listExecutionNodes: vi.fn().mockResolvedValue([normalImportNode]) }
    const refs = scope.run(() => useNormalImportReferences(form, api))!
    await refs.loadNodes()
    api.listExecutionNodes.mockResolvedValue([{ ...normalImportNode, displayName: '合成新名称' }])
    await refs.loadNodes()
    expect(form.nodeId).toBe(normalImportNode.id)
    expect(refs.selectedNode.value?.displayName).toBe('合成新名称')
    api.listExecutionNodes.mockRejectedValue(new Error('synthetic-read-failure'))
    await refs.loadNodes()
    expect(refs.selectedNode.value).toBeUndefined()
    api.listExecutionNodes.mockResolvedValue([{ ...normalImportNode, acceptsNewTasks: false }])
    await refs.loadNodes()
    expect(form.nodeId).toBe('')
    const pending = deferred<DataSourceSummary[]>()
    api.listDataSources.mockReturnValue(pending.promise)
    const read = refs.loadSources()
    scope.stop()
    pending.resolve([source]); await read; await nextTick()
    expect(refs.sources.value).toEqual([])
    expect(refs.loadingSources.value).toBe(false)
  })

  it('按平台策略重排节点仍保留选择，不为空选择补首项', async () => {
    const scope = effectScope(); scopes.push(scope); const form = configuredForm()
    const api = { listDataSources: vi.fn().mockResolvedValue([source]), listExecutionNodes: vi.fn().mockResolvedValue([{ ...normalImportNode, id: 'synthetic-first' }, normalImportNode]) }
    const refs = scope.run(() => useNormalImportReferences(form, api))!
    await refs.loadNodes(); expect(api.listExecutionNodes).toHaveBeenCalledWith({ forTaskSelection: true }); expect(form.nodeId).toBe(normalImportNode.id)
    form.nodeId = ''; await refs.loadNodes(); expect(form.nodeId).toBe('')
  })
})
