<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'

import {
  browserApi,
  type ApiError,
  type CommandPreview,
  type DataSourceConnectionTest,
  type DataSourceSummary,
  type DataSourceWrite,
  type ExportDraft,
  type ExportDraftInput,
  type Precheck,
  type TaskDetail,
  type TaskLog,
} from '@/api/browser'
import { runtimeConfig } from '@/config/runtime'

const api = browserApi()
type DraftForm = { -readonly [Key in keyof ExportDraftInput]: ExportDraftInput[Key] }
type DataSourceForm = { -readonly [Key in keyof DataSourceWrite]: DataSourceWrite[Key] }

const sources = ref<DataSourceSummary[]>([])
const sourcesLoading = ref(true)
const draft = ref<ExportDraft>()
const preview = ref<CommandPreview>()
const precheck = ref<Precheck>()
const task = ref<TaskDetail>()
const logs = ref<TaskLog[]>([])
const busyAction = ref('')
const notice = ref('')
const failure = ref('')
const existingTaskID = ref('')
const showSourceEditor = ref(false)
const connectionTest = ref<DataSourceConnectionTest>()
const sourceForm = reactive<DataSourceForm>({
  displayName: '', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '', port: 2883, username: '', defaultDatabase: '', password: '',
})
const form = reactive<DraftForm>({
  dataSourceId: '',
  nodeId: '',
  database: '',
  table: '',
  format: 'CSV',
  filePath: '',
})

const selectedSource = computed(() => sources.value.find((item) => item.id === form.dataSourceId))
const formErrors = computed(() => validateDraft(form))
const canWrite = computed(() => formErrors.value.length === 0 && !busyAction.value)
const precheckPassed = computed(() => precheck.value?.status === 'SUCCEEDED' && precheck.value.integrityStatus === 'COMPLETE')

onMounted(loadSources)

async function loadSources() {
  sourcesLoading.value = true
  clearFeedback()
  try {
    sources.value = await api.listDataSources()
  } catch (error) {
    showFailure(error)
  } finally {
    sourcesLoading.value = false
  }
}

function selectSource() {
  const source = selectedSource.value
  if (source?.defaultDatabase && !form.database) {
    form.database = source.defaultDatabase
  }
}

async function createDataSource() {
  if (!sourceForm.displayName.trim() || !sourceForm.host.trim() || !sourceForm.username.trim() || !sourceForm.password || sourceForm.port < 1 || sourceForm.port > 65535) {
    notice.value = '请填写数据源名称、ODP 地址、端口、用户名和密码。'
    return
  }
  await runAction('create-data-source', async () => {
    const sourceID = await api.createDataSource({ ...sourceForm, defaultDatabase: sourceForm.defaultDatabase || undefined })
    sourceForm.password = ''
    showSourceEditor.value = false
    await loadSources()
    form.dataSourceId = sourceID
    selectSource()
    notice.value = '数据源已登记。现在可发起连接测试。'
  })
}

async function testSelectedSource() {
  if (!selectedSource.value) {
    notice.value = '请先选择一个已登记的数据源。'
    return
  }
  await runAction('test-data-source', async () => {
    connectionTest.value = await api.testDataSourceConnection(selectedSource.value!.id)
    notice.value = connectionTest.value.status === 'PENDING' ? '连接测试已交给 Agent 执行，请稍后刷新。' : '连接测试已返回安全结果。'
  })
}

async function createDraft() {
  if (!canWrite.value) {
    notice.value = '请先修正草稿字段。'
    return
  }
  await runAction('create-draft', async () => {
    const draftID = await api.createExportDraft({ ...form })
    draft.value = await api.getExportDraft(draftID)
    preview.value = undefined
    precheck.value = undefined
    task.value = undefined
    logs.value = []
    notice.value = 'CSV 草稿已创建；下一步可查看脱敏命令预览。'
  })
}

async function saveDraft() {
  if (!draft.value) {
    return createDraft()
  }
  if (!canWrite.value) {
    notice.value = '请先修正草稿字段。'
    return
  }
  await runAction('save-draft', async () => {
    draft.value = await api.updateExportDraft({ ...draft.value!, config: { ...form } })
    preview.value = undefined
    precheck.value = undefined
    notice.value = '草稿已保存；原有预检查已不再可用于提交。'
  })
}

async function loadPreview() {
  if (!draft.value) {
    notice.value = '请先创建或保存草稿。'
    return
  }
  await runAction('preview-command', async () => {
    preview.value = await api.previewExportCommand(draft.value!)
    notice.value = '命令已由控制面重新生成并通过本地脱敏校验。'
  })
}

async function startPrecheck() {
  if (!draft.value) {
    notice.value = '请先创建或保存草稿。'
    return
  }
  await runAction('start-precheck', async () => {
    const precheckID = await api.startPrecheck(draft.value!)
    precheck.value = await api.getPrecheck(precheckID)
    notice.value = '预检查已进入合成队列；真实执行始终保持关闭。'
  })
}

async function refreshPrecheck() {
  if (!precheck.value) {
    return
  }
  await runAction('refresh-precheck', async () => {
    precheck.value = await api.getPrecheck(precheck.value!.id)
    notice.value = '已刷新预检查状态。'
  })
}

async function submitTask() {
  if (!draft.value || !precheck.value || !precheckPassed.value) {
    notice.value = '仅当前草稿的有效成功预检查可以提交任务。'
    return
  }
  await runAction('submit-task', async () => {
    const taskID = await api.submitExportDraft(draft.value!, precheck.value!.id)
    await loadTask(taskID)
    notice.value = '任务快照已冻结。真实执行未开放，页面不会启动任何工具。'
  })
}

async function viewExistingTask() {
  if (!existingTaskID.value.trim()) {
    notice.value = '请输入任务标识。'
    return
  }
  await runAction('load-task', async () => loadTask(existingTaskID.value.trim()))
}

async function loadTask(taskID: string) {
  task.value = await api.getTask(taskID)
  logs.value = await api.getTaskLogs(taskID)
}

async function refreshTask() {
  if (!task.value) {
    return
  }
  await runAction('refresh-task', async () => {
    await loadTask(task.value!.id)
    notice.value = '已刷新任务和日志快照。'
  })
}

async function runAction(action: string, operation: () => Promise<void>) {
  busyAction.value = action
  clearFeedback()
  try {
    await operation()
  } catch (error) {
    showFailure(error)
  } finally {
    busyAction.value = ''
  }
}

function showFailure(error: unknown) {
  const apiError = error as Partial<ApiError>
  if (apiError.conflict) {
    failure.value = '数据已发生变化。请重新加载草稿或资源后再试。'
    return
  }
  if (apiError.status === 404) {
    failure.value = '资源不存在或当前身份无权查看。'
    return
  }
  if (apiError.status === 401) {
    failure.value = '浏览器身份未建立或已失效。'
    return
  }
  failure.value = apiError.message || '请求未能完成。'
}

function clearFeedback() {
  notice.value = ''
  failure.value = ''
}

function validateDraft(input: ExportDraftInput): string[] {
  const errors: string[] = []
  if (!input.dataSourceId) errors.push('请选择已授权的数据源。')
  if (!input.nodeId.trim()) errors.push('请输入已登记的执行节点标识。')
  if (!input.database.trim()) errors.push('请输入数据库名称。')
  if (!input.table.trim() || /[*,]/.test(input.table)) errors.push('表名不能为空，且不能包含 * 或 ,。')
  if (!input.filePath.trim()) errors.push('请输入执行节点上的绝对输出路径。')
  return errors
}
</script>

<template>
  <main class="app-shell">
    <header class="app-header">
      <div>
        <p class="product-name">OB Data Orch</p>
        <h1>单表 CSV 导出切片</h1>
      </div>
      <p class="execution-gate" role="status">
        {{ runtimeConfig.stage }} 合成验证 · 真实执行未开放
      </p>
    </header>

    <p class="gate-note">
      数据源密码只在提交时作为 write-only 输入发送给控制面；浏览器不会回显、保存或展示密码。连接测试由 Agent 执行，页面不会直接启动 Java 或 OBDUMPER。
    </p>

    <p v-if="failure" class="feedback feedback-error" role="alert">{{ failure }}</p>
    <p v-else-if="notice" class="feedback feedback-notice" role="status">{{ notice }}</p>

    <section class="workspace-grid" aria-label="最小导出业务链路">
      <article class="panel source-panel">
        <div class="panel-heading">
          <div>
            <p class="step">1</p>
            <h2>选择数据源</h2>
          </div>
          <div class="button-row compact-row">
            <button class="button button-secondary" type="button" :disabled="sourcesLoading" @click="loadSources">刷新</button>
            <button class="button button-primary" type="button" :disabled="Boolean(busyAction)" @click="showSourceEditor = !showSourceEditor">新增数据源</button>
          </div>
        </div>

        <form v-if="showSourceEditor" class="source-form" @submit.prevent="createDataSource">
          <label class="field"><span>名称</span><input v-model.trim="sourceForm.displayName" maxlength="120" autocomplete="off" /></label>
          <label class="field"><span>环境</span><input v-model.trim="sourceForm.environment" maxlength="32" autocomplete="off" /></label>
          <label class="field"><span>ODP 地址</span><input v-model.trim="sourceForm.host" maxlength="253" autocomplete="off" /></label>
          <label class="field"><span>端口</span><input v-model.number="sourceForm.port" type="number" min="1" max="65535" /></label>
          <label class="field"><span>组合用户名</span><input v-model.trim="sourceForm.username" maxlength="256" autocomplete="username" /></label>
          <label class="field"><span>默认数据库（可选）</span><input v-model.trim="sourceForm.defaultDatabase" maxlength="512" autocomplete="off" /></label>
          <label class="field field-wide"><span>密码</span><input v-model="sourceForm.password" type="password" maxlength="4096" autocomplete="new-password" /></label>
          <div class="button-row"><button class="button button-primary" type="submit" :disabled="Boolean(busyAction)">安全登记</button></div>
        </form>

        <p v-if="sourcesLoading" class="state-text">正在加载已授权数据源…</p>
        <p v-else-if="sources.length === 0" class="state-text">当前没有可选择的数据源。请由具备数据源管理权限的用户先完成登记。</p>
        <label v-else class="field">
          <span>已授权数据源</span>
          <select v-model="form.dataSourceId" @change="selectSource">
            <option value="">请选择</option>
            <option v-for="source in sources" :key="source.id" :value="source.id" :disabled="source.state !== 'ENABLED'">
              {{ source.displayName }}（{{ source.environment }} / {{ source.state }}）
            </option>
          </select>
        </label>
        <dl v-if="selectedSource" class="summary-list">
          <div><dt>连接类型</dt><dd>{{ selectedSource.connectionKind }}</dd></div>
          <div><dt>主机</dt><dd>{{ selectedSource.host }}:{{ selectedSource.port }}</dd></div>
          <div><dt>兼容模式</dt><dd>{{ selectedSource.compatibilityMode }}</dd></div>
        </dl>
        <div v-if="selectedSource" class="precheck-actions">
          <button class="button button-primary" type="button" :disabled="Boolean(busyAction) || selectedSource.state !== 'ENABLED'" @click="testSelectedSource">测试连接</button>
          <span v-if="connectionTest" class="fixed-value">{{ connectionTest.status }} · {{ connectionTest.code }}</span>
        </div>
      </article>

      <article class="panel draft-panel">
        <div class="panel-heading">
          <div>
            <p class="step">2</p>
            <h2>CSV 草稿</h2>
          </div>
          <span class="fixed-value">格式：CSV</span>
        </div>
        <form class="draft-form" @submit.prevent="saveDraft">
          <label class="field">
            <span>执行节点标识</span>
            <input v-model.trim="form.nodeId" autocomplete="off" maxlength="200" placeholder="输入已登记节点的标识" />
          </label>
          <label class="field">
            <span>数据库</span>
            <input v-model.trim="form.database" autocomplete="off" maxlength="512" />
          </label>
          <label class="field">
            <span>表</span>
            <input v-model.trim="form.table" autocomplete="off" maxlength="512" />
          </label>
          <label class="field field-wide">
            <span>节点本地绝对输出路径</span>
            <input v-model.trim="form.filePath" autocomplete="off" maxlength="4096" placeholder="Windows 或 Linux 节点上的绝对路径" />
          </label>
          <ul v-if="formErrors.length" class="field-errors" aria-live="polite">
            <li v-for="item in formErrors" :key="item">{{ item }}</li>
          </ul>
          <div class="button-row">
            <button class="button button-primary" type="submit" :disabled="!canWrite">
              {{ draft ? '保存草稿' : '创建草稿' }}
            </button>
            <span v-if="draft" class="revision">草稿版本 rev-{{ draft.revision }}</span>
          </div>
        </form>
        <p class="capability-gap">当前控制面尚未提供执行节点列表 API；节点标识必须由已登记节点事实提供，服务端无权或不存在时统一返回 404。</p>
      </article>

      <article class="panel command-panel">
        <div class="panel-heading">
          <div>
            <p class="step">3</p>
            <h2>脱敏命令与预检查</h2>
          </div>
          <button class="button button-secondary" type="button" :disabled="!draft || Boolean(busyAction)" @click="loadPreview">生成预览</button>
        </div>
        <p v-if="!preview" class="state-text">先保存草稿，再由控制面生成命令。未通过本地脱敏校验的预览不会显示。</p>
        <pre v-else class="command-preview"><code>{{ preview.command }}</code></pre>
        <div class="precheck-actions">
          <button class="button button-primary" type="button" :disabled="!draft || Boolean(busyAction)" @click="startPrecheck">发起预检查</button>
          <button class="button button-secondary" type="button" :disabled="!precheck || Boolean(busyAction)" @click="refreshPrecheck">刷新状态</button>
        </div>
        <dl v-if="precheck" class="summary-list precheck-summary">
          <div><dt>状态</dt><dd>{{ precheck.status }}</dd></div>
          <div><dt>完整性</dt><dd>{{ precheck.integrityStatus }}</dd></div>
          <div><dt>有效至</dt><dd>{{ precheck.validUntil }}</dd></div>
        </dl>
        <button class="button button-primary" type="button" :disabled="!precheckPassed || Boolean(busyAction)" @click="submitTask">提交任务快照</button>
      </article>

      <article class="panel task-panel">
        <div class="panel-heading">
          <div>
            <p class="step">4</p>
            <h2>任务与日志</h2>
          </div>
          <button class="button button-secondary" type="button" :disabled="!task || Boolean(busyAction)" @click="refreshTask">刷新</button>
        </div>
        <form class="task-lookup" @submit.prevent="viewExistingTask">
          <label class="field">
            <span>查看已有任务</span>
            <input v-model.trim="existingTaskID" autocomplete="off" placeholder="任务标识" />
          </label>
          <button class="button button-secondary" type="submit" :disabled="Boolean(busyAction)">查看</button>
        </form>
        <p v-if="!task" class="state-text">提交成功后会显示冻结快照；也可输入有权限的任务标识查看。</p>
        <template v-else>
          <dl class="summary-list task-summary">
            <div><dt>任务状态</dt><dd>{{ task.state }}</dd></div>
            <div><dt>执行标识</dt><dd>{{ task.executionId || '尚未领取' }}</dd></div>
            <div><dt>提交时间</dt><dd>{{ task.submittedAt }}</dd></div>
            <div><dt>真实执行</dt><dd>未开放</dd></div>
          </dl>
          <p class="command-label">冻结的脱敏命令</p>
          <pre class="command-preview"><code>{{ task.plannedCommand }}</code></pre>
          <div class="logs" aria-live="polite">
            <p v-if="logs.length === 0" class="state-text">当前没有可显示的脱敏日志。</p>
            <ol v-else>
              <li v-for="log in logs" :key="`${log.sourceSeq}-${log.receivedAt}`">
                <span>{{ log.kind }} · #{{ log.sourceSeq }} · {{ log.integrityCode }}</span>
                <span>{{ log.message }}</span>
              </li>
            </ol>
          </div>
        </template>
      </article>
    </section>
  </main>
</template>
