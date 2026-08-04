import type { ExecutionNodeAgentAssociationStatus, ExecutionNodePlatform } from '@/api/browser'

export function requiresAgentRegistration(status: ExecutionNodeAgentAssociationStatus, unavailableReasons: readonly string[]): boolean {
  return status === 'PENDING' || unavailableReasons.includes('RUNTIME_CONFIGURATION_MISMATCH')
}

export function agentRegistrationCommand(platform: ExecutionNodePlatform): string {
  if (platform === 'WINDOWS_AMD64') {
    return '.\\agent.exe -register'
  }
  return './agent -register'
}

export function agentRegistrationCode(nodeId: string, enrollmentId: string, enrollmentMaterial: string): string {
  const payload = JSON.stringify({
    formatVersion: 'obdo-r1',
    nodeId,
    enrollmentId,
    enrollmentMaterial,
  })
  return `obdo-r1.${btoa(payload).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/u, '')}`
}
