import { expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import { normalImportNode } from '../../../../tests/fixtures/normal-import'
import { createDirectLoadForm } from './directLoadForm'
import { useDirectLoadReferences } from './useDirectLoadReferences'

it('旁路候选按平台策略重排仍保留选择，不为空选择补首项', async () => {
  const scope = effectScope()
  try {
    const form = createDirectLoadForm(); form.nodeId = normalImportNode.id
    const api = { listDataSources: vi.fn().mockResolvedValue([]), listExecutionNodes: vi.fn().mockResolvedValue([{ ...normalImportNode, id: 'synthetic-first' }, normalImportNode]), getDirectLoadParameters: vi.fn().mockResolvedValue([]) }
    const refs = scope.run(() => useDirectLoadReferences(form, api))!
    await refs.loadNodes(); expect(api.listExecutionNodes).toHaveBeenCalledWith({ forTaskSelection: true }); expect(form.nodeId).toBe(normalImportNode.id); expect(refs.selectedNode.value?.id).toBe(normalImportNode.id)
    form.nodeId = ''; await refs.loadNodes(); expect(form.nodeId).toBe('')
  } finally { scope.stop() }
})
