<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import EmptyState from '@/components/EmptyState.vue'
import { browserApi, dataSourceErrorMessage, type DataSourceSummary } from '@/api/browser'
import { dataSourceDeletionNotice } from './dataSourceDeletionNotice'
import { filterDataSources, type ConnectionStatusFilter } from './dataSourceListFilters'

const api = browserApi()
const router = useRouter()
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

const filteredSources = computed(() => filterDataSources(sources.value, { keyword: keyword.value, environment: environment.value, compatibilityMode: compatibilityMode.value, connectionStatus: connectionStatus.value, state: state.value }))

onMounted(loadSources)

async function loadSources() {
  loading.value = true
  loadFailure.value = ''
  try {
    sources.value = await api.listDataSources()
  } catch (error) {
    loadFailure.value = dataSourceErrorMessage(error, '无法加载数据源，请稍后重试。')
  } finally {
    loading.value = false
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

function lastTestTimeLabel(source: DataSourceSummary) {
  if (!source.lastTestStatus) return '尚未测试'
  if (!source.lastTestedAt) return '控制面未返回完成时间'
  return new Date(source.lastTestedAt).toLocaleString()
}

async function runAction(source: DataSourceSummary, action: 'toggle' | 'archive') {
  actionID.value = source.id
  feedback.value = ''
  notice.value = ''
  try {
    if (action === 'archive') {
      if (!window.confirm(`确认删除或归档数据源“${source.displayName}”？历史任务不会被修改。`)) return
      const result = await api.deleteOrArchiveDataSource(source.id, source.revision)
      sources.value = sources.value.filter((item) => item.id !== source.id)
      notice.value = dataSourceDeletionNotice(result)
      return
    }
    const result = await api.changeDataSourceState(source.id, source.revision, source.state === 'ENABLED' ? 'DISABLED' : 'ENABLED')
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
  <section class="page-heading"><div><h1>数据源管理</h1><p>统一维护任务向导复用的私有 ODP 数据源；连接测试只验证网络、认证和基础数据库连接。</p></div><button type="button" class="button button-primary" @click="router.push('/data-sources/new')">新增数据源</button></section>
  <p class="context-note">数据源状态不代表导入、导出权限、对象权限、性能或任务可执行性；这些检查将在任务预检查阶段进行。</p>
  <section class="filter-bar" aria-label="数据源筛选"><label>关键字<input v-model.trim="keyword" placeholder="名称或地址" /></label><label>环境<select v-model="environment"><option value="">全部环境</option><option value="DEVELOPMENT">开发</option><option value="TEST">测试</option><option value="STAGING">预生产</option><option value="PRODUCTION">生产</option></select></label><label>数据源类型<select v-model="compatibilityMode"><option value="">全部类型</option><option value="MYSQL">OceanBase MySQL</option><option value="ORACLE">OceanBase Oracle</option><option value="UNKNOWN">待迁移类型</option></select></label><label>连接状态<select v-model="connectionStatus"><option value="">全部状态</option><option value="UNTESTED">未测试</option><option value="SUCCEEDED">可连接</option><option value="FAILED">连接失败</option><option value="UNKNOWN">结果未知</option><option value="EXPIRED">测试已过期</option><option value="INVALIDATED">测试已失效</option><option value="PENDING">测试已请求</option><option value="UNAVAILABLE">测试不可用</option></select></label><label>启用状态<select v-model="state"><option value="">全部状态</option><option value="ENABLED">已启用</option><option value="DISABLED">已禁用</option></select></label><button type="button" class="button button-secondary" @click="resetFilters">重置</button><button type="button" class="button button-secondary" :disabled="loading" @click="loadSources">刷新</button></section>
  <p v-if="feedback" class="feedback feedback-error" role="alert">{{ feedback }}</p>
  <p v-else-if="notice" class="feedback feedback-notice" role="status">{{ notice }}</p>
  <section v-if="loading" class="content-card loading-state">正在加载已授权数据源…</section>
  <section v-else-if="loadFailure" class="content-card empty-state"><h2>无法加载数据源</h2><p>{{ loadFailure }}</p><button type="button" class="button button-secondary" @click="loadSources">重试</button></section>
  <EmptyState v-else-if="sources.length === 0" title="还没有数据源" description="新增并完成一次成功连接测试后，数据源才能被任务向导选择。" action="新增数据源" @action="router.push('/data-sources/new')" />
  <EmptyState v-else-if="filteredSources.length === 0" title="没有匹配的数据源" description="保留当前筛选条件；可重置筛选后重新查看。" action="重置筛选" @action="resetFilters" />
  <section v-else class="content-card table-card"><table><thead><tr><th>数据源名称</th><th>环境</th><th>连接方式</th><th>地址 / SQL 端口</th><th>数据源类型</th><th>连接状态</th><th>启用状态</th><th>最近测试</th><th>操作</th></tr></thead><tbody><tr v-for="source in filteredSources" :key="source.id"><td><strong>{{ source.displayName }}</strong><small>{{ source.id }}</small></td><td><span class="environment-tag" :class="source.environment.toLowerCase()">{{ environmentLabel(source.environment) }}</span></td><td>{{ source.connectionKind === 'ODP' ? '私有 ODP' : source.connectionKind }}</td><td>{{ source.host }}:{{ source.port }}</td><td>{{ source.compatibilityMode === 'MYSQL' ? 'OceanBase MySQL' : source.compatibilityMode === 'ORACLE' ? 'OceanBase Oracle' : '待迁移类型' }}</td><td><span class="status-dot" :class="source.lastTestStatus === 'SUCCEEDED' ? 'success' : source.lastTestStatus === 'FAILED' ? 'danger' : 'neutral'" />{{ connectionStatusLabel(source) }}</td><td><span class="status-dot" :class="source.state === 'ENABLED' ? 'success' : 'muted'" />{{ stateLabel(source) }}</td><td>{{ lastTestTimeLabel(source) }}</td><td class="table-actions"><button type="button" class="link-button" :disabled="actionID === source.id" @click="router.push(`/data-sources/${source.id}`)">编辑</button><button type="button" class="link-button" :disabled="actionID === source.id" @click="router.push({ path: `/data-sources/${source.id}`, query: { test: '1' } })">测试</button><button type="button" class="link-button" :disabled="actionID === source.id" @click="runAction(source, 'toggle')">{{ source.state === 'ENABLED' ? '禁用' : '启用' }}</button><button type="button" class="link-button danger-link" :disabled="actionID === source.id" @click="runAction(source, 'archive')">删除 / 归档</button></td></tr></tbody></table></section>
</template>
