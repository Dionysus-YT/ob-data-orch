import { describe, expect, it } from 'vitest'

import { precheckCheckLabel, precheckResultDetail, precheckResultLabel } from './exportPrecheckPresentation'

describe('预检查结果展示', () => {
  it('将对象不可用说明为未执行而非未知故障', () => {
    const result = { check: 'OBJECT_ACCESS' as const, status: 'UNKNOWN' as const, evidenceCode: 'OBJECT_ACCESS_UNAVAILABLE' }

    expect(precheckCheckLabel(result.check)).toBe('所选表可读取')
    expect(precheckResultLabel(result, false)).toBe('未执行')
    expect(precheckResultDetail(result)).toContain('依赖数据源连接')
  })

  it('将未取得连接结果说明为未完成', () => {
    const result = { check: 'DATABASE_CONNECTIVITY' as const, status: 'UNKNOWN' as const, evidenceCode: 'DATABASE_CONNECTION_UNAVAILABLE' }

    expect(precheckResultLabel(result, false)).toBe('未完成')
    expect(precheckResultDetail(result)).toContain('未取得可验证的数据库连接结果')
  })

  it('保留常规通过和等待状态', () => {
    const result = { check: 'TOOL_ENVIRONMENT' as const, status: 'PASSED' as const, evidenceCode: 'TOOL_RUNTIME_READY' }

    expect(precheckResultLabel(result, false)).toBe('通过')
    expect(precheckResultLabel(undefined, true)).toBe('等待所选 Agent 领取')
  })

  it('说明对象通过包含元数据和零行读取验证', () => {
    const result = { check: 'OBJECT_ACCESS' as const, status: 'PASSED' as const, evidenceCode: 'OBJECT_ACCESSIBLE' }

    expect(precheckResultDetail(result)).toContain('不返回业务行')
  })

  it('为路径未通过提供可操作的安全原因', () => {
    const result = { check: 'OUTPUT_PATH' as const, status: 'FAILED' as const, evidenceCode: 'OUTPUT_PATH_NOT_WRITABLE' }

    expect(precheckResultLabel(result, false)).toBe('未通过')
    expect(precheckResultDetail(result)).toContain('允许根目录')
    expect(precheckResultDetail(result)).toContain('最近已有父目录不可写')
  })

  it('为非空目录未通过说明可选处置', () => {
    const result = { check: 'OUTPUT_EMPTY' as const, status: 'FAILED' as const, evidenceCode: 'OUTPUT_PATH_NOT_EMPTY' }

    expect(precheckResultDetail(result)).toContain('跳过导出目录是否为空的检查')
  })

  it.each([
    { check: 'DATABASE_CONNECTIVITY' as const, evidenceCode: 'DATABASE_CONNECTION_FAILED' },
    { check: 'OBJECT_ACCESS' as const, evidenceCode: 'OBJECT_NOT_ACCESSIBLE' },
    { check: 'TOOL_ENVIRONMENT' as const, evidenceCode: 'TOOL_RUNTIME_INVALID' },
    { check: 'AVAILABLE_SPACE' as const, evidenceCode: 'OUTPUT_SPACE_INSUFFICIENT' },
  ])('为 $evidenceCode 提供失败说明', (result) => {
    expect(precheckResultDetail({ ...result, status: 'FAILED' })).not.toBe('')
  })
})
