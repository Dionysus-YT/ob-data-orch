import type { ExecutionNodeAgentAssociationStatus, ExecutionNodePlatform } from '@/api/browser'

export function requiresAgentRegistration(status: ExecutionNodeAgentAssociationStatus): boolean {
  return status === 'PENDING'
}

export function agentRegistrationCommand(platform: ExecutionNodePlatform): string {
  if (platform === 'WINDOWS_AMD64') {
    return '.\\启动.cmd'
  }
  return './start.sh'
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
