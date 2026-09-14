<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Archive, CircleAlert, CircleCheck, CircleDashed, CircleHelp, CircleMinus, CircleX, Ellipsis, Info, LoaderCircle, Pencil, Plus, Power, RefreshCw, Search, SlidersHorizontal, Trash2, TriangleAlert, Unplug } from '@lucide/vue'

import { browserApi, dataSourceErrorMessage, type ApiError, type DataSourceSummary } from '@/api/browser'
import FilterToolbar from '@/components/FilterToolbar.vue'
import LifecycleActionMenuItem from '@/components/LifecycleActionMenuItem.vue'
import WorkbenchAlertDialog from '@/components/WorkbenchAlertDialog.vue'
import WorkbenchButton from '@/components/WorkbenchButton.vue'
import WorkbenchIconButton from '@/components/WorkbenchIconButton.vue'
import WorkbenchStatus from '@/components/WorkbenchStatus.vue'
import WorkbenchTable from '@/components/WorkbenchTable.vue'
import { useWorkbenchFoundation } from '@/components/workbenchFoundation'
import DataSourceEditDrawer from './DataSourceEditDrawer.vue'
import { filterDataSources, type ConnectionStatusFilter } from './dataSourceListFilters'
import { dataSourceUiFixtureForSearch } from './dataSourceUiFixture'

type LifecycleAction = 'enable' | 'disable' | 'delete' | 'archive'
type PendingAction = { source: DataSourceSummary; action: Exclude<LifecycleAction, 'enable'> }
type StatusTone = 'success' | 'danger' | 'warning' | 'info' | 'neutral'

const api = browserApi()
const foundation = useWorkbenchFoundation()
const route = useRoute()
const sources = ref<DataSourceSummary[]>([])
const initialLoading = ref(true)
const refreshing = ref(false)
const loadFailure = ref('')
const refreshFailure = ref('')
const feedback = ref('')
const notice = ref('')
const permissionRestricted = ref(false)
const keyword = ref('')
const searchInput = ref<HTMLInputElement>()
const environment = ref('')
const state = ref('')
const compatibilityMode = ref('')
const connectionStatus = ref<ConnectionStatusFilter>('')
const actionID = ref('')
const drawerVisible = ref(false)
const drawerSourceID = ref<string | null>(null)
const drawerFocusTest = ref(false)
const pendingAction = ref<PendingAction>()
const openActionMenuID = ref('')
const actionMenuElement = ref<HTMLElement>()
const actionMenuTrigger = ref<HTMLButtonElement>()
const actionMenuPosition = ref({ top: 0, left: 0 })
const uiFixtureMode = ref(false)

const hasActiveFilters = computed(() => Boolean(keyword.value || environment.value || state.value || compatibilityMode.value || connectionStatus.value))
const filtersAvailable = computed(() => uiFixtureMode.value)
const displayedSources = computed(() => filtersAvailable.value
  ? filterDataSources(sources.value, { keyword: keyword.value, environment: environment.value, compatibilityMode: compatibilityMode.value, connectionStatus: connectionStatus.value, state: state.value })
  : sources.value)
// 编辑与连接测试不依赖生命周期资格投影；后者只决定菜单中的启用、停用、删除与归档是否可执行。
const hasManagementActions = computed(() => !permissionRestricted.value)
const openActionMenuSource = computed(() => displayedSources.value.find((source) => source.id === openActionMenuID.value))
const tableColumnCount = computed(() => hasManagementActions.value ? 8 : 7)
const isSearchOnlyEmpty = computed(() => Boolean(keyword.value) && !environment.value && !state.value && !compatibilityMode.value && !connectionStatus.value)
const confirmTitle = computed(() => pendingAction.value?.action === 'disable' ? '停用数据源？' : pendingAction.value?.action === 'delete' ? '永久删除数据源？' : '归档数据源？')
const confirmDescription = computed(() => pendingAction.value?.action === 'disable'
  ? '停用后，该数据源不能再被新任务选择。'
  : pendingAction.value?.action === 'delete' ? '此操作会永久删除数据源及其当前凭据修订，无法恢复。'
    : archiveReferenceDescription(pendingAction.value?.source))
const confirmLabel = computed(() => pendingAction.value?.action === 'disable' ? '确认停用' : pendingAction.value?.action === 'delete' ? '永久删除' : '确认归档')
const confirmImpacts = computed(() => pendingAction.value?.action === 'disable'
  ? ['不会取消已经运行的任务。', '等待中的任务在启动前仍会重新检查数据源状态。']
  : pendingAction.value?.action === 'delete' ? ['不会改写历史任务，因为当前没有需要保留的历史引用。']
    : ['归档后不能用于创建新任务。', '不会修改已提交或运行中的任务快照。'])
const confirmObjectContext = computed(() => {
  const source = pendingAction.value?.source
  return source ? `${environmentLabel(source.environment)} · ${source.clusterName} / ${source.tenantName}` : ''
})

onMounted(async () => {
  await loadSources()
  openDrawerFromRoute()
  document.addEventListener('pointerdown', onGlobalPointerDown)
  document.addEventListener('keydown', onGlobalKeydown)
  window.addEventListener('resize', dismissActionMenu)
  window.addEventListener('scroll', dismissActionMenu, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onGlobalPointerDown)
  document.removeEventListener('keydown', onGlobalKeydown)
  window.removeEventListener('resize', dismissActionMenu)
  window.removeEventListener('scroll', dismissActionMenu, true)
})

watch(drawerVisible, (visible) => {
  if (visible) {
    dismissActionMenu()
    return
  }
  drawerSourceID.value = null
  drawerFocusTest.value = false
})

watch(() => route.query.edit, openDrawerFromRoute)

function openDrawerFromRoute() {
  const edit = typeof route.query.edit === 'string' ? route.query.edit : ''
  if (!edit || uiFixtureMode.value) return
  drawerSourceID.value = edit
  drawerFocusTest.value = route.query.test === '1'
  drawerVisible.value = true
}

function openCreateDrawer() {
  if (uiFixtureMode.value) {
    notice.value = 'UI Review Fixture 仅用于页面视觉与状态回归，不会打开真实数据源表单或执行业务操作。'
    return
  }
  drawerSourceID.value = null
  drawerFocusTest.value = false
  drawerVisible.value = true
}

function openEditDrawer(source: DataSourceSummary, focusTest = false) {
  if (uiFixtureMode.value) {
    notice.value = 'UI Review Fixture 仅用于页面视觉与状态回归，不会读取或修改真实数据源。'
    return
  }
  closeActionMenu(true)
  drawerSourceID.value = source.id
  drawerFocusTest.value = focusTest
  drawerVisible.value = true
}

async function onDrawerSaved(id: string) {
  drawerSourceID.value = id
  await loadSources(true)
}

async function loadSources(preserveRows = false) {
  if (preserveRows && sources.value.length > 0) refreshing.value = true
  else initialLoading.value = true
  loadFailure.value = ''
  refreshFailure.value = ''
  permissionRestricted.value = false
  try {
    const fixture = dataSourceUiFixtureForSearch(window.location.search)
    if (fixture) {
      uiFixtureMode.value = true
      sources.value = fixture
      return
    }
    uiFixtureMode.value = false
    sources.value = await api.listDataSources()
  } catch (error) {
    const apiError = error as Partial<ApiError>
    permissionRestricted.value = apiError.status === 403
    const message = dataSourceErrorMessage(error, '无法加载数据源，请稍后重试。')
    if (preserveRows && sources.value.length > 0) refreshFailure.value = message
    else loadFailure.value = message
  } finally {
    initialLoading.value = false
    refreshing.value = false
  }
}

function resetFilters() {
  keyword.value = ''
  environment.value = ''
  compatibilityMode.value = ''
  connectionStatus.value = ''
  state.value = ''
  searchInput.value?.focus()
}

function stateLabel(source: DataSourceSummary) {
  return source.state === 'ENABLED' ? '已启用' : '已禁用'
}

function environmentLabel(value: string) {
  return { DEVELOPMENT: '开发', TEST: '测试', STAGING: '预生产', PRODUCTION: '生产' }[value] ?? value
}

function compatibilityModeLabel(value: string) {
  return value === 'MYSQL' ? 'MySQL' : value === 'ORACLE' ? 'Oracle' : '待校验'
}

function identityMetadata(source: DataSourceSummary) {
  return [source.username, source.defaultDatabase].filter(Boolean).join(' · ')
}

function connectionIcon(source: DataSourceSummary) {
  switch (source.lastTestStatus) {
    case 'PENDING': case 'LEASED': return LoaderCircle
    case 'SUCCEEDED': return CircleCheck
    case 'FAILED': return CircleX
    case 'INVALIDATED': case 'EXPIRED': return TriangleAlert
    case 'UNKNOWN': return CircleHelp
    default: return CircleDashed
  }
}

function connectionPresentation(source: DataSourceSummary): { label: string; detail: string; tone: StatusTone } {
  const completedAt = source.lastTestedAt ? new Date(source.lastTestedAt).toLocaleString() : ''
  switch (source.lastTestStatus) {
    case 'PENDING': return { label: '测试中', detail: '等待节点受控回写', tone: 'info' }
    case 'LEASED': return { label: '测试中', detail: '执行节点已领取测试', tone: 'info' }
    case 'SUCCEEDED': return { label: '最近测试成功', detail: completedAt || '验证来源未在列表投影中提供', tone: 'neutral' }
    case 'FAILED': return { label: '连接失败', detail: completedAt || '请在抽屉查看安全诊断', tone: 'danger' }
    case 'INVALIDATED': return { label: '结果已失效', detail: completedAt || '连接配置或凭据已变更', tone: 'warning' }
    case 'EXPIRED': return { label: '结果已过期', detail: completedAt || '需要重新测试', tone: 'warning' }
    case 'UNKNOWN': return { label: '状态待确认', detail: completedAt || '未形成可验证结论', tone: 'neutral' }
    default: return { label: '未测试', detail: '尚未形成基础连接结论', tone: 'neutral' }
  }
}

function lifecycleEligibility(source: DataSourceSummary, action: LifecycleAction) {
  return source.lifecycleEligibility?.[action]
}

function archiveReferenceDescription(source: DataSourceSummary | undefined) {
  const referenceCount = source?.lifecycleEligibility?.archive?.referenceCount ?? source?.lifecycleEligibility?.delete?.referenceCount
  return referenceCount && referenceCount > 0
    ? `该数据源被 ${referenceCount} 个历史任务引用。本次将归档而非删除，以保留引用与审计记录。`
    : '该数据源存在历史任务引用。本次将归档而非删除，以保留引用与审计记录。'
}

function requestAction(source: DataSourceSummary, action: LifecycleAction) {
  dismissActionMenu()
  if (uiFixtureMode.value) {
    notice.value = 'UI Review Fixture 不执行启用、停用、删除或归档操作。'
    return
  }
  if (!lifecycleEligibility(source, action)?.allowed) return
  if (action === 'enable') {
    void performAction(source, action)
    return
  }
  pendingAction.value = { source, action }
}

async function toggleActionMenu(source: DataSourceSummary, event: MouseEvent) {
  if (!(event.currentTarget instanceof HTMLButtonElement)) return
  if (openActionMenuID.value === source.id) {
    closeActionMenu(true)
    return
  }
  openActionMenuID.value = source.id
  actionMenuTrigger.value = event.currentTarget
  await nextTick()
  positionActionMenu()
  actionMenuElement.value?.querySelector<HTMLButtonElement>('button:not([disabled])')?.focus()
}

function positionActionMenu() {
  const trigger = actionMenuTrigger.value
  const menu = actionMenuElement.value
  if (!trigger || !menu) return
  const triggerRect = trigger.getBoundingClientRect()
  const menuWidth = menu.offsetWidth
  const menuHeight = menu.offsetHeight
  const left = Math.min(window.innerWidth - menuWidth - 8, Math.max(8, triggerRect.right - menuWidth))
  const preferredTop = triggerRect.bottom + 4
  actionMenuPosition.value = { left, top: preferredTop + menuHeight <= window.innerHeight - 8 ? preferredTop : Math.max(8, triggerRect.top - menuHeight - 4) }
}

function closeActionMenu(restoreFocus = false) {
  const trigger = actionMenuTrigger.value
  openActionMenuID.value = ''
  actionMenuTrigger.value = undefined
  if (restoreFocus) trigger?.focus()
}

function dismissActionMenu() {
  closeActionMenu()
}

function onGlobalPointerDown(event: PointerEvent) {
  const target = event.target
  if (!(target instanceof Node)) return
  const filters = document.querySelector<HTMLDetailsElement>('.data-source-filter details[open]')
  if (filters && !filters.contains(target)) filters.open = false
  if (actionMenuElement.value?.contains(target) || actionMenuTrigger.value?.contains(target)) return
  dismissActionMenu()
}

function onGlobalKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  const filters = document.querySelector<HTMLDetailsElement>('.data-source-filter details[open]')
  if (filters) {
    event.preventDefault()
    filters.open = false
    filters.querySelector('summary')?.focus()
  }
  if (!openActionMenuID.value) return
  event.preventDefault()
  closeActionMenu(true)
}

function onActionMenuKeydown(event: KeyboardEvent) {
  const items = actionMenuElement.value ? Array.from(actionMenuElement.value.querySelectorAll<HTMLButtonElement>('button:not([disabled])')) : []
  const index = items.indexOf(document.activeElement as HTMLButtonElement)
  if (!items.length || index < 0) return
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    items[(index + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length]?.focus()
  }
  if (event.key === 'Home') { event.preventDefault(); items[0]?.focus() }
  if (event.key === 'End') { event.preventDefault(); items.at(-1)?.focus() }
  if (event.key === 'Tab') closeActionMenu(true)
}

async function confirmPendingAction() {
  const current = pendingAction.value
  if (!current) return
  await performAction(current.source, current.action)
  pendingAction.value = undefined
}

async function performAction(source: DataSourceSummary, action: LifecycleAction) {
  actionID.value = source.id
  feedback.value = ''
  notice.value = ''
  try {
    if (action === 'delete') {
      await api.deleteDataSource(source.id, source.revision)
      notice.value = '数据源已永久删除。'
    } else if (action === 'archive') {
      await api.archiveDataSource(source.id, source.revision)
      notice.value = '数据源已归档，历史引用已保留。'
    } else {
      const result = await api.changeDataSourceState(source.id, source.revision, action === 'enable' ? 'ENABLED' : 'DISABLED')
      notice.value = result.state === 'ENABLED' ? '数据源已启用；实际任务资格仍由服务端门禁决定。' : '数据源已停用；不会取消既有任务。'
    }
    await loadSources(true)
  } catch (error) {
    feedback.value = dataSourceErrorMessage(error, '数据源操作失败。')
    await loadSources(true)
  } finally {
    actionID.value = ''
  }
}
</script>

<template>
  <div class="data-source-page">
    <header class="data-source-page-header">
      <h1>数据源管理</h1>
      <WorkbenchButton v-if="!permissionRestricted" variant="primary" @click="openCreateDrawer">
        <template #icon><Plus :size="16" aria-hidden="true" /></template>新增数据源
      </WorkbenchButton>
    </header>

    <section class="data-source-workspace" aria-labelledby="data-source-table-title">
      <h2 id="data-source-table-title" class="visually-hidden">当前范围内的数据源</h2>
      <FilterToolbar class="data-source-filter">
        <label class="filter-keyword">
          <span class="visually-hidden">搜索数据源</span>
          <Search class="filter-search-icon" :size="16" aria-hidden="true" />
          <input ref="searchInput" v-model.trim="keyword" :disabled="!filtersAvailable" :aria-describedby="!filtersAvailable ? 'data-source-filter-contract-note' : undefined" placeholder="搜索名称、地址、集群或租户" />
        </label>
        <label class="filter-select filter-primary"><span>环境</span><select v-model="environment" :disabled="!filtersAvailable"><option value="">全部</option><option value="DEVELOPMENT">开发</option><option value="TEST">测试</option><option value="STAGING">预生产</option><option value="PRODUCTION">生产</option></select></label>
        <label class="filter-select filter-primary"><span>可用性</span><select v-model="state" :disabled="!filtersAvailable"><option value="">全部</option><option value="ENABLED">已启用</option><option value="DISABLED">已禁用</option></select></label>
        <label class="filter-select filter-primary filter-test"><span>连接测试</span><select v-model="connectionStatus" :disabled="!filtersAvailable"><option value="">全部</option><option value="UNTESTED">未测试</option><option value="TESTING">测试中</option><option value="SUCCEEDED">最近测试成功</option><option value="FAILED">连接失败</option><option value="INVALIDATED">结果已失效</option><option value="UNKNOWN">状态待确认</option></select></label>
        <label class="filter-select filter-compatibility"><span>兼容模式</span><select v-model="compatibilityMode" :disabled="!filtersAvailable"><option value="">全部</option><option value="MYSQL">MySQL</option><option value="ORACLE">Oracle</option></select></label>
        <details class="filter-more filter-compatibility-more">
          <summary><SlidersHorizontal :size="16" aria-hidden="true" />更多筛选</summary>
          <div class="filter-options"><label class="filter-select"><span>兼容模式</span><select v-model="compatibilityMode" :disabled="!filtersAvailable"><option value="">全部</option><option value="MYSQL">MySQL</option><option value="ORACLE">Oracle</option></select></label></div>
        </details>
        <details class="filter-more filter-compact-more">
          <summary><SlidersHorizontal :size="16" aria-hidden="true" />筛选</summary>
          <div class="filter-options">
            <label class="filter-select"><span>环境</span><select v-model="environment" :disabled="!filtersAvailable"><option value="">全部</option><option value="DEVELOPMENT">开发</option><option value="TEST">测试</option><option value="STAGING">预生产</option><option value="PRODUCTION">生产</option></select></label>
            <label class="filter-select"><span>可用性</span><select v-model="state" :disabled="!filtersAvailable"><option value="">全部</option><option value="ENABLED">已启用</option><option value="DISABLED">已禁用</option></select></label>
            <label class="filter-select"><span>连接测试</span><select v-model="connectionStatus" :disabled="!filtersAvailable"><option value="">全部</option><option value="UNTESTED">未测试</option><option value="TESTING">测试中</option><option value="SUCCEEDED">最近测试成功</option><option value="FAILED">连接失败</option><option value="INVALIDATED">结果已失效</option><option value="UNKNOWN">状态待确认</option></select></label>
            <label class="filter-select"><span>兼容模式</span><select v-model="compatibilityMode" :disabled="!filtersAvailable"><option value="">全部</option><option value="MYSQL">MySQL</option><option value="ORACLE">Oracle</option></select></label>
          </div>
        </details>
        <WorkbenchButton v-if="hasActiveFilters" variant="text" @click="resetFilters">清除筛选</WorkbenchButton>
        <div class="filter-utilities">
          <WorkbenchIconButton :label="initialLoading || refreshing ? '正在刷新数据源' : '刷新数据源'" :disabled="initialLoading || refreshing" @click="loadSources(sources.length > 0)">
            <RefreshCw :size="16" :class="{ 'is-spinning': initialLoading || refreshing }" aria-hidden="true" />
          </WorkbenchIconButton>
        </div>
      </FilterToolbar>

      <p v-if="!filtersAvailable && !initialLoading && !loadFailure" id="data-source-filter-contract-note" class="data-source-contract-note"><Info :size="15" aria-hidden="true" />当前仅展示已加载的数据源，搜索和筛选暂不可用。</p>
      <p v-if="feedback" class="data-source-feedback is-error" role="alert"><CircleAlert :size="16" aria-hidden="true" />{{ feedback }}</p>
      <p v-else-if="refreshFailure" class="data-source-feedback is-error" role="status">刷新失败：{{ refreshFailure }} <button type="button" @click="loadSources(true)">重试</button></p>
      <p v-else-if="notice" class="data-source-feedback is-notice" role="status">{{ notice }}</p>

      <WorkbenchTable class="data-source-table-scroll" aria-label="数据源列表" :aria-busy="initialLoading || refreshing" tabindex="0">
        <table>
          <caption class="visually-hidden">当前授权范围内的数据源；连接测试状态不代表对象权限、性能或任务可行性。</caption>
          <colgroup>
            <col class="column-name" /><col class="column-environment" /><col class="column-endpoint" /><col class="column-cluster" /><col class="column-compatibility" /><col class="column-connection" /><col class="column-availability" /><col v-if="hasManagementActions" class="column-actions" />
          </colgroup>
          <thead><tr><th scope="col">数据源</th><th scope="col">环境</th><th scope="col">Endpoint</th><th scope="col">集群 / 租户</th><th scope="col" class="column-compatibility">兼容模式</th><th scope="col">连接测试</th><th scope="col">可用性</th><th v-if="hasManagementActions" scope="col" class="column-actions">操作</th></tr></thead>
          <tbody v-if="initialLoading"><tr><td :colspan="tableColumnCount" class="data-source-table-message"><span class="loading-lines" aria-label="正在加载数据源"><i v-for="index in 7" :key="index" /></span></td></tr></tbody>
          <tbody v-else-if="permissionRestricted"><tr><td :colspan="tableColumnCount" class="data-source-table-message"><CircleAlert :size="24" aria-hidden="true" /><strong>无权访问数据源</strong><span>当前身份没有查看此范围数据源的权限。</span></td></tr></tbody>
          <tbody v-else-if="loadFailure"><tr><td :colspan="tableColumnCount" class="data-source-table-message is-error"><Unplug :size="24" aria-hidden="true" /><strong>无法加载数据源</strong><span>{{ loadFailure }}</span><WorkbenchButton @click="loadSources()">重试</WorkbenchButton></td></tr></tbody>
          <tbody v-else-if="sources.length === 0"><tr><td :colspan="tableColumnCount" class="data-source-table-message"><strong>暂无数据源</strong><span>新增并完成基础连接测试后，数据源才可能用于任务配置。</span></td></tr></tbody>
          <tbody v-else-if="displayedSources.length === 0"><tr><td :colspan="tableColumnCount" class="data-source-table-message"><Search :size="24" aria-hidden="true" /><strong>{{ isSearchOnlyEmpty ? '没有匹配搜索条件的数据源' : '没有符合当前筛选条件的数据源' }}</strong><span>{{ isSearchOnlyEmpty ? '请调整搜索词，或清除搜索。' : '请调整或清除当前筛选条件。' }}</span><WorkbenchButton variant="text" @click="resetFilters">{{ isSearchOnlyEmpty ? '清除搜索' : '清除筛选' }}</WorkbenchButton></td></tr></tbody>
          <tbody v-else>
            <tr v-for="source in displayedSources" :key="source.id" :class="{ 'is-disabled': source.state === 'DISABLED', 'is-current': drawerVisible && drawerSourceID === source.id }">
              <td class="data-source-name">
                <div class="cell-stack">
                  <button type="button" class="identity-action" :title="source.displayName" @click="openEditDrawer(source)">{{ source.displayName }}</button>
                  <small class="identity-metadata" :title="identityMetadata(source)"><span class="identity-compatibility">{{ compatibilityModeLabel(source.compatibilityMode) }} · </span>{{ identityMetadata(source) }}</small>
                </div>
              </td>
              <td><span class="environment-context">{{ environmentLabel(source.environment) }}</span></td>
              <td><div class="cell-stack"><strong class="technical-fact" :title="source.host">{{ source.host }}</strong><small>端口 <span class="technical-fact">{{ source.port }}</span></small></div></td>
              <td><div class="cell-stack"><strong :title="source.clusterName || '-'">{{ source.clusterName || '-' }}</strong><small :title="source.tenantName">{{ source.tenantName }}</small></div></td>
              <td class="column-compatibility">{{ compatibilityModeLabel(source.compatibilityMode) }}</td>
              <td>
                <div class="cell-stack">
                  <WorkbenchStatus :tone="connectionPresentation(source).tone"><template #icon><component :is="connectionIcon(source)" :size="14" :class="{ 'is-spinning': source.lastTestStatus === 'PENDING' || source.lastTestStatus === 'LEASED' }" aria-hidden="true" /></template>{{ connectionPresentation(source).label }}</WorkbenchStatus>
                  <small :title="connectionPresentation(source).detail">{{ connectionPresentation(source).detail }}</small>
                </div>
              </td>
              <td><WorkbenchStatus :tone="source.state === 'ENABLED' ? 'success' : 'danger'"><template #icon><CircleCheck v-if="source.state === 'ENABLED'" :size="14" aria-hidden="true" /><CircleMinus v-else :size="14" aria-hidden="true" /></template>{{ stateLabel(source) }}</WorkbenchStatus></td>
              <td v-if="hasManagementActions" class="data-source-row-actions">
                <div class="row-actions-group">
                  <div class="data-source-inline-actions"><button type="button" class="row-action" :disabled="actionID === source.id" @click="openEditDrawer(source)">编辑</button><button type="button" class="row-action" :disabled="actionID === source.id" @click="openEditDrawer(source, true)">测试连接</button></div>
                  <button type="button" class="row-action-more-button" :aria-label="`更多操作：${source.displayName}`" :title="`更多操作：${source.displayName}`" aria-haspopup="menu" :aria-expanded="openActionMenuID === source.id" :disabled="actionID === source.id" @click.stop="toggleActionMenu(source, $event)"><Ellipsis :size="16" aria-hidden="true" /></button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </WorkbenchTable>
      <!-- 当前 API 无 cursor 响应字段，不渲染页码、总数或虚构的翻页能力。 -->
    </section>
  </div>

  <Teleport to="body">
    <div v-if="openActionMenuSource" ref="actionMenuElement" class="data-source-row-action-popover" :class="{ 'workbench-foundation': foundation }" role="menu" :aria-label="`更多操作：${openActionMenuSource.displayName}`" :style="{ top: `${actionMenuPosition.top}px`, left: `${actionMenuPosition.left}px` }" @pointerdown.stop @keydown="onActionMenuKeydown">
      <button type="button" class="popover-direct-action" role="menuitem" @click="openEditDrawer(openActionMenuSource)"><Pencil :size="14" aria-hidden="true" />编辑</button>
      <button type="button" class="popover-direct-action" role="menuitem" @click="openEditDrawer(openActionMenuSource, true)"><Unplug :size="14" aria-hidden="true" />测试连接</button>
      <div class="menu-divider" role="separator" />
      <LifecycleActionMenuItem :eligibility="lifecycleEligibility(openActionMenuSource, openActionMenuSource.state === 'ENABLED' ? 'disable' : 'enable')" :label="openActionMenuSource.state === 'ENABLED' ? '停用数据源' : '启用数据源'" :busy="actionID === openActionMenuSource.id" @execute="requestAction(openActionMenuSource, openActionMenuSource.state === 'ENABLED' ? 'disable' : 'enable')"><template #icon><Power :size="14" aria-hidden="true" /></template></LifecycleActionMenuItem>
      <LifecycleActionMenuItem :eligibility="lifecycleEligibility(openActionMenuSource, 'delete')" label="删除数据源" destructive :busy="actionID === openActionMenuSource.id" @execute="requestAction(openActionMenuSource, 'delete')"><template #icon><Trash2 :size="14" aria-hidden="true" /></template></LifecycleActionMenuItem>
      <LifecycleActionMenuItem :eligibility="lifecycleEligibility(openActionMenuSource, 'archive')" label="归档数据源" destructive :busy="actionID === openActionMenuSource.id" @execute="requestAction(openActionMenuSource, 'archive')"><template #icon><Archive :size="14" aria-hidden="true" /></template></LifecycleActionMenuItem>
    </div>
  </Teleport>
  <DataSourceEditDrawer v-model="drawerVisible" :data-source-id="drawerSourceID" :focus-test="drawerFocusTest" @saved="onDrawerSaved" />
  <WorkbenchAlertDialog :open="Boolean(pendingAction)" :title="confirmTitle" :description="confirmDescription" :object-name="pendingAction?.source.displayName" :object-context="confirmObjectContext" :impacts="confirmImpacts" :confirm-label="confirmLabel" :destructive="pendingAction?.action === 'delete' || pendingAction?.action === 'archive'" :busy="Boolean(actionID)" @cancel="pendingAction = undefined" @confirm="confirmPendingAction" />
</template>

<style scoped>
.data-source-page { min-width: 0; color: var(--text-primary); }
.data-source-page-header { display: flex; align-items: center; justify-content: space-between; gap: 24px; margin-bottom: 24px; }
.data-source-page-header h1 { margin: 0; font-size: var(--text-page-title-size); font-weight: 600; line-height: var(--text-page-title-line-height); }
.data-source-workspace { min-width: 0; background: var(--surface-primary); }
.data-source-filter { position: relative; min-height: 68px; padding: 16px; gap: 8px; border-bottom: 0; }
.filter-keyword { position: relative; width: 304px; min-width: 200px; flex: 0 1 304px; }
.filter-search-icon { position: absolute; top: 10px; left: 12px; color: var(--text-secondary); pointer-events: none; }
.filter-keyword input { width: 100%; padding-left: 36px; }
.filter-keyword input::placeholder { color: var(--text-secondary); }
.filter-select { display: flex; min-width: 0; height: var(--size-control); align-items: center; border: 1px solid var(--color-border-strong); border-radius: var(--radius-control); background: var(--surface-primary); color: var(--text-secondary); font-size: 12px; }
.filter-select > span { flex: none; padding-left: 12px; }
.filter-select select { width: 88px; height: calc(var(--size-control) - 2px); padding-left: 8px; border: 0; background-color: transparent; font-size: 12px; }
.filter-test select { width: 136px; }
.filter-select:has(select:disabled) { background: var(--surface-disabled); }
.filter-select:focus-within { border-color: var(--border-focus); }
.filter-select select:disabled { background-color: transparent; }
.filter-more { position: relative; display: none; }
.filter-more summary { display: flex; height: var(--size-control); align-items: center; gap: 8px; padding-inline: 12px; border: 1px solid var(--color-border-strong); border-radius: var(--radius-control); color: var(--text-secondary); background: var(--surface-primary); font-size: 12px; cursor: pointer; list-style: none; }
.filter-more summary::-webkit-details-marker { display: none; }
.filter-more[open] summary { background: var(--surface-subtle); border-color: var(--text-secondary); }
.filter-options { position: absolute; top: calc(100% + 8px); right: 0; z-index: 5; display: grid; min-width: 248px; gap: 12px; padding: 16px; border: 1px solid var(--border-default); border-radius: var(--radius-overlay); background: var(--surface-primary); box-shadow: 0 12px 28px rgb(29 50 76 / 18%); }
.filter-options .filter-select { justify-content: space-between; }
.filter-options select { width: 150px; }
.filter-utilities { display: flex; flex: none; margin-left: auto; padding-left: 12px; border-left: 1px solid var(--border-subtle); }
.data-source-contract-note, .data-source-feedback { display: flex; align-items: flex-start; gap: 8px; margin: 0; padding: 0 16px 12px; color: var(--text-secondary); font-size: 12px; line-height: 18px; }
.data-source-contract-note svg, .data-source-feedback svg { flex: none; margin-top: 1px; }
.data-source-feedback { margin: 0 16px 12px; padding: 8px 12px; border-left: 3px solid var(--status-info); background: var(--surface-subtle); }
.data-source-feedback.is-error { border-color: var(--status-error); color: var(--status-error); background: var(--color-danger-bg); }
.data-source-feedback button { padding: 0; border: 0; color: var(--interactive-primary); background: transparent; font: inherit; text-decoration: underline; cursor: pointer; }
.data-source-table-scroll { border-top: 1px solid var(--border-default); border-bottom: 1px solid var(--border-default); overflow-x: auto; }
.data-source-table-scroll :deep(table) { table-layout: fixed; }
.data-source-table-scroll :deep(th), .data-source-table-scroll :deep(td) { overflow: hidden; padding: 0 12px; border-bottom: 1px solid var(--border-subtle); text-align: left; vertical-align: middle; }
.data-source-table-scroll :deep(th) { height: var(--size-table-header); color: var(--text-secondary); background: var(--surface-subtle); font-size: 12px; font-weight: 600; line-height: 18px; white-space: nowrap; }
.data-source-table-scroll :deep(td) { height: var(--size-table-row); color: var(--text-primary); font-size: 13px; line-height: 18px; }
.data-source-table-scroll :deep(tbody tr:last-child td) { border-bottom: 0; }
.data-source-table-scroll :deep(tbody tr:hover td) { background: var(--surface-subtle); }
.data-source-table-scroll :deep(tbody tr.is-current td) { background: var(--interactive-selected); }
.column-name { width: 24%; }
.column-environment { width: 5%; }
.column-endpoint { width: 16%; }
.column-cluster { width: 15%; }
.column-compatibility { width: 6%; }
.column-connection { width: 19%; }
.column-availability { width: 6%; }
.column-actions { width: 9%; }
.cell-stack { display: flex; flex-direction: column; min-width: 0; gap: 0; }
.cell-stack strong, .cell-stack small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cell-stack strong { font-weight: 400; line-height: 18px; }
.cell-stack small { color: var(--text-secondary); font-size: 12px; line-height: 18px; }
.technical-fact { font-family: "Cascadia Mono", Consolas, ui-monospace, monospace; font-variant-numeric: tabular-nums; }
.identity-action { width: fit-content; max-width: 100%; overflow: hidden; padding: 0; border: 0; border-radius: 2px; color: var(--text-primary); background: transparent; font: inherit; font-weight: 600; text-align: left; text-overflow: ellipsis; white-space: nowrap; cursor: pointer; }
.identity-action:hover { color: var(--interactive-primary); text-decoration: underline; text-underline-offset: 3px; }
.identity-compatibility { display: none; }
.environment-context { display: inline-flex; min-height: 22px; align-items: center; padding-inline: 6px; border: 1px solid var(--border-subtle); border-radius: 4px; color: var(--text-secondary); background: var(--surface-subtle); font-size: 12px; white-space: nowrap; }
.data-source-row-actions { white-space: nowrap; }
.row-actions-group, .data-source-inline-actions { display: flex; align-items: center; gap: 4px; }
.row-actions-group { justify-content: flex-end; }
.row-action, .row-action-more-button, .popover-direct-action { display: inline-flex; height: 32px; align-items: center; justify-content: center; gap: 8px; padding: 0 4px; border: 0; border-radius: var(--radius-control); color: var(--interactive-primary); background: transparent; font: inherit; font-size: 12px; white-space: nowrap; cursor: pointer; }
.row-action:hover, .row-action-more-button:hover, .popover-direct-action:hover { background: var(--interactive-selected); }
.row-action:disabled, .row-action-more-button:disabled { color: var(--text-disabled); cursor: not-allowed; }
.row-action-more-button { width: 32px; flex: none; color: var(--text-secondary); }
.data-source-table-message { height: 280px !important; padding: 24px !important; text-align: center !important; }
.data-source-table-message > svg { margin-bottom: 12px; color: var(--text-secondary); }
.data-source-table-message > strong, .data-source-table-message > span { display: block; }
.data-source-table-message > strong { margin-bottom: 8px; font-size: 16px; line-height: 24px; }
.data-source-table-message > span { color: var(--text-secondary); }
.data-source-table-message > :deep(button) { margin-top: 16px; }
.data-source-table-message.is-error strong { color: var(--status-error); }
.loading-lines { display: grid; gap: 16px; }
.loading-lines i { display: block; height: 16px; background: var(--surface-page); }
.loading-lines i:nth-child(2n) { width: 84%; }
.data-source-row-action-popover { position: fixed; z-index: 110; display: grid; width: 248px; max-height: calc(100dvh - 16px); overflow-y: auto; padding: 6px; border: 1px solid var(--border-default); border-radius: var(--radius-overlay); background: var(--surface-primary); box-shadow: 0 12px 28px rgb(29 50 76 / 18%); }
.popover-direct-action { justify-content: flex-start; width: 100%; padding: 0 10px; color: var(--text-primary); font-size: 13px; text-align: left; }
.menu-divider { height: 1px; margin: 6px 4px; background: var(--border-subtle); }
.visually-hidden { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
@media (max-width: 1440px) {
  .filter-keyword { flex-basis: 264px; }
  .filter-compatibility, .column-compatibility, .data-source-inline-actions { display: none; }
  .filter-compatibility-more { display: block; }
  .identity-compatibility { display: inline; }
  .column-name { width: 25%; }
  .column-environment { width: 6%; }
  .column-endpoint { width: 18%; }
  .column-cluster { width: 18%; }
  .column-connection { width: 21%; }
  .column-availability { width: 8%; }
  .column-actions { width: 4%; }
}
@media (max-width: 1280px) {
  .filter-primary, .filter-compatibility-more, .identity-compatibility { display: none; }
  .filter-compact-more { display: block; }
  .filter-keyword { flex: 0 1 360px; }
}
@media (max-width: 760px) {
  .data-source-page-header { gap: 12px; flex-wrap: wrap; margin-bottom: 20px; }
  .data-source-filter { padding: 12px; }
  .filter-keyword { flex: 1 1 200px; }
  .data-source-table-scroll :deep(table) { min-width: 820px; }
  .filter-options { right: auto; left: 0; }
}
</style>
