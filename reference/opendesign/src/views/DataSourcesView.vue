<script setup lang="ts">
import { computed, ref } from 'vue'
import { connectionLabels, environmentLabels, fixtureDataSources, fixtureNodes } from '../data/fixtures'
import type { ConnectionStatus, DataSource, SummaryMetric, Tone } from '../types'
import AppIcon from '../components/AppIcon.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import DemoNote from '../components/DemoNote.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import StatusSummary from '../components/StatusSummary.vue'

type SortKey = 'name' | 'testedAt'
type ConfirmKind = 'toggle' | 'archive' | 'discard'

interface PendingConfirm {
  kind: ConfirmKind
  sourceId?: string
}

const sources = ref<DataSource[]>(fixtureDataSources.map((item) => ({ ...item })))
const keyword = ref('')
const environment = ref('')
const mode = ref('')
const connection = ref('')
const state = ref('')
const sortKey = ref<SortKey>('name')
const sortDirection = ref<'asc' | 'desc'>('asc')
const menuId = ref<string | null>(null)
const drawerOpen = ref(false)
const drawerDirty = ref(false)
const editingId = ref<string | null>(null)
const testing = ref(false)
const formError = ref('')
const feedback = ref('')
const pendingConfirm = ref<PendingConfirm | null>(null)
const environmentOptions: Array<[DataSource['environment'], string]> = [
  ['DEVELOPMENT', '开发'],
  ['TEST', '测试'],
  ['STAGING', '预生产'],
  ['PRODUCTION', '生产'],
]

function newSource(): DataSource {
  return { id: '', name: '', environment: 'DEVELOPMENT', mode: 'MYSQL', host: '', port: 2881, cluster: '', tenant: '', database: '', credential: 'UNAVAILABLE', state: 'ENABLED', testStatus: 'UNTESTED', testedAt: '' }
}

const draft = ref<DataSource>(newSource())
const visibleSources = computed(() => {
  const term = keyword.value.trim().toLowerCase()
  const filtered = sources.value.filter((source) => {
    const haystack = `${source.name} ${source.host} ${source.cluster} ${source.tenant}`.toLowerCase()
    return (!term || haystack.includes(term)) && (!environment.value || source.environment === environment.value) && (!mode.value || source.mode === mode.value) && (!connection.value || source.testStatus === connection.value) && (!state.value || source.state === state.value)
  })
  return [...filtered].sort((left, right) => {
    const leftValue = sortKey.value === 'name' ? left.name : left.testedAt
    const rightValue = sortKey.value === 'name' ? right.name : right.testedAt
    const comparison = leftValue.localeCompare(rightValue, 'zh-CN')
    return sortDirection.value === 'asc' ? comparison : -comparison
  })
})
const metrics = computed<SummaryMetric[]>(() => {
  const total = sources.value.length
  const count = (predicate: (source: DataSource) => boolean) => sources.value.filter(predicate).length
  const percentage = (value: number) => total ? Math.round((value / total) * 100) : 0
  const connected = count((source) => source.testStatus === 'SUCCEEDED')
  const pending = count((source) => source.testStatus !== 'SUCCEEDED')
  const disabled = count((source) => source.state === 'DISABLED')
  return [
    { key: 'registered', label: '已登记', value: total, note: '当前授权范围内的数据源', progress: total ? 100 : 0, tone: 'neutral' },
    { key: 'connected', label: '连接正常', value: connected, note: '最近连接测试已通过', progress: percentage(connected), tone: 'success' },
    { key: 'pending', label: '测试待处理', value: pending, note: '失败、失效或尚未测试', progress: percentage(pending), tone: 'warning' },
    { key: 'disabled', label: '已禁用', value: disabled, note: '不会进入新任务候选项', progress: percentage(disabled), tone: 'neutral' },
  ]
})
const menuSource = computed(() => sources.value.find((source) => source.id === menuId.value) ?? null)
const confirmTitle = computed(() => pendingConfirm.value?.kind === 'discard' ? '放弃未保存的修改？' : pendingConfirm.value?.kind === 'archive' ? '确认删除或归档数据源？' : '确认切换数据源状态？')
const confirmDescription = computed(() => {
  if (pendingConfirm.value?.kind === 'discard') return '关闭抽屉将放弃本地输入，不会修改任何数据源。'
  const source = sources.value.find((item) => item.id === pendingConfirm.value?.sourceId)
  if (pendingConfirm.value?.kind === 'archive') return `服务端应根据引用关系决定 ${source?.name ?? '此数据源'} 的删除或归档结果；本地原型不会发送请求。`
  return `${source?.state === 'ENABLED' ? '禁用' : '启用'}后，最终可用性仍应以服务端的权限、连接测试和任务校验为准。`
})
const confirmImpacts = computed(() => pendingConfirm.value?.kind === 'toggle' ? ['不会取消任何已运行任务。', '本地原型只会更新当前列表的演示状态。'] : pendingConfirm.value?.kind === 'archive' ? ['不会删除本地文件、凭据或真实数据库对象。', '不会向后端发送删除或归档请求。'] : ['草稿测试结果与输入会被清除。'])

function toneForConnection(status: ConnectionStatus): Tone { return connectionLabels[status][1] as Tone }
function labelForConnection(status: ConnectionStatus): string { return connectionLabels[status][0] }
function resetFilters(): void { keyword.value = ''; environment.value = ''; mode.value = ''; connection.value = ''; state.value = '' }
function sortBy(key: SortKey): void { sortDirection.value = sortKey.value === key && sortDirection.value === 'asc' ? 'desc' : 'asc'; sortKey.value = key }
function openDrawer(source?: DataSource): void { editingId.value = source?.id ?? null; draft.value = source ? { ...source } : newSource(); drawerDirty.value = false; formError.value = ''; feedback.value = ''; menuId.value = null; drawerOpen.value = true }
function requestCloseDrawer(): void { if (drawerDirty.value) { pendingConfirm.value = { kind: 'discard' }; return }; drawerOpen.value = false }
function validate(): boolean { if (!draft.value.name.trim() || !draft.value.host.trim() || !draft.value.cluster.trim() || !draft.value.tenant.trim() || !Number.isInteger(draft.value.port) || draft.value.port < 1 || draft.value.port > 65535) { formError.value = '请填写名称、主机、集群、租户和有效端口后再继续。'; return false }; formError.value = ''; return true }
function testConnection(): void { if (!validate()) return; testing.value = true; window.setTimeout(() => { draft.value.testStatus = 'SUCCEEDED'; draft.value.testedAt = '2026-08-24 10:36'; testing.value = false; feedback.value = '连接测试已通过（仅安全 Fixture 演示，未建立真实连接）。' }, 520) }
function save(): void { if (!validate()) return; const source = { ...draft.value, id: editingId.value ?? `ui-fixture-data-source-${String(sources.value.length + 1).padStart(2, '0')}` }; const index = sources.value.findIndex((item) => item.id === source.id); if (index >= 0) sources.value[index] = source; else sources.value.push(source); drawerDirty.value = false; drawerOpen.value = false; feedback.value = editingId.value ? '数据源已更新为本地演示状态。' : '数据源已登记为本地演示状态。' }
function requestAction(kind: 'toggle' | 'archive', sourceId: string): void { pendingConfirm.value = { kind, sourceId }; menuId.value = null }
function confirm(): void { const pending = pendingConfirm.value; if (!pending) return; if (pending.kind === 'discard') { drawerDirty.value = false; drawerOpen.value = false } else { const source = sources.value.find((item) => item.id === pending.sourceId); if (source && pending.kind === 'toggle') { source.state = source.state === 'ENABLED' ? 'DISABLED' : 'ENABLED'; feedback.value = `${source.name} 已在本地原型中${source.state === 'ENABLED' ? '启用' : '禁用'}。` } if (source && pending.kind === 'archive') { sources.value = sources.value.filter((item) => item.id !== source.id); feedback.value = `${source.name} 已从本地演示列表移除。` } } pendingConfirm.value = null }
</script>

<template>
  <div class="page page-wide" :aria-busy="testing">
    <PageHeader title="数据源管理" description="管理任务可选择的数据源，并区分环境、启用状态与连接测试事实。" data-id="data-source-title"><template #actions><button class="btn btn-primary" type="button" data-od-id="new-data-source-cta" @click="openDrawer()"><AppIcon name="plus" />新增数据源</button></template></PageHeader>
    <DemoNote text="当前使用仓库内的安全 UI Fixture；不会连接数据库、保存凭据或执行真实启停与删除。" />
    <p v-if="feedback" class="inline-feedback" role="status">{{ feedback }}</p>
    <StatusSummary :metrics="metrics" label="数据源状态摘要" />
    <section class="workspace-section"><div class="filter-bar data-source-filters"><label class="filter-field is-search"><span>搜索</span><AppIcon name="search" /><input v-model="keyword" placeholder="名称、主机、集群或租户" /></label><label class="filter-field"><span>环境</span><select v-model="environment"><option value="">全部环境</option><option v-for="[key, label] in environmentOptions" :key="key" :value="key">{{ label }}</option></select></label><label class="filter-field"><span>兼容模式</span><select v-model="mode"><option value="">全部模式</option><option value="MYSQL">MySQL</option><option value="ORACLE">Oracle</option></select></label><label class="filter-field"><span>连接状态</span><select v-model="connection"><option value="">全部状态</option><option v-for="([label], key) in connectionLabels" :key="key" :value="key">{{ label }}</option></select></label><label class="filter-field"><span>启用状态</span><select v-model="state"><option value="">全部状态</option><option value="ENABLED">已启用</option><option value="DISABLED">已禁用</option></select></label><button class="btn" type="button" @click="resetFilters">重置筛选</button></div>
      <div class="table-result-head"><div><h2>数据源</h2><span aria-live="polite">{{ visibleSources.length }} 项安全样本</span></div><p>环境、连接结果与启用状态是三类独立事实。</p></div>
      <div class="table-wrap data-source-table"><table><caption class="sr-only">数据源安全样本列表</caption><thead><tr><th :aria-sort="sortKey === 'name' ? (sortDirection === 'asc' ? 'ascending' : 'descending') : 'none'"><button class="sort-button" type="button" @click="sortBy('name')">名称<AppIcon name="chevron-down" /></button></th><th>环境</th><th>模式</th><th>连接地址</th><th class="source-col-credential">系统租户</th><th>连接状态</th><th>启用状态</th><th :aria-sort="sortKey === 'testedAt' ? (sortDirection === 'asc' ? 'ascending' : 'descending') : 'none'"><button class="sort-button" type="button" @click="sortBy('testedAt')">最近测试<AppIcon name="chevron-down" /></button></th><th><span class="sr-only">操作</span></th></tr></thead><tbody><tr v-for="source in visibleSources" :key="source.id"><td data-label="名称"><span class="cell-primary truncate" :title="source.name">{{ source.name }}</span><span class="cell-meta mono">{{ source.id }}</span></td><td data-label="环境"><span class="tag" :class="{ 'is-prod': source.environment === 'PRODUCTION' }">{{ environmentLabels[source.environment] }}</span></td><td data-label="模式">{{ source.mode === 'MYSQL' ? 'MySQL' : 'Oracle' }}</td><td data-label="连接地址"><span class="connection-address">{{ source.host }}:{{ source.port }}</span><span class="cell-meta truncate" :title="`${source.cluster} / ${source.tenant}`">{{ source.cluster }} / {{ source.tenant }}</span></td><td class="source-col-credential" data-label="系统租户"><span class="tag">{{ source.credential === 'AVAILABLE' ? '已配置' : '未配置' }}</span></td><td data-label="连接状态"><StatusBadge :label="labelForConnection(source.testStatus)" :tone="toneForConnection(source.testStatus)" /></td><td data-label="启用状态"><StatusBadge :label="source.state === 'ENABLED' ? '已启用' : '已禁用'" :tone="source.state === 'ENABLED' ? 'success' : 'neutral'" /></td><td data-label="最近测试"><span class="mono">{{ source.testedAt || '尚未测试' }}</span></td><td class="source-actions" data-label="操作"><button class="table-action-button" type="button" :aria-label="`管理 ${source.name}`" :aria-expanded="menuId === source.id" @click="menuId = menuId === source.id ? null : source.id"><AppIcon name="more" /></button></td></tr></tbody></table><div v-if="!visibleSources.length" class="empty-inline" role="status">没有符合当前筛选条件的数据源。重置筛选后可查看全部安全样本。</div></div>
      <div v-if="menuSource" class="action-menu-inline" role="menu"><button type="button" role="menuitem" @click="openDrawer(menuSource)">编辑数据源</button><button type="button" role="menuitem" @click="requestAction('toggle', menuSource.id)">{{ menuSource.state === 'ENABLED' ? '禁用数据源' : '启用数据源' }}</button><button class="is-danger" type="button" role="menuitem" @click="requestAction('archive', menuSource.id)">删除或归档</button></div>
    </section>
    <div v-if="drawerOpen" class="drawer-backdrop" @click.self="requestCloseDrawer"><aside class="drawer" role="dialog" aria-modal="true" aria-label="数据源编辑抽屉" @keydown.esc="requestCloseDrawer"><header class="drawer-head"><div><h2>{{ editingId ? '编辑数据源' : '新增数据源' }}</h2><p>本地 Fixture 表单仅演示校验、测试失效和保存反馈。</p></div><button class="icon-button" type="button" aria-label="关闭数据源抽屉" @click="requestCloseDrawer"><AppIcon name="x" /></button></header><form class="drawer-body" @submit.prevent="save"><section class="form-section"><div class="form-section-head"><h3>基本信息</h3><span>控制面字段</span></div><div class="field-grid"><label class="field span-2"><span>数据源名称 *</span><input v-model="draft.name" required maxlength="200" @input="drawerDirty = true" /><small>用于任务选择和审计记录。</small></label><label class="field"><span>环境 *</span><select v-model="draft.environment" @change="drawerDirty = true"><option v-for="[key, label] in environmentOptions" :key="key" :value="key">{{ label }}</option></select></label><label class="field"><span>兼容模式 *</span><select v-model="draft.mode" @change="drawerDirty = true"><option value="MYSQL">MySQL</option><option value="ORACLE">Oracle</option></select></label></div></section><section class="form-section"><div class="form-section-head"><h3>连接信息</h3><span>修改会使测试失效</span></div><div class="field-grid"><label class="field"><span>主机 *</span><input v-model="draft.host" required placeholder="例如 fixture-db.test" @input="drawerDirty = true; draft.testStatus = 'INVALIDATED'" /></label><label class="field"><span>端口 *</span><input v-model.number="draft.port" required type="number" min="1" max="65535" @input="drawerDirty = true; draft.testStatus = 'INVALIDATED'" /></label><label class="field"><span>集群 *</span><input v-model="draft.cluster" required @input="drawerDirty = true; draft.testStatus = 'INVALIDATED'" /></label><label class="field"><span>租户 *</span><input v-model="draft.tenant" required @input="drawerDirty = true; draft.testStatus = 'INVALIDATED'" /></label><label class="field span-2"><span>默认数据库</span><input v-model="draft.database" @input="drawerDirty = true; draft.testStatus = 'INVALIDATED'" /><small>不显示密码、令牌或可连接凭据。</small></label></div></section><section class="form-section"><div class="form-section-head"><h3>连接测试</h3><span>固定探针语义</span></div><div class="test-panel"><label class="field"><span>执行节点</span><select><option v-for="nodeItem in fixtureNodes.filter((item) => item.schedule === 'schedulable')" :key="nodeItem.id">{{ nodeItem.id }} · {{ nodeItem.displayName }}</option></select></label><div><StatusBadge :label="labelForConnection(draft.testStatus)" :tone="toneForConnection(draft.testStatus)" /><p v-if="draft.testedAt" class="muted mono">最近测试：{{ draft.testedAt }}</p></div><button class="btn" type="button" :disabled="testing" :aria-busy="testing" @click="testConnection"><AppIcon name="refresh" />{{ testing ? '正在测试' : '测试连接' }}</button></div></section><p v-if="formError" class="feedback feedback-danger" role="alert">{{ formError }}</p><p v-if="feedback && drawerOpen" class="feedback feedback-success" role="status">{{ feedback }}</p></form><footer class="drawer-foot"><button class="btn" type="button" @click="requestCloseDrawer">取消</button><button class="btn btn-primary" type="button" :disabled="testing" @click="save">保存数据源</button></footer></aside></div>
    <ConfirmDialog :open="Boolean(pendingConfirm)" :title="confirmTitle" :description="confirmDescription" :impacts="confirmImpacts" :confirm-label="pendingConfirm?.kind === 'discard' ? '放弃修改' : pendingConfirm?.kind === 'archive' ? '模拟删除或归档' : '确认切换'" :danger="pendingConfirm?.kind !== 'discard'" @cancel="pendingConfirm = null" @confirm="confirm" />
  </div>
</template>
