import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { dataSourceErrorMessage, type DataSourceConnectionTest, type DataSourceDetail, type ExecutionNodeCandidate } from '@/api/browser'
import { dataSourceFieldErrorsFromApi, validateDataSourceForm, type DataSourceFormErrors } from '@/views/dataSourceFormErrors'
import { parseDataSourceConnectionString } from '@/views/dataSourceConnectionString'
import { blankSourceForm, invalidatesConnection, sourceUpdate } from './sourcePresentation'
import type { SourceGateway } from './sourceGateway'

// 每个编辑器实例固定兼容模式；解析只能回填同模式字段，不能改变数据源类型。
export function useSourceEditor(api: SourceGateway, initialId: string | null, onSaved: (id: string) => void, initialMode: 'MYSQL' | 'ORACLE' = 'MYSQL') {
  const activeId = ref(initialId)
  const source = ref<DataSourceDetail>()
  const initialForm = { ...blankSourceForm(), compatibilityMode: initialMode }
  const form = reactive({ ...initialForm })
  const clearSys = ref(false)
  const parserInput = ref('')
  const errors = ref<DataSourceFormErrors>({})
  const failure = ref('')
  const feedback = ref('')
  const loading = ref(Boolean(initialId))
  const saving = ref(false)
  const testing = ref(false)
  const nodes = ref<ExecutionNodeCandidate[]>([])
  const nodeLoading = ref(false)
  const nodeFailure = ref('')
  const nodeId = ref('')
  const test = ref<DataSourceConnectionTest>()
  let disposed = false
  let poll: ReturnType<typeof setTimeout> | undefined
  const isNew = computed(() => !activeId.value)
  const update = computed(() => source.value ? sourceUpdate(form, source.value, clearSys.value) : {})
  const dirty = computed(() => {
    if (loading.value) return false
    if (source.value) return Object.keys(update.value).length > 0 || Boolean(form.sysUser || form.sysPassword || parserInput.value)
    const initial = initialForm
    return Object.keys(initial).some((key) => form[key as keyof typeof form] !== initial[key as keyof typeof initial]) || Boolean(parserInput.value)
  })
  const connectionDirty = computed(() => invalidatesConnection(update.value) || Boolean(form.sysUser || form.sysPassword))
  const busy = computed(() => loading.value || saving.value || testing.value)
  const invalidated = computed(() => connectionDirty.value || source.value?.lastTestStatus === 'INVALIDATED')
  const testBlocked = computed(() => isNew.value ? '请先保存配置，再独立测试连接。' : connectionDirty.value ? '连接配置有未保存更改，请先保存。' : !nodeId.value ? '选择执行节点后可发起测试。' : '')

  function fill(value: DataSourceDetail) {
    source.value = value
    Object.assign(form, blankSourceForm(), { displayName: value.displayName, environment: value.environment, compatibilityMode: value.compatibilityMode, host: value.host, port: value.port, clusterName: value.clusterName, tenantName: value.tenantName, username: value.username ?? '', defaultDatabase: value.defaultDatabase ?? '' })
    clearSys.value = false
    parserInput.value = ''
    errors.value = {}
  }
  async function load() {
    failure.value = ''
    if (!activeId.value) return
    loading.value = true
    try { const value = await api.getDataSource(activeId.value); if (!disposed) fill(value) }
    catch (error) { if (!disposed) failure.value = dataSourceErrorMessage(error, '无法读取数据源配置。') }
    finally { if (!disposed) loading.value = false }
  }
  async function loadNodes() {
    nodeLoading.value = true; nodeFailure.value = ''
    try {
      const value = await api.listDataSourceConnectionTestNodeCandidates()
      if (!disposed) { nodes.value = value; if (!value.some((node) => node.id === nodeId.value)) nodeId.value = '' }
    } catch (error) { if (!disposed) { nodes.value = []; nodeId.value = ''; nodeFailure.value = dataSourceErrorMessage(error, '无法读取可测试的执行节点。') } }
    finally { if (!disposed) nodeLoading.value = false }
  }
  function parseConnection() {
    const parsed = parseDataSourceConnectionString(parserInput.value)
    if (!parsed) { failure.value = '无法解析连接串。请检查 mysql / obclient 地址、端口和用户@租户，可选 #集群。'; return }
    // 模式不匹配时拒绝整次回填，避免部分字段或秘密污染当前草稿。
    if (parsed.compatibilityMode !== form.compatibilityMode) { failure.value = `当前是 OceanBase ${form.compatibilityMode === 'MYSQL' ? 'MySQL' : 'Oracle'} 页面，请使用 ${form.compatibilityMode === 'MYSQL' ? 'mysql' : 'obclient'} 连接串。`; feedback.value = ''; return }
    Object.assign(form, parsed, { defaultDatabase: parsed.defaultDatabase ?? '', password: parsed.password || form.password })
    parserInput.value = ''; errors.value = {}; failure.value = ''; feedback.value = '已解析到表单，尚未保存。'
  }
  async function save() {
    if (busy.value) return false
    errors.value = validateDataSourceForm(form, isNew.value)
    if (Object.keys(errors.value).length) return false
    const environment = form.environment
    if (!environment) return false
    saving.value = true; failure.value = ''; feedback.value = ''
    try {
      let updated: DataSourceDetail
      if (!activeId.value) {
        const id = await api.createDataSource({ ...form, environment, displayName: form.displayName.trim(), host: form.host.trim(), clusterName: form.clusterName.trim(), tenantName: form.tenantName.trim(), username: form.username.trim(), defaultDatabase: form.compatibilityMode === 'MYSQL' ? form.defaultDatabase || undefined : undefined, sysUser: form.sysUser?.trim() || undefined, sysPassword: form.sysPassword || undefined })
        // 创建成功立即清空秘密；详情读取失败时保留 ID，重试不能重复创建。
        activeId.value = id; form.password = ''; form.sysPassword = ''; form.sysUser = ''; parserInput.value = ''
        onSaved(id)
        updated = await api.getDataSource(id)
      } else {
        if (!source.value) { await load(); return false }
        const changed = update.value
        if (!Object.keys(changed).length) { parserInput.value = ''; feedback.value = '没有需要保存的更改。'; return true }
        updated = await api.updateDataSource(activeId.value, source.value.revision, changed)
        if (invalidatesConnection(changed)) test.value = undefined
      }
      if (disposed) return false
      fill(updated)
      feedback.value = '配置已保存。连接测试是独立操作。'
      onSaved(updated.id)
      return true
    } catch (error) {
      if (!disposed) { errors.value = dataSourceFieldErrorsFromApi(error); if (!Object.keys(errors.value).length) failure.value = dataSourceErrorMessage(error, '保存失败，输入仍保留在本页。') }
      return false
    } finally { if (!disposed) saving.value = false }
  }
  async function pollTest(id: string) {
    try {
      const result = await api.getDataSourceConnectionTest(id)
      if (disposed) return
      test.value = result
      if (result.status === 'PENDING' || result.status === 'LEASED') { poll = setTimeout(() => void pollTest(id), 1000); return }
      testing.value = false
      if (activeId.value) { await load(); if (!disposed) onSaved(activeId.value) }
    } catch (error) { if (!disposed) { testing.value = false; failure.value = dataSourceErrorMessage(error, '暂时无法读取测试结果。已提交的测试可能继续运行。') } }
  }
  async function startTest() {
    if (busy.value || testBlocked.value || !source.value || !nodes.value.some((node) => node.id === nodeId.value)) return
    testing.value = true; failure.value = ''; feedback.value = ''; test.value = undefined
    try { const request = await api.startDataSourceConnectionTest(source.value.id, source.value.revision, nodeId.value); if (!disposed) await pollTest(request.id) }
    catch (error) { if (!disposed) { testing.value = false; failure.value = dataSourceErrorMessage(error, '无法发起连接测试。') } }
  }
  onMounted(() => { void load(); void loadNodes() })
  onBeforeUnmount(() => { disposed = true; if (poll) clearTimeout(poll); form.password = ''; form.sysPassword = ''; parserInput.value = '' })
  return { activeId, source, form, clearSys, parserInput, errors, failure, feedback, loading, saving, testing, nodes, nodeLoading, nodeFailure, nodeId, test, isNew, dirty, connectionDirty, busy, invalidated, testBlocked, load, loadNodes, parseConnection, save, startTest }
}
