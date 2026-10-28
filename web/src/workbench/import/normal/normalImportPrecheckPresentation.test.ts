import { describe, expect, it } from 'vitest'
import type { NormalImportCheck } from '@/api/normalImport'
import { normalImportBlockingSummary, normalImportPrecheckFixAction, normalImportPrecheckPresentation } from './normalImportPrecheckPresentation'

function check(status: NormalImportCheck['status'] = 'FAILED'): NormalImportCheck {
  return { id: 'check', draftId: 'draft', draftRevision: 1, configFingerprint: 'a'.repeat(64), checkKind: 'NORMAL_IMPORT_PREFLIGHT', status, integrityStatus: 'COMPLETE', validUntil: '2099-01-01T00:00:00Z', executionEnabled: false, results: [] }
}

describe('普通导入预检查反馈', () => {
  it('权限缺项、无法展开授权、DDL 未识别与目标结构问题分别解释', () => {
    for (const [name, status, code, detail] of [
      ['NODE_RESOURCE', 'FAILED', 'IMPORT_JVM_HEAP_TOO_SMALL', '700 MiB'],
      ['WRITE_ACCESS', 'FAILED', 'IMPORT_WRITE_DROP_REQUIRED', 'DROP'],
      ['WRITE_ACCESS', 'UNKNOWN', 'IMPORT_WRITE_GRANTS_UNAVAILABLE', '读取当前账号的授权信息失败'],
      ['WRITE_ACCESS', 'UNKNOWN', 'IMPORT_WRITE_GRANTS_UNSUPPORTED', '角色'],
      ['WRITE_ACCESS', 'UNKNOWN', 'IMPORT_WRITE_DDL_UNSUPPORTED', '完整核算权限'],
      ['INPUT_MAPPING', 'UNKNOWN', 'IMPORT_INPUT_DEFINITION_UNSUPPORTED', '同名建表定义未被完整识别'],
      ['INPUT_MAPPING', 'UNKNOWN', 'IMPORT_TARGET_METADATA_UNAVAILABLE', '读取目标表列元信息失败'],
      ['INPUT_MAPPING', 'UNKNOWN', 'IMPORT_TARGET_STRUCTURE_MISSING', '同名建表定义'],
      ['INPUT_MAPPING', 'FAILED', 'IMPORT_COLUMN_COUNT_MISMATCH', '列数'],
      ['INPUT_MAPPING', 'FAILED', 'IMPORT_CSV_WHITESPACE_UNQUOTED', '首尾空白'],
      ['INPUT_MAPPING', 'FAILED', 'IMPORT_REQUIRED_COLUMN_MISSING', '必填目标列'],
    ] as const) {
      const row = normalImportPrecheckPresentation(check(), { check: name, status, evidenceCode: code }, false)
      expect(row.text).toBe(status === 'FAILED' ? '未通过' : '无法确认'); expect(row.detail).toContain(detail)
    }
  })
  it('区分工具失败、工具前置阻断和数据库前置阻断', () => {
    const failed = normalImportPrecheckPresentation(check(), { check: 'TOOL_ENVIRONMENT', status: 'FAILED', evidenceCode: 'TOOL_RUNTIME_INVALID' }, false)
    expect(failed).toMatchObject({ status: 'error', text: '未通过' }); expect(failed.detail).toContain('OBLOADER')
    for (const name of ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'INPUT_MAPPING']) {
      const blocked = normalImportPrecheckPresentation(check(), { check: name, status: 'UNKNOWN', evidenceCode: 'PRECHECK_BLOCKED_BY_TOOL' }, false)
      expect(blocked).toMatchObject({ status: 'warning', text: '前置阻断，未执行' }); expect(blocked.detail).toContain('执行工具检查未通过')
    }
    expect(normalImportPrecheckPresentation(check(), { check: 'WRITE_ACCESS', status: 'UNKNOWN', evidenceCode: 'PRECHECK_BLOCKED_BY_DATABASE' }, false).detail).toContain('目标数据库连接检查未通过')
  })
  it('未启用检查明确反馈 Agent 配置问题，不伪装连接或权限失败', () => {
    for (const name of ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS', 'INPUT_MAPPING']) {
      const row = normalImportPrecheckPresentation(check(), { check: name, status: 'UNKNOWN', evidenceCode: 'NORMAL_IMPORT_PROBE_NOT_CONFIGURED' }, false)
      expect(row).toMatchObject({ status: 'warning', text: '未启用' }); expect(row.detail).toContain('节点未配置')
    }
  })
  it('等待和失效优先于旧通过事实，未知代码不作为原文展示', () => {
    const result = { check: 'TOOL_ENVIRONMENT', status: 'PASSED' as const, evidenceCode: 'TOOL_RUNTIME_READY' }
    expect(normalImportPrecheckPresentation(check('PENDING'), result, false).text).toBe('等待检查结果')
    expect(normalImportPrecheckPresentation(check('EXPIRED'), result, false).text).toBe('已失效')
    expect(normalImportPrecheckPresentation(check('SUCCEEDED'), result, true).status).toBe('warning')
    expect(normalImportPrecheckPresentation(check('SUCCEEDED'), result, false).status).toBe('success')
    const unknown = normalImportPrecheckPresentation(check(), { ...result, status: 'UNKNOWN', evidenceCode: 'synthetic-untrusted-detail' }, false)
    expect(unknown.text).toBe('无法确认'); expect(unknown.detail).not.toContain('synthetic-untrusted-detail')
    expect(normalImportPrecheckPresentation(null, undefined, false).text).toBe('未开始')
    expect(normalImportPrecheckPresentation(check('PENDING'), undefined, false).text).toBe('等待检查结果')
    expect(normalImportPrecheckPresentation(check('EXPIRED'), undefined, false).text).toBe('已失效')
  })
  it('总览按固定清单区分失败、前置阻断和缺项，额外结果不能填补缺项', () => {
    const report = check()
    report.results = [
      { check: 'TOOL_ENVIRONMENT', status: 'FAILED', evidenceCode: 'TOOL_RUNTIME_INVALID' },
      { check: 'DATABASE_CONNECTIVITY', status: 'UNKNOWN', evidenceCode: 'PRECHECK_BLOCKED_BY_TOOL' },
      { check: 'INPUT_FILES', status: 'PASSED', evidenceCode: 'IMPORT_VERIFIED' },
      { check: 'synthetic-extra', status: 'PASSED', evidenceCode: 'IMPORT_VERIFIED' },
    ]
    expect(normalImportBlockingSummary(report)).toBe('1 项未通过，1 项因前置条件未执行，3 项无法确认')
  })
  it('检查能力缺失与工具失败指向节点，业务失败指向对应配置', () => {
    const report = check()
    expect(normalImportPrecheckFixAction(report, { check: 'DATABASE_CONNECTIVITY', status: 'UNKNOWN', evidenceCode: 'NORMAL_IMPORT_PROBE_NOT_CONFIGURED' }, false)).toEqual({ target: 'NODE', label: '查看执行节点' })
    expect(normalImportPrecheckFixAction(report, { check: 'TOOL_ENVIRONMENT', status: 'FAILED', evidenceCode: 'TOOL_RUNTIME_INVALID' }, false)?.target).toBe('NODE')
    expect(normalImportPrecheckFixAction(report, { check: 'WRITE_ACCESS', status: 'FAILED', evidenceCode: 'IMPORT_WRITE_INSERT_REQUIRED' }, false)).toMatchObject({ target: 'STEP', step: 1 })
    expect(normalImportPrecheckFixAction(report, { check: 'INPUT_MAPPING', status: 'FAILED', evidenceCode: 'IMPORT_COLUMN_COUNT_MISMATCH' }, false)).toMatchObject({ target: 'STEP', step: 3 })
    expect(normalImportPrecheckFixAction(report, { check: 'NODE_RESOURCE', status: 'FAILED', evidenceCode: 'IMPORT_JVM_HEAP_TOO_SMALL' }, false)).toMatchObject({ target: 'STEP', step: 4 })
  })
  it('等待、失效、通过、缺项和前置阻断不产生误导修正动作', () => {
    const result: NormalImportCheck['results'][number] = { check: 'DATABASE_CONNECTIVITY', status: 'UNKNOWN', evidenceCode: 'NORMAL_IMPORT_PROBE_NOT_CONFIGURED' }
    for (const status of ['PENDING', 'LEASED', 'INVALIDATED', 'EXPIRED'] as const) expect(normalImportPrecheckFixAction(check(status), result, false)).toBeUndefined()
    expect(normalImportPrecheckFixAction(check(), result, true)).toBeUndefined()
    expect(normalImportPrecheckFixAction(null, result, false)).toBeUndefined()
    expect(normalImportPrecheckFixAction(check(), undefined, false)).toBeUndefined()
    expect(normalImportPrecheckFixAction(check(), { ...result, status: 'PASSED' }, false)).toBeUndefined()
    for (const evidenceCode of ['PRECHECK_BLOCKED_BY_TOOL', 'PRECHECK_BLOCKED_BY_DATABASE', 'DATABASE_CONNECTION_UNAVAILABLE']) expect(normalImportPrecheckFixAction(check(), { ...result, evidenceCode }, false)).toBeUndefined()
  })
})
