import { computed, reactive, ref, watch, type Ref } from 'vue'
import { executionNodeErrorMessage, type ExecutionNodeDetail, type ExecutionNodePlatform, type ExecutionNodeWrite } from '@/api/browser'
import { fieldIds, fieldLabels, nodeApiFieldErrors, validateNodeForm, type NodeFormErrors, type NodeFormField } from './nodeFormRules'
import { nodeCPUArchitectureOptions, nodeOperatingSystemOptions, nodePlatformClassification, nodePlatformFromSelection, type NodeCPUArchitecture, type NodeOperatingSystem } from './nodePlatformRules'
import { useNodeSession, type NodeApiFactory } from './nodeSession'

// 新建与编辑共用一个表单所有者；页面只提供导航和错误汇总聚焦行为。
export function useNodeForm(nodeID: Readonly<Ref<string>>, isNew: Readonly<Ref<boolean>>, created: (id: string) => Promise<unknown>, focusErrors: (active: () => boolean) => Promise<void>, createApi?: NodeApiFactory) {
  const key = computed(() => `${isNew.value ? 'new' : 'edit'}:${nodeID.value}`)
  const session = useNodeSession(key, createApi)
  const node = ref<ExecutionNodeDetail>()
  const loading = ref(false)
  const busy = ref(false)
  const restricted = ref(false)
  const failure = ref('')
  const notice = ref('')
  const formErrors = reactive<NodeFormErrors>({})
  const blankForm = () => ({ displayName: '', platform: 'WINDOWS_AMD64' as ExecutionNodePlatform, allowedRootsText: '', toolHome: '', javaPath: '', systemReservedGiB: undefined as number | null | undefined, taskOverheadGiB: undefined as number | null | undefined })
  const form = reactive(blankForm())
  const canSelectPlatform = () => session.active() && !busy.value && !loading.value && !restricted.value
  // 两个控件只是唯一 platform 的投影；切换为 Windows 时收敛到唯一受支持的 x86_64。
  const operatingSystem = computed<NodeOperatingSystem>({
    get: () => nodePlatformClassification(form.platform).operatingSystem,
    set: value => {
      if (!canSelectPlatform()) return
      const platform = nodePlatformFromSelection(value, cpuArchitecture.value) ?? nodePlatformFromSelection(value, 'x86_64')
      if (platform) { form.platform = platform; clearError('platform') }
    },
  })
  const cpuArchitecture = computed<NodeCPUArchitecture>({
    get: () => nodePlatformClassification(form.platform).cpuArchitecture,
    set: value => {
      if (!canSelectPlatform()) return
      const platform = nodePlatformFromSelection(operatingSystem.value, value)
      if (platform) { form.platform = platform; clearError('platform') }
    },
  })
  const cpuArchitectureOptions = computed(() => nodeCPUArchitectureOptions.map(option => ({
    ...option, disabled: !nodePlatformFromSelection(operatingSystem.value, option.value),
  })))
  // 只保存表单指纹以判断未保存输入，不建立第二份可编辑配置；空预算与零均表示自动策略。
  const fingerprint = () => JSON.stringify({ ...form, systemReservedGiB: form.systemReservedGiB ?? 0, taskOverheadGiB: form.taskOverheadGiB ?? 0 })
  const savedFingerprint = ref(fingerprint())
  const dirty = computed(() => fingerprint() !== savedFingerprint.value)
  const errorEntries = computed(() => (Object.keys(formErrors) as NodeFormField[]).filter(field => Boolean(formErrors[field]))
    .map(field => ({ field, id: fieldIds[field], label: fieldLabels[field], message: formErrors[field] })))
  let readVersion = 0
  session.onReset(() => {
    readVersion++
    node.value = undefined
    loading.value = busy.value = false
    restricted.value = false
    failure.value = notice.value = ''
    Object.assign(form, blankForm())
    savedFingerprint.value = fingerprint()
    clearErrors()
  })
  watch(key, () => { if (!isNew.value) void loadNode() }, { immediate: true, flush: 'sync' })

  async function loadNode() {
    if (!session.active() || busy.value || isNew.value) return
    const id = nodeID.value
    if (!id) { failure.value = '未指定执行节点。'; loading.value = false; return }
    const active = session.capture()
    const version = ++readVersion
    const valid = () => active() && version === readVersion
    loading.value = true
    failure.value = ''
    try {
      const result = await session.api.getExecutionNode(id)
      if (!valid()) return
      restricted.value = false
      node.value = result
      fillForm(result)
    } catch (error) {
      if (!valid()) return
      if (accessUnavailable(error)) revokeAccess()
      else failure.value = executionNodeErrorMessage(error, '执行节点加载失败。')
    } finally {
      if (valid()) loading.value = false
    }
  }
  function fillForm(value: ExecutionNodeDetail) {
    Object.assign(form, { displayName: value.displayName, platform: value.platform, allowedRootsText: value.allowedRoots.join('\n'), toolHome: value.toolHome, javaPath: value.javaPath })
    form.systemReservedGiB = value.budgetPolicy?.systemReservedBytes ? value.budgetPolicy.systemReservedBytes / 2 ** 30 : undefined
    form.taskOverheadGiB = value.budgetPolicy?.taskOverheadBytes ? value.budgetPolicy.taskOverheadBytes / 2 ** 30 : undefined
    savedFingerprint.value = fingerprint()
    clearErrors()
  }
  function writeInput(): ExecutionNodeWrite {
    const budgetPolicy = node.value?.budgetPolicy || form.systemReservedGiB != null || form.taskOverheadGiB != null
      ? { systemReservedBytes: Math.ceil((form.systemReservedGiB ?? 0) * 2 ** 30), taskOverheadBytes: Math.ceil((form.taskOverheadGiB ?? 0) * 2 ** 30) } : undefined
    return { displayName: form.displayName.trim(), platform: form.platform, allowedRoots: form.allowedRootsText.split(/\r?\n/).map(root => root.trim()).filter(Boolean), toolHome: form.toolHome.trim(), javaPath: form.javaPath.trim(), ...(budgetPolicy ? { budgetPolicy } : {}) }
  }
  async function save() {
    if (!session.active() || restricted.value || busy.value || loading.value) return false
    const active = session.capture()
    const submittedFingerprint = fingerprint()
    const input = writeInput()
    setErrors(validateNodeForm(input))
    if (Object.keys(formErrors).length) { await focusErrors(active); return false }
    const current = node.value
    const creating = isNew.value
    if (!creating && !current) return false
    const version = ++readVersion
    // 授权撤销同样使写响应失效，重新读取授权后也不能接受撤销前的迟到回调。
    const valid = () => active() && version === readVersion && !restricted.value
    busy.value = true
    failure.value = notice.value = ''
    try {
      if (creating) {
        const id = await session.api.createExecutionNode(input)
        if (!valid()) return false
        savedFingerprint.value = submittedFingerprint
        await created(id)
      } else if (current) {
        const result = await session.api.updateExecutionNode(current.id, current.revision, input)
        if (!valid()) return false
        node.value = result
        fillForm(result)
        notice.value = '节点配置已保存。'
      }
      return true
    } catch (error) {
      if (!valid()) return false
      if (accessUnavailable(error)) { revokeAccess(); return false }
      setErrors(nodeApiFieldErrors(error))
      if (!Object.keys(formErrors).length) failure.value = executionNodeErrorMessage(error, '执行节点保存失败。')
      else await focusErrors(active)
      return false
    } finally {
      if (valid()) busy.value = false
    }
  }
  function accessUnavailable(error: unknown) {
    const status = (error as { status?: number })?.status
    return status === 401 || status === 403 || status === 404
  }
  function revokeAccess() {
    readVersion++
    restricted.value = true
    node.value = undefined
    loading.value = busy.value = false
    notice.value = ''
    Object.assign(form, blankForm())
    savedFingerprint.value = fingerprint()
    clearErrors()
    failure.value = '当前身份或节点已不可访问，请重新确认权限后打开。'
  }
  function clearErrors() { for (const field of Object.keys(formErrors) as NodeFormField[]) delete formErrors[field] }
  function clearError(field: NodeFormField) { delete formErrors[field] }
  function setErrors(errors: NodeFormErrors) { clearErrors(); Object.assign(formErrors, errors) }
  function validateField(field: NodeFormField) {
    const message = validateNodeForm(writeInput())[field]
    if (message) formErrors[field] = message
    else delete formErrors[field]
  }
  return { session, node, loading, busy, restricted, dirty, failure, notice, formErrors, form, operatingSystem, cpuArchitecture, operatingSystemOptions: nodeOperatingSystemOptions, cpuArchitectureOptions, errorEntries, loadNode, save, clearError, validateField, revokeAccess }
}
