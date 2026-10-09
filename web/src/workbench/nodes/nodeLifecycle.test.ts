import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref, type EffectScope } from 'vue'
import type { ExecutionNodeDetail, ExecutionNodeEnrollment } from '@/api/browser'
import type { NodeApi } from './nodeSession'
import { useNodeDetail } from './useNodeDetail'
import { useNodeEnrollment } from './useNodeEnrollment'
import { useNodeForm } from './useNodeForm'
import { useNodeList } from './useNodeList'
import { useNodeListActions } from './useNodeListActions'
import { nodeApiFieldErrors, validateNodeForm } from './nodeFormRules'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
function detail(id = 'synthetic-a'): ExecutionNodeDetail {
  return { id, displayName: id, platform: 'WINDOWS_AMD64', managementState: 'DISABLED', agentAssociationStatus: 'ASSOCIATED', heartbeatStatus: 'ONLINE', lastHeartbeatAt: '2026-10-09T00:00:00Z', environmentStatus: 'NORMAL', capacityStatus: 'AVAILABLE', acceptsNewTasks: false, unavailableReasons: ['NODE_DISABLED'], revision: 1, updatedAt: '2026-10-09T00:00:00Z', createdAt: '2026-10-09T00:00:00Z', allowedRoots: ['/E:/synthetic'], toolHome: 'E:\\synthetic-tools', javaPath: 'C:\\synthetic-java' }
}
function apiMock() {
  return { getExecutionNode: vi.fn(async (id: string) => detail(id)), listExecutionNodes: vi.fn(async () => [detail()]), enableExecutionNode: vi.fn(async (id: string) => ({ ...detail(id), revision: 2 })), requestExecutionNodeEnvironmentCheck: vi.fn(async (id: string) => ({ ...detail(id), revision: 2 })), createExecutionNode: vi.fn(async () => 'synthetic-created'), updateExecutionNode: vi.fn(async (id: string) => ({ ...detail(id), revision: 2 })), deleteOrArchiveExecutionNode: vi.fn(async () => ({ outcome: 'ARCHIVED' as const, revision: 2, agentAccessRevoked: true })), issueExecutionNodeEnrollment: vi.fn(async (id: string): Promise<ExecutionNodeEnrollment> => ({ requestId: 'synthetic-request', enrollmentId: 'synthetic-enrollment', nodeId: id, enrollmentMaterial: 'synthetic-only-material', expiresAt: '2099-01-01T00:00:00Z', displayedOnce: true })) } satisfies NodeApi
}
const scopes: EffectScope[] = []
function mount<T>(fn: () => T) { const scope = effectScope(); scopes.push(scope); return { value: scope.run(fn)!, stop: () => scope.stop() } }
async function flush() { await Promise.resolve(); await Promise.resolve(); await Promise.resolve() }
afterEach(() => { for (const scope of scopes.splice(0)) scope.stop(); vi.useRealTimers() })

describe('节点详情事实与操作生命周期', () => {
  it('A → B → A 复用时旧读取不能复活，路由切换立即清空旧事实并取消等待', async () => {
    const api = apiMock(); const old = deferred<ExecutionNodeDetail>(); api.getExecutionNode.mockImplementationOnce(() => old.promise)
    const id = ref('synthetic-a'); const signals: AbortSignal[] = []
    const { value } = mount(() => useNodeDetail(id, signal => { signals.push(signal); return api }))
    id.value = 'synthetic-b'; expect(value.node.value).toBeUndefined(); await flush()
    expect(value.node.value?.id).toBe('synthetic-b'); expect(signals[0]?.aborted).toBe(true)
    id.value = 'synthetic-a'; await flush(); old.resolve({ ...detail(), revision: 99 }); await flush()
    expect(value.node.value?.revision).toBe(1)
  })
  it('旧写响应和 finally 不覆盖新节点或解除新节点的操作锁', async () => {
    const api = apiMock(); const a = deferred<ExecutionNodeDetail>(); const b = deferred<ExecutionNodeDetail>()
    api.enableExecutionNode.mockImplementationOnce(() => a.promise).mockImplementationOnce(() => b.promise)
    const id = ref('synthetic-a'); const { value } = mount(() => useNodeDetail(id, () => api)); await flush()
    const first = value.performPrimaryAction(); id.value = 'synthetic-b'; await flush(); const second = value.performPrimaryAction()
    a.resolve(detail()); await first; expect(value.node.value?.id).toBe('synthetic-b'); expect(value.actionBusy.value).toBe(true); expect(value.actionNotice.value).toBe('')
    b.resolve({ ...detail('synthetic-b'), revision: 2 }); await second; expect(value.node.value?.revision).toBe(2)
  })
  it('写事务使先前刷新失效且入口防重，环境检查最多一个定时器', async () => {
    vi.useFakeTimers(); const api = apiMock(); const read = deferred<ExecutionNodeDetail>(); const write = deferred<ExecutionNodeDetail>()
    const { value } = mount(() => useNodeDetail(ref('synthetic-a'), () => api)); await flush()
    api.getExecutionNode.mockImplementationOnce(() => read.promise); const refreshing = value.loadNode(true)
    api.enableExecutionNode.mockImplementationOnce(() => write.promise); const saving = value.performPrimaryAction(); await value.performPrimaryAction(); await value.loadNode(true)
    expect(api.enableExecutionNode).toHaveBeenCalledTimes(1)
    write.resolve({ ...detail(), revision: 2 }); await saving; read.resolve({ ...detail(), revision: 0 }); await refreshing
    expect(value.node.value?.revision).toBe(2)
    value.node.value = { ...detail(), environmentStatus: 'EXPIRED' }
    await value.performPrimaryAction(); value.node.value = { ...detail(), environmentStatus: 'EXPIRED' }; await value.performPrimaryAction()
    expect(vi.getTimerCount()).toBe(1); await vi.advanceTimersByTimeAsync(2500); expect(api.getExecutionNode).toHaveBeenCalledTimes(3)
  })
  it.each([401, 403, 404])('身份或对象失效 %s 时清除可信事实', async status => {
    const api = apiMock(); const { value } = mount(() => useNodeDetail(ref('synthetic-a'), () => api)); await flush()
    api.getExecutionNode.mockRejectedValueOnce({ status }); await value.loadNode(true); expect(value.node.value).toBeUndefined()
  })
  it('刷新服务错误保留事实，卸载后读取和环境操作不得回写或创建定时器', async () => {
    vi.useFakeTimers(); const api = apiMock(); const id = ref('synthetic-a'); const { value, stop } = mount(() => useNodeDetail(id, () => api)); await flush()
    api.getExecutionNode.mockRejectedValueOnce({ status: 503 }); await value.loadNode(true); expect(value.node.value?.id).toBe(id.value)
    value.node.value = { ...detail(), environmentStatus: 'EXPIRED' }; const write = deferred<ExecutionNodeDetail>(); api.requestExecutionNodeEnvironmentCheck.mockImplementationOnce(() => write.promise)
    const pending = value.performPrimaryAction(); stop(); write.resolve(detail()); await pending
    expect(value.node.value).toBeUndefined(); expect(value.actionNotice.value).toBe(''); expect(vi.getTimerCount()).toBe(0)
    await value.loadNode(); expect(api.getExecutionNode).toHaveBeenCalledTimes(2)
  })
  it('节点切换和卸载均取消已安排的环境复读', async () => {
    vi.useFakeTimers(); const api = apiMock(); api.getExecutionNode.mockImplementation(async id => ({ ...detail(id), environmentStatus: 'EXPIRED' }))
    const id = ref('synthetic-a'); const { value, stop } = mount(() => useNodeDetail(id, () => api)); await flush()
    await value.performPrimaryAction(); expect(vi.getTimerCount()).toBe(1); id.value = 'synthetic-b'; await flush(); expect(vi.getTimerCount()).toBe(0)
    await value.performPrimaryAction(); stop(); await vi.advanceTimersByTimeAsync(3000); expect(api.getExecutionNode).toHaveBeenCalledTimes(2)
  })
})

describe('节点登记与编辑唯一表单', () => {
  it('组件卸载后迟到读取不能回填表单或详情，HTTP 信号已取消', async () => {
    const api = apiMock(); const read = deferred<ExecutionNodeDetail>(); api.getExecutionNode.mockImplementation(() => read.promise)
    const signals: AbortSignal[] = []
    const form = mount(() => useNodeForm(ref('synthetic-a'), ref(false), vi.fn(), async () => {}, signal => { signals.push(signal); return api }))
    const page = mount(() => useNodeDetail(ref('synthetic-a'), signal => { signals.push(signal); return api }))
    form.stop(); page.stop(); read.resolve(detail()); await flush()
    expect(form.value.form.displayName).toBe(''); expect(form.value.failure.value).toBe(''); expect(page.value.node.value).toBeUndefined(); expect(signals.every(signal => signal.aborted)).toBe(true)
  })

  it('编辑 A → B → 新建，清空旧表单并忽略迟到读取', async () => {
    const api = apiMock(); const old = deferred<ExecutionNodeDetail>(); api.getExecutionNode.mockImplementationOnce(() => old.promise)
    const id = ref('synthetic-a'); const isNew = ref(false); const { value } = mount(() => useNodeForm(id, isNew, vi.fn(), async () => {}, () => api))
    id.value = 'synthetic-b'; expect(value.form.displayName).toBe(''); await flush(); expect(value.form.displayName).toBe('synthetic-b')
    isNew.value = true; old.resolve(detail()); await flush(); expect(value.form.displayName).toBe(''); expect(value.node.value).toBeUndefined(); expect(value.loading.value).toBe(false)
  })
  it('新建保存防重并保持 DTO；离开新建后迟到成功不得导航', async () => {
    const api = apiMock(); const request = deferred<string>(); api.createExecutionNode.mockImplementationOnce(() => request.promise)
    const id = ref(''); const isNew = ref(true); const navigate = vi.fn(); const { value } = mount(() => useNodeForm(id, isNew, navigate, async () => {}, () => api))
    Object.assign(value.form, { displayName: ' 合成节点 ', toolHome: ' E:/synthetic-tools ', javaPath: ' C:/synthetic-java ', allowedRootsText: ' /E:/synthetic \r\n /F:/synthetic ' })
    const pending = value.save(); await value.save(); expect(api.createExecutionNode).toHaveBeenCalledTimes(1)
    expect(api.createExecutionNode).toHaveBeenCalledWith({ displayName: '合成节点', platform: 'WINDOWS_AMD64', toolHome: 'E:/synthetic-tools', javaPath: 'C:/synthetic-java', allowedRoots: ['/E:/synthetic', '/F:/synthetic'] })
    isNew.value = false; id.value = 'synthetic-b'; await flush(); request.resolve('synthetic-created'); await pending
    expect(navigate).not.toHaveBeenCalled(); expect(value.form.displayName).toBe('synthetic-b')
  })
  it('编辑捕获原节点修订；切换节点后迟到错误不得回写或聚焦', async () => {
    const api = apiMock(); const write = deferred<ExecutionNodeDetail>(); api.updateExecutionNode.mockImplementationOnce(() => write.promise)
    const id = ref('synthetic-a'); const focus = vi.fn(async () => {}); const { value } = mount(() => useNodeForm(id, ref(false), vi.fn(), focus, () => api)); await flush()
    const pending = value.save(); await value.save(); expect(api.updateExecutionNode).toHaveBeenCalledTimes(1); expect(api.updateExecutionNode.mock.calls[0]?.slice(0, 2)).toEqual(['synthetic-a', 1])
    id.value = 'synthetic-b'; await flush(); write.reject({ fieldErrors: [{ field: 'displayName', message: '合成错误' }] }); await pending
    expect(value.formErrors).toEqual({}); expect(focus).not.toHaveBeenCalled(); expect(value.busy.value).toBe(false)
  })
  it('成功新建导航及编辑回填保持原行为，卸载后不导航', async () => {
    const api = apiMock(); const navigate = vi.fn(); const { value, stop } = mount(() => useNodeForm(ref('synthetic-a'), ref(false), navigate, async () => {}, () => api)); await flush()
    await value.save(); expect(value.node.value?.revision).toBe(2); expect(value.notice.value).toContain('无需重新注册')
    stop(); await value.save(); expect(api.updateExecutionNode).toHaveBeenCalledTimes(1)
    const newForm = mount(() => useNodeForm(ref(''), ref(true), navigate, async () => {}, () => api)).value
    Object.assign(newForm.form, { displayName: '合成节点', toolHome: 'synthetic', javaPath: 'synthetic', allowedRootsText: '/E:/synthetic' }); await newForm.save(); expect(navigate).toHaveBeenCalledWith('synthetic-created')
  })
  it('缺少节点 ID 结束加载，非法路径与服务端字段白名单保持原规则', async () => {
    const api = apiMock(); const focus = vi.fn(async () => {}); const { value } = mount(() => useNodeForm(ref(''), ref(false), vi.fn(), focus, () => api))
    expect(value.loading.value).toBe(false); expect(value.failure.value).toBe('未指定执行节点。'); expect(api.getExecutionNode).not.toHaveBeenCalled()
    expect(validateNodeForm({ displayName: '', platform: 'WINDOWS_AMD64', toolHome: '', javaPath: '', allowedRoots: ['/E:/../synthetic'] }).allowedRoots).toBeDefined()
    expect(validateNodeForm({ displayName: '合成', platform: 'LINUX_ARM64', toolHome: '/synthetic', javaPath: '/synthetic', allowedRoots: ['relative'] }).allowedRoots).toBeDefined()
    expect(nodeApiFieldErrors({ fieldErrors: [{ field: 'unknown', message: '合成错误' }, { field: 'platform', message: '合成错误' }] })).toEqual({ platform: '合成错误' })
  })
})

describe('节点列表读取、启用与删除生命周期', () => {
  it('旧列表响应不得覆盖启用新修订，失败刷新保留可信事实，403 清空列表', async () => {
    const api = apiMock(); const { value: { list, actions } } = mount(() => { const list = useNodeList(() => api); return { list, actions: useNodeListActions(list) } }); await list.loadNodes()
    const read = deferred<ExecutionNodeDetail[]>(); api.listExecutionNodes.mockImplementationOnce(() => read.promise); const pending = list.loadNodes(true)
    await actions.runPrimaryAction(detail(), 'enable'); read.resolve([detail()]); await pending; expect(list.nodes.value[0]?.revision).toBe(2)
    api.listExecutionNodes.mockRejectedValueOnce({ status: 503 }); await list.loadNodes(true); expect(list.nodes.value).toHaveLength(1)
    api.listExecutionNodes.mockRejectedValueOnce({ status: 403 }); await list.loadNodes(true); expect(list.nodes.value).toHaveLength(0); expect(list.restricted.value).toBe(true)
  })
  it('删除防重保持版本及归档撤销事实；卸载后没有刷新或反馈', async () => {
    const api = apiMock(); const deletion = deferred<{ outcome: 'ARCHIVED'; revision: number; agentAccessRevoked: boolean }>(); api.deleteOrArchiveExecutionNode.mockImplementationOnce(() => deletion.promise)
    const { value: { list, actions }, stop } = mount(() => { const list = useNodeList(() => api); return { list, actions: useNodeListActions(list) } }); await list.loadNodes()
    actions.deletionTarget.value = detail(); const pending = actions.deleteOrArchiveNode(); await actions.deleteOrArchiveNode(); expect(api.deleteOrArchiveExecutionNode).toHaveBeenCalledWith('synthetic-a', 1)
    deletion.resolve({ outcome: 'ARCHIVED', revision: 2, agentAccessRevoked: true }); await pending; expect(actions.notice.value).toContain('已撤销'); expect(api.deleteOrArchiveExecutionNode).toHaveBeenCalledTimes(1)
    const late = deferred<ExecutionNodeDetail>(); api.requestExecutionNodeEnvironmentCheck.mockImplementationOnce(() => late.promise)
    const running = actions.runPrimaryAction({ ...detail(), environmentStatus: 'EXPIRED' }, 'environment-check'); const notice = actions.notice.value; stop(); late.resolve(detail()); await running
    expect(actions.notice.value).toBe(notice); expect(api.listExecutionNodes).toHaveBeenCalledTimes(2)
  })
})

describe('一次性注册材料的内存生命周期', () => {
  it('卸载后迟到签发不能恢复材料或弹层状态', async () => {
    const api = apiMock(); api.getExecutionNode.mockImplementation(async id => ({ ...detail(id), agentAssociationStatus: 'PENDING' }))
    const request = deferred<ExecutionNodeEnrollment>(); api.issueExecutionNodeEnrollment.mockImplementation(() => request.promise)
    const { value, stop } = mount(() => { const state = useNodeDetail(ref('synthetic-a'), () => api); return useNodeEnrollment(state.node, state.session) }); await flush()
    const pending = value.issueEnrollment(); stop(); request.reject({ status: 503 }); await pending
    expect(value.enrollment.value).toBeUndefined(); expect(value.enrollmentFailure.value).toBe(''); expect(value.enrollmentDialogOpen.value).toBe(false); expect(value.enrollmentBusy.value).toBe(false)
  })

  it('关闭后迟到签发不得恢复材料；重新打开只接受本次签发', async () => {
    const api = apiMock(); api.getExecutionNode.mockImplementation(async id => ({ ...detail(id), agentAssociationStatus: 'PENDING' })); const first = deferred<ExecutionNodeEnrollment>(); api.issueExecutionNodeEnrollment.mockImplementationOnce(() => first.promise)
    const { value: { enrollment } } = mount(() => { const detailState = useNodeDetail(ref('synthetic-a'), () => api); detailState.node.value = { ...detail(), agentAssociationStatus: 'PENDING' }; return { enrollment: useNodeEnrollment(detailState.node, detailState.session) } })
    const pending = enrollment.issueEnrollment(); await enrollment.issueEnrollment(); expect(api.issueExecutionNodeEnrollment).toHaveBeenCalledTimes(1)
    enrollment.clearEnrollment(); await enrollment.issueEnrollment(); first.resolve({ requestId: 'synthetic-old-request', enrollmentId: 'synthetic-old-enrollment', nodeId: 'synthetic-a', enrollmentMaterial: 'synthetic-old-material', expiresAt: '2099-01-01T00:00:00Z', displayedOnce: true }); await pending
    expect(enrollment.enrollment.value?.enrollmentId).toBe('synthetic-enrollment'); enrollment.clearEnrollment(); expect(enrollment.registrationCode.value).toBe(''); expect(enrollment.enrollment.value).toBeUndefined()
  })
  it('复制迟到反馈不进入关闭或新节点弹层，关联成功与卸载立即清除材料', async () => {
    const api = apiMock(); api.getExecutionNode.mockImplementation(async id => ({ ...detail(id), agentAssociationStatus: 'PENDING' }))
    const copy = deferred<void>(); const id = ref('synthetic-a')
    const { value: { state, enrollment }, stop } = mount(() => { const state = useNodeDetail(id, () => api); return { state, enrollment: useNodeEnrollment(state.node, state.session, () => copy.promise) } }); await flush()
    await enrollment.issueEnrollment(); const pending = enrollment.copyEnrollmentValue('注册码', enrollment.registrationCode.value)
    id.value = 'synthetic-b'; expect(enrollment.registrationCode.value).toBe(''); await flush(); await enrollment.issueEnrollment(); copy.resolve(); await pending
    expect(enrollment.copyNotice.value).toBe(''); state.node.value = detail('synthetic-b'); expect(enrollment.enrollmentDialogOpen.value).toBe(false); expect(enrollment.enrollment.value).toBeUndefined()
    state.node.value = { ...detail('synthetic-b'), agentAssociationStatus: 'PENDING' }; await enrollment.issueEnrollment(); stop(); expect(enrollment.registrationCode.value).toBe(''); expect(enrollment.enrollment.value).toBeUndefined()
  })
})
