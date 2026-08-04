import { describe, expect, it } from 'vitest'

import { agentRegistrationCode, agentRegistrationCommand, requiresAgentRegistration } from './executionNodeEnrollmentInstructions'

describe('执行节点 Agent 关联指引', () => {
  it('为 Windows 生成不含注册码的单命令注册入口', () => {
    const command = agentRegistrationCommand('WINDOWS_AMD64')

    expect(command).toBe('.\\agent.exe -register')
    expect(command).not.toMatch(/material|token|password/i)
  })

  it('为 Linux 保留相同的单命令注册语义', () => {
    const command = agentRegistrationCommand('LINUX_ARM64')

    expect(command).toBe('./agent -register')
    expect(command).not.toMatch(/material|token|password/i)
  })

  it('将关联所需标识和一次性材料封装为可粘贴注册码', () => {
    const code = agentRegistrationCode('node-1', 'enrollment-1', 'synthetic-enrollment-material-0123456789')
    const encoded = code.slice('obdo-r1.'.length).replaceAll('-', '+').replaceAll('_', '/')
    const payload = JSON.parse(atob(encoded))

    expect(code.startsWith('obdo-r1.')).toBe(true)
    expect(payload).toEqual({
      formatVersion: 'obdo-r1',
      nodeId: 'node-1',
      enrollmentId: 'enrollment-1',
      enrollmentMaterial: 'synthetic-enrollment-material-0123456789',
    })
  })

  it('配置漂移时允许已关联节点重新签发注册码', () => {
    expect(requiresAgentRegistration('PENDING', [])).toBe(true)
    expect(requiresAgentRegistration('ASSOCIATED', ['RUNTIME_CONFIGURATION_MISMATCH'])).toBe(true)
    expect(requiresAgentRegistration('ASSOCIATED', ['NODE_DISABLED'])).toBe(false)
  })
})
