import { describe, expect, it } from 'vitest'
import type { ExecutionNodeSummary } from '@/api/browser'
import { nodePrimaryAction, nodeUnavailableReasonLabel } from './executionNodePresentation'

const base: ExecutionNodeSummary = {
  id: 'synthetic-node', displayName: '合成节点', platform: 'WINDOWS_AMD64',
  managementState: 'DISABLED', agentAssociationStatus: 'ASSOCIATED', heartbeatStatus: 'ONLINE',
  lastHeartbeatAt: '2026-09-16T00:00:00Z', environmentStatus: 'NORMAL', capacityStatus: 'AVAILABLE',
  acceptsNewTasks: false, unavailableReasons: ['NODE_DISABLED'], revision: 1, updatedAt: '2026-09-16T00:00:00Z',
}

describe('执行节点页面动作与阻断事实', () => {
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
