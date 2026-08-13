<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Archive, ArrowDown, ArrowUp, ChevronsUpDown, Ellipsis as MoreHorizontal, Info, Plus, Power, RefreshCw, Search } from '@lucide/vue'

import { browserApi, dataSourceErrorMessage, type DataSourceSummary } from '@/api/browser'
import WorkbenchButton from '@/components/WorkbenchButton.vue'
import WorkbenchStatus from '@/components/WorkbenchStatus.vue'
import DataSourceConfirmDialog from './DataSourceConfirmDialog.vue'
import DataSourceEditDrawer from './DataSourceEditDrawer.vue'
import { dataSourceDeletionNotice } from './dataSourceDeletionNotice'
import { filterDataSources, type ConnectionStatusFilter } from './dataSourceListFilters'
import { sortDataSources, type DataSourceSortKey, type SortDirection } from './dataSourceListSort'

type PendingAction = { source: DataSourceSummary; action: 'disable' | 'archive' }

const api = browserApi()
const sources = ref<DataSourceSummary[]>([])
const loading = ref(true)
const loadFailure = ref('')
const feedback = ref('')
const notice = ref('')
const keyword = ref('')
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
const sortKey = ref<DataSourceSortKey>('name')
const sortDirection = ref<SortDirection>('asc')

const filteredSources = computed(() => filterDataSources(sources.value, {
  keyword: keyword.value,
  environment: environment.value,
  compatibilityMode: compatibilityMode.value,
  connectionStatus: connectionStatus.value,
  state: state.value,
}))
const visibleSources = computed(() => sortDataSources(filteredSources.value, sortKey.value, sortDirection.value))
const hasActiveFilters = computed(() => Boolean(keyword.value || environment.value || state.value || compatibilityMode.value || connectionStatus.value))
const openActionMenuSource = computed(() => sources.value.find((source) => source.id === openActionMenuID.value))
const confirmTitle = computed(() => pendingAction.value?.action === 'disable' ? '禁用数据源？' : '删除或归档数据源？')
const confirmDescription = computed(() => pendingAction.value?.action === 'disable'
  ? '禁用后，该数据源不能再被新任务选择。'
  : '系统会根据引用关系决定物理删除或归档。')
const confirmLabel = computed(() => pendingAction.value?.action === 'disable' ? '确认禁用' : '确认删除 / 归档')
const confirmImpacts = computed(() => pendingAction.value?.action === 'disable'
  ? ['不会取消已经运行的任务。', '等待中的任务在启动前仍会重新检查数据源状态。']
  : ['不会改写历史任务。', '界面只展示最终删除或归档结果。'])

onMounted(() => {
  void loadSources()
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

function openCreateDrawer() {
  drawerSourceID.value = null
  drawerFocusTest.value = false
  drawerVisible.value = true
}

function openEditDrawer(source: DataSourceSummary, focusTest = false) {
  drawerSourceID.value = source.id
  drawerFocusTest.value = focusTest
  drawerVisible.value = true
}

async function onDrawerSaved(id: string) {
  drawerSourceID.value = id
  await refreshSources()
}

async function loadSources() {
  loading.value = true
  await refreshSources()
  loading.value = false
}

async function refreshSources() {
  loadFailure.value = ''
  try {
    sources.value = await api.listDataSources()
  } catch (error) {
    loadFailure.value = dataSourceErrorMessage(error, '无法加载数据源，请稍后重试。')
  }
}

function resetFilters() {
  keyword.value = ''
  environment.value = ''
  compatibilityMode.value = ''
  connectionStatus.value = ''
  state.value = ''
}

function stateLabel(source: DataSourceSummary) {
  return source.state === 'ENABLED' ? '已启用' : '已禁用'
}

function environmentLabel(value: string) {
  return { DEVELOPMENT: '开发', TEST: '测试', STAGING: '预生产', PRODUCTION: '生产' }[value] ?? value
}

function compatibilityModeLabel(value: string) {
  return value === 'MYSQL' ? 'MySQL' : value === 'ORACLE' ? 'Oracle' : '待迁移类型'
}

function toggleSort(key: DataSourceSortKey) {
  if (sortKey.value === key) {
    sortDirection.value = sortDirection.value === 'asc' ? 'desc' : 'asc'
    return
  }
  sortKey.value = key
  sortDirection.value = 'asc'
}

function sortAria(key: DataSourceSortKey): 'none' | 'ascending' | 'descending' {
  if (sortKey.value !== key) return 'none'
  return sortDirection.value === 'asc' ? 'ascending' : 'descending'
}

function connectionStatusLabel(source: DataSourceSummary) {
  return {
    SUCCEEDED: '可连接',
    FAILED: '连接失败',
    UNKNOWN: '结果未知',
    EXPIRED: '测试已过期',
    INVALIDATED: '测试已失效',
    PENDING: '测试已请求',
    UNAVAILABLE: '测试不可用',
  }[source.lastTestStatus ?? ''] ?? '未测试'
}

function connectionStatusTone(source: DataSourceSummary) {
  if (source.lastTestStatus === 'SUCCEEDED') return 'success'
  if (source.lastTestStatus === 'FAILED') return 'danger'
  if (source.lastTestStatus === 'PENDING') return 'info'
  if (source.lastTestStatus === 'EXPIRED' || source.lastTestStatus === 'INVALIDATED') return 'warning'
  return 'neutral'
}

function lastTestTimeLabel(source: DataSourceSummary) {
  if (!source.lastTestStatus) return '尚未测试'
  if (!source.lastTestedAt) return '暂无完成时间'
  return new Date(source.lastTestedAt).toLocaleString()
}

function requestAction(source: DataSourceSummary, action: 'toggle' | 'archive') {
  dismissActionMenu()
  if (action === 'archive') {
    pendingAction.value = { source, action: 'archive' }
    return
  }
  if (source.state === 'ENABLED') {
    pendingAction.value = { source, action: 'disable' }
    return
  }
  void performAction(source, 'enable')
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
  const top = preferredTop + menuHeight <= window.innerHeight - 8
    ? preferredTop
    : Math.max(8, triggerRect.top - menuHeight - 4)
  actionMenuPosition.value = { top, left }
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
  if (actionMenuElement.value?.contains(target) || actionMenuTrigger.value?.contains(target)) return
  dismissActionMenu()
}

function onGlobalKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape' || !openActionMenuID.value) return
  event.preventDefault()
  closeActionMenu(true)
}

async function confirmPendingAction() {
  const current = pendingAction.value
  if (!current) return
  await performAction(current.source, current.action)
  pendingAction.value = undefined
}

async function performAction(source: DataSourceSummary, action: 'enable' | 'disable' | 'archive') {
  actionID.value = source.id
  feedback.value = ''
  notice.value = ''
  try {
    if (action === 'archive') {
      const result = await api.deleteOrArchiveDataSource(source.id, source.revision)
      sources.value = sources.value.filter((item) => item.id !== source.id)
      notice.value = dataSourceDeletionNotice(result)
      return
    }
    const result = await api.changeDataSourceState(source.id, source.revision, action === 'enable' ? 'ENABLED' : 'DISABLED')
    sources.value = sources.value.map((item) => item.id === source.id ? { ...item, state: result.state, revision: result.revision } : item)
    notice.value = result.state === 'ENABLED' ? '数据源已启用。是否可被任务选择仍由服务端门禁决定。' : '数据源已禁用；不会取消既有任务。'
  } catch (error) {
    feedback.value = dataSourceErrorMessage(error, '数据源操作失败。')
  } finally {
    actionID.value = ''
  }
}
</script>

<template>
  <div class="data-source-page">
    <header class="data-source-page-header">
      <div>
        <h1>数据源管理</h1>
        <p>管理私有 ODP 数据源、连接可用性和启用状态。</p>
      </div>
      <WorkbenchButton variant="primary" @click="openCreateDrawer"><template #icon><Plus :size="15" aria-hidden="true" /></template>新增数据源</WorkbenchButton>
    </header>

    <section class="data-source-filter" aria-label="数据源筛选">
      <label class="filter-keyword"><span class="visually-hidden">关键字</span><Search class="filter-search-icon" :size="15" aria-hidden="true" /><input v-model.trim="keyword" placeholder="名称或 IP / 域名" /></label>
      <label><span class="visually-hidden">环境</span><select v-model="environment" aria-label="环境"><option value="">全部环境</option><option value="DEVELOPMENT">开发</option><option value="TEST">测试</option><option value="STAGING">预生产</option><option value="PRODUCTION">生产</option></select></label>
      <label><span class="visually-hidden">租户模式</span><select v-model="compatibilityMode" aria-label="租户模式"><option value="">全部模式</option><option value="MYSQL">MySQL</option><option value="ORACLE">Oracle</option><option value="UNKNOWN">待迁移类型</option></select></label>
      <label><span class="visually-hidden">连接状态</span><select v-model="connectionStatus" aria-label="连接状态"><option value="">全部连接状态</option><option value="UNTESTED">未测试</option><option value="SUCCEEDED">可连接</option><option value="FAILED">连接失败</option><option value="UNKNOWN">结果未知</option><option value="EXPIRED">测试已过期</option><option value="INVALIDATED">测试已失效</option><option value="PENDING">测试已请求</option><option value="UNAVAILABLE">测试不可用</option></select></label>
      <label><span class="visually-hidden">启用状态</span><select v-model="state" aria-label="启用状态"><option value="">全部启用状态</option><option value="ENABLED">已启用</option><option value="DISABLED">已禁用</option></select></label>
      <WorkbenchButton v-if="hasActiveFilters" variant="text" @click="resetFilters">重置</WorkbenchButton>
      <span class="filter-grow" />
      <WorkbenchButton :disabled="loading" @click="loadSources"><template #icon><RefreshCw :size="14" :class="{ 'is-spinning': loading }" aria-hidden="true" /></template>{{ loading ? '刷新中…' : '刷新' }}</WorkbenchButton>
    </section>

    <p v-if="feedback" class="data-source-feedback is-error" role="alert">{{ feedback }}</p>
    <p v-else-if="notice" class="data-source-feedback is-notice" role="status">{{ notice }}</p>

    <section class="data-source-table-region" aria-labelledby="data-source-table-title">
      <div class="data-source-table-meta">
        <h2 id="data-source-table-title">数据源</h2>
        <span v-if="!loading && !loadFailure" aria-label="当前数据源数量">{{ filteredSources.length }}</span>
      </div>

      <div class="data-source-table-scroll">
        <table>
          <thead>
            <tr>
              <th class="column-name" :aria-sort="sortAria('name')"><button type="button" class="sort-header" @click="toggleSort('name')"><span>数据源</span><ArrowUp v-if="sortKey === 'name' && sortDirection === 'asc'" :size="13" aria-hidden="true" /><ArrowDown v-else-if="sortKey === 'name'" :size="13" aria-hidden="true" /><ChevronsUpDown v-else :size="13" aria-hidden="true" /></button></th>
              <th class="column-environment" :aria-sort="sortAria('environment')"><button type="button" class="sort-header" @click="toggleSort('environment')"><span>环境</span><ArrowUp v-if="sortKey === 'environment' && sortDirection === 'asc'" :size="13" aria-hidden="true" /><ArrowDown v-else-if="sortKey === 'environment'" :size="13" aria-hidden="true" /><ChevronsUpDown v-else :size="13" aria-hidden="true" /></button></th>
              <th class="column-host" :aria-sort="sortAria('host')"><button type="button" class="sort-header" @click="toggleSort('host')"><span>IP/域名</span><ArrowUp v-if="sortKey === 'host' && sortDirection === 'asc'" :size="13" aria-hidden="true" /><ArrowDown v-else-if="sortKey === 'host'" :size="13" aria-hidden="true" /><ChevronsUpDown v-else :size="13" aria-hidden="true" /></button></th>
              <th class="column-port" :aria-sort="sortAria('port')"><button type="button" class="sort-header" @click="toggleSort('port')"><span>端口</span><ArrowUp v-if="sortKey === 'port' && sortDirection === 'asc'" :size="13" aria-hidden="true" /><ArrowDown v-else-if="sortKey === 'port'" :size="13" aria-hidden="true" /><ChevronsUpDown v-else :size="13" aria-hidden="true" /></button></th>
              <th class="column-tenant" :aria-sort="sortAria('tenant')"><button type="button" class="sort-header" @click="toggleSort('tenant')"><span>租户名</span><ArrowUp v-if="sortKey === 'tenant' && sortDirection === 'asc'" :size="13" aria-hidden="true" /><ArrowDown v-else-if="sortKey === 'tenant'" :size="13" aria-hidden="true" /><ChevronsUpDown v-else :size="13" aria-hidden="true" /></button></th>
              <th class="column-mode" :aria-sort="sortAria('mode')"><button type="button" class="sort-header" @click="toggleSort('mode')"><span>租户模式</span><ArrowUp v-if="sortKey === 'mode' && sortDirection === 'asc'" :size="13" aria-hidden="true" /><ArrowDown v-else-if="sortKey === 'mode'" :size="13" aria-hidden="true" /><ChevronsUpDown v-else :size="13" aria-hidden="true" /></button></th>
              <th class="column-connection" :aria-sort="sortAria('connection')" title="连接状态只表示网络、认证和基础数据库连接，不代表导入导出权限、对象权限、性能或任务一定可执行。"><button type="button" class="sort-header" @click="toggleSort('connection')"><span class="column-heading-with-help">连接状态<Info :size="13" aria-hidden="true" /></span><ArrowUp v-if="sortKey === 'connection' && sortDirection === 'asc'" :size="13" aria-hidden="true" /><ArrowDown v-else-if="sortKey === 'connection'" :size="13" aria-hidden="true" /><ChevronsUpDown v-else :size="13" aria-hidden="true" /></button></th>
              <th class="column-state" :aria-sort="sortAria('state')"><button type="button" class="sort-header" @click="toggleSort('state')"><span>启用状态</span><ArrowUp v-if="sortKey === 'state' && sortDirection === 'asc'" :size="13" aria-hidden="true" /><ArrowDown v-else-if="sortKey === 'state'" :size="13" aria-hidden="true" /><ChevronsUpDown v-else :size="13" aria-hidden="true" /></button></th>
              <th class="column-actions">操作</th>
            </tr>
          </thead>
          <tbody v-if="loading">
            <tr><td colspan="9" class="data-source-table-message">正在加载已授权数据源…</td></tr>
          </tbody>
          <tbody v-else-if="loadFailure">
            <tr><td colspan="9" class="data-source-table-message is-error"><strong>无法加载数据源</strong><span>{{ loadFailure }}</span><WorkbenchButton @click="loadSources">重试</WorkbenchButton></td></tr>
          </tbody>
          <tbody v-else-if="sources.length === 0">
            <tr><td colspan="9" class="data-source-table-message"><strong>暂无数据源</strong><span>新增数据源后，可以在任务配置中选择并使用。</span></td></tr>
          </tbody>
          <tbody v-else-if="filteredSources.length === 0">
            <tr><td colspan="9" class="data-source-table-message"><strong>没有符合当前筛选条件的数据源</strong><span>请调整筛选条件，或清除当前筛选。</span><WorkbenchButton variant="text" @click="resetFilters">清除筛选</WorkbenchButton></td></tr>
          </tbody>
          <tbody v-else>
            <tr v-for="source in visibleSources" :key="source.id">
              <td class="data-source-name"><div class="cell-stack"><strong>{{ source.displayName }}</strong><small>{{ source.id }}</small></div></td>
              <td><span class="environment-label" :class="`is-${source.environment.toLowerCase()}`">{{ environmentLabel(source.environment) }}</span></td>
              <td class="data-source-host" :title="source.host">{{ source.host }}</td>
              <td class="data-source-port">{{ source.port }}</td>
              <td class="data-source-tenant" :title="source.tenantName">{{ source.tenantName }}</td>
              <td>{{ compatibilityModeLabel(source.compatibilityMode) }}</td>
              <td class="data-source-connection"><div class="cell-stack"><WorkbenchStatus :tone="connectionStatusTone(source)">{{ connectionStatusLabel(source) }}</WorkbenchStatus><small>{{ lastTestTimeLabel(source) }}</small></div></td>
              <td><WorkbenchStatus appearance="tag" :tone="source.state === 'ENABLED' ? 'enabled' : 'neutral'">{{ stateLabel(source) }}</WorkbenchStatus></td>
              <td class="data-source-row-actions">
                <button type="button" class="row-action" :disabled="actionID === source.id" @click="openEditDrawer(source)">编辑</button>
                <button type="button" class="row-action" :disabled="actionID === source.id" @click="openEditDrawer(source, true)">测试</button>
                <button
                  type="button"
                  class="row-action-more-button"
                  :aria-label="`更多操作：${source.displayName}`"
                  aria-haspopup="menu"
                  :aria-expanded="openActionMenuID === source.id"
                  :disabled="actionID === source.id"
                  @click.stop="toggleActionMenu(source, $event)"
                >
                  <MoreHorizontal :size="18" :stroke-width="2" aria-hidden="true" />
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>

  <Teleport to="body">
    <div
      v-if="openActionMenuSource"
      ref="actionMenuElement"
      class="data-source-row-action-popover"
      role="menu"
      :aria-label="`更多操作：${openActionMenuSource.displayName}`"
      :style="{ top: `${actionMenuPosition.top}px`, left: `${actionMenuPosition.left}px` }"
      @pointerdown.stop
    >
      <button type="button" role="menuitem" :disabled="actionID === openActionMenuSource.id" @click="requestAction(openActionMenuSource, 'toggle')">
        <Power :size="14" aria-hidden="true" />
        {{ openActionMenuSource.state === 'ENABLED' ? '禁用' : '启用' }}
      </button>
      <button type="button" role="menuitem" class="is-danger" :disabled="actionID === openActionMenuSource.id" @click="requestAction(openActionMenuSource, 'archive')">
        <Archive :size="14" aria-hidden="true" />
        删除 / 归档
      </button>
    </div>
  </Teleport>

  <DataSourceEditDrawer v-model="drawerVisible" :data-source-id="drawerSourceID" :focus-test="drawerFocusTest" @saved="onDrawerSaved" />
  <DataSourceConfirmDialog
    :open="Boolean(pendingAction)"
    :title="confirmTitle"
    :description="confirmDescription"
    :object-name="pendingAction?.source.displayName"
    :impacts="confirmImpacts"
    :confirm-label="confirmLabel"
    :danger="pendingAction?.action === 'archive'"
    :busy="Boolean(actionID)"
    @cancel="pendingAction = undefined"
    @confirm="confirmPendingAction"
  />
</template>

<style scoped>
.data-source-page {
  color: var(--color-text-primary);
}

.data-source-page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-6);
  margin-bottom: var(--space-6);
}

.data-source-page-header h1 {
  margin: 0;
  font-size: 22px;
  font-weight: 600;
  letter-spacing: -.01em;
  line-height: 30px;
}

.data-source-page-header p {
  margin: var(--space-1) 0 0;
  color: var(--color-text-secondary);
  font-size: 13px;
  line-height: 20px;
}

.row-action:focus-visible,
.row-action-more-button:focus-visible {
  outline: 2px solid rgb(37 103 185 / 30%);
  outline-offset: 2px;
}

.visually-hidden {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

.data-source-filter {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
  padding: 0 0 var(--space-4);
  border-bottom: 1px solid var(--color-border-default);
}

.data-source-filter label {
  position: relative;
  min-width: 128px;
}

.data-source-filter .filter-keyword {
  min-width: 240px;
  flex: 0 1 300px;
  max-width: 340px;
}

.filter-search-icon {
  position: absolute;
  top: 10px;
  left: 10px;
  z-index: 1;
  color: var(--color-text-tertiary);
  pointer-events: none;
}

.data-source-filter input,
.data-source-filter select {
  width: 100%;
  height: var(--size-control);
  padding: 0 10px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-control);
  color: #2b3037;
  background: var(--color-bg-surface);
  font-size: 13px;
}

.data-source-filter .filter-keyword input { padding-left: 32px; }

.data-source-filter input:focus,
.data-source-filter select:focus {
  border-color: var(--color-primary);
  outline: 2px solid rgb(37 103 185 / 14%);
  outline-offset: 0;
}

.filter-grow {
  flex: 1 1 0;
}

.data-source-feedback {
  margin: var(--space-3) 0 0;
  padding: var(--space-2) var(--space-3);
  border-left: 3px solid #8fb0dc;
  color: #526174;
  background: #f4f7fb;
  font-size: 13px;
  line-height: 20px;
}

.data-source-feedback.is-error {
  border-left-color: #d45a52;
  color: #9f2f28;
  background: #fff5f4;
}

.data-source-table-region {
  margin-top: var(--space-6);
}

.data-source-table-meta {
  display: flex;
  align-items: baseline;
  justify-content: flex-start;
  gap: var(--space-2);
  padding: 0 0 var(--space-2);
}

.data-source-table-meta h2 {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  line-height: 22px;
}

.data-source-table-meta span {
  display: inline-grid;
  min-width: 22px;
  height: 20px;
  place-items: center;
  padding-inline: var(--space-1);
  border-radius: var(--radius-control);
  color: var(--color-text-secondary);
  background: var(--color-bg-subtle);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}

.data-source-table-scroll {
  overflow-x: auto;
  border-top: 1px solid var(--color-border-strong);
  border-bottom: 1px solid var(--color-border-strong);
  background: var(--color-bg-surface);
}

.data-source-table-scroll table {
  width: 100%;
  min-width: 1020px;
  border-collapse: collapse;
  table-layout: fixed;
}

.data-source-table-scroll th,
.data-source-table-scroll td {
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid #e6e9ed;
  text-align: left;
  vertical-align: middle;
}

.data-source-table-scroll th {
  height: var(--size-table-header);
  color: #59616b;
  background: #f4f5f6;
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
}

.data-source-table-scroll td {
  height: var(--size-table-row);
  padding-block: var(--space-1);
  color: #363c44;
  font-size: 13px;
  line-height: 20px;
}

.data-source-table-scroll tbody tr:hover td {
  background: #f7f8f9;
}

.data-source-table-scroll tbody tr:last-child td {
  border-bottom: 0;
}

.column-name { width: 176px; }
.column-environment { width: 64px; }
.column-host { width: 156px; }
.column-port { width: 62px; }
.column-tenant { width: 116px; }
.column-mode { width: 86px; }
.column-connection { width: 132px; }
.column-state { width: 84px; }
.column-actions { width: 132px; }

.column-heading-with-help { display: inline-flex; align-items: center; gap: var(--space-1); }
.column-heading-with-help svg { color: var(--color-text-tertiary); }

.cell-stack {
  display: grid;
  align-content: center;
  gap: 1px;
}

.data-source-name strong {
  overflow: hidden;
  color: #1e2329;
  font-size: 14px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.data-source-name small,
.data-source-connection small {
  overflow: hidden;
  color: var(--color-text-tertiary);
  font-size: 11px;
  line-height: 17px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.data-source-host,
.data-source-tenant {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.data-source-port {
  font-variant-numeric: tabular-nums;
}

.environment-label {
  display: inline-flex;
  width: fit-content;
  align-items: center;
  min-height: 20px;
  padding: 1px 6px;
  border: 1px solid var(--color-border-default);
  border-radius: 3px;
  color: var(--color-text-secondary);
  background: var(--color-bg-subtle);
  font-size: 12px;
  font-weight: 500;
  line-height: 18px;
  white-space: nowrap;
}

.sort-header {
  display: inline-flex;
  width: 100%;
  height: 30px;
  align-items: center;
  justify-content: flex-start;
  gap: var(--space-1);
  margin: -4px -6px;
  padding: 0 6px;
  border: 0;
  border-radius: 3px;
  color: inherit;
  background: transparent;
  font-family: inherit;
  font-size: inherit;
  font-weight: inherit;
  cursor: pointer;
}

.sort-header:hover { color: #2b3037; background: #e9ecef; }
.sort-header:focus-visible { outline: 2px solid rgb(37 103 185 / 30%); outline-offset: 0; }
.sort-header > svg { flex: none; color: #7b838d; }

.environment-label.is-production {
  border-color: #e4cfac;
  color: #80571e;
  background: #fff9ee;
}

.environment-label.is-development {
  border-color: #d8dde2;
  color: #59616b;
  background: #f4f5f6;
}

.environment-label.is-test {
  border-color: #c8d8e9;
  color: #37648d;
  background: #f1f6fb;
}

.environment-label.is-staging {
  border-color: #dfd1b5;
  color: #775b24;
  background: #faf7ef;
}

.data-source-row-actions {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-1);
  white-space: nowrap;
}

.row-action,
.row-action-more-button {
  height: 30px;
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: 0 6px;
  border: 0;
  border-radius: 4px;
  color: #245d9e;
  background: transparent;
  font-family: inherit;
  font-size: 12px;
  font-weight: 500;
  line-height: 30px;
  cursor: pointer;
}

.row-action-more-button {
  width: 30px;
  justify-content: center;
  padding: 0;
}

.row-action:hover:not(:disabled),
.row-action-more-button:hover:not(:disabled) {
  background: var(--color-primary-soft);
}

.row-action:focus-visible,
.row-action-more-button:focus-visible {
  outline: 2px solid rgb(37 103 185 / 28%);
  outline-offset: 1px;
}

.row-action:disabled,
.row-action-more-button:disabled {
  color: #99a3b0;
  cursor: not-allowed;
}

.data-source-row-action-popover {
  position: fixed;
  z-index: 90;
  display: grid;
  width: 132px;
  padding: 4px;
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-surface);
  background: var(--color-bg-surface);
  box-shadow: 0 8px 20px rgb(20 27 35 / 14%);
}

.data-source-row-action-popover button {
  display: flex;
  height: 32px;
  align-items: center;
  gap: var(--space-2);
  padding: 0 9px;
  border: 0;
  border-radius: 3px;
  color: #344256;
  background: transparent;
  text-align: left;
  font-family: inherit;
  font-size: 13px;
  font-weight: 500;
  line-height: 1;
  cursor: pointer;
}

.data-source-row-action-popover button:hover:not(:disabled) {
  background: #f2f5f8;
}

.data-source-row-action-popover button:focus-visible {
  outline: 2px solid #2563c9;
  outline-offset: -2px;
}

.data-source-row-action-popover button.is-danger {
  color: #b42318;
}

.data-source-table-message {
  height: 160px !important;
  color: var(--color-text-secondary) !important;
  text-align: center !important;
}

.data-source-table-message strong,
.data-source-table-message span {
  display: block;
}

.data-source-table-message strong {
  margin-bottom: 4px;
  color: #2b394d;
  font-size: 14px;
}

.data-source-table-message span {
  margin-bottom: 12px;
  color: var(--color-text-tertiary);
  font-size: 12px;
}

.data-source-table-message.is-error strong {
  color: #a6352c;
}

@media (min-width: 1800px) {
  .column-name { width: 16%; }
  .column-environment { width: 7%; }
  .column-host { width: 15%; }
  .column-port { width: 7%; }
  .column-tenant { width: 12%; }
  .column-mode { width: 9%; }
  .column-connection { width: 14%; }
  .column-state { width: 9%; }
  .column-actions { width: 9%; }
}

@media (max-width: 1180px) {
  .data-source-filter .filter-grow {
    display: none;
  }

  .data-source-table-scroll table {
    min-width: 1020px;
  }
}

@media (max-width: 760px) {
  .data-source-page-header {
    align-items: stretch;
    flex-direction: column;
    gap: 14px;
  }

  .data-source-page-header :deep(.workbench-button) {
    align-self: flex-start;
  }

  .data-source-filter label,
  .data-source-filter .filter-keyword {
    min-width: calc(50% - 6px);
    max-width: none;
    flex: 1 1 calc(50% - 6px);
  }

  .data-source-filter > :deep(.workbench-button) {
    flex: 0 0 auto;
  }
}

@media (max-width: 520px) {
  .data-source-filter label,
  .data-source-filter .filter-keyword {
    min-width: 100%;
  }
}
</style>
