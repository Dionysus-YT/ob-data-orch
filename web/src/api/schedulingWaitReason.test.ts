import { expect, it } from 'vitest'
import { parseTaskExecutionFacts } from './taskExecutionFacts'
it('等待原因仅对未分配的等待任务提供固定平台约束，不接收其他任务事实', () => {
  expect(parseTaskExecutionFacts({ state: 'WAITING_SCHEDULE', schedulingWaitReason: 'PLATFORM_CONCURRENCY_LIMIT' }).schedulingWaitReason).toBe('PLATFORM_CONCURRENCY_LIMIT')
  for (const raw of [{ state: 'RUNNING', schedulingWaitReason: 'PLATFORM_CONCURRENCY_LIMIT' }, { state: 'WAITING_SCHEDULE', executionId: 'synthetic-execution', schedulingWaitReason: 'PLATFORM_CONCURRENCY_LIMIT' }, { state: 'WAITING_SCHEDULE', schedulingWaitReason: 'other-task-name' }]) expect(() => parseTaskExecutionFacts(raw)).toThrow()
})
