import type { ExportDraftInput } from '@/api/browser'

export interface ExportDraftFormValues {
  readonly dataSourceId: string
  readonly nodeId: string
  readonly platform: string
  readonly database: string
  readonly table: string
  readonly filePath: string
  readonly logPath: string
  readonly skipCheckDir: boolean
}

export type ExportDraftInputValidation =
  | { readonly valid: true; readonly input: ExportDraftInput }
  | { readonly valid: false; readonly message: string }

export function validateExportDraftInput(values: ExportDraftFormValues): ExportDraftInputValidation {
  const dataSourceId = values.dataSourceId.trim()
  const nodeId = values.nodeId.trim()
  const database = values.database.trim()
  const table = values.table.trim()
  const filePath = values.filePath.trim()
  const logPath = values.logPath.trim()
  if (!dataSourceId) return { valid: false, message: '请先选择一个完成基础连接测试的数据源。' }
  if (!database) return { valid: false, message: '请填写默认数据库或 Schema。' }
  if (!table || table.includes('*') || table.includes(',')) return { valid: false, message: '首条切片只支持一个明确的表名，不能使用通配符或多个表。' }
  if (!nodeId) return { valid: false, message: '请选择一个已授权的执行节点。' }
  if (!isAbsolutePathForPlatform(filePath, values.platform)) return { valid: false, message: '导出路径必须与所选节点平台匹配；Windows 使用 /E:/exports 形式。' }
  if (logPath && !isAbsolutePathForPlatform(logPath, values.platform)) return { valid: false, message: '日志路径必须与所选节点平台匹配；Windows 使用 /E:/exports 形式。' }
  return { valid: true, input: { dataSourceId, nodeId, database, table, format: 'CSV', filePath, logPath, skipCheckDir: values.skipCheckDir } }
}

function isAbsolutePathForPlatform(filePath: string, platform: string): boolean {
  if (platform === 'WINDOWS_AMD64') return /^\/[A-Za-z]:\//.test(filePath) && !filePath.includes('\\') && !filePath.split('/').some((part) => part === '.' || part === '..')
  if (platform === 'LINUX_AMD64' || platform === 'LINUX_ARM64') return filePath.startsWith('/')
  return false
}
