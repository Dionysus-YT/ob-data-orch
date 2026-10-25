import { describe, expect, it } from 'vitest'
import { exportParameterCatalog, exportParameterRows } from './exportParameterCatalog'

describe('导出参数目录的资格说明', () => {
  it('列出已开放参数及受管连接项，排除历史格式和重试入口', () => {
    const names = exportParameterCatalog.map(row => row.name)
    expect(new Set(names).size).toBe(names.length)
    for (const name of ['query-sql', 'compression-level', 'sequence-policy', 'timestamp-ltz-value-format', 'password']) expect(names).toContain(name)
    for (const name of ['pos', 'par', 'orc', 'avro', 'ctl-path', 'retry']) expect(names).not.toContain(name)
    expect(exportParameterCatalog.every(row => row.step >= 1 && row.step <= 4 && row.description)).toBe(true)
  })
  it('随格式、租户及压缩选择解释当前不适用原因', () => {
    const rows = exportParameterRows({ content: 'DATA_ONLY', format: 'SQL', scope: 'ALL', output: 'LOCAL', mode: 'MYSQL', compressed: true, algorithm: 'gzip' })
    expect(rows.find(row => row.name === 'column-separator')?.reason).toContain('仅 CSV')
    expect(rows.find(row => row.name === 'fetch-size')?.reason).toContain('Oracle')
    expect(rows.find(row => row.name === 'compression-level')?.reason).toContain('不支持')
    expect(rows.find(row => row.name === 'password')?.reason).toContain('不展示')
  })
})
