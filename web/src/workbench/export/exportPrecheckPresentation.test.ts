import { describe, expect, it } from 'vitest'

import { groupPrecheckRows, precheckBlockingSummary, precheckChecksForOutput, precheckCheckLabel, precheckFixAction, precheckResultBlocked, precheckResultBlocksSubmission, precheckResultDetail, precheckResultLabel } from './exportPrecheckPresentation'

describe('预检查结果展示', () => {
  it('将对象未知说明为无法确认且不伪造阻断依赖', () => {
    const result = { check: 'OBJECT_ACCESS' as const, status: 'UNKNOWN' as const, evidenceCode: 'OBJECT_ACCESS_UNAVAILABLE' }

    expect(precheckCheckLabel(result.check)).toBe('对象与参数访问')
    expect(precheckResultLabel(result, false)).toBe('无法确认')
    expect(precheckResultDetail(result)).toContain('未得到可验证结论')
    expect(precheckResultDetail(result)).not.toContain('连接未确认')
  })

  it('将未取得连接结果说明为无法确认', () => {
    const result = { check: 'DATABASE_CONNECTIVITY' as const, status: 'UNKNOWN' as const, evidenceCode: 'DATABASE_CONNECTION_UNAVAILABLE' }

    expect(precheckResultLabel(result, false)).toBe('无法确认')
    expect(precheckResultDetail(result)).toContain('未取得可验证的数据库连接结果')
  })

  it('保留常规通过和等待状态', () => {
    const result = { check: 'TOOL_ENVIRONMENT' as const, status: 'PASSED' as const, evidenceCode: 'TOOL_RUNTIME_READY' }

    expect(precheckResultLabel(result, false)).toBe('通过')
    expect(precheckResultLabel(undefined, true)).toBe('等待检查结果')
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

  it('将失败和未知结果都计为提交阻断项', () => {
    expect(precheckResultBlocksSubmission({ check: 'OBJECT_ACCESS', status: 'FAILED', evidenceCode: 'OBJECT_NOT_ACCESSIBLE' })).toBe(true)
    expect(precheckResultBlocksSubmission({ check: 'OBJECT_ACCESS', status: 'UNKNOWN', evidenceCode: 'OBJECT_ACCESS_UNAVAILABLE' })).toBe(true)
    expect(precheckResultBlocksSubmission({ check: 'OBJECT_ACCESS', status: 'PASSED', evidenceCode: 'OBJECT_ACCESSIBLE' })).toBe(false)
  })

  it.each([
    { check: 'DATABASE_CONNECTIVITY' as const, evidenceCode: 'PRECHECK_BLOCKED_BY_TOOL', cause: '工具环境', step: 2 },
    { check: 'OBJECT_ACCESS' as const, evidenceCode: 'PRECHECK_BLOCKED_BY_DATABASE', cause: '数据源连接', step: 1 },
    { check: 'STORAGE_AUTH' as const, evidenceCode: 'PRECHECK_BLOCKED_BY_STORAGE_CONNECTIVITY', cause: '存储端点', step: 4 },
    { check: 'OUTPUT_EMPTY' as const, evidenceCode: 'PRECHECK_BLOCKED_BY_OUTPUT_PATH', cause: '导出目录', step: 4 },
  ])('为 $evidenceCode 展示真实依赖与修正入口', ({ check, evidenceCode, cause, step }) => {
    const result = { check, evidenceCode, status: 'UNKNOWN' as const }
    expect(precheckResultLabel(result, false)).toBe('前置阻断，未执行')
    expect(precheckResultDetail(result)).toContain(cause)
    expect(precheckResultBlocked(result)).toBe(true)
    expect(precheckFixAction({ check, result }).step).toBe(step)
  })

  it('仅按配置跳过目录空性，不把未知结果伪装成跳过', () => {
    expect(precheckResultLabel({ check: 'OUTPUT_EMPTY', status: 'PASSED', evidenceCode: 'OUTPUT_EMPTY_CHECK_SKIPPED' }, false)).toBe('按配置跳过')
    expect(precheckResultLabel({ check: 'OUTPUT_EMPTY', status: 'UNKNOWN', evidenceCode: 'OUTPUT_EMPTY_CHECK_SKIPPED' }, false)).toBe('无法确认')
  })

  it('分组和汇总保留缺项，不把依赖阻断计为检查失败', () => {
    const rows = precheckChecksForOutput(false).map(check => ({ check, result: check === 'OUTPUT_PATH' ? { check, status: 'FAILED' as const, evidenceCode: 'OUTPUT_PATH_NOT_WRITABLE' } : check === 'OUTPUT_EMPTY' ? { check, status: 'UNKNOWN' as const, evidenceCode: 'PRECHECK_BLOCKED_BY_OUTPUT_PATH' } : undefined }))
    expect(groupPrecheckRows(rows).map(group => group.title)).toEqual(['执行环境', '输出与空间', '数据库与对象'])
    expect(precheckBlockingSummary(rows)).toBe('1 项未通过，1 项因前置条件未执行，5 项无法确认')
    expect(precheckChecksForOutput(true)).not.toContain('OUTPUT_EMPTY')
    expect(groupPrecheckRows(precheckChecksForOutput(true).map(check => ({ check, result: undefined }))).at(-1)?.title).toBe('对象存储')
  })

  it('本地空间与对象范围说明不夸大检查结论', () => {
    const space = { check: 'AVAILABLE_SPACE' as const, status: 'UNKNOWN' as const, evidenceCode: 'OUTPUT_SPACE_UNAVAILABLE' }
    expect(precheckResultDetail(space)).toContain('导出目录')
    expect(precheckResultDetail(space, true)).toContain('临时分块目录')
    const object = { check: 'OBJECT_ACCESS' as const, status: 'PASSED' as const, evidenceCode: 'OBJECT_ACCESSIBLE' }
    expect(precheckResultDetail(object, false, 'ALL')).toContain('数据库级可达性')
    expect(precheckResultDetail(object, false, 'QUERY_RESULT')).toContain('不执行查询 SQL')
  })

  it('资源通过仅表示静态额度且列分区失败定位到筛选步骤', () => {
    expect(precheckResultDetail({ check: 'NODE_RESOURCE', status: 'PASSED', evidenceCode: 'NODE_RESOURCE_READY' })).toContain('尚未预留资源')
    const result = { check: 'OBJECT_ACCESS' as const, status: 'FAILED' as const, evidenceCode: 'OBJECT_SELECTION_REJECTED' }
    expect(precheckResultDetail(result)).toContain('子分区')
    expect(precheckFixAction({ check: result.check, result })).toEqual({ step: 2, label: '修改列与分区筛选' })
  })

  it('存储写权限独立失败关闭且认证失败有明确阻断原因', () => {
    const result = { check: 'STORAGE_WRITE' as const, status: 'UNKNOWN' as const, evidenceCode: 'PRECHECK_BLOCKED_BY_STORAGE_AUTH' }
    expect(precheckResultBlocked(result)).toBe(true)
    expect(precheckResultDetail(result)).toContain('存储凭据有效性未通过')
    expect(precheckResultDetail({ ...result, evidenceCode: 'STORAGE_WRITE_UNAVAILABLE' })).toContain('不能用端点连通或认证通过代替')
    expect(precheckResultBlocksSubmission({ ...result, evidenceCode: 'STORAGE_WRITE_CLEANUP_UNCONFIRMED' })).toBe(true)
  })
})
