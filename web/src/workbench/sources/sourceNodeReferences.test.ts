import { afterEach, describe, expect, it, vi } from 'vitest'
import { createRenderer, nextTick } from 'vue'
import type { ExecutionNodeCandidate } from '@/api/browser'
import { DATA_SOURCE_UI_FIXTURES } from './dataSourceUiFixture'
import { createVisualSourceGateway } from './sourceGateway'
import { useSourceEditor } from './useSourceEditor'

interface TestElement { children: TestElement[] }
const element = (): TestElement => ({ children: [] })
const renderer = createRenderer<TestElement, TestElement>({
  patchProp() {}, insert(child, parent) { parent.children.push(child) }, remove() {},
  createElement: element, createText: element, createComment: element,
  setText() {}, setElementText() {}, parentNode: () => null, nextSibling: () => null,
})
const apps: Array<() => void> = []
afterEach(() => { apps.splice(0).forEach(unmount => unmount()) })
const node: ExecutionNodeCandidate = { id: 'synthetic-node', displayName: '合成旧名称', platform: 'WINDOWS_AMD64' }

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

async function setup() {
  const api = {
    ...createVisualSourceGateway(DATA_SOURCE_UI_FIXTURES),
    listDataSourceConnectionTestNodeCandidates: vi.fn(async (): Promise<ExecutionNodeCandidate[]> => [node]),
    startDataSourceConnectionTest: vi.fn(),
  }
  let editor!: ReturnType<typeof useSourceEditor>
  const app = renderer.createApp({ setup() { editor = useSourceEditor(api, DATA_SOURCE_UI_FIXTURES[0]!.id, vi.fn()); return () => null } })
  app.mount(element())
  let mounted = true
  const unmount = () => { if (mounted) { mounted = false; app.unmount() } }
  apps.push(unmount)
  await nextTick()
  await Promise.resolve()
  expect(editor.source.value).toBeDefined()
  return { editor, api, unmount }
}

describe('数据源节点引用读取', () => {
  it('更新同一标识的名称，旧响应及旧失败不能覆盖新候选', async () => {
    const { editor, api } = await setup()
    editor.nodeId.value = node.id
    const old = deferred<ExecutionNodeCandidate[]>()
    const newer = deferred<ExecutionNodeCandidate[]>()
    api.listDataSourceConnectionTestNodeCandidates.mockReturnValueOnce(old.promise).mockReturnValueOnce(newer.promise)
    const first = editor.loadNodes()
    const second = editor.loadNodes()
    newer.resolve([{ ...node, displayName: '合成新名称' }])
    await second
    old.reject({ status: 403 })
    await first
    expect(editor.nodes.value[0]?.displayName).toBe('合成新名称')
    expect(editor.nodeId.value).toBe(node.id)
    expect(editor.nodeFailure.value).toBe('')
    expect(editor.nodeLoading.value).toBe(false)
  })

  it('刷新期间阻断连接测试，原节点删除后清除选择且不按同名替换', async () => {
    const { editor, api } = await setup()
    editor.nodeId.value = node.id
    const response = deferred<ExecutionNodeCandidate[]>()
    api.listDataSourceConnectionTestNodeCandidates.mockReturnValueOnce(response.promise)
    const loading = editor.loadNodes()
    await editor.startTest()
    expect(api.startDataSourceConnectionTest).not.toHaveBeenCalled()
    response.resolve([{ ...node, id: 'synthetic-new-node' }])
    await loading
    expect(editor.nodeId.value).toBe('')
    expect(editor.nodes.value[0]?.id).toBe('synthetic-new-node')
    await editor.startTest()
    expect(api.startDataSourceConnectionTest).not.toHaveBeenCalled()
  })

  it('卸载后的候选响应与新读取不再恢复旧会话', async () => {
    const { editor, api, unmount } = await setup()
    const response = deferred<ExecutionNodeCandidate[]>()
    api.listDataSourceConnectionTestNodeCandidates.mockReturnValueOnce(response.promise)
    const loading = editor.loadNodes()
    unmount()
    response.resolve([{ ...node, displayName: '合成迟到名称' }])
    await loading
    await editor.loadNodes()
    expect(editor.nodes.value[0]?.displayName).toBe('合成旧名称')
    expect(api.listDataSourceConnectionTestNodeCandidates).toHaveBeenCalledTimes(2)
  })
})
