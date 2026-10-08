import type { ExportDraft } from '@/api/browser'
import type { ExportForm } from './exportForm'
import type { ExportCatalog } from './useExportCatalog'

// 回填只还原受控配置；调用方保持 hydration 标记直到 Vue watcher 刷新结束。
export function populateFormFromDraft(deps: ExportForm & Pick<ExportCatalog, 'candidateObjectNames' | 'candidateObjectKeyword' | 'selectedObjectKeyword'>, draft: ExportDraft) {
  const {
    selectedDataSourceID, selectedNodeID, database, scopeKind, objectType, clearSelectedObjects,
    selectedObjectsByType, candidateObjectNames, objectNames, candidateObjectKeyword,
    selectedObjectKeyword, excludeTablesText, contentKind, formatKind, filePath, logPath, skipCheckDir,
    noNestedDir, maxFileSize, retainEmptyFiles, compress, compressionAlgo, compressionLevel,
    controlFilePath, tmpPath, outputKind, storageBucket, storagePath, storageEndpoint, storageRegion,
    storageCredentialID, skipHeader, columnSeparator, columnQuote, columnQuoteMode, escapeCharacter,
    lineSeparator, nullString, fileEncoding, withTrim, columnSplitter, trailDelimiter, removeNewline,
    dateValueFormat, datetimeValueFormat, querySql, queryResultLimit, where, includeColumnNames,
    excludeColumnNames, excludeVirtualColumns, flashbackScn, flashbackTimestamp, snapshot, partition,
    excludeDataTypes, thread, pageSize, parallelMacro, fetchSize, jvmMemory, blockSize, dropObject,
    retainSchema, compactSchema,
  } = deps
  // populateFormFromDraft 把服务端冻结配置回填到向导表单（纯展示映射，不改变语义）。

  const config = draft.config
  selectedDataSourceID.value = draft.dataSourceId
  selectedNodeID.value = draft.nodeId
  database.value = config.objectScope.database
  scopeKind.value = config.objectScope.scopeKind
  objectType.value = config.objectScope.objectTypes?.[0] ?? 'TABLE'
  clearSelectedObjects()
  for (const expression of config.objectScope.expressions ?? []) {
    const type = expression.objectType ?? objectType.value
    selectedObjectsByType[type] = [...selectedObjectsByType[type], expression.name]
  }
  candidateObjectNames.value = objectNames.value.filter(Boolean)
  candidateObjectKeyword.value = ''
  selectedObjectKeyword.value = ''
  excludeTablesText.value = (config.objectScope.excludeTables ?? []).join(',')
  contentKind.value = config.contentSelection.contentKind
  formatKind.value = config.dataFormat?.formatKind ?? 'CSV'
  const output = config.outputConfig
  filePath.value = output.outputKind === 'LOCAL' ? output.filePath : ''
  logPath.value = output.logPath ?? ''
  skipCheckDir.value = output.skipCheckDir ?? false
  noNestedDir.value = output.noNestedDir ?? false
  maxFileSize.value = output.maxFileSize !== undefined ? String(output.maxFileSize) : ''
  retainEmptyFiles.value = output.retainEmptyFiles ?? false
  compress.value = output.compress ?? false
  compressionAlgo.value = output.compressionAlgo ?? ''
  compressionLevel.value = output.compressionLevel !== undefined ? String(output.compressionLevel) : ''
  controlFilePath.value = output.controlFilePath ?? ''
  tmpPath.value = output.tmpPath ?? ''
  if (output.outputKind !== 'LOCAL') {
    outputKind.value = output.outputKind
    const parsed = parseControlledStorageURI(output.filePath)
    storageBucket.value = parsed.bucket
    storagePath.value = parsed.path
    storageEndpoint.value = parsed.endpoint
    storageRegion.value = parsed.region
    storageCredentialID.value = output.storageCredential?.storageCredentialId ?? ''
  }
  const csv = config.dataFormat?.csvOptions
  if (csv) {
    skipHeader.value = csv.skipHeader ?? false
    columnSeparator.value = csv.columnSeparator ?? ''
    columnQuote.value = csv.columnQuote ?? ''
    columnQuoteMode.value = csv.columnQuoteMode ?? ''
    escapeCharacter.value = csv.escapeCharacter ?? ''
    lineSeparator.value = csv.lineSeparator ?? ''
    nullString.value = csv.nullString ?? ''
    fileEncoding.value = csv.fileEncoding ?? ''
    withTrim.value = csv.withTrim ?? false
    columnSplitter.value = csv.columnSplitter ?? ''
  }
  const cut = config.dataFormat?.cutOptions
  if (cut) {
    trailDelimiter.value = cut.trailDelimiter ?? false
    removeNewline.value = cut.removeNewline ?? false
  }
  const formats = config.dataFormat?.timestampFormats
  if (formats) {
    dateValueFormat.value = formats.dateValueFormat ?? ''
    datetimeValueFormat.value = formats.datetimeValueFormat ?? ''
  }
  const filter = config.filterConfig
  querySql.value = ''
  queryResultLimit.value = ''
  if (filter) {
    querySql.value = filter.querySql ?? ''
    queryResultLimit.value = filter.queryResultLimit !== undefined ? String(filter.queryResultLimit) : ''
    where.value = filter.where ?? ''
    includeColumnNames.value = (filter.includeColumnNames ?? []).join(',')
    excludeColumnNames.value = (filter.excludeColumnNames ?? []).join(',')
    excludeVirtualColumns.value = filter.excludeVirtualColumns ?? false
    flashbackScn.value = filter.flashbackScn !== undefined ? String(filter.flashbackScn) : ''
    flashbackTimestamp.value = filter.flashbackTimestamp ?? ''
    snapshot.value = filter.snapshot ?? false
    partition.value = filter.partition ?? ''
    excludeDataTypes.value = (filter.excludeDataTypes ?? []).join(',')
  }
  const performance = config.performanceConfig
  if (performance) {
    thread.value = performance.thread !== undefined ? String(performance.thread) : ''
    pageSize.value = performance.pageSize !== undefined ? String(performance.pageSize) : ''
    parallelMacro.value = performance.parallelMacro !== undefined ? String(performance.parallelMacro) : ''
    fetchSize.value = performance.fetchSize !== undefined ? String(performance.fetchSize) : ''
    jvmMemory.value = performance.jvmMemory ?? ''
    blockSize.value = performance.blockSize ?? ''
  }
  const ddl = config.ddlBehavior
  if (ddl) {
    dropObject.value = ddl.dropObject ?? false
    retainSchema.value = ddl.retainSchema ?? false
    compactSchema.value = ddl.compactSchema ?? false
  }

// parseControlledStorageURI 把向导生成的受控 URI 还原为表单字段（与 buildOutputFilePath 互为逆操作）。
function parseControlledStorageURI(uri: string): { bucket: string; path: string; endpoint: string; region: string } {
  const schemeIndex = uri.indexOf('://')
  const queryIndex = uri.indexOf('?')
  const authority = uri.slice(schemeIndex + 3, queryIndex >= 0 ? queryIndex : undefined)
  const slashIndex = authority.indexOf('/')
  const bucket = slashIndex >= 0 ? authority.slice(0, slashIndex) : authority
  const path = slashIndex >= 0 ? authority.slice(slashIndex) : '/'
  let endpoint = ''
  let region = ''
  if (queryIndex >= 0) {
    for (const pair of uri.slice(queryIndex + 1).split('&')) {
      const [key, value] = pair.split('=')
      if (key === 'endpoint') endpoint = decodeURIComponent(value ?? '')
      if (key === 'region') region = decodeURIComponent(value ?? '')
    }
  }
  return { bucket, path, endpoint, region }
}
}
