import { describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import type { TaskListItem } from '@/api/browser'
import { useTaskListActions, type TaskListActionApi } from './useTaskListActions'

describe('任务保存模板的未决请求', () => {
  it('响应丢失后冻结名称、原键重试，成功后清理并允许显式新保存', async () => {
    const save = vi.fn(async () => 'synthetic-template')
    const api = { saveTaskTemplate: save } as unknown as TaskListActionApi
    const scope = effectScope()
    const actions = scope.run(() => useTaskListActions({ createApi: () => api, navigate: vi.fn(), beginMutation: vi.fn(), endMutation: vi.fn() }))!
    const task = { id: 'synthetic-task', type: 'OBDUMPER_EXPORT', availableActions: ['SAVE_TEMPLATE'] } as unknown as TaskListItem
    try {
      actions.request(task, 'SAVE_TEMPLATE'); actions.updateTemplateName('合成模板')
      save.mockRejectedValueOnce({ status: 503 }); await actions.confirm()
      expect(actions.templateResultUnknown.value).toBe(true)
      actions.updateTemplateName('不应替换的名称')
      expect(actions.templateName.value).toBe('合成模板')
      await actions.confirm()
      const calls = save.mock.calls as unknown as unknown[][]
      expect(calls[0]).toEqual(calls[1])
      expect(actions.templateName.value).toBe('')
      expect(actions.templateResultUnknown.value).toBe(false)
      expect(actions.failure.value).toBe('')
      actions.request(task, 'SAVE_TEMPLATE'); actions.updateTemplateName('合成模板'); await actions.confirm()
      expect(calls[2]?.[2]).not.toEqual(calls[0]?.[2])
    } finally { scope.stop() }
  })
})
