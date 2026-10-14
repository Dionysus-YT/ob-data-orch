// 参数追踪只核对唯一生成器的展示形态，不拼装或执行工具命令。
export function expectedFlags(config) {
  const flags = ['--database', '--file-path']
  const scope = config.objectScope
  if (scope.scopeKind === 'ALL') flags.push('--all')
  if (scope.scopeKind === 'SPECIFIED') for (const type of scope.objectTypes) flags.push({ TABLE: '--table', VIEW: '--view', FUNCTION: '--function', PROCEDURE: '--procedure', SEQUENCE: '--sequence' }[type])
  if (config.contentSelection.contentKind !== 'DATA_ONLY') flags.push('--ddl')
  if (config.dataFormat) flags.push({ CSV: '--csv', CUT: '--cut', SQL: '--sql' }[config.dataFormat.formatKind])
  const names = {
    excludeTables: '--exclude-table', skipHeader: '--skip-header', columnSeparator: '--column-separator', columnQuote: '--column-quote', columnQuoteMode: '--column-quote-mode', escapeCharacter: '--escape-character', lineSeparator: '--line-separator', nullString: '--null-string', fileEncoding: '--file-encoding', withTrim: '--with-trim', columnSplitter: '--column-splitter', trailDelimiter: '--trail-delimiter', removeNewline: '--remove-newline',
    logPath: '--log-path', tmpPath: '--tmp-path', skipCheckDir: '--skip-check-dir', noNestedDir: '--no-nested-dir', maxFileSize: '--max-file-size', retainEmptyFiles: '--retain-empty-files', compress: '--compress', compressionAlgo: '--compression-algo', compressionLevel: '--compression-level',
    where: '--where', partition: '--partition', snapshot: '--snapshot', weakRead: '--weak-read', enableHiddenPk: '--enable-hidden-pk', excludeVirtualColumns: '--exclude-virtual-columns', excludeDataTypes: '--exclude-data-types', includeColumnNames: '--include-column-names', excludeColumnNames: '--exclude-column-names', flashbackScn: '--flashback-scn', querySql: '--query-sql',
    thread: '--thread', pageSize: '--page-size', parallelMacro: '--parallel-macro', jvmMemory: '--mem', blockSize: '--block-size',
    dropObject: '--drop-object', retainSchema: '--retain-schema', compactSchema: '--compact-schema', addExtraMessage: '--add-extra-message', sequencePolicy: '--sequence-policy', dateValueFormat: '--date-value-format', timeValueFormat: '--time-value-format', datetimeValueFormat: '--datetime-value-format', timestampValueFormat: '--timestamp-value-format', preserveZeroDatetime: '--preserve-zero-datetime',
  }
  function visit(value) {
    for (const [key, field] of Object.entries(value)) {
      if (names[key] && field !== undefined && field !== false && field !== '' && (!Array.isArray(field) || field.length)) flags.push(names[key])
      else if (field && !Array.isArray(field) && typeof field === 'object') visit(field)
    }
  }
  visit(config)
  return [...new Set(flags.filter(Boolean))]
}
