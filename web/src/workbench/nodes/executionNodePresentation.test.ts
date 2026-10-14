import { describe, expect, it } from 'vitest'
import type { ExecutionNodeSummary } from '@/api/browser'
import { nodeCategoryStates, nodePrimaryAction, nodeUnavailableReasonLabel } from './executionNodePresentation'
import { nodeAgentPlatformLabel } from './nodePlatformRules'

const base: ExecutionNodeSummary = {
  id: 'synthetic-node', displayName: '合成节点', platform: 'WINDOWS_AMD64',
  managementState: 'DISABLED', agentAssociationStatus: 'ASSOCIATED', heartbeatStatus: 'ONLINE',
  lastHeartbeatAt: '2026-09-16T00:00:00Z', environmentStatus: 'NORMAL', capacityStatus: 'AVAILABLE',
  acceptsNewTasks: false, unavailableReasons: ['NODE_DISABLED'], revision: 1, updatedAt: '2026-09-16T00:00:00Z',
}

describe('执行节点页面动作与阻断事实', () => {
  it.each([
    ['WINDOWS', 'AMD64', 'Windows · x86_64'],
    ['linux', 'amd64', 'Linux · x86_64'],
    ['LINUX', 'ARM64', 'Linux · aarch64'],
    ['synthetic-os', 'synthetic-arch', 'synthetic-os · synthetic-arch'],
  ])('上报平台 %s / %s 仅格式化已知名称，未知值保留供核对', (os, arch, label) => {
    expect(nodeAgentPlatformLabel(os, arch)).toBe(label)
  })
  it('各类状态独立映射，通用资格不覆盖异常类别或推导模块可用', () => {
    const states = nodeCategoryStates({ ...base, acceptsNewTasks: true, heartbeatStatus: 'OFFLINE', environmentStatus: 'EXPIRED', capacityStatus: 'BUSY' })
    expect(states.map(state => [state.key, state.text, state.status])).toEqual([
      ['management', '已禁用', 'default'], ['agent', '离线', 'error'], ['environment', '检查已失效', 'warning'], ['capacity', '已满', 'warning'],
    ])
    expect(nodeCategoryStates({ ...base, agentAssociationStatus: 'PENDING', heartbeatStatus: 'NEVER_CONNECTED' })[1]?.text).toBe('待关联')
    expect(nodeCategoryStates({ ...base, unavailableReasons: ['ENVIRONMENT_CHECK_IN_PROGRESS'] })[2]?.text).toBe('检查中')
    expect(nodeCategoryStates({ ...base, capacityStatus: 'UNKNOWN' })[3]?.text).toBe('未知')
  })
  it('只在当前 Agent 与环境事实允许时提供启用或只读环境检查', () => {
    expect(nodePrimaryAction(base)).toBe('enable')
    expect(nodePrimaryAction({ ...base, heartbeatStatus: 'OFFLINE' })).toBeUndefined()
    expect(nodePrimaryAction({ ...base, environmentStatus: 'EXPIRED' })).toBe('environment-check')
    expect(nodePrimaryAction({ ...base, environmentStatus: 'EXPIRED', unavailableReasons: ['ENVIRONMENT_CHECK_IN_PROGRESS'] })).toBeUndefined()
    expect(nodePrimaryAction({ ...base, environmentStatus: 'EXPIRED', unavailableReasons: ['RUNTIME_CONFIGURATION_MISMATCH'] })).toBeUndefined()
    expect(nodePrimaryAction({ ...base, capacityStatus: 'BUSY' })).toBeUndefined()
  })

  it('没有可解释原因时保持未知，不显示可用结论', () => {
    expect(nodeUnavailableReasonLabel('')).toBe('控制面未提供具体阻断原因')
    expect(nodeUnavailableReasonLabel('UNRECOGNIZED_REASON')).toContain('未识别')
  })
})
