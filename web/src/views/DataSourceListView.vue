<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import EmptyState from '@/components/EmptyState.vue'
import { browserApi, type ApiError, type DataSourceSummary } from '@/api/browser'

const api = browserApi()
const router = useRouter()
const sources = ref<DataSourceSummary[]>([])
const loading = ref(true)
const failure = ref('')
const keyword = ref('')
const environment = ref('')
const state = ref('')

const filteredSources = computed(() => sources.value.filter((source) => {
  const keywordMatched = !keyword.value || [source.displayName, source.host].some((item) => item.toLowerCase().includes(keyword.value.toLowerCase()))
  return keywordMatched && (!environment.value || source.environment === environment.value) && (!state.value || source.state === state.value)
}))

onMounted(loadSources)

async function loadSources() {
  loading.value = true
  failure.value = ''
  try {
    sources.value = await api.listDataSources()
  } catch (error) {
    const apiError = error as Partial<ApiError>
    failure.value = apiError.message || '无法加载数据源，请稍后重试。'
  } finally {
    loading.value = false
  }
}

function resetFilters() {
  keyword.value = ''
  environment.value = ''
  state.value = ''
}

function stateLabel(source: DataSourceSummary) {
  return source.state === 'ENABLED' ? '已启用 / 待测试' : '已禁用'
}
</script>

<template>
  <section class="page-heading"><div><h1>数据源管理</h1><p>统一维护任务向导复用的私有 ODP 数据源；连接测试只验证网络、认证和基础数据库连接。</p></div><button type="button" class="button button-primary" @click="router.push('/data-sources/new')">新增数据源</button></section>
  <p class="context-note">数据源状态不代表导入、导出权限、对象权限、性能或任务可执行性；这些检查将在任务预检查阶段进行。</p>
  <section class="filter-bar" aria-label="数据源筛选"><label>关键字<input v-model.trim="keyword" placeholder="名称或地址" /></label><label>环境<select v-model="environment"><option value="">全部环境</option><option value="DEV">开发</option><option value="TEST">测试</option><option value="STAGING">预生产</option><option value="PROD">生产</option></select></label><label>启用状态<select v-model="state"><option value="">全部状态</option><option value="ENABLED">已启用</option><option value="DISABLED">已禁用</option></select></label><button type="button" class="button button-secondary" @click="resetFilters">重置</button><button type="button" class="button button-secondary" :disabled="loading" @click="loadSources">刷新</button></section>
  <p v-if="failure" class="feedback feedback-error" role="alert">{{ failure }}</p>
  <section v-if="loading" class="content-card loading-state">正在加载已授权数据源…</section>
  <EmptyState v-else-if="!failure && sources.length === 0" title="还没有数据源" description="新增并完成一次成功连接测试后，数据源才能被任务向导选择。" action="新增数据源" @action="router.push('/data-sources/new')" />
  <EmptyState v-else-if="!failure && filteredSources.length === 0" title="没有匹配的数据源" description="保留当前筛选条件；可重置筛选后重新查看。" action="重置筛选" @action="resetFilters" />
  <section v-else class="content-card table-card"><table><thead><tr><th>数据源名称</th><th>环境</th><th>连接方式</th><th>地址 / SQL 端口</th><th>兼容模式</th><th>连接状态</th><th>启用状态</th><th>最近测试</th><th>操作</th></tr></thead><tbody><tr v-for="source in filteredSources" :key="source.id"><td><strong>{{ source.displayName }}</strong><small>{{ source.id }}</small></td><td><span class="environment-tag" :class="source.environment.toLowerCase()">{{ source.environment }}</span></td><td>{{ source.connectionKind === 'ODP' ? '私有 ODP' : source.connectionKind }}</td><td>{{ source.host }}:{{ source.port }}</td><td>{{ source.compatibilityMode === 'MYSQL' ? 'MySQL' : source.compatibilityMode }}</td><td><span class="status-dot neutral" />未测试</td><td><span class="status-dot" :class="source.state === 'ENABLED' ? 'success' : 'muted'" />{{ stateLabel(source) }}</td><td>尚未测试</td><td class="table-actions"><button type="button" class="link-button" @click="router.push(`/data-sources/${source.id}`)">编辑</button><button type="button" class="link-button" disabled>测试</button></td></tr></tbody></table></section>
</template>
