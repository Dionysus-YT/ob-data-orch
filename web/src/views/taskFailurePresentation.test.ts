import { describe, expect, it } from 'vitest'

import { taskFailureSummary } from './taskFailurePresentation'

describe('任务失败摘要', () => {
  it('优先展示最后一条具体且已脱敏的工具错误，不使用通用退出码覆盖它', () => {
    const summary = taskFailureSummary('FAILED', [
      { sourceSeq: 1, kind: 'LOG', message: '2026-07-31 [ERROR] Dump failed! Error: synthetic native output access failed', integrityCode: '', receivedAt: '2026-07-31T01:00:00Z' },
      { sourceSeq: 2, kind: 'LOG', message: '2026-07-31 [ERROR] System exit 1', integrityCode: '', receivedAt: '2026-07-31T01:00:01Z' },
    ])

    expect(summary).toContain('synthetic native output access failed')
    expect(summary).not.toContain('System exit 1')
  })

  it('没有工具错误日志时保留失败事实但不猜测原因', () => {
    expect(taskFailureSummary('FAILED', [])).toContain('暂未收到')
    expect(taskFailureSummary('RUNNING', [])).toBeUndefined()
  })
})
