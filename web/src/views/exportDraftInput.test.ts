import { describe, expect, it } from 'vitest'

import { validateExportDraftInput, type ExportDraftFormValues } from './exportDraftInput'

const completeValues: ExportDraftFormValues = {
  dataSourceId: 'source-1',
  nodeId: 'node-1',
  platform: 'WINDOWS_AMD64',
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
  querySql: '',
  includeColumnNames: '',
  excludeColumnNames: '',
  excludeVirtualColumns: false,
  flashbackScn: '',
  flashbackTimestamp: '',
  thread: '',
  pageSize: '',
  parallelMacro: '',
  fetchSize: '',
  jvmMemory: '',
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
})
