import { describe, expect, it } from 'vitest'

import { loadRuntimeConfig } from './runtime'

describe('loadRuntimeConfig', () => {
  it('keeps real execution disabled by default', () => {
    expect(loadRuntimeConfig({})).toEqual({
      stage: 'G2',
      realExecutionEnabled: false,
    })
  })

  it('rejects attempts to enable real execution', () => {
    expect(() => loadRuntimeConfig({ VITE_ENABLE_REAL_EXECUTION: 'true' })).toThrow(
      'Real execution cannot be enabled at development gate G2',
    )
  })
})
