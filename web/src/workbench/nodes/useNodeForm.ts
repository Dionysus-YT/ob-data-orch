import { computed, reactive, ref, watch, type Ref } from 'vue'
import { executionNodeErrorMessage, type ExecutionNodeDetail, type ExecutionNodePlatform, type ExecutionNodeWrite } from '@/api/browser'
import { fieldIds, fieldLabels, nodeApiFieldErrors, validateNodeForm, type NodeFormErrors, type NodeFormField } from './nodeFormRules'
import { useNodeSession, type NodeApiFactory } from './nodeSession'

// 新建与编辑共用一个表单所有者；页面只提供导航和错误汇总聚焦行为。
export function useNodeForm(nodeID: Readonly<Ref<string>>, isNew: Readonly<Ref<boolean>>, created: (id: string) => Promise<unknown>, focusErrors: (active: () => boolean) => Promise<void>, createApi?: NodeApiFactory) {
  const key = computed(() => `${isNew.value ? 'new' : 'edit'}:${nodeID.value}`)
  const session = useNodeSession(key, createApi)
  const node = ref<ExecutionNodeDetail>()
  const loading = ref(false)
  const busy = ref(false)
  const failure = ref('')
  const notice = ref('')
  const formErrors = reactive<NodeFormErrors>({})
  const blankForm = () => ({ displayName: '', platform: 'WINDOWS_AMD64' as ExecutionNodePlatform, allowedRootsText: '', toolHome: '', javaPath: '' })
  const form = reactive(blankForm())
  const errorEntries = computed(() => (Object.keys(formErrors) as NodeFormField[]).filter(field => Boolean(formErrors[field]))
    .map(field => ({ field, id: fieldIds[field], label: fieldLabels[field], message: formErrors[field] })))
  let readVersion = 0
  session.onReset(() => {
    readVersion++
    node.value = undefined
    loading.value = busy.value = false
    failure.value = notice.value = ''
    Object.assign(form, blankForm())
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
      node.value = result
      fillForm(result)
    } catch (error) {
      if (valid()) failure.value = executionNodeErrorMessage(error, '执行节点加载失败。')
    } finally {
      if (valid()) loading.value = false
    }
  }
  function fillForm(value: ExecutionNodeDetail) {
    Object.assign(form, { displayName: value.displayName, platform: value.platform, allowedRootsText: value.allowedRoots.join('\n'), toolHome: value.toolHome, javaPath: value.javaPath })
    clearErrors()
  }
  function writeInput(): ExecutionNodeWrite {
    return { displayName: form.displayName.trim(), platform: form.platform, allowedRoots: form.allowedRootsText.split(/\r?\n/).map(root => root.trim()).filter(Boolean), toolHome: form.toolHome.trim(), javaPath: form.javaPath.trim() }
  }
  async function save() {
    if (!session.active() || busy.value || loading.value) return
    const active = session.capture()
    const input = writeInput()
    setErrors(validateNodeForm(input))
    if (Object.keys(formErrors).length) { await focusErrors(active); return }
    const current = node.value
    const creating = isNew.value
    if (!creating && !current) return
    readVersion++
    busy.value = true
    failure.value = notice.value = ''
    try {
      if (creating) {
        const id = await session.api.createExecutionNode(input)
        if (active()) await created(id)
      } else if (current) {
        const result = await session.api.updateExecutionNode(current.id, current.revision, input)
        if (!active()) return
        node.value = result
        fillForm(result)
        notice.value = '节点配置已保存，Agent 将在空闲时自动同步并检查环境，无需重新注册。'
      }
    } catch (error) {
      if (!active()) return
      setErrors(nodeApiFieldErrors(error))
      if (!Object.keys(formErrors).length) failure.value = executionNodeErrorMessage(error, '执行节点保存失败。')
      else await focusErrors(active)
    } finally {
      if (active()) busy.value = false
    }
  }
  function clearErrors() { for (const field of Object.keys(formErrors) as NodeFormField[]) delete formErrors[field] }
  function clearError(field: NodeFormField) { delete formErrors[field] }
  function setErrors(errors: NodeFormErrors) { clearErrors(); Object.assign(formErrors, errors) }
  function validateField(field: NodeFormField) {
    const message = validateNodeForm(writeInput())[field]
    if (message) formErrors[field] = message
    else delete formErrors[field]
  }
  return { session, node, loading, busy, failure, notice, formErrors, form, errorEntries, loadNode, save, clearError, validateField }
}
