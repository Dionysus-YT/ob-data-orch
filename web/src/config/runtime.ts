export interface RuntimeConfig {
  stage: 'G1'
  realExecutionEnabled: false
}

interface RuntimeEnvironment {
  VITE_ENABLE_REAL_EXECUTION?: string
}

export function loadRuntimeConfig(environment: RuntimeEnvironment): RuntimeConfig {
  if (environment.VITE_ENABLE_REAL_EXECUTION?.trim().toLowerCase() === 'true') {
    throw new Error('Real execution cannot be enabled at development gate G1')
  }
  return {
    stage: 'G1',
    realExecutionEnabled: false,
  }
}

export const runtimeConfig = loadRuntimeConfig(import.meta.env)
