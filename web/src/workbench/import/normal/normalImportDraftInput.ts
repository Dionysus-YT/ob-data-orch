import type { NormalImportConfig } from '@/api/normalImport'
import { initialNormalImportForm, type NormalImportForm } from './normalImportForm'
import type { projectNormalImport } from './normalImportRules'

// 保存只投影当前活动格式；引用事实由控制面重新读取，不由浏览器提交。
export function normalImportDraftInput(p: ReturnType<typeof projectNormalImport>): NormalImportConfig {
  return {
    dataSourceId: p.dataSourceId, nodeId: p.nodeId, database: p.database, filePath: p.filePath,
    fileSuffix: p.fileSuffix, fileEncoding: p.fileEncoding, fileRegex: p.fileRegex, logPath: p.logPath,
    content: p.content, format: p.format, scope: p.scope, objects: p.objects, excludeTables: p.excludeTables,
    parser: p.parser, mapping: p.mapping, execution: p.execution, compatMysql: p.compatMysql,
    replaceData: p.replaceData, replaceObject: p.replaceObject, cleanup: p.cleanup, taskName: p.taskName,
  }
}

// Go 的 omitempty 省略未设置值；比较时只消除这些等价默认表示，保留所有显式有效参数。
export function normalImportConfigSignature(config: NormalImportConfig): string {
  function normalize(value: unknown): unknown {
    if (value === '' || value === false || value === undefined) return undefined
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      const entries = Object.entries(value).sort(([left], [right]) => left.localeCompare(right)).map(([key, entry]) => [key, normalize(entry)] as const).filter(([, entry]) => entry !== undefined)
      return entries.length ? Object.fromEntries(entries) : undefined
    }
    return value
  }
  return JSON.stringify(normalize(config))
}

export function hydrateNormalImport(form: NormalImportForm, config: NormalImportConfig) {
  const next = initialNormalImportForm()
  for (const key of ['dataSourceId', 'nodeId', 'database', 'filePath', 'fileSuffix', 'fileEncoding', 'fileRegex', 'logPath', 'excludeTables', 'taskName'] as const) next[key] = config[key] ?? ''
  Object.assign(next, { content: config.content, format: config.format ?? '', scope: config.scope, cleanup: config.cleanup, compatMysql: config.compatMysql ?? false, replaceData: config.replaceData ?? false, replaceObject: config.replaceObject ?? false })
  Object.assign(next.objects, config.objects)
  Object.assign(next.execution, config.execution)
  if (next.format) {
    const fields = next.formats[next.format]
    Object.assign(fields, config.parser)
    if (config.mapping) Object.assign(fields, { mapping: config.mapping.kind, ctlPath: config.mapping.ctlPath ?? '', includeColumns: config.mapping.includeColumns ?? '', excludeColumns: config.mapping.excludeColumns ?? '', excludeDataTypes: config.mapping.excludeDataTypes ?? '' })
  }
  Object.assign(form, next)
}
