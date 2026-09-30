<script setup lang="ts">
import { Alert as AAlert, Badge as ABadge, message as antMessage, Skeleton, Button as AButton, Input as AInput, Select as ASelect, SelectOption as ASelectOption, Dropdown as ADropdown, Menu as AMenu, MenuItem as AMenuItem, Switch as ASwitch, type TableColumnsType } from 'ant-design-vue'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { DownOutlined, LeftOutlined, RightOutlined, ClearOutlined, PlusOutlined, ReloadOutlined, SearchOutlined, ApiOutlined } from '@ant-design/icons-vue'
import EmptyState from '@/components/EmptyState.vue'
import { browserApi, dataSourceErrorMessage, type ApiError, type DataSourceSummary } from '@/api/browser'
import { filterDataSources, type ConnectionStatusFilter } from '@/views/dataSourceListFilters'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import OrchSourceActions from './OrchSourceActions.vue'
import OrchOperationalTable from '@/components/OrchOperationalTable.vue'
import SourceEditor from './SourceEditor.vue'
import { sourceGateway } from './sourceGateway'
import { product } from '@/platform/tokens'
import { ConfigProvider, Tag } from 'ant-design-vue'
import { environmentTagTheme, managementTheme } from '@/platform/theme'
import { antBadgeStatus, connectionListFact, environmentLabel, environments } from './sourcePresentation'

const route = useRoute()
const { api, preview } = sourceGateway(window.location.search)
const rows = ref<DataSourceSummary[]>([])
const loading = ref(true)
const refreshing = ref(false)
const error = ref('')
const actionError = ref('')
const restricted = ref(false)
const [messageApi, MessageContext] = antMessage.useMessage()
const keyword = ref('')
const environment = ref('')
const state = ref('')
const connectionStatus = ref<ConnectionStatusFilter>('')
const compatibilityMode = ref('')
const searchInput = ref<{ focus: () => void }>()
const editor = ref<{ id: string | null; focusTest: boolean; mode: 'MYSQL' | 'ORACLE'; instance: number }>()
const selectedId = ref<string | null>(null)
let editorSequence = 0
let editorInvoker: HTMLElement | null = null
function rememberEditorInvoker(event: MouseEvent) { editorInvoker = event.currentTarget as HTMLElement }
async function closeEditor() {
  editor.value = undefined
  await nextTick()
  if (editorInvoker?.isConnected) editorInvoker.focus({ preventScroll: true })
}
type LifecycleAction = 'enable' | 'disable' | 'delete'
const actionBusy = ref(false)
const busySourceId = ref<string | null>(null)
const pendingDelete = ref<DataSourceSummary>()
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
  editorInvoker = document.activeElement instanceof HTMLElement ? document.activeElement : null
  selectedId.value = row?.id ?? null
  editor.value = { id: row?.id ?? null, focusTest, mode: row?.compatibilityMode === 'ORACLE' ? 'ORACLE' : 'MYSQL', instance: ++editorSequence }
}
function create(mode: string) {
  if (restricted.value || (mode !== 'MYSQL' && mode !== 'ORACLE')) return
  selectedId.value = null
  editor.value = { id: null, focusTest: false, mode, instance: ++editorSequence }
}
const sourceTypes = [{ id: 'MYSQL', label: 'OceanBase MySQL' }, { id: 'ORACLE', label: 'OceanBase Oracle' }]
const columns: TableColumnsType<DataSourceSummary> = [
  { key: 'identity', title: '数据源', width: '26%', className: 'orch-identity-cell' },
  { key: 'environment', title: '环境', width: '7%' },
  { key: 'endpoint', title: '连接地址', width: '16%' },
  { key: 'tenant', title: '集群 / 租户', width: '13%' },
  { key: 'mode', title: '兼容模式', width: '8%', className: 'orch-mode-cell orch-short-fact', align: 'center' },
  { key: 'test', title: '连接测试', width: '15%', className: 'orch-short-fact', align: 'center' },
  { key: 'state', title: '可用状态', width: '8%', className: 'orch-short-fact', align: 'center' },
  { key: 'actions', title: '操作', width: '7%', className: 'orch-row-actions' },
]
function clearFilters() { keyword.value = ''; environment.value = ''; state.value = ''; connectionStatus.value = ''; compatibilityMode.value = ''; searchInput.value?.focus() }
function stateEligibility(row: DataSourceSummary) { return row.lifecycleEligibility?.[row.state === 'ENABLED' ? 'disable' : 'enable'] }
function rowAction(row: DataSourceSummary, action: string) {
  if (actionBusy.value) return
  if (action === 'edit') { edit(row); return }
  if (!['enable', 'disable', 'delete'].includes(action) || !row.lifecycleEligibility?.[action as LifecycleAction].allowed) return
  if (action === 'delete') { pendingDelete.value = row; return }
  void performAction({ row, action: action as LifecycleAction })
}
async function confirmDelete() {
  if (!pendingDelete.value || actionBusy.value) return
  await performAction({ row: pendingDelete.value, action: 'delete' })
  pendingDelete.value = undefined
}
async function performAction(target: { row: DataSourceSummary; action: LifecycleAction }) {
  if (!target || actionBusy.value) return
  const { row, action } = target
  actionBusy.value = true
  busySourceId.value = row.id
  actionError.value = ''
  try {
    if (action === 'delete') await api.deleteDataSource(row.id, row.revision)
    else await api.changeDataSourceState(row.id, row.revision, action === 'enable' ? 'ENABLED' : 'DISABLED')
    if (action === 'delete') void messageApi.success(`数据源“${row.displayName}”已删除。`)
    if (action === 'delete' && selectedId.value === row.id) selectedId.value = null
    await load(true)
  } catch (caught) {
    actionError.value = dataSourceErrorMessage(caught, '操作未完成，请刷新后重试。')
    // 失败后重读资格与版本，不乐观移除记录，也不把删除改成归档。
    await load(true)
  }
  finally { actionBusy.value = false; busySourceId.value = null }
}
function saved(id: string) { selectedId.value = id; void load(true) }
onMounted(async () => {
  await load()
  const id = typeof route.query.edit === 'string' ? route.query.edit : undefined
  if (id && !restricted.value) { const row = rows.value.find((item) => item.id === id); if (row) edit(row, route.query.test === '1') }
})
</script>

<template>
  <ConfigProvider :theme="managementTheme">
    <MessageContext />
    <div class="orch-ui">
      <div class="orch-source-workspace">
        <div v-if="preview" class="orch-preview-note" role="note" title="VISUAL VALIDATION ONLY · NON-AUTHORITATIVE BUSINESS DATA · SCHEMA-BOUND"><span>视觉验证</span>合成样本 · 操作仅在本页内存生效，不连接真实数据库</div>
        <header class="orch-page-header">
          <div class="orch-page-title"><h1>数据源管理</h1><span class="orch-context-label">OceanBase / ODP</span></div>
        </header>
        <AAlert v-if="error && rows.length" class="orch-feedback" type="error" message="刷新未完成" :description="`${error} 当前仍展示上一次加载的记录。`" show-icon />
        <AAlert v-if="actionError" class="orch-feedback" type="error" message="操作未完成" :description="actionError" show-icon />
        <section class="orch-management" aria-label="数据源列表">
          <div class="orch-table-meta">
            <h2>数据源列表</h2>
            <div class="orch-page-actions"><AButton :disabled="loading || refreshing" type="text" aria-label="刷新数据源" class="orch-icon-action" @click="load(true)"><template #icon><ReloadOutlined class="product-icon" :spin="refreshing" aria-hidden="true" /></template></AButton><ADropdown v-if="!restricted" :trigger="['click']"><AButton type="primary" @click="rememberEditorInvoker"><template #icon><PlusOutlined class="product-icon" aria-hidden="true" /></template>新建数据源<DownOutlined class="product-icon" aria-hidden="true" /></AButton><template #overlay><AMenu @click="({key}) => create(String(key))"><AMenuItem v-for="item in sourceTypes" :key="item.id">{{ item.label }}</AMenuItem></AMenu></template></ADropdown></div>
          </div>
          <div class="orch-toolbar" :aria-busy="refreshing">
            <label class="source-search"><span class="orch-sr-only">搜索数据源</span><AInput ref="searchInput" v-model:value="keyword" aria-label="搜索数据源" :disabled="restricted" placeholder="搜索名称、地址或租户"><template #prefix><SearchOutlined class="product-icon" aria-hidden="true" /></template></AInput></label>
            <label class="orch-filter"><span class="orch-sr-only">环境</span><ASelect v-model:value="environment" aria-label="环境" :disabled="restricted"><ASelectOption value="">全部环境</ASelectOption><ASelectOption v-for="item in environments" :key="item.value" :value="item.value">{{ item.label }}</ASelectOption></ASelect></label>
            <label class="orch-filter orch-filter-test"><span class="orch-sr-only">连接测试</span><ASelect v-model:value="connectionStatus" aria-label="连接测试" :disabled="restricted"><ASelectOption value="">全部测试结果</ASelectOption><ASelectOption value="UNTESTED">未测试</ASelectOption><ASelectOption value="TESTING">测试中</ASelectOption><ASelectOption value="SUCCEEDED">测试成功</ASelectOption><ASelectOption value="FAILED">连接失败</ASelectOption><ASelectOption value="INVALIDATED">已失效 / 已过期</ASelectOption><ASelectOption value="UNKNOWN">待确认</ASelectOption></ASelect></label>
            <label class="orch-filter"><span class="orch-sr-only">可用状态</span><ASelect v-model:value="state" aria-label="可用状态" :disabled="restricted"><ASelectOption value="">全部可用状态</ASelectOption><ASelectOption value="ENABLED">已启用</ASelectOption><ASelectOption value="DISABLED">已停用</ASelectOption></ASelect></label>
            <label class="orch-filter orch-filter-mode"><span class="orch-sr-only">兼容模式</span><ASelect v-model:value="compatibilityMode" aria-label="兼容模式" :disabled="restricted"><ASelectOption value="">全部兼容模式</ASelectOption><ASelectOption value="MYSQL">MySQL</ASelectOption><ASelectOption value="ORACLE">Oracle</ASelectOption></ASelect></label>
            <AButton v-if="hasFilters" type="text" aria-label="清除筛选" class="orch-icon-action" @click="clearFilters"><template #icon><ClearOutlined class="product-icon" aria-hidden="true" /></template></AButton>
          </div>
          <Skeleton v-if="loading" active :paragraph="{ rows: 7 }" role="status" aria-label="正在加载数据源" />
          <AAlert v-else-if="error && !rows.length" class="orch-feedback" :type="restricted ? 'warning' : 'error'" :message="restricted ? '当前身份无权访问数据源' : '暂时无法读取数据源'" :description="restricted ? '请联系管理员确认数据源访问权限。' : error" show-icon><template #action><AButton v-if="!restricted" @click="load()"><template #icon><ReloadOutlined class="product-icon" aria-hidden="true" /></template>重新加载</AButton></template></AAlert>
          <EmptyState v-else-if="!rows.length && !hasFilters" class="orch-empty-region" title="尚未创建数据源" description="配置 OceanBase 连接信息后，再单独测试连接。"><ADropdown :trigger="['click']"><AButton type="primary" @click="rememberEditorInvoker"><template #icon><PlusOutlined class="product-icon" aria-hidden="true" /></template>新建数据源<DownOutlined class="product-icon" aria-hidden="true" /></AButton><template #overlay><AMenu @click="({key}) => create(String(key))"><AMenuItem v-for="item in sourceTypes" :key="item.id">{{ item.label }}</AMenuItem></AMenu></template></ADropdown></EmptyState>
          <template v-else>
            <OrchOperationalTable :rows="visibleRows" :columns="columns" row-key="id" :selected-key="selectedId" :minimum-width="product.source.tableMinimumWidth" label="当前已加载的授权数据源">
              <template #bodyCell="{ column, record: row }">
                <template v-if="column.key === 'identity'"><div><AButton :title="row.displayName" type="link" class="orch-fact-link" @click="edit(row)">{{ row.displayName }}</AButton><span class="orch-secondary-fact" :title="[row.username, row.defaultDatabase].filter(Boolean).join(' · ')">{{ [row.username, row.defaultDatabase].filter(Boolean).join(' · ') || '身份信息未提供' }}<span class="orch-inline-mode"> · {{ row.compatibilityMode === 'MYSQL' ? 'MySQL' : row.compatibilityMode === 'ORACLE' ? 'Oracle' : '待校验' }}</span></span></div></template>
                <template v-if="column.key === 'environment'"><ConfigProvider :theme="environmentTagTheme(row.environment)"><Tag :bordered="false">{{ environmentLabel(row.environment) }}</Tag></ConfigProvider></template>
                <template v-if="column.key === 'endpoint'"><code class="orch-endpoint" :title="`${row.host}:${row.port}`">{{ row.host }}<span>:{{ row.port }}</span></code></template>
                <template v-if="column.key === 'tenant'"><code class="orch-fact-value" :title="row.clusterName || '-'">{{ row.clusterName || '-' }}</code><span class="orch-secondary-fact" :title="row.tenantName">{{ row.tenantName }}</span></template>
                <template v-if="column.key === 'mode'"><span class="orch-mode">{{ row.compatibilityMode === 'MYSQL' ? 'MySQL' : row.compatibilityMode === 'ORACLE' ? 'Oracle' : '待校验' }}</span></template>
                <template v-if="column.key === 'test'"><ABadge :status="antBadgeStatus(connectionListFact(row).tone)" :text="connectionListFact(row).label" :aria-label="connectionListFact(row).label" /></template>
                <template v-if="column.key === 'state'"><ASwitch :loading="busySourceId === row.id" :checked="row.state === 'ENABLED'" :aria-label="`${row.displayName} 的可用状态`" :disabled="actionBusy || !stateEligibility(row)?.allowed" @change="rowAction(row, row.state === 'ENABLED' ? 'disable' : 'enable')" /><span v-if="!stateEligibility(row)?.allowed" class="orch-secondary-fact">{{ stateEligibility(row)?.reason ?? '暂无操作权限' }}</span></template>
                <template v-if="column.key === 'actions'"><div><AButton type="text" :aria-label="`测试 ${row.displayName}`" class="orch-row-test orch-icon-action" @click="edit(row, true)"><template #icon><ApiOutlined class="product-icon" aria-hidden="true" /></template></AButton><OrchSourceActions :source="row" :busy="actionBusy" @edit="edit(row)" @delete="rowAction(row, 'delete')" /></div></template>
              </template>
              <template #empty><EmptyState v-if="!filtered.length" class="orch-empty-region" :title="hasOnlySearch ? '未找到匹配的数据源' : '当前筛选条件下没有结果'" description="清除或调整条件后重新查看。" action="清除筛选" @action="clearFilters" /></template>
            </OrchOperationalTable><footer class="orch-table-footer"><span>共 {{ resultTotal }} 条</span><nav class="source-pagination" aria-label="数据源分页"><span>10 条/页</span><AButton :disabled="!previousCursors.length || loading || refreshing" type="text" aria-label="上一页" class="orch-icon-action" @click="changePage(false)"><template #icon><LeftOutlined class="product-icon" aria-hidden="true" /></template></AButton><span aria-live="polite">第 {{ previousCursors.length + 1 }} 页</span><AButton :disabled="!canNext || loading || refreshing" type="text" aria-label="下一页" class="orch-icon-action" @click="changePage(true)"><template #icon><RightOutlined class="product-icon" aria-hidden="true" /></template></AButton></nav></footer>
          </template>
        </section>
      </div>
      <SourceEditor v-if="editor" :key="editor.instance" :source-id="editor.id" :initial-mode="editor.mode" :focus-test="editor.focusTest" :api="api" :preview="preview" @close="closeEditor" @saved="saved" />
      <OrchDangerConfirm :open="Boolean(pendingDelete)" title="删除数据源" confirm-label="删除" destructive :busy="actionBusy" @cancel="pendingDelete = undefined" @confirm="confirmDelete">
        <p>确定删除数据源“{{ pendingDelete?.displayName }}”吗？</p>
        <p>删除后无法恢复，连接配置及凭据将被清除，历史任务和执行记录保留。存在未完成或状态待核对的任务时不能删除。</p>
      </OrchDangerConfirm>
    </div>
  </ConfigProvider>
</template>
