<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { browserApi, executionNodeErrorMessage, type ApiError, type ExecutionNodeDetail, type ExecutionNodePlatform, type ExecutionNodeWrite } from '@/api/browser'

type NodeFormField = 'displayName' | 'platform' | 'allowedRoots' | 'toolHome' | 'javaPath'
type NodeFormErrors = Partial<Record<NodeFormField, string>>

const api = browserApi()
const route = useRoute()
const router = useRouter()
const node = ref<ExecutionNodeDetail>()
const loading = ref(false)
const busy = ref(false)
const failure = ref('')
const notice = ref('')
const formErrors = reactive<NodeFormErrors>({})
const form = reactive({
  displayName: '',
  platform: 'WINDOWS_AMD64' as ExecutionNodePlatform,
  allowedRootsText: '',
  toolHome: '',
  javaPath: '',
})

const nodeID = computed(() => typeof route.params.id === 'string' ? route.params.id : '')
const isNew = computed(() => route.name === 'node-new')
const rootPlaceholder = computed(() => form.platform === 'WINDOWS_AMD64' ? '/E:/ob-data/exports' : '/var/lib/ob-data-orch/exports')
const toolHomePlaceholder = computed(() => form.platform === 'WINDOWS_AMD64' ? 'E:\\tools\\ob-loader-dumper-4.3.5-RELEASE' : '/opt/ob-loader-dumper-4.3.5-RELEASE')
const javaPathPlaceholder = computed(() => form.platform === 'WINDOWS_AMD64' ? 'C:\\Program Files\\Java\\jdk8\\bin\\java.exe' : '/usr/lib/jvm/java-8/bin/java')
const platformLabel = computed(() => {
  if (form.platform === 'WINDOWS_AMD64') return 'Windows AMD64'
  if (form.platform === 'LINUX_AMD64') return 'Kylin Linux AMD64'
  return 'Kylin Linux ARM64'
})

onMounted(() => {
  if (!isNew.value) void loadNode()
})

async function loadNode() {
  if (!nodeID.value) {
    failure.value = '未指定执行节点。'
    return
  }
  loading.value = true
  failure.value = ''
  try {
    node.value = await api.getExecutionNode(nodeID.value)
    fillForm(node.value)
  } catch (error) {
    failure.value = executionNodeErrorMessage(error, '执行节点加载失败。')
  } finally {
    loading.value = false
  }
}

function fillForm(value: ExecutionNodeDetail) {
  form.displayName = value.displayName
  form.platform = value.platform
  form.allowedRootsText = value.allowedRoots.join('\n')
  form.toolHome = value.toolHome
  form.javaPath = value.javaPath
  clearErrors()
}

function writeInput(): ExecutionNodeWrite {
  return {
    displayName: form.displayName.trim(),
    platform: form.platform,
    allowedRoots: form.allowedRootsText.split(/\r?\n/).map((root) => root.trim()).filter(Boolean),
    toolHome: form.toolHome.trim(),
    javaPath: form.javaPath.trim(),
  }
}

function validate(input: ExecutionNodeWrite): NodeFormErrors {
  const errors: NodeFormErrors = {}
  if (!input.displayName || input.displayName.length > 200) errors.displayName = '请输入不超过 200 个字符的节点名称。'
  if (!input.toolHome) errors.toolHome = '请填写 OB Loader/Dumper 安装目录。'
  if (!input.javaPath) errors.javaPath = '请填写工具专用 Java 8 可执行文件路径。'
  if (!input.allowedRoots.length) errors.allowedRoots = '请按行填写至少一个节点侧导出数据目录。'
  if (input.platform === 'WINDOWS_AMD64' && input.allowedRoots.some((root) => !/^\/[A-Za-z]:\//.test(root) || root.includes('\\') || root.split('/').some((part) => part === '.' || part === '..'))) {
    errors.allowedRoots = 'Windows 节点的每个导出数据目录必须使用 /E:/exports 形式。'
  }
  if (input.platform !== 'WINDOWS_AMD64' && input.allowedRoots.some((root) => !root.startsWith('/'))) {
    errors.allowedRoots = 'Linux 节点的每个导出数据目录必须是以 / 开头的绝对路径。'
  }
  return errors
}

async function save() {
  const input = writeInput()
  const validation = validate(input)
  setErrors(validation)
  if (Object.keys(validation).length) return
  busy.value = true
  failure.value = ''
  notice.value = ''
  try {
    if (isNew.value) {
      const createdID = await api.createExecutionNode(input)
      await router.replace({ name: 'node-detail', params: { id: createdID }, query: { registration: 'created' } })
      return
    }
    if (!node.value) return
    node.value = await api.updateExecutionNode(node.value.id, node.value.revision, input)
    fillForm(node.value)
    notice.value = '节点配置已保存。工具或目录配置变更后，请重新生成注册码并在目标机器重新关联 Agent。'
  } catch (error) {
    applyApiErrors(error)
    if (!Object.keys(formErrors).length) failure.value = executionNodeErrorMessage(error, '执行节点保存失败。')
  } finally {
    busy.value = false
  }
}

function applyApiErrors(error: unknown) {
  clearErrors()
  const apiError = error as Partial<ApiError>
  for (const fieldError of apiError.fieldErrors ?? []) {
    if (fieldError.field === 'displayName' || fieldError.field === 'platform' || fieldError.field === 'allowedRoots' || fieldError.field === 'toolHome' || fieldError.field === 'javaPath') {
      formErrors[fieldError.field] = fieldError.message || '该字段不符合要求。'
    }
  }
}

function clearErrors() {
  for (const field of Object.keys(formErrors) as NodeFormField[]) delete formErrors[field]
}

function clearError(field: NodeFormField) {
  delete formErrors[field]
}

function setErrors(errors: NodeFormErrors) {
  clearErrors()
  Object.assign(formErrors, errors)
}
</script>

<template>
  <section class="page-heading">
    <div>
      <h1>{{ isNew ? '注册执行节点' : '编辑执行节点' }}</h1>
      <p>在这里登记目标机器上的工具、Java 和导出数据目录，再进入 Agent 一次性关联。节点 IP 和主机名不参与注册或控制面连接。</p>
    </div>
    <RouterLink class="button button-secondary" to="/nodes">返回执行节点</RouterLink>
  </section>

  <p v-if="failure" class="feedback feedback-error" role="alert">{{ failure }}</p>
  <p v-else-if="notice" class="feedback feedback-notice" role="status">{{ notice }}</p>

  <section v-if="loading" class="content-card loading-state">正在加载执行节点…</section>
  <section v-else-if="!isNew && !node" class="content-card empty-state">
    <h2>无法打开执行节点</h2>
    <p>{{ failure || '节点详情不可用。' }}</p>
    <button type="button" class="button button-secondary" @click="loadNode">重试</button>
  </section>

  <div v-else class="form-layout">
    <form class="content-card form-card" @submit.prevent="save">
      <h2>节点管理信息</h2>
      <div class="form-grid">
        <label class="field-label">
          节点名称 <b>*</b>
          <input v-model.trim="form.displayName" maxlength="200" :aria-invalid="formErrors.displayName ? 'true' : undefined" @input="clearError('displayName')" />
          <span v-if="formErrors.displayName" class="field-error" role="alert">{{ formErrors.displayName }}</span>
        </label>
        <label class="field-label">
          目标平台 <b>*</b>
          <select v-model="form.platform" :aria-invalid="formErrors.platform ? 'true' : undefined" @change="clearError('platform')">
            <option value="WINDOWS_AMD64">Windows AMD64</option>
            <option value="LINUX_AMD64">Kylin Linux AMD64</option>
            <option value="LINUX_ARM64">Kylin Linux ARM64</option>
          </select>
          <span class="field-help">这是任务路由声明；实际操作系统与架构以后续 Agent 上报为准。</span>
          <span v-if="formErrors.platform" class="field-error" role="alert">{{ formErrors.platform }}</span>
        </label>
        <label class="field-label">
          OB Loader/Dumper 安装目录 <b>*</b>
          <input v-model.trim="form.toolHome" :placeholder="toolHomePlaceholder" :aria-invalid="formErrors.toolHome ? 'true' : undefined" @input="clearError('toolHome')" />
          <span class="field-help">填写目标执行机上的工具安装根目录；首次关联后由 Agent 在本机核验。</span>
          <span v-if="formErrors.toolHome" class="field-error" role="alert">{{ formErrors.toolHome }}</span>
        </label>
        <label class="field-label">
          工具专用 Java 8 路径 <b>*</b>
          <input v-model.trim="form.javaPath" :placeholder="javaPathPlaceholder" :aria-invalid="formErrors.javaPath ? 'true' : undefined" @input="clearError('javaPath')" />
          <span class="field-help">填写 Java 可执行文件的绝对路径；不会读取系统 PATH，也不修改机器环境变量。</span>
          <span v-if="formErrors.javaPath" class="field-error" role="alert">{{ formErrors.javaPath }}</span>
        </label>
        <label class="field-label field-span">
          导出数据目录白名单 <b>*</b>
          <textarea v-model="form.allowedRootsText" rows="5" :placeholder="rootPlaceholder" :aria-invalid="formErrors.allowedRoots ? 'true' : undefined" @input="clearError('allowedRoots')" />
          <span class="field-help">每行一个目标执行机上的绝对目录。Windows 使用 /E:/exports 形式；导出文件只能写入这些目录，首次关联与任务提交时都由 Agent 复核路径、可写性和空间。</span>
          <span v-if="formErrors.allowedRoots" class="field-error" role="alert">{{ formErrors.allowedRoots }}</span>
        </label>
      </div>
      <div class="inline-actions">
        <button type="submit" class="button button-primary" :disabled="busy">{{ busy ? '正在保存…' : isNew ? '保存并继续 Agent 关联' : '保存修改' }}</button>
        <RouterLink class="button button-secondary" to="/nodes">取消</RouterLink>
      </div>
    </form>

    <aside class="detail-aside">
      <section class="content-card">
        <h2>登记边界</h2>
        <dl>
          <div><dt>目标平台</dt><dd>{{ platformLabel }}</dd></div>
          <div><dt>初始管理状态</dt><dd>已禁用</dd></div>
          <div><dt>Agent 关联</dt><dd>待关联</dd></div>
          <div><dt>工具运行时</dt><dd>待 Agent 本机核验</dd></div>
        </dl>
      </section>
      <section class="content-card node-form-note">
        <h2>后续准入</h2>
        <p>保存后的路径只是管理员声明。首次关联 Agent 会在目标机器验证工具、Java 和数据目录；未取得这些事实时，节点不会被列为可接收新任务。</p>
      </section>
    </aside>
  </div>
</template>

<style scoped>
.field-error { display: block; margin: 0; color: #b42318; font-size: 12px; line-height: 1.5; }
.field-help { color: #7a899c; font-size: 12px; line-height: 1.5; }
.field-label input[aria-invalid='true'], .field-label select[aria-invalid='true'], .field-label textarea[aria-invalid='true'] { border-color: #d94841; box-shadow: 0 0 0 2px rgb(217 72 65 / 12%); }
.node-form-note { padding: 17px; }
.node-form-note p { margin: 0; color: #738195; font-size: 13px; line-height: 1.65; }
</style>
