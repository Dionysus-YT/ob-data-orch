import { describe, expect, it } from 'vitest'

import { validateExportDraftInput } from './exportDraftInput'

const completeValues = {
  dataSourceId: 'source-1',
  nodeId: 'node-1',
  platform: 'WINDOWS_AMD64',
  database: 'synthetic_db',
  table: 'synthetic_table',
  filePath: '/E:/workespace/output',
  logPath: '/E:/workespace/logs',
  skipCheckDir: true,
}

describe('首条 CSV 草稿输入', () => {
  it('只生成固定单表 CSV 的结构化输入', () => {
    expect(validateExportDraftInput(completeValues)).toEqual({
      valid: true,
      input: {
        dataSourceId: 'source-1',
        nodeId: 'node-1',
        database: 'synthetic_db',
        table: 'synthetic_table',
        format: 'CSV',
        filePath: '/E:/workespace/output',
        logPath: '/E:/workespace/logs',
        skipCheckDir: true,
      },
    })
  })

  it('拒绝空字段、多表表达式和不匹配节点平台的输出路径', () => {
    expect(validateExportDraftInput({ ...completeValues, dataSourceId: '' })).toMatchObject({ valid: false, message: expect.stringContaining('数据源') })
    expect(validateExportDraftInput({ ...completeValues, table: 'first_table,second_table' })).toMatchObject({ valid: false, message: expect.stringContaining('一个明确的表名') })
    expect(validateExportDraftInput({ ...completeValues, filePath: 'E:\\workespace\\output' })).toMatchObject({ valid: false, message: expect.stringContaining('/E:/exports') })
    expect(validateExportDraftInput({ ...completeValues, logPath: 'E:\\workespace\\logs' })).toMatchObject({ valid: false, message: expect.stringContaining('日志路径') })
    expect(validateExportDraftInput({ ...completeValues, platform: 'LINUX_ARM64', filePath: '/var/output' })).toMatchObject({ valid: true })
  })
})
