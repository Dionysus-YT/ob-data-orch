<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { browserApi, type ApiError, type DataSourceConnectionTest, type DataSourceWrite } from '@/api/browser'

const api = browserApi()
const route = useRoute()
const router = useRouter()
type DataSourceForm = { -readonly [Key in keyof DataSourceWrite]: DataSourceWrite[Key] }
const isNew = computed(() => route.params.id === 'new')
const busy = ref(false)
const failure = ref('')
const notice = ref('')
const connectionTest = ref<DataSourceConnectionTest>()
const form = reactive<DataSourceForm>({ displayName: '', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '', port: 2883, username: '', defaultDatabase: '', password: '' })

function isValid() { return Boolean(form.displayName.trim() && form.host.trim() && form.username.trim() && form.password && form.port >= 1 && form.port <= 65535) }

async function save() {
  if (!isNew.value) { notice.value = '编辑保存接口将在数据源模块功能接入时实现；当前页面先完成已确认字段和交互基线。'; return }
  if (!isValid()) { failure.value = '请填写数据源名称、ODP 地址、SQL 端口、用户名和密码。'; return }
  busy.value = true; failure.value = ''; notice.value = ''
  try {
    await api.createDataSource({ ...form, defaultDatabase: form.defaultDatabase || undefined })
    form.password = ''
    notice.value = '数据源已安全登记。请在列表中选择后发起连接测试。'
  } catch (error) { failure.value = (error as Partial<ApiError>).message || '数据源保存失败。' } finally { busy.value = false }
}

async function testConnection() {
  notice.value = '新增数据源需要先保存后才能使用已登记凭据测试；页面不会直接连接数据库。'
}
</script>

<template>
  <section class="page-heading"><div><h1>{{ isNew ? '新增数据源' : '编辑数据源' }}</h1><p>数据源只保存任务复用的基础连接信息；私有 ODP 是当前 V1.0 的固定连接方式。</p></div></section>
  <p v-if="failure" class="feedback feedback-error" role="alert">{{ failure }}</p><p v-else-if="notice" class="feedback feedback-notice" role="status">{{ notice }}</p>
  <div class="form-layout"><form class="content-card form-card" @submit.prevent="save"><h2>1. 基本信息</h2><div class="form-grid"><label class="field-label">数据源名称 <b>*</b><input v-model.trim="form.displayName" maxlength="120" /></label><fieldset class="field-label"><legend>环境 <b>*</b></legend><div class="radio-row"><label><input v-model="form.environment" type="radio" value="DEV" />开发</label><label><input v-model="form.environment" type="radio" value="TEST" />测试</label><label><input v-model="form.environment" type="radio" value="STAGING" />预生产</label><label><input v-model="form.environment" type="radio" value="PROD" />生产</label></div></fieldset><label class="field-label field-span">描述<textarea placeholder="说明用途和业务范围（可选）" /></label></div><h2>2. 连接信息</h2><p class="fixed-field">连接方式：<strong>私有 ODP</strong><span>当前版本固定，不提供切换。</span></p><div class="form-grid"><label class="field-label">ODP 地址 <b>*</b><input v-model.trim="form.host" maxlength="253" placeholder="IP、域名或 VIP" /></label><label class="field-label">SQL 端口 <b>*</b><input v-model.number="form.port" type="number" min="1" max="65535" /></label><label class="field-label">租户 <b>*</b><input placeholder="租户字段将在连接契约接入后保存" disabled /></label><label class="field-label">集群 <span class="optional">条件必填</span><input placeholder="待官方参数映射确认" disabled /></label><label class="field-label">用户名 <b>*</b><input v-model.trim="form.username" maxlength="256" autocomplete="username" /></label><label class="field-label">默认数据库 / Schema <span class="optional">可选</span><input v-model.trim="form.defaultDatabase" maxlength="512" /></label><label class="field-label field-span">密码 <b>*</b><input v-model="form.password" type="password" maxlength="4096" autocomplete="new-password" placeholder="新增时必填；页面不会回显或保存密码" /></label></div><div class="inline-actions"><button type="button" class="button button-secondary" :disabled="busy" @click="testConnection">测试连接</button><span>测试仅覆盖网络、认证和基础数据库连接。</span></div><section v-if="connectionTest" class="test-result"><strong>连接测试：{{ connectionTest.status }}</strong><span>{{ connectionTest.code }}</span></section></form><aside class="detail-aside"><section class="content-card"><h2>数据源状态</h2><dl><div><dt>连接状态</dt><dd>未测试</dd></div><div><dt>最近测试时间</dt><dd>尚未测试</dd></div><div><dt>启用状态</dt><dd>保存并成功测试后可启用</dd></div></dl></section><section class="content-card"><h2>连接测试说明</h2><p>不会检查导入/导出权限、对象权限、性能，也不会判断任务是否一定可执行。</p></section></aside></div>
  <footer class="page-footer"><button type="button" class="button button-secondary" @click="router.push('/data-sources')">取消</button><span class="footer-grow" /><button type="button" class="button button-primary" :disabled="busy" @click="save">{{ busy ? '保存中…' : '保存数据源' }}</button></footer>
</template>
