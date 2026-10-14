import { describe, expect, it } from 'vitest'
import type { NodeInspectionItem, NodeRuntimeInspection } from '@/api/browser'
import { nodeInspectionKinds } from '@/api/nodeRuntimeFacts'
import { nodeUnavailableReasonLabel } from './executionNodePresentation'
import { inspectionBlockingSummary, inspectionEmptyMessage, inspectionItemGuidance } from './nodeInspectionPresentation'

function item(kind: NodeInspectionItem['kind'], status: NodeInspectionItem['status'], code: string, extra: Partial<NodeInspectionItem> = {}): NodeInspectionItem {
  return { kind, status, code, required: !['OBLOADER_RUNTIME', 'TOOL_LOG', 'DIRECT_LOAD_CONFIG'].includes(kind), ...extra }
}

function report(changes: NodeInspectionItem[]): NodeRuntimeInspection {
  return { checkId: 'synthetic-check', runtimeConfigurationDigest: 'a'.repeat(64), observedAt: '2026-10-12T00:00:00Z', versionProbeAllowed: true,
    items: nodeInspectionKinds.map(kind => changes.find(change => change.kind === kind) ?? item(kind, 'PASSED', 'READY')),
    directLoad: { status: 'MISSING', hasUnrecognizedKeys: false } }
}

describe('环境检查的可行动解释', () => {
  it('缺文件与目录写入失败分别指向配置和运行账号权限，隐藏受控路径', () => {
    const missing = inspectionItemGuidance(item('OBDUMPER_RUNTIME', 'FAILED', 'MISSING', { path: 'E:\\synthetic-private-tool\\bin\\windows\\obdumper.bat' }))!
    expect(missing.reason).toContain('文件或目录不存在')
    expect(missing.action).toContain('安装根目录')
    const permission = inspectionItemGuidance(item('WORKSPACE', 'FAILED', 'NOT_WRITABLE'))!
    expect(permission.reason).toContain('创建或清理临时文件')
    expect(permission.action).toContain('写入和删除权限')
    expect(JSON.stringify(missing)).not.toContain('synthetic-private-tool')
  })

  it('版本不兼容显示已取得版本和正确基线，不把未执行或超时说成版本不支持', () => {
    const java = inspectionItemGuidance(item('JAVA_RUNTIME', 'FAILED', 'VERSION_UNSUPPORTED', { version: '17.0.1' }))!
    const tool = inspectionItemGuidance(item('OBDUMPER_RUNTIME', 'FAILED', 'VERSION_UNSUPPORTED', { version: '4.3.6-RELEASE' }))!
    expect(java.reason).toContain('17.0.1')
    expect(java.reason).toContain('Java 8')
    expect(tool.reason).toContain('4.3.5')
    const unrequested = inspectionItemGuidance(item('JAVA_RUNTIME', 'UNKNOWN', 'VERSION_PROBE_NOT_REQUESTED'))!
    expect(unrequested.reason).toContain('没有执行版本检查')
    expect(unrequested.action).toContain('若刚检查过')
    expect(unrequested.action).toContain('同步更新')
    const timeout = inspectionItemGuidance(item('OBDUMPER_RUNTIME', 'UNKNOWN', 'VERSION_PROBE_TIMEOUT'))!
    expect(timeout.reason).toContain('超过等待时间')
    expect(timeout.reason).not.toContain('不支持')
  })

  it('必需项失败和未确认分开汇总，可选项异常不造成阻断', () => {
    const inspection = report([item('JAVA_RUNTIME', 'UNKNOWN', 'VERSION_UNAVAILABLE'), item('WORKSPACE', 'FAILED', 'NOT_WRITABLE'), item('OBLOADER_RUNTIME', 'FAILED', 'MISSING')])
    const summary = inspectionBlockingSummary(inspection)!
    expect(summary).toContain('工作目录未通过')
    expect(summary).toContain('Java 环境未取得结果')
    expect(summary).not.toContain('导入工具')
    expect(inspectionBlockingSummary(report([item('OBLOADER_RUNTIME', 'FAILED', 'MISSING')]))).toBeUndefined()
    expect(inspectionBlockingSummary(report([item('JAVA_RUNTIME', 'UNKNOWN', 'VERSION_UNAVAILABLE')]))).toContain('结果待确认')
  })

  it('只有当前异常检查的清单能解释当前阻断，旧清单和过期事实不冒充当前原因', () => {
    const inspection = report([item('WORKSPACE', 'FAILED', 'NOT_WRITABLE')])
    const context = { environmentStatus: 'ABNORMAL' as const, environmentCheck: { checkId: inspection.checkId, status: 'FAILED' as const, factsRevision: 1, inspection } }
    expect(nodeUnavailableReasonLabel('TOOL_RUNTIME_INVALID', context)).toContain('工作目录未通过')
    expect(nodeUnavailableReasonLabel('TOOL_RUNTIME_INVALID', { ...context, environmentCheck: { ...context.environmentCheck, checkId: 'synthetic-new-check' } })).not.toContain('工作目录')
    expect(nodeUnavailableReasonLabel('TOOL_RUNTIME_INVALID', { ...context, environmentStatus: 'EXPIRED' })).not.toContain('工作目录')
    expect(nodeUnavailableReasonLabel('TOOL_RUNTIME_UNAVAILABLE')).toContain('没有取得结果')
  })

  it('通过项不添加建议，未知码保持未知且不回显，不把等待和失败空明细混为一谈', () => {
    expect(inspectionItemGuidance(item('JAVA_RUNTIME', 'PASSED', 'READY'))).toBeUndefined()
    const unknown = inspectionItemGuidance(item('WORKSPACE', 'UNKNOWN', 'synthetic-unrecognized-code'))!
    expect(unknown.reason).toContain('尚未取得')
    expect(JSON.stringify(unknown)).not.toContain('synthetic-unrecognized-code')
    expect(inspectionEmptyMessage({ status: 'PENDING', factsRevision: 1 })).toContain('点击「刷新」查看')
    expect(inspectionEmptyMessage({ status: 'FAILED', factsRevision: 1 })).toContain('同步更新后重新检查')
    expect(inspectionEmptyMessage(undefined)).toContain('Agent 在线后')
  })
})
