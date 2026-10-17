import { reactive } from 'vue'

export type ImportContent = '' | 'DDL_ONLY' | 'DATA_ONLY' | 'DDL_AND_DATA'
export type ImportFormat = 'CSV' | 'CUT' | 'SQL'
export type ImportMapping = 'POSITION' | 'CONTROL' | 'AUTO' | 'EXPLICIT'
export type ImportObjectType = 'table' | 'table-group' | 'view' | 'trigger' | 'obj-user' | 'role' | 'sequence' | 'synonym' | 'type' | 'type-body' | 'package' | 'package-body' | 'function' | 'procedure'

// 每个格式拥有独立的输入命名空间；切换只改变活动投影，不复制或同步表单。
function createFormatFields() {
  return {
    columnSeparator: '', columnQuote: '', columnSplitter: '', lineSeparator: '', escapeCharacter: '',
    skipHeader: false, skipFooter: false, ignoreEscape: false, trailDelimiter: false, withTrim: false,
    nullString: '', emptyString: '', compress: false, compressionAlgo: '', compressionLevel: '',
    mapping: 'POSITION' as ImportMapping, ctlPath: '', includeColumns: '', excludeColumns: '',
    excludeDataTypes: '', dateFormat: '', defaultDate: '',
  }
}

export type ImportFormatFields = ReturnType<typeof createFormatFields>

export function initialNormalImportForm() {
  return {
    dataSourceId: '', database: '', nodeId: '', filePath: '', fileSuffix: '', fileEncoding: '', fileRegex: '', logPath: '',
    content: '' as ImportContent, format: '' as ImportFormat | '', scope: '' as '' | 'ALL' | 'SPECIFIC',
    objects: Object.fromEntries(importObjectTypes.map(type => [type, ''])) as Record<ImportObjectType, string>,
    excludeTables: '',
    formats: Object.fromEntries(importFormats.map(format => [format, createFormatFields()])) as Record<ImportFormat, ImportFormatFields>,
    execution: {
      maxDiscards: '', maxErrors: '', strict: '', thread: '', rw: '', batch: '', blockSize: '', maxTps: '',
      maxWaitTimeout: '', memory: '', nlsDateFormat: '',
      nlsTimestampFormat: '', nlsTimestampTzFormat: '',
    },
    compatMysql: false, replaceData: false, replaceObject: false, cleanup: 'NONE' as 'NONE' | 'TRUNCATE' | 'DELETE', taskName: '',
  }
}

export const importFormats: readonly ImportFormat[] = ['CSV', 'CUT', 'SQL']
export const importContents: readonly ImportContent[] = ['DDL_ONLY', 'DATA_ONLY', 'DDL_AND_DATA']
export const importObjectTypes: readonly ImportObjectType[] = ['table', 'table-group', 'view', 'trigger', 'obj-user', 'role', 'sequence', 'synonym', 'type', 'type-body', 'package', 'package-body', 'function', 'procedure']

// createNormalImportForm 是一个向导实例中唯一的业务输入所有者；不持久化到浏览器存储。
export function createNormalImportForm() {
  return reactive(initialNormalImportForm())
}

export type NormalImportForm = ReturnType<typeof createNormalImportForm>
export type ImportExecutionKey = keyof NormalImportForm['execution']
