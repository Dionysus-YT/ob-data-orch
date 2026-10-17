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
  return { getExecutionNode: vi.fn(async (id: string) => detail(id)), listExecutionNodes: vi.fn(async () => [detail()]), enableExecutionNode: vi.fn(async (id: string) => ({ ...detail(id), revision: 2 })), requestExecutionNodeEnvironmentCheck: vi.fn(async (id: string) => ({ ...detail(id), revision: 2 })), createExecutionNode: vi.fn(async () => 'synthetic-created'), updateExecutionNode: vi.fn(async (id: string) => ({ ...detail(id), revision: 2 })), deleteExecutionNode: vi.fn(async () => ({ outcome: 'DELETED' as const, revision: 0, agentAccessRevoked: true })), issueExecutionNodeEnrollment: vi.fn(async (id: string): Promise<ExecutionNodeEnrollment> => ({ requestId: 'synthetic-request', enrollmentId: 'synthetic-enrollment', nodeId: id, enrollmentMaterial: 'synthetic-only-material', expiresAt: '2099-01-01T00:00:00Z', displayedOnce: true })) } satisfies NodeApi
}
const scopes: EffectScope[] = []
function mount<T>(fn: () => T) { const scope = effectScope(); scopes.push(scope); return { value: scope.run(fn)!, stop: () => scope.stop() } }
async function flush() { await Promise.resolve(); await Promise.resolve(); await Promise.resolve() }
afterEach(() => { for (const scope of scopes.splice(0)) scope.stop(); vi.useRealTimers() })

describe('节点详情事实与操作生命周期', () => {
  it.each(['ENABLED', 'DISABLED'] as const)('打开详情承接自动检查，完成后保留 %s 管理意图且不发送写请求', async managementState => {
    vi.useFakeTimers()
    const api = apiMock()
    const pending: ExecutionNodeDetail = { ...detail(), managementState, environmentStatus: 'UNCONFIRMED', unavailableReasons: ['ENVIRONMENT_CHECK_IN_PROGRESS'], environmentCheck: { checkId: 'automatic-check', status: 'PENDING', factsRevision: 2 } }
    api.getExecutionNode.mockResolvedValueOnce(pending).mockResolvedValue({ ...detail(), managementState, acceptsNewTasks: managementState === 'ENABLED', unavailableReasons: managementState === 'ENABLED' ? [] : ['NODE_DISABLED'], environmentCheck: { checkId: 'automatic-check', status: 'PASSED', code: 'TOOL_RUNTIME_READY', factsRevision: 2 } })
    const { value } = mount(() => useNodeDetail(ref('synthetic-a'), () => api))
    await flush()
    expect(value.node.value?.environmentCheck?.status).toBe('PENDING')
    await vi.advanceTimersByTimeAsync(2500)
    expect(value.node.value?.managementState).toBe(managementState)
    expect(value.node.value?.acceptsNewTasks).toBe(managementState === 'ENABLED')
    expect(api.getExecutionNode).toHaveBeenCalledTimes(2)
    expect(api.requestExecutionNodeEnvironmentCheck).not.toHaveBeenCalled()
    expect(api.enableExecutionNode).not.toHaveBeenCalled()
    expect(value.refreshCompleted.value).toBe(false)
    expect(value.actionNotice.value).toBe('')
  })

  it('正常已启用节点也可主动复验，方法入口防重且不发送启用动作', async () => {
    const api = apiMock()
    api.getExecutionNode.mockResolvedValue({ ...detail(), managementState: 'ENABLED', acceptsNewTasks: true, unavailableReasons: [] })
    const { value } = mount(() => useNodeDetail(ref('synthetic-a'), () => api))
    await flush()
    const completion = deferred<ExecutionNodeDetail>()
    api.requestExecutionNodeEnvironmentCheck.mockImplementationOnce(() => completion.promise)
    const running = value.requestEnvironmentCheck()
    await value.requestEnvironmentCheck()
    expect(api.requestExecutionNodeEnvironmentCheck).toHaveBeenCalledTimes(1)
    completion.resolve({ ...detail(), managementState: 'ENABLED', environmentStatus: 'UNCONFIRMED', unavailableReasons: ['ENVIRONMENT_CHECK_IN_PROGRESS'], environmentCheck: { status: 'PENDING', factsRevision: 2 } })
    await running
    expect(value.node.value?.managementState).toBe('ENABLED')
    expect(api.enableExecutionNode).not.toHaveBeenCalled()
  })

  it('自动复验同一检查最多复读两分钟，手动刷新不无限续期，待授权不自动复读', async () => {
    vi.useFakeTimers()
    const api = apiMock()
    const pending: ExecutionNodeDetail = { ...detail(), environmentStatus: 'UNCONFIRMED', unavailableReasons: ['ENVIRONMENT_CHECK_IN_PROGRESS'], environmentCheck: { checkId: 'automatic-check', status: 'PENDING', factsRevision: 2 } }
    api.getExecutionNode.mockResolvedValue(pending)
    const { value } = mount(() => useNodeDetail(ref('synthetic-a'), () => api))
    await flush()
    await vi.advanceTimersByTimeAsync(120000)
    expect(vi.getTimerCount()).toBe(0)
    await value.loadNode(true)
    expect(vi.getTimerCount()).toBe(0)
    const calls = api.getExecutionNode.mock.calls.length
    await vi.advanceTimersByTimeAsync(120000)
    expect(api.getExecutionNode).toHaveBeenCalledTimes(calls)
    api.getExecutionNode.mockResolvedValue({ ...pending, unavailableReasons: ['ENVIRONMENT_CHECK_AUTHORIZATION_REQUIRED'], environmentCheck: { ...pending.environmentCheck!, checkId: 'unauthorized-check' } })
    await value.loadNode(true)
    expect(vi.getTimerCount()).toBe(0)
    expect(api.requestExecutionNodeEnvironmentCheck).not.toHaveBeenCalled()
  })

  it.each(['切换节点', '卸载', '读取失败', '离线'])('自动复验在%s后停止，迟到读取不能恢复旧会话或继续计时', async boundary => {
    vi.useFakeTimers()
    const api = apiMock()
    const pending: ExecutionNodeDetail = { ...detail(), environmentStatus: 'UNCONFIRMED', unavailableReasons: ['ENVIRONMENT_CHECK_IN_PROGRESS'], environmentCheck: { checkId: 'automatic-check', status: 'PENDING', factsRevision: 2 } }
    const read = deferred<ExecutionNodeDetail>()
    api.getExecutionNode.mockResolvedValueOnce(pending).mockImplementationOnce(() => read.promise)
    const id = ref('synthetic-a')
    const { value, stop } = mount(() => useNodeDetail(id, () => api))
    await flush()
    await vi.advanceTimersByTimeAsync(2500)
    if (boundary === '切换节点') { id.value = 'synthetic-b'; await flush(); read.resolve(pending) }
    else if (boundary === '卸载') { stop(); read.resolve(pending) }
    else if (boundary === '读取失败') read.reject({ status: 503, code: 'EXECUTION_NODE_QUERY_UNAVAILABLE' })
    else read.resolve({ ...pending, heartbeatStatus: 'OFFLINE' })
    await flush()
    expect(vi.getTimerCount()).toBe(0)
    const calls = api.getExecutionNode.mock.calls.length
    await vi.advanceTimersByTimeAsync(10000)
    expect(api.getExecutionNode).toHaveBeenCalledTimes(calls)
    if (boundary === '切换节点') expect(value.node.value?.id).toBe('synthetic-b')
    if (boundary === '卸载') expect(value.node.value).toBeUndefined()
    expect(value.actionNotice.value).toBe('')
  })

  it('A → B → A 复用时旧读取不能复活，路由切换立即清空旧事实并取消等待', async () => {
    const api = apiMock(); const old = deferred<ExecutionNodeDetail>(); api.getExecutionNode.mockImplementationOnce(() => old.promise)
    const id = ref('synthetic-a'); const signals: AbortSignal[] = []
    const { value } = mount(() => useNodeDetail(id, signal => { signals.push(signal); return api }))
    id.value = 'synthetic-b'; expect(value.node.value).toBeUndefined(); await flush()
    expect(value.node.value?.id).toBe('synthetic-b'); expect(signals[0]?.aborted).toBe(true)
    id.value = 'synthetic-a'; await flush(); old.resolve({ ...detail(), revision: 99 }); await flush()
    expect(value.node.value?.revision).toBe(1)
    expect(value.refreshCompleted.value).toBe(false)
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
    expect(value.lastLoadedAt.value).toBe(''); expect(value.refreshCompleted.value).toBe(false)
  })
  it('手动刷新入口防重，成功时间与反馈不能被失败读取或卸载后响应改写', async () => {
    vi.useFakeTimers(); vi.setSystemTime(new Date('2026-10-12T00:00:00Z'))
    const api = apiMock(); const { value, stop } = mount(() => useNodeDetail(ref('synthetic-a'), () => api)); await flush()
    const initialTime = value.lastLoadedAt.value
    const read = deferred<ExecutionNodeDetail>(); api.getExecutionNode.mockImplementationOnce(() => read.promise)
    vi.setSystemTime(new Date('2026-10-12T00:01:00Z'))
    const pending = value.loadNode(true); await value.loadNode(true)
    expect(api.getExecutionNode).toHaveBeenCalledTimes(2); expect(value.refreshing.value).toBe(true)
    read.resolve({ ...detail(), revision: 2 }); await pending
    expect(value.refreshCompleted.value).toBe(true); expect(value.lastLoadedAt.value).not.toBe(initialTime)
    const completedTime = value.lastLoadedAt.value
    api.getExecutionNode.mockRejectedValueOnce({ status: 503 }); await value.loadNode(true)
    expect(value.node.value?.revision).toBe(2); expect(value.lastLoadedAt.value).toBe(completedTime); expect(value.refreshCompleted.value).toBe(false)
    const late = deferred<ExecutionNodeDetail>(); api.getExecutionNode.mockImplementationOnce(() => late.promise)
    const refreshing = value.loadNode(true); stop(); late.resolve(detail()); await refreshing
    expect(value.lastLoadedAt.value).toBe(''); expect(value.refreshCompleted.value).toBe(false)
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
  it('检查复读串行等待真实结果，完成后停止且不产生手动刷新成功反馈', async () => {
    vi.useFakeTimers()
    const api = apiMock()
    const pending: ExecutionNodeDetail = { ...detail(), environmentStatus: 'NOT_CHECKED', unavailableReasons: ['ENVIRONMENT_CHECK_IN_PROGRESS'], environmentCheck: { status: 'PENDING', factsRevision: 1 } }
    api.requestExecutionNodeEnvironmentCheck.mockResolvedValueOnce(pending)
    const { value } = mount(() => useNodeDetail(ref('synthetic-a'), () => api)); await flush()
    value.node.value = { ...detail(), environmentStatus: 'EXPIRED' }; await value.performPrimaryAction()
    const read = deferred<ExecutionNodeDetail>(); api.getExecutionNode.mockImplementationOnce(() => read.promise)
    await vi.advanceTimersByTimeAsync(2500); expect(value.refreshing.value).toBe(true); expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(15000); expect(api.getExecutionNode).toHaveBeenCalledTimes(2)
    read.resolve(pending); await flush(); expect(vi.getTimerCount()).toBe(1)
    await vi.advanceTimersByTimeAsync(2500); expect(api.getExecutionNode).toHaveBeenCalledTimes(3)
    expect(vi.getTimerCount()).toBe(0); expect(value.refreshCompleted.value).toBe(false)
  })
  it('检查复读在两分钟上限或节点离线后停止', async () => {
    vi.useFakeTimers()
    const api = apiMock()
    const pending: ExecutionNodeDetail = { ...detail(), environmentStatus: 'NOT_CHECKED', unavailableReasons: ['ENVIRONMENT_CHECK_IN_PROGRESS'], environmentCheck: { status: 'PENDING', factsRevision: 1 } }
    api.requestExecutionNodeEnvironmentCheck.mockResolvedValue(pending)
    const { value } = mount(() => useNodeDetail(ref('synthetic-a'), () => api)); await flush()
    value.node.value = { ...detail(), environmentStatus: 'EXPIRED' }; await value.performPrimaryAction()
    api.getExecutionNode.mockResolvedValue(pending)
    await vi.advanceTimersByTimeAsync(120000); expect(vi.getTimerCount()).toBe(0)
    const calls = api.getExecutionNode.mock.calls.length
    await vi.advanceTimersByTimeAsync(120000); expect(api.getExecutionNode).toHaveBeenCalledTimes(calls)
    value.node.value = { ...detail(), environmentStatus: 'EXPIRED' }; await value.performPrimaryAction()
    api.getExecutionNode.mockResolvedValue({ ...pending, heartbeatStatus: 'OFFLINE' })
    await vi.advanceTimersByTimeAsync(2500); expect(vi.getTimerCount()).toBe(0)
  })
})

describe('节点登记与编辑唯一表单', () => {
  it.each([
    ['WINDOWS_AMD64', 'WINDOWS', 'x86_64', '/E:/synthetic'],
    ['LINUX_AMD64', 'LINUX', 'x86_64', '/synthetic'],
    ['LINUX_ARM64', 'LINUX', 'aarch64', '/synthetic'],
  ] as const)('读取 %s 派生两类选择，原值重选与保存保持平台协议', async (platform, operatingSystem, cpuArchitecture, root) => {
    const api = apiMock()
    const initial = { ...detail(), platform, allowedRoots: [root] }
    api.getExecutionNode.mockResolvedValueOnce(initial)
    api.updateExecutionNode.mockResolvedValueOnce({ ...initial, revision: 2 })
    const { value } = mount(() => useNodeForm(ref(initial.id), ref(false), vi.fn(), async () => {}, () => api))
    value.operatingSystem.value = 'LINUX'; value.cpuArchitecture.value = 'aarch64'
    expect(value.form.platform).toBe('WINDOWS_AMD64')
    await flush()
    expect(value.operatingSystem.value).toBe(operatingSystem)
    expect(value.cpuArchitecture.value).toBe(cpuArchitecture)
    value.operatingSystem.value = operatingSystem; value.cpuArchitecture.value = cpuArchitecture
    expect(value.dirty.value).toBe(false)
    expect(await value.save()).toBe(true)
    expect(api.updateExecutionNode).toHaveBeenCalledWith(initial.id, 1, {
      displayName: initial.displayName, platform, toolHome: initial.toolHome, javaPath: initial.javaPath, allowedRoots: [root],
    })
  })
  it('分类切换只形成支持的组合，Windows 拒绝 aarch64，路径不自动转换', () => {
    const { value } = mount(() => useNodeForm(ref(''), ref(true), vi.fn(), async () => {}, () => apiMock()))
    value.form.allowedRootsText = '/E:/synthetic'
    value.cpuArchitecture.value = 'aarch64'
    expect(value.form.platform).toBe('WINDOWS_AMD64')
    expect(value.cpuArchitectureOptions.value.find(option => option.value === 'aarch64')?.disabled).toBe(true)
    value.operatingSystem.value = 'LINUX'
    expect(value.form.platform).toBe('LINUX_AMD64')
    expect(value.cpuArchitectureOptions.value.every(option => !option.disabled)).toBe(true)
    value.cpuArchitecture.value = 'aarch64'; expect(value.form.platform).toBe('LINUX_ARM64')
    value.operatingSystem.value = 'WINDOWS'
    expect(value.form.platform).toBe('WINDOWS_AMD64'); expect(value.cpuArchitecture.value).toBe('x86_64')
    value.operatingSystem.value = 'LINUX'
    expect(value.form.platform).toBe('LINUX_AMD64')
    expect(value.form.allowedRootsText).toBe('/E:/synthetic')
  })
  it('保存、授权失效与卸载后分类写入口不能修改唯一平台值', async () => {
    const api = apiMock(); const request = deferred<string>(); api.createExecutionNode.mockImplementationOnce(() => request.promise)
    const { value, stop } = mount(() => useNodeForm(ref(''), ref(true), vi.fn(), async () => {}, () => api))
    Object.assign(value.form, { displayName: '合成节点', toolHome: 'E:/synthetic-tools', javaPath: 'C:/synthetic-java', allowedRootsText: '/E:/synthetic' })
    const pending = value.save()
    value.operatingSystem.value = 'LINUX'; value.cpuArchitecture.value = 'aarch64'
    expect(value.form.platform).toBe('WINDOWS_AMD64')
    request.resolve('synthetic-created'); await pending
    value.operatingSystem.value = 'LINUX'; value.cpuArchitecture.value = 'aarch64'
    expect(value.form.platform).toBe('LINUX_ARM64')
    value.revokeAccess(); value.operatingSystem.value = 'LINUX'; value.cpuArchitecture.value = 'aarch64'
    expect(value.form.platform).toBe('WINDOWS_AMD64')
    stop(); value.operatingSystem.value = 'LINUX'; value.cpuArchitecture.value = 'aarch64'
    expect(value.form.platform).toBe('WINDOWS_AMD64')
    expect(api.createExecutionNode).toHaveBeenCalledTimes(1)
  })
  it('未保存状态覆盖全部输入，预算空值与零等价，保存成功后重新建立基线', async () => {
    const api = apiMock()
    const { value } = mount(() => useNodeForm(ref('synthetic-a'), ref(false), vi.fn(), async () => {}, () => api))
    await flush()
    expect(value.dirty.value).toBe(false)
    value.form.displayName = '合成修改'; expect(value.dirty.value).toBe(true)
    value.form.displayName = 'synthetic-a'; expect(value.dirty.value).toBe(false)
    value.form.systemReservedGiB = null; value.form.taskOverheadGiB = 0; expect(value.dirty.value).toBe(false)
    value.form.taskOverheadGiB = 1.25; expect(value.dirty.value).toBe(true)
    expect(await value.save()).toBe(true); expect(value.dirty.value).toBe(false)
    value.form.javaPath = 'C:/synthetic-other-java'; expect(value.dirty.value).toBe(true)
  })
  it('保存失败保留当前会话的草稿与未保存状态，不假报成功', async () => {
    const api = apiMock(); api.updateExecutionNode.mockRejectedValueOnce({ status: 503 })
    const { value } = mount(() => useNodeForm(ref('synthetic-a'), ref(false), vi.fn(), async () => {}, () => api))
    await flush(); value.form.displayName = '合成草稿'
    expect(await value.save()).toBe(false)
    expect(value.form.displayName).toBe('合成草稿'); expect(value.dirty.value).toBe(true)
    expect(value.notice.value).toBe(''); expect(value.failure.value).not.toBe(''); expect(value.busy.value).toBe(false)
  })
  it.each([401, 403, 404])('读取授权或对象失效 %s 清除旧配置，只有新授权读取才能恢复', async status => {
    const api = apiMock()
    const { value } = mount(() => useNodeForm(ref('synthetic-a'), ref(false), vi.fn(), async () => {}, () => api))
    await flush(); value.form.toolHome = 'E:/synthetic-draft'
    api.getExecutionNode.mockRejectedValueOnce({ status }); await value.loadNode()
    expect(value.restricted.value).toBe(true); expect(value.node.value).toBeUndefined()
    expect(value.form.toolHome).toBe(''); expect(value.dirty.value).toBe(false)
    await value.save(); expect(api.updateExecutionNode).not.toHaveBeenCalled()
    await value.loadNode(); expect(value.restricted.value).toBe(false); expect(value.form.displayName).toBe('synthetic-a')
  })
  it.each([401, 403, 404])('写入授权或对象失效 %s 清除旧配置与错误，不允许重交', async status => {
    const api = apiMock(); api.updateExecutionNode.mockRejectedValueOnce({ status })
    const { value } = mount(() => useNodeForm(ref('synthetic-a'), ref(false), vi.fn(), async () => {}, () => api))
    await flush(); value.form.displayName = '合成草稿'
    await value.save(); await value.save()
    expect(value.restricted.value).toBe(true); expect(value.node.value).toBeUndefined()
    expect(value.form.displayName).toBe(''); expect(value.formErrors).toEqual({})
    expect(value.busy.value).toBe(false); expect(api.updateExecutionNode).toHaveBeenCalledTimes(1)
  })
  it('列表撤销授权后重新读取相同节点，旧写响应不能恢复配置或成功反馈', async () => {
    const api = apiMock(); const write = deferred<ExecutionNodeDetail>()
    api.updateExecutionNode.mockImplementationOnce(() => write.promise)
    const { value } = mount(() => useNodeForm(ref('synthetic-a'), ref(false), vi.fn(), async () => {}, () => api))
    await flush(); value.form.displayName = '合成旧修改'; const pending = value.save()
    value.revokeAccess(); await value.loadNode()
    write.resolve({ ...detail(), displayName: '合成旧修改', revision: 99 }); expect(await pending).toBe(false)
    expect(value.form.displayName).toBe('synthetic-a'); expect(value.node.value?.revision).toBe(1)
    expect(value.notice.value).toBe(''); expect(value.dirty.value).toBe(false)
  })
  it('新建清空数字输入后保留自动预算，创建完成才清除未保存状态并导航', async () => {
    const api = apiMock(); let dirtyAtNavigation = true
    const { value } = mount(() => useNodeForm(ref(''), ref(true), async () => { dirtyAtNavigation = value.dirty.value }, async () => {}, () => api))
    Object.assign(value.form, { displayName: '合成节点', toolHome: 'E:/synthetic-tools', javaPath: 'C:/synthetic-java', allowedRootsText: '/E:/synthetic', systemReservedGiB: null, taskOverheadGiB: null })
    expect(value.dirty.value).toBe(true); expect(await value.save()).toBe(true)
    expect(api.createExecutionNode).toHaveBeenCalledWith({ displayName: '合成节点', platform: 'WINDOWS_AMD64', toolHome: 'E:/synthetic-tools', javaPath: 'C:/synthetic-java', allowedRoots: ['/E:/synthetic'] })
    expect(dirtyAtNavigation).toBe(false)
  })
  it('预算配置按 GiB 编辑并在 DTO 边界转换，恢复自动策略不保留旧预算', async () => {
    const api = apiMock()
    api.getExecutionNode.mockResolvedValue({ ...detail(), budgetPolicy: { systemReservedBytes: 2 ** 31, taskOverheadBytes: 2 ** 30 } })
    const id = ref('synthetic-a'); const isNew = ref(false)
    const { value } = mount(() => useNodeForm(id, isNew, vi.fn(), async () => {}, () => api))
    await flush()
    expect(value.form.systemReservedGiB).toBe(2)
    expect(value.form.taskOverheadGiB).toBe(1)
    value.form.systemReservedGiB = undefined
    value.form.taskOverheadGiB = 1.25
    await value.save()
    expect(api.updateExecutionNode).toHaveBeenCalledWith('synthetic-a', 1, expect.objectContaining({ budgetPolicy: { systemReservedBytes: 0, taskOverheadBytes: 1342177280 } }))
    isNew.value = true; id.value = ''
    expect(value.form.systemReservedGiB).toBeUndefined()
    expect(value.form.taskOverheadGiB).toBeUndefined()
    expect(validateNodeForm({ displayName: '合成', platform: 'WINDOWS_AMD64', toolHome: 'E:/synthetic', javaPath: 'E:/synthetic/java', allowedRoots: ['/E:/synthetic'], budgetPolicy: { taskOverheadBytes: -1 } }).budgetPolicy).toBeDefined()
  })
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
    await value.save(); expect(value.node.value?.revision).toBe(2); expect(value.notice.value).toBe('节点配置已保存。')
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
  it('删除防重保持版本及身份撤销事实；卸载后没有刷新或反馈', async () => {
    const api = apiMock(); const deletion = deferred<{ outcome: 'DELETED'; revision: number; agentAccessRevoked: boolean }>(); api.deleteExecutionNode.mockImplementationOnce(() => deletion.promise)
    const { value: { list, actions }, stop } = mount(() => { const list = useNodeList(() => api); return { list, actions: useNodeListActions(list) } }); await list.loadNodes()
    actions.deletionTarget.value = detail(); const pending = actions.deleteNode(); await actions.deleteNode(); expect(api.deleteExecutionNode).toHaveBeenCalledWith('synthetic-a', 1)
    deletion.resolve({ outcome: 'DELETED', revision: 0, agentAccessRevoked: true }); await pending; expect(actions.notice.value).toContain('已永久删除'); expect(actions.notice.value).toContain('已撤销'); expect(api.deleteExecutionNode).toHaveBeenCalledTimes(1)
    const late = deferred<ExecutionNodeDetail>(); api.requestExecutionNodeEnvironmentCheck.mockImplementationOnce(() => late.promise)
    const running = actions.runPrimaryAction({ ...detail(), environmentStatus: 'EXPIRED' }, 'environment-check'); const notice = actions.notice.value; stop(); late.resolve(detail()); await running
    expect(actions.notice.value).toBe(notice); expect(api.listExecutionNodes).toHaveBeenCalledTimes(2)
  })
})

describe('节点自动刷新生命周期', () => {
  it('默认每 60 秒保留筛选刷新，仅成功后更新页面刷新时间', async () => {
    vi.useFakeTimers()
    const api = apiMock(); const { value } = mount(() => useNodeList(() => api))
    await value.loadNodes(); value.keyword.value = 'synthetic'; value.management.value = 'DISABLED'
    expect(value.refreshCompleted.value).toBe(false)
    const firstTime = value.lastLoadedAt.value
    await vi.advanceTimersByTimeAsync(59999); expect(api.listExecutionNodes).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1); expect(api.listExecutionNodes).toHaveBeenCalledTimes(2)
    expect(value.refreshCompleted.value).toBe(true)
    expect(value.lastLoadedAt.value).not.toBe(firstTime); expect(value.keyword.value).toBe('synthetic'); expect(value.management.value).toBe('DISABLED')
    const successTime = value.lastLoadedAt.value
    api.listExecutionNodes.mockRejectedValueOnce({ status: 503 }); await vi.advanceTimersByTimeAsync(60000)
    expect(value.nodes.value).toHaveLength(1); expect(value.lastLoadedAt.value).toBe(successTime)
    expect(value.refreshCompleted.value).toBe(false)
  })
  it('切换刷新间隔仅保留一个计时器，关闭后不继续请求', async () => {
    vi.useFakeTimers()
    const api = apiMock(); const { value } = mount(() => useNodeList(() => api)); await value.loadNodes()
    await vi.advanceTimersByTimeAsync(20000); value.refreshInterval.value = 30
    expect(vi.getTimerCount()).toBe(1)
    await vi.advanceTimersByTimeAsync(29999); expect(api.listExecutionNodes).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1); expect(api.listExecutionNodes).toHaveBeenCalledTimes(2)
    value.refreshInterval.value = 0; expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(300000); expect(api.listExecutionNodes).toHaveBeenCalledTimes(2)
  })
  it('慢请求跨多个间隔不叠加，等待期间切换间隔不恢复旧计时器', async () => {
    vi.useFakeTimers()
    const api = apiMock(); const late = deferred<ExecutionNodeDetail[]>()
    const { value } = mount(() => useNodeList(() => api)); await value.loadNodes()
    api.listExecutionNodes.mockImplementationOnce(() => late.promise)
    vi.advanceTimersByTime(60000); await flush(); expect(value.refreshing.value).toBe(true)
    expect(value.refreshCompleted.value).toBe(false)
    await value.loadNodes(true); expect(api.listExecutionNodes).toHaveBeenCalledTimes(2)
    expect(value.refreshing.value).toBe(true)
    value.refreshInterval.value = 15
    await vi.advanceTimersByTimeAsync(120000); expect(api.listExecutionNodes).toHaveBeenCalledTimes(2)
    late.resolve([detail()]); await flush(); expect(vi.getTimerCount()).toBe(1)
    expect(value.refreshCompleted.value).toBe(true)
    await vi.advanceTimersByTimeAsync(15000); expect(api.listExecutionNodes).toHaveBeenCalledTimes(3)
  })
  it('写事务期间跳过读取；卸载取消定时器并隔离迟到自动响应', async () => {
    vi.useFakeTimers()
    const api = apiMock(); const late = deferred<ExecutionNodeDetail[]>()
    const signals: AbortSignal[] = []
    const { value, stop } = mount(() => useNodeList(signal => { signals.push(signal); return api })); await value.loadNodes()
    value.beginMutation(); await vi.advanceTimersByTimeAsync(60000); expect(api.listExecutionNodes).toHaveBeenCalledTimes(1)
    value.endMutation(); api.listExecutionNodes.mockImplementationOnce(() => late.promise)
    vi.advanceTimersByTime(60000); await flush(); const time = value.lastLoadedAt.value
    stop(); late.resolve([{ ...detail(), revision: 99 }]); await flush()
    expect(value.nodes.value[0]?.revision).toBe(1); expect(value.lastLoadedAt.value).toBe(time)
    expect(value.refreshCompleted.value).toBe(false); expect(value.refreshing.value).toBe(false)
    expect(signals[0]?.aborted).toBe(true); expect(vi.getTimerCount()).toBe(0)
    await vi.advanceTimersByTimeAsync(120000); expect(api.listExecutionNodes).toHaveBeenCalledTimes(2)
  })
  it.each([401, 403])('自动刷新遇到授权失效 %s 后清空事实且停止周期请求', async status => {
    vi.useFakeTimers()
    const api = apiMock(); const { value } = mount(() => useNodeList(() => api)); await value.loadNodes()
    api.listExecutionNodes.mockRejectedValueOnce({ status }); await vi.advanceTimersByTimeAsync(60000)
    expect(value.nodes.value).toHaveLength(0); expect(value.restricted.value).toBe(true)
    await vi.advanceTimersByTimeAsync(180000); expect(api.listExecutionNodes).toHaveBeenCalledTimes(2)
  })
  it('自动刷新失去授权清除已打开的删除对象，方法入口拒绝旧目标', async () => {
    vi.useFakeTimers()
    const api = apiMock()
    const { value: { list, actions } } = mount(() => { const list = useNodeList(() => api); return { list, actions: useNodeListActions(list) } })
    await list.loadNodes(); actions.deletionTarget.value = detail()
    api.listExecutionNodes.mockRejectedValueOnce({ status: 403 }); await vi.advanceTimersByTimeAsync(60000)
    expect(actions.deletionTarget.value).toBeUndefined()
    actions.deletionTarget.value = detail(); await actions.deleteNode(); expect(api.deleteExecutionNode).not.toHaveBeenCalled()
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


describe('注册材料复制结果语义', () => {
  it('失败只进入错误，重试成功与关闭清除两类反馈', async () => {
    const api = apiMock()
    api.getExecutionNode.mockImplementation(async id => ({ ...detail(id), agentAssociationStatus: 'PENDING' }))
    const copy = vi.fn().mockRejectedValueOnce(new Error('synthetic-denied')).mockResolvedValueOnce(undefined)
    const { value } = mount(() => {
      const state = useNodeDetail(ref('synthetic-a'), () => api)
      return useNodeEnrollment(state.node, state.session, copy)
    })
    await flush()
    await value.issueEnrollment()
    await value.copyEnrollmentValue('注册码', value.registrationCode.value)
    expect(value.copyNotice.value).toBe('')
    expect(value.copyFailure.value).toBe('注册码复制失败，请手动选择复制。')
    await value.copyEnrollmentValue('注册码', value.registrationCode.value)
    expect(value.copyFailure.value).toBe('')
    expect(value.copyNotice.value).toBe('注册码已复制。')
    value.clearEnrollment()
    expect(value.copyNotice.value).toBe('')
    expect(value.copyFailure.value).toBe('')
  })
})
