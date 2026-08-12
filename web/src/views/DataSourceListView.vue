<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { browserApi, dataSourceErrorMessage, type DataSourceSummary } from '@/api/browser'
import DataSourceConfirmDialog from './DataSourceConfirmDialog.vue'
import DataSourceEditDrawer from './DataSourceEditDrawer.vue'
import { dataSourceDeletionNotice } from './dataSourceDeletionNotice'
import { filterDataSources, type ConnectionStatusFilter } from './dataSourceListFilters'

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

const filteredSources = computed(() => filterDataSources(sources.value, {
  keyword: keyword.value,
  environment: environment.value,
  compatibilityMode: compatibilityMode.value,
  connectionStatus: connectionStatus.value,
  state: state.value,
}))
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
  if (source.lastTestStatus === 'SUCCEEDED') return 'is-success'
  if (source.lastTestStatus === 'FAILED') return 'is-danger'
  if (source.lastTestStatus === 'PENDING') return 'is-info'
  if (source.lastTestStatus === 'EXPIRED' || source.lastTestStatus === 'INVALIDATED') return 'is-warning'
  return 'is-neutral'
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
      <button type="button" class="data-source-button is-primary" @click="openCreateDrawer">新增数据源</button>
    </header>

    <p class="data-source-boundary-note">连接状态只表示网络、认证和基础数据库连接，不代表导入导出权限、对象权限、性能或任务一定可执行。</p>

    <section class="data-source-filter" aria-label="数据源筛选">
      <label class="filter-keyword"><span>关键字</span><input v-model.trim="keyword" placeholder="名称或 IP / 域名" /></label>
      <label><span>环境</span><select v-model="environment"><option value="">全部环境</option><option value="DEVELOPMENT">开发</option><option value="TEST">测试</option><option value="STAGING">预生产</option><option value="PRODUCTION">生产</option></select></label>
      <label><span>租户模式</span><select v-model="compatibilityMode"><option value="">全部模式</option><option value="MYSQL">MySQL</option><option value="ORACLE">Oracle</option><option value="UNKNOWN">待迁移类型</option></select></label>
      <label><span>连接状态</span><select v-model="connectionStatus"><option value="">全部状态</option><option value="UNTESTED">未测试</option><option value="SUCCEEDED">可连接</option><option value="FAILED">连接失败</option><option value="UNKNOWN">结果未知</option><option value="EXPIRED">测试已过期</option><option value="INVALIDATED">测试已失效</option><option value="PENDING">测试已请求</option><option value="UNAVAILABLE">测试不可用</option></select></label>
      <label><span>启用状态</span><select v-model="state"><option value="">全部状态</option><option value="ENABLED">已启用</option><option value="DISABLED">已禁用</option></select></label>
      <button v-if="hasActiveFilters" type="button" class="data-source-button is-text" @click="resetFilters">重置</button>
      <span class="filter-grow" />
      <button type="button" class="data-source-button is-secondary" :disabled="loading" @click="loadSources">{{ loading ? '刷新中…' : '刷新' }}</button>
    </section>

    <p v-if="feedback" class="data-source-feedback is-error" role="alert">{{ feedback }}</p>
    <p v-else-if="notice" class="data-source-feedback is-notice" role="status">{{ notice }}</p>

    <section class="data-source-table-region" aria-labelledby="data-source-table-title">
      <div class="data-source-table-meta">
        <h2 id="data-source-table-title">数据源</h2>
        <span v-if="!loading && !loadFailure">当前显示 {{ filteredSources.length }} 条</span>
      </div>

      <div class="data-source-table-scroll">
        <table>
          <thead>
            <tr>
              <th class="column-name">数据源</th>
              <th class="column-environment">环境</th>
              <th class="column-host">IP/域名</th>
              <th class="column-port">端口</th>
              <th class="column-tenant">租户名</th>
              <th class="column-mode">租户模式</th>
              <th class="column-connection">连接状态</th>
              <th class="column-state">启用状态</th>
              <th class="column-actions">操作</th>
            </tr>
          </thead>
          <tbody v-if="loading">
            <tr><td colspan="9" class="data-source-table-message">正在加载已授权数据源…</td></tr>
          </tbody>
          <tbody v-else-if="loadFailure">
            <tr><td colspan="9" class="data-source-table-message is-error"><strong>无法加载数据源</strong><span>{{ loadFailure }}</span><button type="button" class="data-source-button is-secondary" @click="loadSources">重试</button></td></tr>
          </tbody>
          <tbody v-else-if="sources.length === 0">
            <tr><td colspan="9" class="data-source-table-message"><strong>暂无数据源</strong><span>新增数据源后，可以在任务配置中选择并使用。</span></td></tr>
          </tbody>
          <tbody v-else-if="filteredSources.length === 0">
            <tr><td colspan="9" class="data-source-table-message"><strong>没有符合当前筛选条件的数据源</strong><span>请调整筛选条件，或清除当前筛选。</span><button type="button" class="data-source-button is-text" @click="resetFilters">清除筛选</button></td></tr>
          </tbody>
          <tbody v-else>
            <tr v-for="source in filteredSources" :key="source.id">
              <td class="data-source-name"><strong>{{ source.displayName }}</strong><small>{{ source.id }}</small></td>
              <td><span class="environment-tag" :class="`is-${source.environment.toLowerCase()}`">{{ environmentLabel(source.environment) }}</span></td>
              <td class="data-source-host" :title="source.host">{{ source.host }}</td>
              <td class="data-source-port">{{ source.port }}</td>
              <td class="data-source-tenant" :title="source.tenantName">{{ source.tenantName }}</td>
              <td>{{ compatibilityModeLabel(source.compatibilityMode) }}</td>
              <td class="data-source-connection"><span class="status-badge" :class="connectionStatusTone(source)">{{ connectionStatusLabel(source) }}</span><small>{{ lastTestTimeLabel(source) }}</small></td>
              <td><span class="status-badge is-state" :class="source.state === 'ENABLED' ? 'is-enabled' : 'is-disabled'">{{ stateLabel(source) }}</span></td>
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
                  更多
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
        {{ openActionMenuSource.state === 'ENABLED' ? '禁用' : '启用' }}
      </button>
      <button type="button" role="menuitem" class="is-danger" :disabled="actionID === openActionMenuSource.id" @click="requestAction(openActionMenuSource, 'archive')">
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
  --ds-border: #dde3ea;
  --ds-border-strong: #c9d2dd;
  --ds-text: #1f2937;
  --ds-text-secondary: #526174;
  --ds-text-tertiary: #6b778a;
  --ds-primary: #2563c9;
  color: var(--ds-text);
}

.data-source-page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 20px;
}

.data-source-page-header h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
  line-height: 32px;
}

.data-source-page-header p {
  margin: 5px 0 0;
  color: var(--ds-text-secondary);
  font-size: 14px;
  line-height: 22px;
}

.data-source-button {
  height: 36px;
  padding: 0 14px;
  border: 1px solid var(--ds-border-strong);
  border-radius: 4px;
  color: #344256;
  background: #fff;
  font: 500 13px/1 "Segoe UI", "Microsoft YaHei UI", sans-serif;
  white-space: nowrap;
  cursor: pointer;
}

.data-source-button:hover:not(:disabled) {
  border-color: #9aa8ba;
  background: #f7f9fc;
}

.data-source-button.is-primary {
  border-color: var(--ds-primary);
  color: #fff;
  background: var(--ds-primary);
}

.data-source-button.is-primary:hover:not(:disabled) {
  border-color: #1f54ad;
  background: #1f54ad;
}

.data-source-button.is-text {
  padding-inline: 8px;
  border-color: transparent;
  color: #315f9f;
  background: transparent;
}

.data-source-button:focus-visible,
.row-action:focus-visible,
.row-action-more-button:focus-visible {
  outline: 2px solid var(--ds-primary);
  outline-offset: 2px;
}

.data-source-button:disabled {
  color: #8a95a5;
  border-color: #dce2e9;
  background: #f1f3f6;
  cursor: not-allowed;
}

.data-source-boundary-note {
  margin: 0 0 20px;
  padding: 7px 12px;
  border-left: 3px solid #8fb0dc;
  color: #526b88;
  background: #f4f7fb;
  font-size: 12px;
  line-height: 20px;
}

.data-source-filter {
  display: flex;
  flex-wrap: wrap;
  align-items: end;
  gap: 10px 12px;
  padding: 0 0 18px;
  border-bottom: 1px solid var(--ds-border);
}

.data-source-filter label {
  display: grid;
  gap: 5px;
  min-width: 128px;
}

.data-source-filter label > span {
  color: var(--ds-text-tertiary);
  font-size: 12px;
  font-weight: 500;
  line-height: 18px;
}

.data-source-filter .filter-keyword {
  min-width: 220px;
  flex: 1 1 220px;
  max-width: 280px;
}

.data-source-filter input,
.data-source-filter select {
  width: 100%;
  height: 36px;
  padding: 0 10px;
  border: 1px solid var(--ds-border-strong);
  border-radius: 4px;
  color: #273548;
  background: #fff;
  font: 400 13px/1 "Segoe UI", "Microsoft YaHei UI", sans-serif;
}

.data-source-filter input:focus,
.data-source-filter select:focus {
  border-color: var(--ds-primary);
  outline: 2px solid rgb(37 99 201 / 14%);
  outline-offset: 0;
}

.filter-grow {
  flex: 1 1 0;
}

.data-source-feedback {
  margin: 14px 0 0;
  padding: 8px 11px;
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
  margin-top: 22px;
}

.data-source-table-meta {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
  padding: 0 2px 10px;
}

.data-source-table-meta h2 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
}

.data-source-table-meta span {
  color: var(--ds-text-tertiary);
  font-size: 12px;
}

.data-source-table-scroll {
  overflow-x: auto;
  border-top: 1px solid var(--ds-border-strong);
  border-bottom: 1px solid var(--ds-border);
  background: #fff;
}

.data-source-table-scroll table {
  width: 100%;
  min-width: 990px;
  border-collapse: collapse;
  table-layout: fixed;
}

.data-source-table-scroll th,
.data-source-table-scroll td {
  padding: 9px 10px;
  border-bottom: 1px solid #e5e9ef;
  text-align: left;
  vertical-align: middle;
}

.data-source-table-scroll th {
  height: 40px;
  color: #566578;
  background: #f7f9fc;
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
}

.data-source-table-scroll td {
  height: 56px;
  color: #344256;
  font-size: 13px;
  line-height: 20px;
}

.data-source-table-scroll tbody tr:hover td {
  background: #fafbfd;
}

.data-source-table-scroll tbody tr:last-child td {
  border-bottom: 0;
}

.column-name { width: 150px; }
.column-environment { width: 64px; }
.column-host { width: 142px; }
.column-port { width: 62px; }
.column-tenant { width: 106px; }
.column-mode { width: 86px; }
.column-connection { width: 132px; }
.column-state { width: 84px; }
.column-actions { width: 160px; }

.data-source-name,
.data-source-connection {
  display: grid;
  align-content: center;
  gap: 1px;
}

.data-source-name strong {
  overflow: hidden;
  color: #243247;
  font-size: 14px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.data-source-name small,
.data-source-connection small {
  overflow: hidden;
  color: var(--ds-text-tertiary);
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

.environment-tag,
.status-badge {
  display: inline-flex;
  width: fit-content;
  align-items: center;
  min-height: 22px;
  padding: 1px 7px;
  border: 1px solid #d4dce6;
  border-radius: 4px;
  color: #526174;
  background: #f4f6f8;
  font-size: 12px;
  line-height: 18px;
  white-space: nowrap;
}

.environment-tag.is-production {
  border-color: #e4c39a;
  color: #86531c;
  background: #fff8eb;
}

.environment-tag.is-staging {
  border-color: #c8d6e8;
  color: #405f84;
  background: #f2f6fb;
}

.status-badge.is-success {
  border-color: #b9dec9;
  color: #19734a;
  background: #edf8f2;
}

.status-badge.is-danger {
  border-color: #e7b8b3;
  color: #b42318;
  background: #fff2f0;
}

.status-badge.is-warning {
  border-color: #e8d1a2;
  color: #8a5a12;
  background: #fff7e6;
}

.status-badge.is-info {
  border-color: #b9ceeb;
  color: #245da8;
  background: #eef4fc;
}

.status-badge.is-enabled {
  border-color: #bfcddd;
  color: #3a5575;
  background: #f2f6fa;
}

.status-badge.is-disabled {
  color: #687588;
  background: #f4f5f7;
}

.data-source-row-actions {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
  white-space: nowrap;
}

.row-action,
.row-action-more-button {
  height: 30px;
  padding: 0 6px;
  border: 0;
  border-radius: 4px;
  color: #315f9f;
  background: transparent;
  font: 500 12px/30px "Segoe UI", "Microsoft YaHei UI", sans-serif;
  cursor: pointer;
}

.row-action:hover:not(:disabled),
.row-action-more-button:hover:not(:disabled) {
  background: #edf3fa;
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
  border: 1px solid #dde3ea;
  border-radius: 4px;
  background: #fff;
  box-shadow: 0 8px 20px rgb(15 23 42 / 14%);
}

.data-source-row-action-popover button {
  height: 32px;
  padding: 0 9px;
  border: 0;
  border-radius: 3px;
  color: #344256;
  background: transparent;
  text-align: left;
  font: 400 13px/1 "Segoe UI", "Microsoft YaHei UI", sans-serif;
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
  color: var(--ds-text-secondary) !important;
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
  color: var(--ds-text-tertiary);
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
  .column-actions { width: 11%; }
}

@media (max-width: 1180px) {
  .data-source-filter .filter-grow {
    display: none;
  }

  .data-source-table-scroll table {
    min-width: 960px;
  }
}

@media (max-width: 760px) {
  .data-source-page-header {
    align-items: stretch;
    flex-direction: column;
    gap: 14px;
  }

  .data-source-page-header .data-source-button {
    align-self: flex-start;
  }

  .data-source-filter label,
  .data-source-filter .filter-keyword {
    min-width: calc(50% - 6px);
    max-width: none;
    flex: 1 1 calc(50% - 6px);
  }

  .data-source-filter > .data-source-button {
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
