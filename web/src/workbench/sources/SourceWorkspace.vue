<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ChevronDown, ChevronLeft, ChevronRight, Database, FilterX, LockKeyhole, Pencil, Plus, Power, RefreshCw, Search, ServerCrash, Trash2, Unplug } from '@lucide/vue'
import { browserApi, dataSourceErrorMessage, type ApiError, type DataSourceSummary } from '@/api/browser'
import { filterDataSources, type ConnectionStatusFilter } from '@/views/dataSourceListFilters'
import OrchButton from '../components/OrchButton.vue'
import OrchDialog from '../components/OrchDialog.vue'
import OrchMenu from '../components/OrchMenu.vue'
import OrchStatus from '../components/OrchStatus.vue'
import SourceEditor from './SourceEditor.vue'
import { sourceGateway } from './sourceGateway'
import { connectionListFact, environmentLabel, environments } from './sourcePresentation'

const route = useRoute()
const { api, preview } = sourceGateway(window.location.search)
const rows = ref<DataSourceSummary[]>([])
const loading = ref(true)
const refreshing = ref(false)
const error = ref('')
const restricted = ref(false)
const message = ref('')
const keyword = ref('')
const environment = ref('')
const state = ref('')
const connectionStatus = ref<ConnectionStatusFilter>('')
const compatibilityMode = ref('')
const searchInput = ref<HTMLInputElement>()
const editor = ref<{ id: string | null; focusTest: boolean; mode: 'MYSQL' | 'ORACLE'; instance: number }>()
const selectedId = ref<string | null>(null)
let editorSequence = 0
type LifecycleAction = 'enable' | 'disable' | 'delete'
const pending = ref<{ row: DataSourceSummary; action: LifecycleAction }>()
const actionBusy = ref(false)
const cursor = ref('')
const previousCursors = ref<string[]>([])
const nextCursor = ref('')
const total = ref(0)
let requestSequence = 0
let searchTimer: ReturnType<typeof setTimeout> | undefined
const query = computed(() => Object.fromEntries(Object.entries({ keyword: keyword.value.trim(), environment: environment.value, state: state.value, connectionStatus: connectionStatus.value, compatibilityMode: compatibilityMode.value }).filter(([, value]) => value)))
const filtered = computed(() => preview ? filterDataSources(rows.value, { keyword: keyword.value, environment: environment.value, state: state.value, connectionStatus: connectionStatus.value, compatibilityMode: compatibilityMode.value }) : rows.value)
const hasFilters = computed(() => Boolean(keyword.value || environment.value || state.value || connectionStatus.value || compatibilityMode.value))
const hasOnlySearch = computed(() => Boolean(keyword.value) && !environment.value && !state.value && !connectionStatus.value && !compatibilityMode.value)
const actionLabel = computed(() => ({ enable: '启用', disable: '停用', delete: '永久删除' })[pending.value?.action ?? 'disable'])
const visibleRows = computed(() => preview ? filtered.value.slice(Number(cursor.value || 0), Number(cursor.value || 0) + 10) : rows.value)
const resultTotal = computed(() => preview ? filtered.value.length : total.value)
const canNext = computed(() => preview ? Number(cursor.value || 0) + 10 < filtered.value.length : Boolean(nextCursor.value))
watch(query, () => {
  clearTimeout(searchTimer)
  // 条件改变即作废旧响应，防止慢请求覆盖新筛选；输入合并后重新请求首屏。
  requestSequence++
  cursor.value = ''; previousCursors.value = []; nextCursor.value = ''
  loading.value = true
  searchTimer = setTimeout(() => { void load() }, 250)
})
onBeforeUnmount(() => { clearTimeout(searchTimer); requestSequence++ })
async function changePage(forward: boolean) {
  if (loading.value || refreshing.value) return
  const oldCursor = cursor.value
  const oldHistory = [...previousCursors.value]
  if (forward) { previousCursors.value.push(cursor.value); cursor.value = preview ? String(Number(cursor.value || 0) + 10) : nextCursor.value }
  else cursor.value = previousCursors.value.pop() ?? ''
  if (!await load()) { cursor.value = oldCursor; previousCursors.value = oldHistory }
}

async function load(preserve = false) {
  const sequence = ++requestSequence
  if (preserve) refreshing.value = true
  else loading.value = true
  error.value = ''
  try {
    if (preview) {
      const result = await api.listDataSources()
      if (sequence !== requestSequence) return false
      rows.value = result
    } else {
      const result = await browserApi().listDataSourcePage({ ...query.value, ...(cursor.value ? { cursor: cursor.value } : {}) })
      if (sequence !== requestSequence) return false
      rows.value = result.items; nextCursor.value = result.nextCursor; total.value = result.total
    }
    restricted.value = false
    if (!rows.value.length && cursor.value) { cursor.value = ''; previousCursors.value = []; return await load() }
    return true
  }
  catch (caught) {
    if (sequence !== requestSequence) return false
    const status = (caught as Partial<ApiError>).status
    restricted.value = status === 401 || status === 403
    if (restricted.value || !preserve) rows.value = []
    error.value = dataSourceErrorMessage(caught, '无法读取数据源，请稍后重试。')
    nextCursor.value = ''
    return false
  } finally { if (sequence === requestSequence) { loading.value = false; refreshing.value = false } }
}
function edit(row?: DataSourceSummary, focusTest = false) {
  selectedId.value = row?.id ?? null
  editor.value = { id: row?.id ?? null, focusTest, mode: row?.compatibilityMode === 'ORACLE' ? 'ORACLE' : 'MYSQL', instance: ++editorSequence }
}
function create(mode: string) {
  if (restricted.value || (mode !== 'MYSQL' && mode !== 'ORACLE')) return
  selectedId.value = null
  editor.value = { id: null, focusTest: false, mode, instance: ++editorSequence }
}
const sourceTypes = [{ id: 'MYSQL', label: 'OceanBase MySQL', icon: Database }, { id: 'ORACLE', label: 'OceanBase Oracle', icon: Database }]
function clearFilters() { keyword.value = ''; environment.value = ''; state.value = ''; connectionStatus.value = ''; compatibilityMode.value = ''; searchInput.value?.focus() }
function menuItems(row: DataSourceSummary) {
  const eligibility = row.lifecycleEligibility
  const lifecycle = (action: LifecycleAction) => ({ disabled: !eligibility?.[action].allowed, reason: eligibility?.[action].reason ?? (!eligibility ? '服务端未提供操作资格，暂不可用。' : undefined) })
  return [
    { id: 'edit', label: '编辑配置', icon: Pencil },
    { id: 'test', label: '测试连接', icon: Unplug },
    { id: row.state === 'ENABLED' ? 'disable' : 'enable', label: row.state === 'ENABLED' ? '停用数据源' : '启用数据源', icon: Power, separator: true, ...lifecycle(row.state === 'ENABLED' ? 'disable' : 'enable') },
    { id: 'delete', label: '永久删除', icon: Trash2, danger: true, ...lifecycle('delete') },
  ]
}
function rowAction(row: DataSourceSummary, action: string) {
  if (action === 'edit' || action === 'test') { edit(row, action === 'test'); return }
  if (!['enable', 'disable', 'delete'].includes(action) || !row.lifecycleEligibility?.[action as LifecycleAction].allowed) return
  pending.value = { row, action: action as LifecycleAction }
}
async function performAction() {
  if (!pending.value || actionBusy.value) return
  const { row, action } = pending.value
  actionBusy.value = true
  try {
    if (action === 'delete') await api.deleteDataSource(row.id, row.revision)
    else await api.changeDataSourceState(row.id, row.revision, action === 'enable' ? 'ENABLED' : 'DISABLED')
    message.value = `数据源已${actionLabel.value}。`
    pending.value = undefined
    await load(true)
  } catch (caught) { error.value = dataSourceErrorMessage(caught, '操作未完成，请刷新后重试。'); pending.value = undefined }
  finally { actionBusy.value = false }
}
function saved(id: string) { selectedId.value = id; void load(true) }
onMounted(async () => {
  await load()
  const id = typeof route.query.edit === 'string' ? route.query.edit : undefined
  if (id && !restricted.value) { const row = rows.value.find((item) => item.id === id); if (row) edit(row, route.query.test === '1') }
})
</script>

<template>
  <div class="orch-ui">
    <div class="orch-source-workspace">
      <div v-if="preview" class="orch-preview-note" role="note" title="VISUAL VALIDATION ONLY · NON-AUTHORITATIVE BUSINESS DATA · SCHEMA-BOUND"><span>视觉验证</span>合成样本 · 操作仅在本页内存生效，不连接真实数据库</div>
      <header class="orch-page-header">
        <div class="orch-page-title"><h1>数据源管理</h1><span class="orch-context-label">OceanBase / ODP</span></div>
      </header>
      <div v-if="error && rows.length" class="orch-alert orch-alert--danger" role="alert"><strong>刷新未完成</strong><p>{{ error }} 当前仍展示上一次加载的记录。</p></div>
      <div v-if="message" class="orch-inline-feedback" role="status">{{ message }}</div>
      <section class="orch-management" aria-label="数据源列表">
        <div class="orch-table-meta">
          <h2>数据源列表</h2>
          <div class="orch-page-actions"><OrchButton variant="quiet" icon-only label="刷新数据源" :disabled="loading || refreshing" @click="load(true)"><RefreshCw :size="17" :class="{ 'orch-spin': refreshing }" /></OrchButton><OrchMenu v-if="!restricted" primary label="新建数据源" :items="sourceTypes" @select="create"><Plus :size="16" />新建数据源<ChevronDown :size="14" /></OrchMenu></div>
        </div>
        <div class="orch-toolbar" :aria-busy="refreshing">
          <label class="orch-search"><Search :size="17" /><span class="orch-sr-only">搜索数据源</span><input ref="searchInput" v-model="keyword" :disabled="restricted" placeholder="搜索名称、地址或租户" /></label>
          <label class="orch-filter"><span class="orch-sr-only">环境</span><select v-model="environment" :disabled="restricted"><option value="">全部环境</option><option v-for="item in environments" :key="item.value" :value="item.value">{{ item.label }}</option></select><ChevronDown :size="14" aria-hidden="true" /></label>
          <label class="orch-filter orch-filter-test"><span class="orch-sr-only">连接测试</span><select v-model="connectionStatus" :disabled="restricted"><option value="">全部测试结果</option><option value="UNTESTED">未测试</option><option value="TESTING">测试中</option><option value="SUCCEEDED">测试成功</option><option value="FAILED">连接失败</option><option value="INVALIDATED">已失效 / 已过期</option><option value="UNKNOWN">待确认</option></select><ChevronDown :size="14" aria-hidden="true" /></label>
          <label class="orch-filter"><span class="orch-sr-only">可用状态</span><select v-model="state" :disabled="restricted"><option value="">全部可用状态</option><option value="ENABLED">已启用</option><option value="DISABLED">已停用</option></select><ChevronDown :size="14" aria-hidden="true" /></label>
          <label class="orch-filter orch-filter-mode"><span class="orch-sr-only">兼容模式</span><select v-model="compatibilityMode" :disabled="restricted"><option value="">全部兼容模式</option><option value="MYSQL">MySQL</option><option value="ORACLE">Oracle</option></select><ChevronDown :size="14" aria-hidden="true" /></label>
          <OrchButton v-if="hasFilters" variant="quiet" icon-only label="清除筛选" @click="clearFilters"><FilterX :size="17" /></OrchButton>
        </div>
        <div v-if="loading" class="orch-table-loading" role="status" aria-label="正在加载数据源"><div v-for="n in 7" :key="n"><span class="orch-skeleton" /><span class="orch-skeleton" /><span class="orch-skeleton" /><span class="orch-skeleton" /></div></div>
        <div v-else-if="!rows.length && (!hasFilters || error)" class="orch-empty"><LockKeyhole v-if="restricted" :size="30" /><ServerCrash v-else-if="error" :size="30" /><Database v-else :size="30" /><h2>{{ restricted ? '当前身份无权访问数据源' : error ? '暂时无法读取数据源' : '尚未创建数据源' }}</h2><p>{{ restricted ? '请联系管理员确认数据源访问权限。' : error || '配置 OceanBase 连接信息后，再单独测试连接。' }}</p><OrchButton v-if="error && !restricted" @click="load()"><RefreshCw :size="16" />重新加载</OrchButton><OrchMenu v-else-if="!restricted" primary label="新建数据源" :items="sourceTypes" @select="create"><Plus :size="16" />新建数据源<ChevronDown :size="14" /></OrchMenu></div>
        <template v-else>
          <div class="orch-table-scroll" tabindex="0" role="region" aria-label="数据源事实表">
            <table class="orch-fact-table">
              <caption class="orch-sr-only">当前已加载的授权数据源{{ hasFilters ? '，已应用搜索或筛选条件' : '' }}</caption>
              <colgroup><col class="orch-col-identity" /><col class="orch-col-env" /><col class="orch-col-endpoint" /><col class="orch-col-tenant" /><col class="orch-col-mode" /><col class="orch-col-test" /><col class="orch-col-state" /><col class="orch-col-actions" /></colgroup>
              <thead><tr><th scope="col">数据源</th><th scope="col">环境</th><th scope="col">连接地址</th><th scope="col">集群 / 租户</th><th scope="col" class="orch-mode-cell">兼容模式</th><th scope="col" class="orch-status-cell">连接测试</th><th scope="col" class="orch-status-cell">可用状态</th><th scope="col"><span class="orch-sr-only">操作</span></th></tr></thead>
              <tbody>
                <tr v-for="row in visibleRows" :key="row.id" :class="{ 'is-selected': selectedId === row.id }">
                  <td class="orch-identity-cell"><span class="orch-source-icon"><Database :size="19" :stroke-width="1.5" /></span><div><button :title="row.displayName" @click="edit(row)">{{ row.displayName }}</button><span class="orch-secondary-fact" :title="[row.username, row.defaultDatabase].filter(Boolean).join(' · ')">{{ [row.username, row.defaultDatabase].filter(Boolean).join(' · ') || '身份信息未提供' }}<span class="orch-inline-mode"> · {{ row.compatibilityMode === 'MYSQL' ? 'MySQL' : row.compatibilityMode === 'ORACLE' ? 'Oracle' : '待校验' }}</span></span></div></td>
                  <td><span class="orch-env" :data-environment="row.environment">{{ environmentLabel(row.environment) }}</span></td>
                  <td><code class="orch-endpoint" :title="`${row.host}:${row.port}`">{{ row.host }}<span>:{{ row.port }}</span></code></td>
                  <td><code class="orch-fact-value" :title="row.clusterName || '-'">{{ row.clusterName || '-' }}</code><span class="orch-secondary-fact" :title="row.tenantName">{{ row.tenantName }}</span></td>
                  <td class="orch-mode-cell"><span class="orch-mode">{{ row.compatibilityMode === 'MYSQL' ? 'MySQL' : row.compatibilityMode === 'ORACLE' ? 'Oracle' : '待校验' }}</span></td>
                  <td class="orch-status-cell"><OrchStatus :label="connectionListFact(row).label" :tone="connectionListFact(row).tone" dot-only /></td>
                  <td class="orch-status-cell"><OrchStatus :tone="row.state === 'ENABLED' ? 'success' : row.state === 'DISABLED' ? 'danger' : 'neutral'" :label="row.state === 'ENABLED' ? '已启用' : row.state === 'DISABLED' ? '已停用' : '待确认'" /></td>
                  <td class="orch-row-actions"><div><OrchButton class="orch-row-test" variant="quiet" icon-only :label="`测试 ${row.displayName}`" @click="edit(row, true)"><Unplug :size="16" /></OrchButton><OrchMenu :label="`${row.displayName} 的操作`" :items="menuItems(row)" @select="rowAction(row, $event)" /></div></td>
                </tr>
              </tbody>
            </table>
          </div><div v-if="!filtered.length" class="orch-empty orch-empty--search"><Search :size="28" /><h2>{{ hasOnlySearch ? '未找到匹配的数据源' : '当前筛选条件下没有结果' }}</h2><p>清除或调整条件后重新查看。</p><OrchButton @click="clearFilters">清除筛选</OrchButton></div><footer class="orch-table-footer"><span>共 {{ resultTotal }} 条</span><nav class="source-pagination" aria-label="数据源分页"><span>10 条/页</span><OrchButton variant="quiet" icon-only label="上一页" :disabled="!previousCursors.length || loading || refreshing" @click="changePage(false)"><ChevronLeft :size="16" /></OrchButton><span aria-live="polite">第 {{ previousCursors.length + 1 }} 页</span><OrchButton variant="quiet" icon-only label="下一页" :disabled="!canNext || loading || refreshing" @click="changePage(true)"><ChevronRight :size="16" /></OrchButton></nav></footer>
        </template>
      </section>
    </div>
    <SourceEditor v-if="editor" :key="editor.instance" :source-id="editor.id" :initial-mode="editor.mode" :focus-test="editor.focusTest" :api="api" :preview="preview" @close="editor = undefined" @saved="saved" />
    <OrchDialog :open="Boolean(pending)" :title="`${actionLabel}数据源？`" :confirm-label="`确认${actionLabel}`" :destructive="pending?.action === 'delete'" :busy="actionBusy" @cancel="pending = undefined" @confirm="performAction"><p class="orch-confirm-object">{{ pending?.row.displayName }}</p><p>{{ pending?.action === 'delete' ? '此操作会永久删除数据源及其当前凭据修订，无法恢复。' : pending?.action === 'disable' ? '停用后不能被新任务选择。不会取消已有运行任务。' : '启用后仍需满足服务端任务资格检查，不代表所有导出能力均可用。' }}</p></OrchDialog>
  </div>
</template>
