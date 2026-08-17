import { describe, expect, it } from 'vitest'

import { validateExportDraftInput, type ExportDraftFormValues } from './exportDraftInput'

const completeValues: ExportDraftFormValues = {
  dataSourceId: 'source-1',
  nodeId: 'node-1',
  platform: 'WINDOWS_AMD64',
  compatibilityMode: 'MYSQL',
  database: 'synthetic_db',
  scopeKind: 'SPECIFIED',
  objectType: 'TABLE',
  objectNames: ['synthetic_table'],
  excludeTables: [],
  contentKind: 'DATA_ONLY',
  formatKind: 'CSV',
  trailDelimiter: false,
  removeNewline: false,
  columnSplitter: '',
  filePath: '/E:/workespace/output',
  logPath: '/E:/workespace/logs',
  skipCheckDir: true,
  outputKind: 'LOCAL',
  storageBucket: '',
  storagePath: '',
  storageEndpoint: '',
  storageRegion: '',
  tmpPath: '',
  storageCredentialId: '',
  storageCredentialRevision: 0,
  storageCredentialProvider: '',
  controlFilePath: '',
  skipHeader: false,
  columnSeparator: '',
  columnQuote: '',
  columnQuoteMode: '',
  escapeCharacter: '',
  lineSeparator: '',
  nullString: '',
  fileEncoding: '',
  withTrim: false,
  noNestedDir: false,
  maxFileSize: '',
  retainEmptyFiles: false,
  compress: false,
  compressionAlgo: '',
  compressionLevel: '',
  querySql: '',
  where: '',
  includeColumnNames: '',
  excludeColumnNames: '',
  excludeVirtualColumns: false,
  flashbackScn: '',
  flashbackTimestamp: '',
  snapshot: false,
  thread: '',
  pageSize: '',
  parallelMacro: '',
  fetchSize: '',
  jvmMemory: '',
  blockSize: '',
  dropObject: false,
  retainSchema: false,
  compactSchema: false,
  addExtraMessage: false,
  partition: '',
  excludeDataTypes: '',
  enableHiddenPk: false,
  dateValueFormat: '',
  timeValueFormat: '',
  datetimeValueFormat: '',
  timestampValueFormat: '',
  timestampTzValueFormat: '',
  timestampLtzValueFormat: '',
  nlsDateFormat: '',
  nlsTimestampFormat: '',
  nlsTimestampTzFormat: '',
}

describe('泛化导出草稿输入', () => {
  it('为指定单表 DATA_ONLY 生成冻结能力兼容的 v6 输入', () => {
    expect(validateExportDraftInput(completeValues)).toEqual({
      valid: true,
      input: {
        configVersion: 'v6',
        dataSourceId: 'source-1',
        nodeId: 'node-1',
        config: {
          objectScope: {
            database: 'synthetic_db',
            scopeKind: 'SPECIFIED',
            objectTypes: ['TABLE'],
            expressions: [{ name: 'synthetic_table' }],
            excludeTables: undefined,
          },
          contentSelection: { contentKind: 'DATA_ONLY' },
          dataFormat: { formatKind: 'CSV' },
          outputConfig: { outputKind: 'LOCAL', filePath: '/E:/workespace/output', logPath: '/E:/workespace/logs', skipCheckDir: true },
        },
      },
    })
  })

  it('支持多表、排除表、全部对象与 DDL 内容组合', () => {
    const multiTable = validateExportDraftInput({ ...completeValues, objectNames: ['table_one', 'table_two'], excludeTables: ['tmp_a'], contentKind: 'DDL_AND_DATA' })
    expect(multiTable).toMatchObject({ valid: true })
    if (multiTable.valid) {
      expect(multiTable.input.config.objectScope.expressions).toEqual([{ name: 'table_one' }, { name: 'table_two' }])
      expect(multiTable.input.config.objectScope.excludeTables).toEqual(['tmp_a'])
      expect(multiTable.input.config.dataFormat).toEqual({ formatKind: 'CSV' })
    }
    const allScope = validateExportDraftInput({ ...completeValues, scopeKind: 'ALL', objectNames: [] })
    expect(allScope).toMatchObject({ valid: true })
    if (allScope.valid) {
      expect(allScope.input.config.objectScope).toEqual({ database: 'synthetic_db', scopeKind: 'ALL' })
    }
    const ddlOnly = validateExportDraftInput({ ...completeValues, contentKind: 'DDL_ONLY' })
    expect(ddlOnly).toMatchObject({ valid: true })
    if (ddlOnly.valid) {
      expect(ddlOnly.input.config.dataFormat).toBeUndefined()
    }
  })

  it('支持 EX-I3 全量选项并执行互斥与边界校验', () => {
    const withOptions = validateExportDraftInput({
      ...completeValues,
      skipHeader: true,
      columnSeparator: '|',
      columnQuoteMode: 'minimal',
      escapeCharacter: '\\',
      withTrim: true,
      noNestedDir: true,
      maxFileSize: '1048576',
      retainEmptyFiles: true,
      compress: true,
      compressionAlgo: 'zstd',
      includeColumnNames: 'col_a,col_b',
      excludeVirtualColumns: true,
      flashbackScn: '100',
      thread: '4',
      pageSize: '1000',
      fetchSize: '100',
      jvmMemory: '4G',
    })
    expect(withOptions).toMatchObject({ valid: true })
    if (withOptions.valid) {
      expect(withOptions.input.config.dataFormat?.csvOptions).toMatchObject({ skipHeader: true, columnSeparator: '|', columnQuoteMode: 'minimal', escapeCharacter: '\\', withTrim: true })
      expect(withOptions.input.config.outputConfig).toMatchObject({ noNestedDir: true, maxFileSize: 1048576, retainEmptyFiles: true, compress: true, compressionAlgo: 'zstd' })
      expect(withOptions.input.config.filterConfig).toMatchObject({ includeColumnNames: ['col_a', 'col_b'], excludeVirtualColumns: true, flashbackScn: 100 })
      expect(withOptions.input.config.performanceConfig).toMatchObject({ thread: 4, pageSize: 1000, fetchSize: 100, jvmMemory: '4G' })
    }
    // 互斥与边界失败关闭。
    expect(validateExportDraftInput({ ...completeValues, querySql: 'select 1', flashbackScn: '100' })).toMatchObject({ valid: false, message: expect.stringContaining('互斥') })
    expect(validateExportDraftInput({ ...completeValues, includeColumnNames: 'a', excludeColumnNames: 'b' })).toMatchObject({ valid: false, message: expect.stringContaining('互斥') })
    expect(validateExportDraftInput({ ...completeValues, compress: true, compressionAlgo: '' })).toMatchObject({ valid: false, message: expect.stringContaining('压缩算法') })
    expect(validateExportDraftInput({ ...completeValues, escapeCharacter: 'ab' })).toMatchObject({ valid: false, message: expect.stringContaining('单字符') })
    expect(validateExportDraftInput({ ...completeValues, maxFileSize: '0' })).toMatchObject({ valid: false, message: expect.stringContaining('正整数') })
    expect(validateExportDraftInput({ ...completeValues, jvmMemory: '4GX' })).toMatchObject({ valid: false, message: expect.stringContaining('K/M/G/T') })
    // 仅 DDL 时数据选项不进入请求。
    const ddlOnly = validateExportDraftInput({ ...completeValues, contentKind: 'DDL_ONLY', querySql: 'select 1', compress: true, compressionAlgo: 'zstd' })
    expect(ddlOnly).toMatchObject({ valid: true })
    if (ddlOnly.valid) {
      expect(ddlOnly.input.config.dataFormat).toBeUndefined()
      expect(ddlOnly.input.config.filterConfig).toBeUndefined()
      expect(ddlOnly.input.config.performanceConfig).toBeUndefined()
    }
  })

  it('为 CUT 格式生成共享文本与 CUT 专属选项，并发送通用文件布局/筛选/性能选项', () => {
    const cut = validateExportDraftInput({
      ...completeValues,
      formatKind: 'CUT',
      trailDelimiter: true,
      removeNewline: true,
      columnSplitter: '|',
      escapeCharacter: '\\',
      lineSeparator: '\\n',
      nullString: 'NULL',
      fileEncoding: 'UTF-8',
      withTrim: true,
      compress: true,
      compressionAlgo: 'zstd',
      noNestedDir: true,
      maxFileSize: '1048576',
      querySql: 'select 1',
      includeColumnNames: 'col_a,col_b',
      thread: '4',
      pageSize: '1000',
      jvmMemory: '4G',
    })
    expect(cut).toMatchObject({ valid: true })
    if (cut.valid) {
      expect(cut.input.config.dataFormat).toMatchObject({
        formatKind: 'CUT',
        cutOptions: { trailDelimiter: true, removeNewline: true },
        csvOptions: { columnSplitter: '|', escapeCharacter: '\\', lineSeparator: '\\n', nullString: 'NULL', fileEncoding: 'UTF-8', withTrim: true },
      })
      expect(cut.input.config.dataFormat?.csvOptions).not.toHaveProperty('skipHeader')
      // 官方复核：文件布局、筛选与性能选项不限定格式，CUT 下同样发送。
      expect(cut.input.config.outputConfig).toMatchObject({ noNestedDir: true, maxFileSize: 1048576, compress: true, compressionAlgo: 'zstd' })
      expect(cut.input.config.filterConfig).toMatchObject({ querySql: 'select 1', includeColumnNames: ['col_a', 'col_b'] })
      expect(cut.input.config.performanceConfig).toMatchObject({ thread: 4, pageSize: 1000, jvmMemory: '4G' })
    }
    // CUT 转义字符仍限单字符；文本选项长度受限。
    expect(validateExportDraftInput({ ...completeValues, formatKind: 'CUT', escapeCharacter: 'ab' })).toMatchObject({ valid: false, message: expect.stringContaining('单字符') })
    expect(validateExportDraftInput({ ...completeValues, formatKind: 'CUT', lineSeparator: 'x'.repeat(257) })).toMatchObject({ valid: false, message: expect.stringContaining('256') })
    // CUT 下 CSV 专属选项不发送；压缩与通用选项互斥校验仍然生效。
    const cutWithCsvOnly = validateExportDraftInput({ ...completeValues, formatKind: 'CUT', skipHeader: true, columnSeparator: '|', columnQuoteMode: 'minimal' })
    expect(cutWithCsvOnly).toMatchObject({ valid: true })
    if (cutWithCsvOnly.valid) {
      expect(cutWithCsvOnly.input.config.dataFormat?.csvOptions).toBeUndefined()
    }
    expect(validateExportDraftInput({ ...completeValues, formatKind: 'CUT', compress: true, compressionAlgo: '' })).toMatchObject({ valid: false, message: expect.stringContaining('压缩算法') })
    expect(validateExportDraftInput({ ...completeValues, formatKind: 'CUT', querySql: 'select 1', flashbackScn: '100' })).toMatchObject({ valid: false, message: expect.stringContaining('互斥') })
  })

  it('为 SQL 格式只发送行分隔符与文件编码，并发送通用文件布局/筛选/性能选项', () => {
    const sql = validateExportDraftInput({
      ...completeValues,
      formatKind: 'SQL',
      lineSeparator: '\\n',
      fileEncoding: 'UTF-8',
      escapeCharacter: '\\',
      nullString: 'NULL',
      withTrim: true,
      compress: true,
      compressionAlgo: 'gzip',
      retainEmptyFiles: true,
      flashbackScn: '100',
      excludeColumnNames: 'col_c',
      thread: '4',
      fetchSize: '100',
      jvmMemory: '4G',
    })
    expect(sql).toMatchObject({ valid: true })
    if (sql.valid) {
      expect(sql.input.config.dataFormat).toEqual({ formatKind: 'SQL', csvOptions: { lineSeparator: '\\n', fileEncoding: 'UTF-8' } })
      expect(sql.input.config.dataFormat?.cutOptions).toBeUndefined()
      // 官方复核：文件布局、筛选与性能选项不限定格式，SQL 下同样发送。
      expect(sql.input.config.outputConfig).toMatchObject({ retainEmptyFiles: true, compress: true, compressionAlgo: 'gzip' })
      expect(sql.input.config.filterConfig).toMatchObject({ flashbackScn: 100, excludeColumnNames: ['col_c'] })
      expect(sql.input.config.performanceConfig).toMatchObject({ thread: 4, fetchSize: 100, jvmMemory: '4G' })
    }
    // SQL 下转义字符、NULL 替换与去除空格不参与校验也不发送。
    const sqlIgnored = validateExportDraftInput({ ...completeValues, formatKind: 'SQL', escapeCharacter: 'ab', nullString: 'NULL', withTrim: true })
    expect(sqlIgnored).toMatchObject({ valid: true })
    if (sqlIgnored.valid) {
      expect(sqlIgnored.input.config.dataFormat?.csvOptions).toBeUndefined()
    }
    // SQL 文本选项长度受限；压缩与通用选项互斥校验仍然生效。
    expect(validateExportDraftInput({ ...completeValues, formatKind: 'SQL', fileEncoding: 'x'.repeat(257) })).toMatchObject({ valid: false, message: expect.stringContaining('256') })
    expect(validateExportDraftInput({ ...completeValues, formatKind: 'SQL', compress: true, compressionAlgo: '' })).toMatchObject({ valid: false, message: expect.stringContaining('压缩算法') })
    expect(validateExportDraftInput({ ...completeValues, formatKind: 'SQL', includeColumnNames: 'a', excludeColumnNames: 'b' })).toMatchObject({ valid: false, message: expect.stringContaining('互斥') })
    expect(validateExportDraftInput({ ...completeValues, formatKind: 'SQL', maxFileSize: '0' })).toMatchObject({ valid: false, message: expect.stringContaining('正整数') })
  })

  it('为 POS 格式要求控制文件目录并发送通用选项，其他格式不携带控制文件路径', () => {
    // POS 缺少控制文件目录时阻断。
    expect(validateExportDraftInput({ ...completeValues, formatKind: 'POS' })).toMatchObject({ valid: false, message: expect.stringContaining('控制文件目录') })
    expect(validateExportDraftInput({ ...completeValues, formatKind: 'POS', controlFilePath: 'E:\\workespace\\controls' })).toMatchObject({ valid: false, message: expect.stringContaining('/E:/exports') })
    const pos = validateExportDraftInput({
      ...completeValues,
      formatKind: 'POS',
      controlFilePath: '/E:/workespace/controls',
      compress: true,
      compressionAlgo: 'zstd',
      noNestedDir: true,
      querySql: 'select 1',
      thread: '4',
      jvmMemory: '4G',
    })
    expect(pos).toMatchObject({ valid: true })
    if (pos.valid) {
      expect(pos.input.config.dataFormat).toEqual({ formatKind: 'POS' })
      // EX-I4 POS 定版：控制文件目录随 outputConfig 发送，序列化选项不适用。
      expect(pos.input.config.outputConfig).toMatchObject({ controlFilePath: '/E:/workespace/controls', compress: true, compressionAlgo: 'zstd', noNestedDir: true })
      expect(pos.input.config.filterConfig).toMatchObject({ querySql: 'select 1' })
      expect(pos.input.config.performanceConfig).toMatchObject({ thread: 4, jvmMemory: '4G' })
    }
    // 非 POS 格式不发送控制文件路径。
    const csv = validateExportDraftInput({ ...completeValues, formatKind: 'CSV', controlFilePath: '/E:/workespace/controls' })
    expect(csv).toMatchObject({ valid: true })
    if (csv.valid) {
      expect(csv.input.config.outputConfig.controlFilePath).toBeUndefined()
    }
  })

  it('为 Parquet/ORC/Avro 结构化格式只发送文件编码与通用选项，并拒绝压缩与序列化选项', () => {
    for (const formatKind of ['PARQUET', 'ORC', 'AVRO'] as const) {
      const structured = validateExportDraftInput({
        ...completeValues,
        formatKind,
        fileEncoding: 'UTF-8',
        compress: true,
        compressionAlgo: 'zstd',
        escapeCharacter: '\\',
        withTrim: true,
      })
      expect(structured).toMatchObject({ valid: false, message: expect.stringContaining('压缩') })
      const clean = validateExportDraftInput({ ...completeValues, formatKind, fileEncoding: 'UTF-8', noNestedDir: true, querySql: 'select 1', thread: '4', jvmMemory: '4G' })
      expect(clean).toMatchObject({ valid: true })
      if (clean.valid) {
        expect(clean.input.config.dataFormat).toEqual({ formatKind, csvOptions: { fileEncoding: 'UTF-8' } })
        expect(clean.input.config.outputConfig).toMatchObject({ noNestedDir: true })
        expect(clean.input.config.outputConfig.compress).toBeUndefined()
        expect(clean.input.config.filterConfig).toMatchObject({ querySql: 'select 1' })
        expect(clean.input.config.performanceConfig).toMatchObject({ thread: 4, jvmMemory: '4G' })
      }
      // 非结构化格式发送压缩；结构化格式文件编码长度仍受限。
      expect(validateExportDraftInput({ ...completeValues, formatKind, fileEncoding: 'x'.repeat(257) })).toMatchObject({ valid: false, message: expect.stringContaining('256') })
    }
  })

  it('为对象存储输出拼装受控 URI 并发送临时分块目录，拒绝缺字段与本地路径混用', () => {
    // 缺 Bucket / 路径非 / 开头 / 缺 Endpoint 与 Region 均阻断。
    expect(validateExportDraftInput({ ...completeValues, outputKind: 'OSS', storagePath: '/exports', storageEndpoint: 'oss-cn-hangzhou.aliyuncs.com' })).toMatchObject({ valid: false, message: expect.stringContaining('Bucket') })
    expect(validateExportDraftInput({ ...completeValues, outputKind: 'S3', storageBucket: 'bucket', storageEndpoint: 's3.example.com' })).toMatchObject({ valid: false, message: expect.stringContaining('/ 开头') })
    expect(validateExportDraftInput({ ...completeValues, outputKind: 'COS', storageBucket: 'bucket', storagePath: '/exports' })).toMatchObject({ valid: false, message: expect.stringContaining('Endpoint') })
    const oss = validateExportDraftInput({
      ...completeValues,
      outputKind: 'OSS',
      storageBucket: 'my-bucket',
      storagePath: '/exports/daily',
      storageEndpoint: 'oss-cn-hangzhou-internal.aliyuncs.com',
      storageRegion: 'cn-hangzhou',
      tmpPath: '/E:/workespace/tmp/upload',
      formatKind: 'CSV',
    })
    expect(oss).toMatchObject({ valid: true })
    if (oss.valid) {
      expect(oss.input.config.outputConfig.outputKind).toBe('OSS')
      expect(oss.input.config.outputConfig.filePath).toBe('oss://my-bucket/exports/daily?endpoint=oss-cn-hangzhou-internal.aliyuncs.com&region=cn-hangzhou')
      expect(oss.input.config.outputConfig.tmpPath).toBe('/E:/workespace/tmp/upload')
    }
    // 本地输出不允许 URI 形态的导出路径。
    expect(validateExportDraftInput({ ...completeValues, outputKind: 'LOCAL', filePath: 'oss://bucket/path' })).toMatchObject({ valid: false, message: expect.stringContaining('/E:/exports') })
    // 临时分块目录必须是节点绝对路径。
    expect(validateExportDraftInput({ ...completeValues, outputKind: 'OBS', storageBucket: 'bucket', storagePath: '/exports', storageRegion: 'cn-north-1', tmpPath: 'relative/tmp' })).toMatchObject({ valid: false, message: expect.stringContaining('临时分块目录') })
  })

  it('对象存储输出按需绑定存储凭据引用，本地输出与 provider 不一致失败关闭', () => {
    // 显式绑定：只发送标识与当前修订，不含任何密钥材料。
    const bound = validateExportDraftInput({
      ...completeValues,
      outputKind: 'OSS',
      storageBucket: 'my-bucket',
      storagePath: '/exports',
      storageEndpoint: 'oss-cn-hangzhou.aliyuncs.com',
      storageCredentialId: 'storage-1',
      storageCredentialRevision: 3,
      storageCredentialProvider: 'OSS',
    })
    expect(bound).toMatchObject({ valid: true })
    if (bound.valid) {
      expect(bound.input.config.outputConfig.storageCredential).toEqual({ storageCredentialId: 'storage-1', revision: 3 })
    }
    // 本地输出携带引用必须失败关闭（与服务端 422 同口径）。
    expect(validateExportDraftInput({ ...completeValues, storageCredentialId: 'storage-1', storageCredentialRevision: 1, storageCredentialProvider: 'OSS' })).toMatchObject({ valid: false, message: expect.stringContaining('本地输出不能绑定') })
    // provider 与输出类型不一致、缺修订均失败关闭。
    expect(validateExportDraftInput({ ...completeValues, outputKind: 'OSS', storageBucket: 'bucket', storagePath: '/exports', storageEndpoint: 'endpoint', storageCredentialId: 'storage-1', storageCredentialRevision: 1, storageCredentialProvider: 'S3' })).toMatchObject({ valid: false, message: expect.stringContaining('不一致') })
    expect(validateExportDraftInput({ ...completeValues, outputKind: 'OSS', storageBucket: 'bucket', storagePath: '/exports', storageEndpoint: 'endpoint', storageCredentialId: 'storage-1', storageCredentialRevision: 0, storageCredentialProvider: 'OSS' })).toMatchObject({ valid: false, message: expect.stringContaining('修订') })
  })

  it('拒绝空字段、非法对象名、视图导出数据和不匹配平台的输出路径', () => {
    expect(validateExportDraftInput({ ...completeValues, dataSourceId: '' })).toMatchObject({ valid: false, message: expect.stringContaining('数据源') })
    expect(validateExportDraftInput({ ...completeValues, objectNames: [''] })).toMatchObject({ valid: false, message: expect.stringContaining('至少填写') })
    expect(validateExportDraftInput({ ...completeValues, objectNames: ['table_*'] })).toMatchObject({ valid: false, message: expect.stringContaining('通配符') })
    expect(validateExportDraftInput({ ...completeValues, objectNames: ['a,b'] })).toMatchObject({ valid: false, message: expect.stringContaining('通配符') })
    expect(validateExportDraftInput({ ...completeValues, objectType: 'VIEW', contentKind: 'DATA_ONLY' })).toMatchObject({ valid: false, message: expect.stringContaining('视图') })
    expect(validateExportDraftInput({ ...completeValues, objectType: 'VIEW', contentKind: 'DDL_ONLY' })).toMatchObject({ valid: true })
    expect(validateExportDraftInput({ ...completeValues, filePath: 'E:\\workespace\\output' })).toMatchObject({ valid: false, message: expect.stringContaining('/E:/exports') })
    expect(validateExportDraftInput({ ...completeValues, logPath: 'E:\\workespace\\logs' })).toMatchObject({ valid: false, message: expect.stringContaining('日志路径') })
    expect(validateExportDraftInput({ ...completeValues, platform: 'LINUX_ARM64', filePath: '/var/output' })).toMatchObject({ valid: true })
  })

  it('POS → DDL_ONLY 回退后不校验、不发送 controlFilePath 且清除数据格式残留', () => {
    // 仅 DDL 时不要求 POS 控制文件目录（残留值为空也通过）。
    expect(validateExportDraftInput({ ...completeValues, contentKind: 'DDL_ONLY', formatKind: 'POS', controlFilePath: '' })).toMatchObject({ valid: true })
    // 即使残留控制文件目录，仅 DDL 内容也不发送到请求。
    const posToDdlOnly = validateExportDraftInput({
      ...completeValues,
      contentKind: 'DDL_ONLY',
      formatKind: 'POS',
      controlFilePath: '/E:/tmp/controls',
      trailDelimiter: true,
      columnSplitter: '|',
    })
    expect(posToDdlOnly).toMatchObject({ valid: true })
    if (posToDdlOnly.valid) {
      expect(posToDdlOnly.input.config.dataFormat).toBeUndefined()
      expect(posToDdlOnly.input.config.outputConfig.controlFilePath).toBeUndefined()
    }
  })

  it('CUT → DDL_AND_DATA 回退后页面校验阻断（DDL + 数据只支持 CSV）', () => {
    expect(validateExportDraftInput({ ...completeValues, contentKind: 'DDL_AND_DATA', formatKind: 'CUT', columnSplitter: '|' })).toMatchObject({
      valid: false,
      message: expect.stringContaining('DDL + 数据只支持 CSV'),
    })
    // DDL + 数据保持 CSV 时正常通过。
    expect(validateExportDraftInput({ ...completeValues, contentKind: 'DDL_AND_DATA', formatKind: 'CSV' })).toMatchObject({ valid: true })
  })

  it('EX-I7 DDL 行为：仅 DDL 内容发送 drop-object/retain-schema，仅数据内容阻断', () => {
    const ddlOnly = validateExportDraftInput({ ...completeValues, contentKind: 'DDL_ONLY', dropObject: true, retainSchema: true })
    expect(ddlOnly).toMatchObject({ valid: true })
    if (ddlOnly.valid) {
      expect(ddlOnly.input.config.ddlBehavior).toEqual({ dropObject: true, retainSchema: true })
      expect(ddlOnly.input.config.dataFormat).toBeUndefined()
    }
    const ddlAndData = validateExportDraftInput({ ...completeValues, contentKind: 'DDL_AND_DATA', dropObject: true })
    expect(ddlAndData).toMatchObject({ valid: true })
    if (ddlAndData.valid) {
      expect(ddlAndData.input.config.ddlBehavior).toEqual({ dropObject: true })
    }
    // 仅数据内容携带 DDL 行为即阻断，未设置时不发送。
    expect(validateExportDraftInput({ ...completeValues, contentKind: 'DATA_ONLY', dropObject: true })).toMatchObject({ valid: false, message: expect.stringContaining('DDL 行为参数仅在导出 DDL 内容时生效') })
    const dataOnlyClean = validateExportDraftInput({ ...completeValues, contentKind: 'DATA_ONLY' })
    expect(dataOnlyClean).toMatchObject({ valid: true })
    if (dataOnlyClean.valid) {
      expect(dataOnlyClean.input.config.ddlBehavior).toBeUndefined()
    }
  })

  it('EX-I7 文件拆分：--block-size 接受正整数与 MB/ROW 后缀并随性能配置发送', () => {
    const valid = validateExportDraftInput({ ...completeValues, blockSize: '256ROW' })
    expect(valid).toMatchObject({ valid: true })
    if (valid.valid) {
      expect(valid.input.config.performanceConfig).toMatchObject({ blockSize: '256ROW' })
    }
    expect(validateExportDraftInput({ ...completeValues, blockSize: '1024' })).toMatchObject({ valid: true })
    expect(validateExportDraftInput({ ...completeValues, blockSize: '1024MB' })).toMatchObject({ valid: true })
    expect(validateExportDraftInput({ ...completeValues, blockSize: '1GB' })).toMatchObject({ valid: false, message: expect.stringContaining('文件拆分') })
    expect(validateExportDraftInput({ ...completeValues, blockSize: '0' })).toMatchObject({ valid: false, message: expect.stringContaining('文件拆分') })
    expect(validateExportDraftInput({ ...completeValues, blockSize: 'abc' })).toMatchObject({ valid: false, message: expect.stringContaining('文件拆分') })
    expect(validateExportDraftInput({ ...completeValues, formatKind: 'PARQUET', blockSize: '1024MB' })).toMatchObject({ valid: false, message: expect.stringContaining('结构化格式') })
  })

  it('EX-I7 压缩等级：按算法分范围（zstd 1~22、zlib -1~9；gzip/snappy 不支持）', () => {
    const zstd = validateExportDraftInput({ ...completeValues, compress: true, compressionAlgo: 'zstd', compressionLevel: '5' })
    expect(zstd).toMatchObject({ valid: true })
    if (zstd.valid) {
      expect(zstd.input.config.outputConfig).toMatchObject({ compress: true, compressionAlgo: 'zstd', compressionLevel: 5 })
    }
    expect(validateExportDraftInput({ ...completeValues, compress: true, compressionAlgo: 'zlib', compressionLevel: '-1' })).toMatchObject({ valid: true })
    expect(validateExportDraftInput({ ...completeValues, compress: true, compressionAlgo: 'zstd', compressionLevel: '0' })).toMatchObject({ valid: false, message: expect.stringContaining('1~22') })
    expect(validateExportDraftInput({ ...completeValues, compress: true, compressionAlgo: 'zstd', compressionLevel: '23' })).toMatchObject({ valid: false, message: expect.stringContaining('1~22') })
    expect(validateExportDraftInput({ ...completeValues, compress: true, compressionAlgo: 'zlib', compressionLevel: '10' })).toMatchObject({ valid: false, message: expect.stringContaining('-1~9') })
    expect(validateExportDraftInput({ ...completeValues, compress: true, compressionAlgo: 'gzip', compressionLevel: '5' })).toMatchObject({ valid: false, message: expect.stringContaining('不支持指定压缩等级') })
    expect(validateExportDraftInput({ ...completeValues, compress: false, compressionAlgo: '', compressionLevel: '5' })).toMatchObject({ valid: false, message: expect.stringContaining('先启用压缩') })
  })

  it('仅为指定表发送条件筛选，并拒绝全部对象或视图残留', () => {
    const table = validateExportDraftInput({ ...completeValues, where: 'id > 100' })
    expect(table).toMatchObject({ valid: true })
    if (table.valid) {
      expect(table.input.config.filterConfig).toMatchObject({ where: 'id > 100' })
    }
    expect(validateExportDraftInput({ ...completeValues, scopeKind: 'ALL', objectNames: [], where: 'id > 100' })).toMatchObject({ valid: false, message: expect.stringContaining('指定表') })
    expect(validateExportDraftInput({ ...completeValues, objectType: 'VIEW', contentKind: 'DDL_ONLY', where: 'id > 100' })).toMatchObject({ valid: false, message: expect.stringContaining('指定表') })
    expect(validateExportDraftInput({ ...completeValues, querySql: 'select 1', where: 'id > 0' })).toMatchObject({ valid: false, message: expect.stringContaining('互斥') })
  })

  it('仅为包含表 DDL 的范围发送紧凑 Schema，并拒绝纯视图残留', () => {
    const specifiedTable = validateExportDraftInput({ ...completeValues, contentKind: 'DDL_ONLY', compactSchema: true })
    expect(specifiedTable).toMatchObject({ valid: true })
    if (specifiedTable.valid) {
      expect(specifiedTable.input.config.ddlBehavior).toMatchObject({ compactSchema: true })
    }
    const allObjects = validateExportDraftInput({ ...completeValues, scopeKind: 'ALL', objectNames: [], contentKind: 'DDL_ONLY', compactSchema: true })
    expect(allObjects).toMatchObject({ valid: true })
    if (allObjects.valid) {
      expect(allObjects.input.config.ddlBehavior).toMatchObject({ compactSchema: true })
    }
    expect(validateExportDraftInput({ ...completeValues, objectType: 'VIEW', contentKind: 'DDL_ONLY', compactSchema: true })).toMatchObject({ valid: false, message: expect.stringContaining('紧凑 Schema') })
    expect(validateExportDraftInput({ ...completeValues, compactSchema: true })).toMatchObject({ valid: false, message: expect.stringContaining('紧凑 Schema') })
  })

  it('快照与闪回参数互斥，并忽略旧表单残留的 weakRead 与 retry', () => {
    const snapshot = validateExportDraftInput({ ...completeValues, snapshot: true })
    expect(snapshot).toMatchObject({ valid: true })
    if (snapshot.valid) {
      expect(snapshot.input.config.filterConfig).toMatchObject({ snapshot: true })
    }
    expect(validateExportDraftInput({ ...completeValues, snapshot: true, flashbackScn: '100' })).toMatchObject({ valid: false, message: expect.stringContaining('一致性快照') })
    expect(validateExportDraftInput({ ...completeValues, snapshot: true, flashbackTimestamp: '2026-08-06 00:00:00' })).toMatchObject({ valid: false, message: expect.stringContaining('一致性快照') })

    // 运行时旧页面残留也不能绕过构造器重新发送已关闭的参数。
    const legacyResidue = validateExportDraftInput({
      ...completeValues,
      snapshot: true,
      thread: '4',
      weakRead: true,
      retry: true,
    } as unknown as ExportDraftFormValues)
    expect(legacyResidue).toMatchObject({ valid: true })
    if (legacyResidue.valid) {
      expect(legacyResidue.input.config.filterConfig).toMatchObject({ snapshot: true })
      expect(legacyResidue.input.config.filterConfig).not.toHaveProperty('weakRead')
      expect(legacyResidue.input.config.performanceConfig).toMatchObject({ thread: 4 })
      expect(legacyResidue.input.config.performanceConfig).not.toHaveProperty('retry')
    }
  })

  // 分区筛选与类型排除已验证；隐藏主键必须在专用预检查完成前保持浏览器门禁。
  it('为指定表数据发送分区筛选与类型排除，并拒绝越界与隐藏主键残留', () => {
    const table = validateExportDraftInput({ ...completeValues, partition: 'p0,p2', excludeDataTypes: 'BLOB,decimal' })
    expect(table).toMatchObject({ valid: true })
    if (table.valid) {
      expect(table.input.config.filterConfig).toMatchObject({ partition: 'p0,p2', excludeDataTypes: ['BLOB', 'decimal'] })
      expect(table.input.config.filterConfig).not.toHaveProperty('enableHiddenPk')
    }
    // 分区筛选仅指定表数据：全部对象、视图与仅 DDL 均拒绝。
    expect(validateExportDraftInput({ ...completeValues, scopeKind: 'ALL', objectNames: [], partition: 'p0' })).toMatchObject({ valid: false, message: expect.stringContaining('分区筛选') })
    expect(validateExportDraftInput({ ...completeValues, contentKind: 'DDL_ONLY', partition: 'p0' })).toMatchObject({ valid: false, message: expect.stringContaining('分区筛选') })
    // 分区名与自定义查询互斥；非法分区名与类型名拒绝。
    expect(validateExportDraftInput({ ...completeValues, partition: 'p0', querySql: 'select 1' })).toMatchObject({ valid: false, message: expect.stringContaining('互斥') })
    expect(validateExportDraftInput({ ...completeValues, partition: 'p0; DROP' })).toMatchObject({ valid: false, message: expect.stringContaining('分区名') })
    expect(validateExportDraftInput({ ...completeValues, excludeDataTypes: 'BLOB;DROP' })).toMatchObject({ valid: false, message: expect.stringContaining('数据类型名') })
    expect(validateExportDraftInput({ ...completeValues, enableHiddenPk: true })).toMatchObject({ valid: false, message: expect.stringContaining('隐藏主键') })
  })

  it('仅为 MySQL CSV/CUT 数据内容发送 DATE/DATETIME 时间格式，并拒绝非 ASCII 空白与未验证字段', () => {
    const csv = validateExportDraftInput({ ...completeValues, dateValueFormat: 'yyyy/MM/dd', datetimeValueFormat: 'yyyy/MM/dd HH:mm:ss' })
    expect(csv).toMatchObject({ valid: true })
    if (csv.valid) {
      expect(csv.input.config.dataFormat).toMatchObject({ timestampFormats: { dateValueFormat: 'yyyy/MM/dd', datetimeValueFormat: 'yyyy/MM/dd HH:mm:ss' } })
    }
    const cut = validateExportDraftInput({ ...completeValues, formatKind: 'CUT', dateValueFormat: 'yyyy/MM/dd' })
    expect(cut).toMatchObject({ valid: true })
    // SQL、仅 DDL 与 Oracle 均不能携带时间格式；格式串拒绝注入字符与制表符、回车、换行。
    expect(validateExportDraftInput({ ...completeValues, formatKind: 'SQL', dateValueFormat: 'yyyy/MM/dd' })).toMatchObject({ valid: false, message: expect.stringContaining('MySQL') })
    expect(validateExportDraftInput({ ...completeValues, contentKind: 'DDL_ONLY', dateValueFormat: 'yyyy/MM/dd' })).toMatchObject({ valid: false, message: expect.stringContaining('MySQL') })
    expect(validateExportDraftInput({ ...completeValues, datetimeValueFormat: 'yyyy;DROP' })).toMatchObject({ valid: false, message: expect.stringContaining('不支持的字符') })
    for (const invalidWhitespace of ['\t', '\r', '\n']) {
      expect(validateExportDraftInput({ ...completeValues, dateValueFormat: `yyyy${invalidWhitespace}MM` })).toMatchObject({ valid: false, message: expect.stringContaining('ASCII 空格') })
      expect(validateExportDraftInput({ ...completeValues, dateValueFormat: invalidWhitespace })).toMatchObject({ valid: false, message: expect.stringContaining('ASCII 空格') })
    }
    expect(validateExportDraftInput({ ...completeValues, dateValueFormat: ' yyyy/MM/dd' })).toMatchObject({ valid: false, message: expect.stringContaining('首尾') })
    expect(validateExportDraftInput({ ...completeValues, dateValueFormat: 'yyyy/MM/dd ' })).toMatchObject({ valid: false, message: expect.stringContaining('首尾') })
    for (const unverifiedTimestampFormat of [
      { timeValueFormat: 'HH:mm:ss' },
      { timestampValueFormat: 'yyyy-MM-dd HH:mm:ss' },
      { timestampTzValueFormat: 'yyyy-MM-dd HH:mm:ss TZD' },
      { timestampLtzValueFormat: 'yyyy-MM-dd HH:mm:ss' },
      { nlsDateFormat: 'yyyy-MM-dd' },
      { nlsTimestampFormat: 'yyyy-MM-dd HH:mm:ss' },
      { nlsTimestampTzFormat: 'yyyy-MM-dd HH:mm:ss TZD' },
    ] as const) {
      expect(validateExportDraftInput({ ...completeValues, ...unverifiedTimestampFormat })).toMatchObject({ valid: false, message: expect.stringContaining('DATE 与 DATETIME') })
    }
    for (const oracleTimestampFormat of [
      { dateValueFormat: 'yyyy/MM/dd' },
      { timeValueFormat: 'HH:mm:ss' },
      { datetimeValueFormat: 'yyyy/MM/dd HH:mm:ss' },
      { timestampValueFormat: 'yyyy-MM-dd HH:mm:ss' },
      { timestampTzValueFormat: 'yyyy-MM-dd HH:mm:ss TZD' },
      { timestampLtzValueFormat: 'yyyy-MM-dd HH:mm:ss' },
      { nlsDateFormat: 'yyyy-MM-dd' },
      { nlsTimestampFormat: 'yyyy-MM-dd HH:mm:ss' },
      { nlsTimestampTzFormat: 'yyyy-MM-dd HH:mm:ss TZD' },
    ] as const) {
      expect(validateExportDraftInput({ ...completeValues, compatibilityMode: 'ORACLE', ...oracleTimestampFormat })).toMatchObject({ valid: false, message: expect.stringContaining('MySQL') })
    }
  })

  it('附加对象信息在 sys 权限预检查完成前对所有内容组合失败关闭', () => {
    for (const contentKind of ['DATA_ONLY', 'DDL_ONLY', 'DDL_AND_DATA'] as const) {
      expect(validateExportDraftInput({ ...completeValues, contentKind, addExtraMessage: true })).toMatchObject({
        valid: false,
        message: expect.stringContaining('sys 权限预检查'),
      })
    }
  })
})
