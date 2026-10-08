import { type ExportContentKind } from '@/api/browser'
import { computed, ref, watch } from 'vue'
import { isTableOnlySelection, BLOCK_SIZE_PATTERN, isValidEscapeCharacter, queryResultInputMessage, validateExportDraftInput } from './exportDraftInput'
import { blockSizeChoicesMB } from './exportPresentation'
import type { ExportForm } from './exportForm'
import type { ExportReferences } from './useExportReferences'

// useExportParameters 管理参数资格、互斥、清值和草稿输入。
export function useExportParameters(deps: ExportForm & Pick<ExportReferences, 'selectedSource' | 'storageCredentials' | 'selectedNode' | 'selectedSourceRevision'>) {
  const {
    scopeKind, contentKind, querySql, queryResultLimit, formatKind, excludeTablesText, where, partition,
    flashbackScn, flashbackTimestamp, snapshot, includeColumnNames, excludeColumnNames,
    excludeVirtualColumns, excludeDataTypes, objectType, allSelectedObjects, splitUnit, blockSize,
    dateValueFormat, datetimeValueFormat, compress, compressionAlgo, compressionLevel, maxFileSize, thread,
    pageSize, parallelMacro, fetchSize, jvmMemory, tmpPath, columnSeparator, columnQuote, columnSplitter,
    escapeCharacter, lineSeparator, outputKind, database, applicableSelectedObjects, selectedDataSourceID,
    selectedNodeID, objectNames, trailDelimiter, removeNewline, filePath, logPath, skipCheckDir,
    storageBucket, storagePath, storageEndpoint, storageRegion, controlFilePath, skipHeader,
    columnQuoteMode, nullString, fileEncoding, withTrim, noNestedDir, retainEmptyFiles, dropObject,
    retainSchema, compactSchema, addExtraMessage, enableHiddenPk, timeValueFormat, timestampValueFormat,
    timestampTzValueFormat, timestampLtzValueFormat, nlsDateFormat, nlsTimestampFormat,
    nlsTimestampTzFormat, hydratingDerivedDraft, clearSelectedObjects, selectedSource, storageCredentials,
    storageCredentialID, selectedNode, selectedSourceRevision,
  } = deps
  type ExportMode = ExportContentKind | 'QUERY_RESULT'

  const exportMode = computed<ExportMode>({
    get: () => scopeKind.value === 'QUERY_RESULT' ? 'QUERY_RESULT' : contentKind.value,
    set: (mode) => {
      const wasQuery = scopeKind.value === 'QUERY_RESULT'
      if (mode === 'QUERY_RESULT') {
        scopeKind.value = 'QUERY_RESULT'
        contentKind.value = 'DATA_ONLY'
        querySql.value = ''
        queryResultLimit.value = '1000'
        if (!['CSV', 'CUT', 'SQL'].includes(formatKind.value)) formatKind.value = 'CSV'
        excludeTablesText.value = ''
        where.value = ''
        partition.value = ''
        flashbackScn.value = ''
        flashbackTimestamp.value = ''
        snapshot.value = false
        includeColumnNames.value = ''
        excludeColumnNames.value = ''
        excludeVirtualColumns.value = false
        excludeDataTypes.value = ''
      } else {
        if (wasQuery) {
          scopeKind.value = 'SPECIFIED'
          querySql.value = ''
          queryResultLimit.value = ''
        }
        contentKind.value = mode
      }
    },
  })

  const dataOptionsActive = computed(() => contentKind.value !== 'DDL_ONLY')

  // 表级选项根据完整已选集合判断；浏览分类不能改变已配置参数的资格。
  const tableOnlySelection = computed(() => isTableOnlySelection({ objectType: objectType.value, objectSelections: allSelectedObjects.value.map((item) => ({ objectType: item.type, name: item.name })) }))
  const excludeTablesSupported = computed(() => (scopeKind.value === 'ALL' && dataOptionsActive.value) || (scopeKind.value === 'SPECIFIED' && tableOnlySelection.value))

  const whereSupported = computed(() => scopeKind.value === 'SPECIFIED' && tableOnlySelection.value)
  const partitionSupported = computed(() => dataOptionsActive.value && whereSupported.value)

  const compactSchemaSupported = computed(() => contentKind.value !== 'DATA_ONLY' && (scopeKind.value === 'ALL' || tableOnlySelection.value))
  const formatChangeNotice = ref('')

  function changeSplitUnit(unit: 'MB' | 'ROW') {
    splitUnit.value = unit
    if (blockSize.value) blockSize.value = blockSize.value.replace(/MB|ROW/g, '') + (unit === 'ROW' ? 'ROW' : '')
  }

  watch(blockSize, (value) => { if (BLOCK_SIZE_PATTERN.test(value)) splitUnit.value = value.endsWith('ROW') ? 'ROW' : 'MB' })

  const splitThreshold = computed({
    get: () => blockSize.value,
    set: (value: string) => { blockSize.value = splitUnit.value === 'ROW' && /^[1-9][0-9]*$/.test(value) ? value + 'ROW' : value },
  })

  function migrateLegacyQuery() {
    const previousQuery = querySql.value
    exportMode.value = 'QUERY_RESULT'
    querySql.value = previousQuery
  }

  const configuredCount = (values: readonly (string | boolean)[]) => values.filter((value) => value !== '' && value !== false).length
  const objectAdvancedCount = computed(() => configuredCount([excludeTablesText.value, includeColumnNames.value, excludeColumnNames.value, excludeDataTypes.value, where.value, partition.value]))

  const formatAdvancedCount = computed(() => configuredCount([flashbackScn.value, flashbackTimestamp.value, dateValueFormat.value, datetimeValueFormat.value, blockSize.value, compress.value, compressionAlgo.value, compressionLevel.value]))
  const executionAdvancedCount = computed(() => configuredCount([maxFileSize.value, thread.value, pageSize.value, parallelMacro.value, fetchSize.value, jvmMemory.value, tmpPath.value]))

  const objectOptionsMessage = computed(() => {
    if (!dataOptionsActive.value) return ''
    if (querySql.value.trim() && (where.value.trim() || partition.value.trim())) return '自定义查询与条件及分区筛选互斥，只能选择其一。'
    if (includeColumnNames.value.trim() && excludeColumnNames.value.trim()) return '包含列与排除列互斥，只能选择其一。'
    return ''
  })

  const formatOptionsMessage = computed(() => {
    if (dataOptionsActive.value && (scopeKind.value === 'QUERY_RESULT' || querySql.value.trim()) && (snapshot.value || flashbackScn.value.trim() || flashbackTimestamp.value.trim())) return '结果集导出不能组合快照或闪回参数。'
    if (dataOptionsActive.value && snapshot.value && (flashbackScn.value.trim() || flashbackTimestamp.value.trim())) return '一致性快照与闪回参数互斥，只能选择其一。'
    if (dataOptionsActive.value && flashbackScn.value.trim() && !/^[1-9][0-9]*$/.test(flashbackScn.value)) return '闪回 SCN 必须是正整数。'
    if (dataOptionsActive.value && formatKind.value === 'CSV' && (Array.from(columnSeparator.value).length > 1 || Array.from(columnQuote.value).length > 1)) return 'CSV 字段分隔符和文本识别符在 OBDUMPER 4.3.5 中仅支持单字符。'
    if (dataOptionsActive.value && formatKind.value === 'CUT' && columnSplitter.value.length > 256) return 'CUT 字段分隔字符串不能超过 256 个字符。'
    if (dataOptionsActive.value && (formatKind.value === 'CSV' || formatKind.value === 'CUT') && !isValidEscapeCharacter(escapeCharacter.value)) return '转义字符当前仅支持单个 ASCII 字符，且不能使用换行或 NUL。'
    if (dataOptionsActive.value && lineSeparator.value.length > 256) return '换行符号不能超过 256 个字符。'
    if (dataOptionsActive.value && blockSize.value.trim() && !BLOCK_SIZE_PATTERN.test(blockSize.value.trim())) return '文件拆分必须为正整数（MB）或正整数+MB/ROW 后缀，例如 1024 或 256ROW。'
    return ''
  })

  // 仅 MySQL CSV/CUT 数据导出开放两个已验证时间格式；Oracle 的九个字段均保持关闭。
  const timestampFormatsSupported = computed(() => selectedSource.value?.compatibilityMode === 'MYSQL' && contentKind.value !== 'DDL_ONLY' && (formatKind.value === 'CSV' || formatKind.value === 'CUT'))

  // EX-I6 存储专用预检查（2026-08-14）：对象存储草稿可以发起预检查；
  // 检查清单切换为存储形态，端点连通性与凭据有效性两项未授权探测保持 UNKNOWN，
  // 提交按结果失败关闭（真实探测归 EX-V1）。
  const storageOutput = computed(() => outputKind.value !== 'LOCAL')

  // EX-I6 存储凭据槽位：只列出与当前输出类型同 provider 的凭据；不指定则依赖 Hadoop 标准配置链。
  const matchingStorageCredentials = computed(() => storageCredentials.value.filter((credential) => credential.provider === outputKind.value))
  const selectedStorageCredential = computed(() => storageCredentials.value.find((credential) => credential.id === storageCredentialID.value))

  const objectInputMessage = computed(() => {
    if (!database.value.trim()) return '请选择数据库或 Schema。'
    if (scopeKind.value === 'QUERY_RESULT') return queryResultInputMessage(querySql.value, queryResultLimit.value)
    if (scopeKind.value === 'ALL') return ''
    const names = applicableSelectedObjects.value.map((item) => item.name)
    if (names.length === 0) return '请至少选择一个导出对象。'
    if (names.some((name) => name.includes('*') || name.includes(',') || name.length > 256)) return '对象名称不能使用通配符或逗号，且不超过 256 个字符；多个对象请分行填写。'
    return ''
  })

  const contentInputMessage = computed(() => {
    return ''
  })

  const excludeTables = computed(() => excludeTablesText.value.split(',').map((name) => name.trim()).filter((name) => name.length > 0))
  const draftValidation = computed(() => validateExportDraftInput({
    dataSourceId: selectedDataSourceID.value,
    nodeId: selectedNodeID.value,
    platform: selectedNode.value?.platform ?? '',
    compatibilityMode: selectedSource.value?.compatibilityMode ?? '',
    database: database.value,
    scopeKind: scopeKind.value,
    objectType: objectType.value,
    objectNames: objectNames.value,
    objectSelections: applicableSelectedObjects.value.map((item) => ({ objectType: item.type, name: item.name })),
    excludeTables: excludeTables.value,
    contentKind: contentKind.value,
    formatKind: formatKind.value,
    trailDelimiter: trailDelimiter.value,
    removeNewline: removeNewline.value,
    columnSplitter: columnSplitter.value,
    filePath: filePath.value,
    logPath: logPath.value,
    skipCheckDir: skipCheckDir.value,
    outputKind: outputKind.value,
    storageBucket: storageBucket.value,
    storagePath: storagePath.value,
    storageEndpoint: storageEndpoint.value,
    storageRegion: storageRegion.value,
    tmpPath: tmpPath.value,
    storageCredentialId: storageCredentialID.value,
    storageCredentialRevision: selectedStorageCredential.value?.currentRevision ?? 0,
    storageCredentialProvider: selectedStorageCredential.value?.provider ?? '',
    controlFilePath: controlFilePath.value,
    skipHeader: skipHeader.value,
    columnSeparator: columnSeparator.value,
    columnQuote: columnQuote.value,
    columnQuoteMode: columnQuoteMode.value,
    escapeCharacter: escapeCharacter.value,
    lineSeparator: lineSeparator.value,
    nullString: nullString.value,
    fileEncoding: fileEncoding.value,
    withTrim: withTrim.value,
    noNestedDir: noNestedDir.value,
    maxFileSize: maxFileSize.value,
    retainEmptyFiles: retainEmptyFiles.value,
    compress: compress.value,
    compressionAlgo: compressionAlgo.value,
    compressionLevel: compressionLevel.value,
    querySql: querySql.value,
    queryResultLimit: queryResultLimit.value,
    where: where.value,
    includeColumnNames: includeColumnNames.value,
    excludeColumnNames: excludeColumnNames.value,
    excludeVirtualColumns: excludeVirtualColumns.value,
    flashbackScn: flashbackScn.value,
    flashbackTimestamp: flashbackTimestamp.value,
    snapshot: snapshot.value,
    thread: thread.value,
    pageSize: pageSize.value,
    parallelMacro: parallelMacro.value,
    fetchSize: fetchSize.value,
    jvmMemory: jvmMemory.value,
    blockSize: blockSize.value,
    dropObject: dropObject.value,
    retainSchema: retainSchema.value,
    compactSchema: compactSchema.value,
    addExtraMessage: addExtraMessage.value,
    partition: partition.value,
    excludeDataTypes: excludeDataTypes.value,
    enableHiddenPk: enableHiddenPk.value,
    dateValueFormat: dateValueFormat.value,
    timeValueFormat: timeValueFormat.value,
    datetimeValueFormat: datetimeValueFormat.value,
    timestampValueFormat: timestampValueFormat.value,
    timestampTzValueFormat: timestampTzValueFormat.value,
    timestampLtzValueFormat: timestampLtzValueFormat.value,
    nlsDateFormat: nlsDateFormat.value,
    nlsTimestampFormat: nlsTimestampFormat.value,
    nlsTimestampTzFormat: nlsTimestampTzFormat.value,
  }))

  const ordinaryFormat = computed(() => contentKind.value === 'DDL_ONLY' || ['CSV', 'CUT', 'SQL'].includes(formatKind.value))
  const draftInput = computed(() => ordinaryFormat.value && draftValidation.value.valid && !contentInputMessage.value && !objectInputMessage.value && !objectOptionsMessage.value && !formatOptionsMessage.value ? draftValidation.value.input : undefined)

  const draftValidationMessage = computed(() => !ordinaryFormat.value ? '当前格式仅保留历史任务读取；普通新建入口只开放 CSV、CUT 和 SQL。' : draftValidation.value.valid ? contentInputMessage.value || objectInputMessage.value || objectOptionsMessage.value || formatOptionsMessage.value : draftValidation.value.message)

  // clearGatedParameters 清除尚未完成专用预检查的开关，避免状态切换后留下可提交残留。
  function clearGatedParameters() {
    addExtraMessage.value = false
    enableHiddenPk.value = false
  }

  // clearTimestampFormats 清除全部时间格式，Oracle 与未知兼容模式不得保留任何待验证字段。
  function clearTimestampFormats() {
    dateValueFormat.value = ''
    timeValueFormat.value = ''
    datetimeValueFormat.value = ''
    timestampValueFormat.value = ''
    timestampTzValueFormat.value = ''
    timestampLtzValueFormat.value = ''
    nlsDateFormat.value = ''
    nlsTimestampFormat.value = ''
    nlsTimestampTzFormat.value = ''
  }

  // clearUnverifiedTimestampFormats 清除 MySQL 已验证 DATE/DATETIME 以外的七个字段。
  function clearUnverifiedTimestampFormats() {
    timeValueFormat.value = ''
    timestampValueFormat.value = ''
    timestampTzValueFormat.value = ''
    timestampLtzValueFormat.value = ''
    nlsDateFormat.value = ''
    nlsTimestampFormat.value = ''
    nlsTimestampTzFormat.value = ''
  }

  watch(selectedSource, (source, previousSource) => {
    if (hydratingDerivedDraft.value) return
    if (source?.id === previousSource?.id) return
    // 服务端不允许更新已保存草稿的数据源绑定；更换后下一次保存必须创建新草稿。
    database.value = ''
    querySql.value = ''
    queryResultLimit.value = ''
    scopeKind.value = 'SPECIFIED'
    objectType.value = 'TABLE'
    clearSelectedObjects()
    excludeTablesText.value = ''
    contentKind.value = 'DATA_ONLY'
    clearGatedParameters()
    clearTimestampFormats()
  })

  watch(selectedSourceRevision, (revision, previousRevision) => {
    if (hydratingDerivedDraft.value || !previousRevision || revision === previousRevision) return
    clearSelectedObjects()
  })

  watch(selectedNodeID, () => {
    if (hydratingDerivedDraft.value) return
    // 执行节点绑定同样不可更新；清除旧草稿避免提交旧节点的预检查证据。
    if (scopeKind.value === 'SPECIFIED') {
      clearSelectedObjects()
    }
  })

  // EX-I6 存储凭据槽位：切换输出类型时清除凭据引用，避免跨 provider 或本地输出残留绑定。
  watch(outputKind, () => {
    if (hydratingDerivedDraft.value) return
    storageCredentialID.value = ''
  })

  watch(scopeKind, () => {
    if (hydratingDerivedDraft.value) return
    clearGatedParameters()
    // 条件筛选只可随指定表发送，切到全部对象或已选视图时清除残留。
    if (!whereSupported.value) {
      where.value = ''
      partition.value = ''
    }
  })

  watch(database, (value, previousValue) => {
    if (hydratingDerivedDraft.value || value === previousValue) return
    // 对象和结果集查询属于当前数据库；切库后不能把旧范围带入新数据库。
    clearSelectedObjects()
    if (scopeKind.value === 'QUERY_RESULT') querySql.value = ''
  })

  // 紧凑 Schema 失去表 DDL 前提时立即清值，避免隐藏残留进入草稿构造。
  watch(compactSchemaSupported, (supported) => {
    if (hydratingDerivedDraft.value) return
    if (!supported) compactSchema.value = false
  })

  // 排除表失去适用范围时清值，避免折叠项隐藏后留下不可见的无效草稿参数。
  watch(excludeTablesSupported, (supported) => {
    if (hydratingDerivedDraft.value) return
    if (!supported) excludeTablesText.value = ''
  })

  // 分区筛选失去指定表数据前提时立即清值；时间格式仅保留 MySQL 的两个已验证字段。
  watch(partitionSupported, (supported) => {
    if (hydratingDerivedDraft.value) return
    if (!supported) partition.value = ''
  })

  watch(whereSupported, (supported) => {
    if (!hydratingDerivedDraft.value && !supported) where.value = ''
  })

  watch(timestampFormatsSupported, (supported) => {
    if (hydratingDerivedDraft.value) return
    clearUnverifiedTimestampFormats()
    if (!supported) {
      dateValueFormat.value = ''
      datetimeValueFormat.value = ''
    }
  }, { immediate: true })

  // 切换到 MySQL 数据源时清除 Oracle 专属值，避免隐藏控件残留造成无法继续。
  watch(() => selectedSource.value?.compatibilityMode, (mode) => {
    if (hydratingDerivedDraft.value || !mode || mode === 'ORACLE') return
    flashbackTimestamp.value = ''
    fetchSize.value = ''
  })

  watch(compress, (enabled) => {
    if (hydratingDerivedDraft.value) return
    // 取消压缩时清除算法与等级值，避免禁用态控件保留旧值造成校验死锁。
    if (!enabled) {
      compressionAlgo.value = ''
      compressionLevel.value = ''
    }
  })

  // 切换压缩算法时清除不适用或越界的等级值（gzip/snappy 不支持等级）。
  watch(compressionAlgo, (algo) => {
    if (hydratingDerivedDraft.value) return
    if (algo === 'gzip' || algo === 'snappy') compressionLevel.value = ''
  })

  watch(contentKind, (kind) => {
    if (hydratingDerivedDraft.value) return
    clearGatedParameters()
    if (kind === 'DDL_ONLY') {
      // 仅 DDL 时清除数据专属选项，避免残留值进入下一个草稿。
      skipHeader.value = false
      columnSeparator.value = ''
      columnQuote.value = ''
      columnQuoteMode.value = ''
      escapeCharacter.value = ''
      lineSeparator.value = ''
      nullString.value = ''
      fileEncoding.value = ''
      withTrim.value = false
      trailDelimiter.value = false
      removeNewline.value = false
      columnSplitter.value = ''
      controlFilePath.value = ''
      noNestedDir.value = false
      blockSize.value = ''
      maxFileSize.value = ''
      retainEmptyFiles.value = false
      compress.value = false
      compressionAlgo.value = ''
      querySql.value = ''
      where.value = ''
      includeColumnNames.value = ''
      excludeColumnNames.value = ''
      excludeVirtualColumns.value = false
      flashbackScn.value = ''
      flashbackTimestamp.value = ''
      snapshot.value = false
      thread.value = ''
      pageSize.value = ''
      parallelMacro.value = ''
      fetchSize.value = ''
      jvmMemory.value = ''
      // 仅 DDL 时清除数据专属筛选与全部时间格式。
      partition.value = ''
      excludeDataTypes.value = ''
      clearTimestampFormats()
    }
    // 结构与数据组合仅允许已映射的文本格式；旧草稿的其他格式收敛为 CSV。
    if (kind === 'DDL_AND_DATA' && !['CSV', 'CUT', 'SQL'].includes(formatKind.value)) formatKind.value = 'CSV'
    // 仅数据内容不携带前置 DROP、保留 Schema 与紧凑 Schema，切换时清除残留。
    if (kind === 'DATA_ONLY') {
      dropObject.value = false
      retainSchema.value = false
      compactSchema.value = false
    }
  })

  // 切换数据格式时清空不适用格式的选项，避免残留值进入下一个草稿。
  watch(formatKind, (kind, previousKind) => {
    if (hydratingDerivedDraft.value) return
    if (kind === previousKind) return
    const resetFields: string[] = []
    if (kind !== 'CSV' && (skipHeader.value || columnSeparator.value || columnQuote.value || columnQuoteMode.value)) resetFields.push('CSV 列头、分隔符或包围设置')
    if (kind !== 'CUT' && (trailDelimiter.value || removeNewline.value || columnSplitter.value)) resetFields.push('CUT 分隔符或行尾设置')
    if (kind === 'SQL' && (escapeCharacter.value || nullString.value || withTrim.value)) resetFields.push('转义、NULL 表示或修剪')
    if (kind === 'SQL' && (dateValueFormat.value || datetimeValueFormat.value)) resetFields.push('日期时间值格式')
    formatChangeNotice.value = resetFields.length ? `已切换为 ${kind}，清除不适用的${resetFields.join('、')}；编码、换行与文件组织设置保留。` : ''
    // 离开 CSV：只清 CSV 专属序列化选项；文件布局、筛选与性能选项官方不限定格式，切换后保留。
    if (kind !== 'CSV') {
      skipHeader.value = false
      columnSeparator.value = ''
      columnQuote.value = ''
      columnQuoteMode.value = ''
    }
    // 离开 CUT：清空 CUT 专属选项（含列分隔字符串）。
    if (kind !== 'CUT') {
      trailDelimiter.value = false
      removeNewline.value = false
      columnSplitter.value = ''
    }
    // 结构化格式不支持文件拆分参数；隐藏输入时同步清空，避免旧值进入新草稿。
    if (kind === 'PARQUET' || kind === 'ORC' || kind === 'AVRO') blockSize.value = ''
    // 离开 POS：清空控制文件目录（--ctl-path）。
    if (kind !== 'POS') controlFilePath.value = ''
    // SQL 只支持行分隔符与文件编码。
    if (kind === 'SQL') {
      escapeCharacter.value = ''
      nullString.value = ''
      withTrim.value = false
    }
  }, { flush: 'sync' })

  watch(objectType, () => { if (!hydratingDerivedDraft.value) clearGatedParameters() })

  const blockSizeChoices = computed(() => splitUnit.value === 'MB' ? blockSizeChoicesMB : [
  { value: '', label: '工具默认（不指定）' },
  ...['1000', '10000', '100000', '1000000'].map((size) => ({ value: size + 'ROW', label: Number(size).toLocaleString('en-US') + ' 行' })),
])

  return {
    blockSizeChoices, exportMode, dataOptionsActive, tableOnlySelection, excludeTablesSupported,
    whereSupported, partitionSupported, compactSchemaSupported, formatChangeNotice, changeSplitUnit,
    splitThreshold, migrateLegacyQuery, configuredCount, objectAdvancedCount, formatAdvancedCount,
    executionAdvancedCount, objectOptionsMessage, formatOptionsMessage, timestampFormatsSupported,
    storageOutput, matchingStorageCredentials, selectedStorageCredential, objectInputMessage,
    contentInputMessage, excludeTables, draftValidation, ordinaryFormat, draftInput,
    draftValidationMessage, clearGatedParameters, clearTimestampFormats, clearUnverifiedTimestampFormats,
  }
}

export type ExportParameters = ReturnType<typeof useExportParameters>
