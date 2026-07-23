<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { browserApi, dataSourceErrorMessage, type DataSourceConnectionTest, type DataSourceSummary, type DataSourceUpdate, type DataSourceWrite } from '@/api/browser'

const api = browserApi()
const route = useRoute()
const router = useRouter()
type DataSourceForm = { -readonly [Key in keyof DataSourceWrite]: DataSourceWrite[Key] }
type MutableDataSourceUpdate = { -readonly [Key in keyof DataSourceUpdate]: DataSourceUpdate[Key] }
const isNew = computed(() => route.params.id === 'new')
const source = ref<DataSourceSummary>()
const loading = ref(!isNew.value)
const loadFailed = ref(false)
const busy = ref(false)
const failure = ref('')
const notice = ref('')
const connectionTest = ref<DataSourceConnectionTest>()
const connectionStatusInvalidated = ref(false)
const form = reactive<DataSourceForm>({ displayName: '', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'UNKNOWN', host: '', port: 2883, username: '', defaultDatabase: '', password: '' })

onMounted(loadSource)

async function loadSource() {
  if (isNew.value) return
  loading.value = true
  loadFailed.value = false
  failure.value = ''
  try {
    source.value = await api.getDataSource(String(route.params.id))
    fillFromSource(source.value)
    connectionStatusInvalidated.value = false
  } catch (error) {
    loadFailed.value = true
    failure.value = dataSourceErrorMessage(error, '无法加载数据源。')
  } finally {
    loading.value = false
  }
}

function fillFromSource(value: DataSourceSummary) {
  form.displayName = value.displayName
  form.environment = value.environment as DataSourceWrite['environment']
  form.connectionKind = 'ODP'
  form.compatibilityMode = value.compatibilityMode as DataSourceWrite['compatibilityMode']
  form.host = value.host
  form.port = value.port
  form.username = ''
  form.defaultDatabase = value.defaultDatabase ?? ''
  form.password = ''
}

function validate() {
  if (!form.displayName.trim() || !form.host.trim() || form.port < 1 || form.port > 65535) return '请填写数据源名称、ODP 地址和合法 SQL 端口。'
  if (isNew.value && (!form.username.trim() || !form.password)) return '新增数据源还需要填写用户名和密码。'
  return ''
}

async function save() {
  const validationMessage = validate()
  if (validationMessage) { failure.value = validationMessage; return }
  busy.value = true; failure.value = ''; notice.value = ''
  try {
    if (isNew.value) {
      const dataSourceID = await api.createDataSource({ ...form, displayName: form.displayName.trim(), host: form.host.trim(), username: form.username.trim(), defaultDatabase: form.defaultDatabase || undefined })
      form.password = ''
      await router.replace(`/data-sources/${dataSourceID}`)
      notice.value = '数据源已安全登记。可在此页发起基础连接测试。'
      return
    }
    if (!source.value) return
    const update = buildUpdate(source.value)
    if (Object.keys(update).length === 0) { notice.value = '没有需要保存的变更。'; return }
    const invalidatesConnectionTest = changesConnectionInput(update)
    const updated = await api.updateDataSource(source.value.id, source.value.revision, update)
    source.value = updated
    fillFromSource(updated)
    connectionStatusInvalidated.value = invalidatesConnectionTest
    notice.value = invalidatesConnectionTest
      ? '数据源已保存。连接输入已变更，页面不会继续使用保存前的连接测试结论。'
      : '数据源已保存。'
  } catch (error) {
    failure.value = dataSourceErrorMessage(error, '数据源保存失败。')
  } finally { busy.value = false }
}

function buildUpdate(current: DataSourceSummary): DataSourceUpdate {
  const update: MutableDataSourceUpdate = {}
  if (form.displayName.trim() !== current.displayName) update.displayName = form.displayName.trim()
  if (form.environment !== current.environment) update.environment = form.environment
  if (form.compatibilityMode !== current.compatibilityMode) update.compatibilityMode = form.compatibilityMode
  if (form.host.trim() !== current.host) update.host = form.host.trim()
  if (form.port !== current.port) update.port = form.port
  if (form.defaultDatabase !== (current.defaultDatabase ?? '')) update.defaultDatabase = form.defaultDatabase
  if (form.username.trim()) update.username = form.username.trim()
  if (form.password) update.password = form.password
  return update
}

function changesConnectionInput(update: DataSourceUpdate) {
  return update.connectionKind !== undefined || update.host !== undefined || update.port !== undefined || update.username !== undefined || update.password !== undefined
}

async function testConnection() {
  if (isNew.value || !source.value) { notice.value = '请先保存数据源；页面只会请求 Agent 执行连接测试，不会直接连接数据库。'; return }
  busy.value = true; failure.value = ''; notice.value = ''
  try {
    connectionTest.value = await api.testDataSourceConnection(source.value.id)
    notice.value = connectionTest.value.status === 'PENDING' ? '连接测试已转交 Agent。当前结果不会由浏览器推断或伪造。' : '连接测试已返回受控状态。'
  } catch (error) { failure.value = dataSourceErrorMessage(error, '当前无法获取 Agent 连接测试结果。') } finally { busy.value = false }
}
</script>

<template>
  <section class="page-heading"><div><h1>{{ isNew ? '新增数据源' : '编辑数据源' }}</h1><p>数据源只保存任务复用的基础连接信息；私有 ODP 是当前 V1.0 的固定连接方式。</p></div></section>
  <p v-if="failure" class="feedback feedback-error" role="alert">{{ failure }}</p><p v-else-if="notice" class="feedback feedback-notice" role="status">{{ notice }}</p>
  <section v-if="loading" class="content-card loading-state">正在加载数据源…</section>
  <section v-else-if="loadFailed" class="content-card empty-state"><h2>无法打开数据源</h2><p>{{ failure }}</p><button type="button" class="button button-secondary" @click="loadSource">重试</button></section>
  <div v-else class="form-layout"><form class="content-card form-card" @submit.prevent="save"><h2>1. 基本信息</h2><div class="form-grid"><label class="field-label">数据源名称 <b>*</b><input v-model.trim="form.displayName" maxlength="120" /></label><fieldset class="field-label"><legend>环境 <b>*</b></legend><div class="radio-row"><label><input v-model="form.environment" type="radio" value="DEVELOPMENT" />开发</label><label><input v-model="form.environment" type="radio" value="TEST" />测试</label><label><input v-model="form.environment" type="radio" value="STAGING" />预生产</label><label><input v-model="form.environment" type="radio" value="PRODUCTION" />生产</label></div></fieldset><label class="field-label field-span">描述<textarea placeholder="说明用途和业务范围（当前接口尚未保存此字段）" disabled /></label></div><h2>2. 连接信息</h2><p class="fixed-field">连接方式：<strong>私有 ODP</strong><span>当前版本固定，不提供切换。</span></p><div class="form-grid"><label class="field-label">ODP 地址 <b>*</b><input v-model.trim="form.host" maxlength="253" placeholder="IP、域名或 VIP" /></label><label class="field-label">SQL 端口 <b>*</b><input v-model.number="form.port" type="number" min="1" max="65535" /></label><label class="field-label">兼容模式 <b>*</b><select v-model="form.compatibilityMode"><option value="UNKNOWN">待校验</option><option value="MYSQL">MySQL</option><option value="ORACLE">Oracle</option></select></label><label class="field-label">租户 <b>*</b><input placeholder="租户字段将在连接契约接入后保存" disabled /></label><label class="field-label">集群 <span class="optional">条件必填</span><input placeholder="待官方参数映射确认" disabled /></label><label class="field-label">用户名 <b v-if="isNew">*</b><span v-else class="optional">留空则不修改</span><input v-model.trim="form.username" maxlength="256" autocomplete="username" :placeholder="isNew ? '' : '为保护现有连接标识，编辑时不回显'" /></label><label class="field-label field-span">默认数据库 / Schema <span class="optional">可选</span><input v-model.trim="form.defaultDatabase" maxlength="512" /></label><label class="field-label field-span">密码 <b v-if="isNew">*</b><span v-else class="optional">留空则不修改</span><input v-model="form.password" type="password" maxlength="4096" autocomplete="new-password" :placeholder="isNew ? '新增时必填；页面不会回显或保存密码' : '输入新密码才会轮换凭据'" /></label></div><div class="inline-actions"><button type="button" class="button button-secondary" :disabled="busy" @click="testConnection">{{ busy ? '处理中…' : '测试连接' }}</button><span>测试仅覆盖网络、认证和基础数据库连接。</span></div><section v-if="connectionTest" class="test-result"><strong>本次受控测试：{{ connectionTest.status }}</strong><span>代码：{{ connectionTest.code }}</span><span v-if="connectionTest.testedAt">完成时间：{{ new Date(connectionTest.testedAt).toLocaleString() }}</span><span v-else>控制面未返回完成时间。</span></section></form><aside class="detail-aside"><section class="content-card"><h2>数据源状态</h2><dl><div><dt>连接状态</dt><dd>{{ connectionStatusInvalidated ? '连接信息已变更，需重新测试' : source?.lastTestStatus === 'SUCCEEDED' ? '可连接' : source?.lastTestStatus === 'FAILED' ? '连接失败' : source?.lastTestStatus === 'PENDING' ? '测试已请求' : '未测试' }}</dd></div><div><dt>最近测试时间</dt><dd>{{ connectionStatusInvalidated ? '已因本次连接信息变更失效' : source?.lastTestStatus ? '控制面摘要未提供时间' : '尚未测试' }}</dd></div><div><dt>启用状态</dt><dd>{{ source ? (source.state === 'ENABLED' ? '已启用' : '已禁用') : '保存并成功测试后可启用' }}</dd></div></dl></section><section class="content-card"><h2>连接测试说明</h2><p>不会检查导入/导出权限、对象权限、性能，也不会判断任务是否一定可执行。</p></section></aside></div>
  <footer v-if="!loading && !loadFailed" class="page-footer"><button type="button" class="button button-secondary" @click="router.push('/data-sources')">取消</button><span class="footer-grow" /><button type="button" class="button button-primary" :disabled="busy" @click="save">{{ busy ? '保存中…' : '保存数据源' }}</button></footer>
</template>
