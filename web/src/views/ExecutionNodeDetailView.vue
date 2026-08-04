<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'

import { browserApi, executionNodeErrorMessage, type ExecutionNodeDetail, type ExecutionNodeEnrollment } from '@/api/browser'
import { agentRegistrationCode, agentRegistrationCommand, requiresAgentRegistration } from '@/views/executionNodeEnrollmentInstructions'

const api = browserApi()
const route = useRoute()
const node = ref<ExecutionNodeDetail>()
const loading = ref(true)
const failure = ref('')
const enrollmentDialogOpen = ref(false)
const enrollment = ref<ExecutionNodeEnrollment>()
const enrollmentBusy = ref(false)
const enrollmentFailure = ref('')
const copyNotice = ref('')
const environmentCheckBusy = ref(false)
const nodeEnableBusy = ref(false)
const nodeActionNotice = ref('')
const nodeActionFailure = ref('')
const nodeID = computed(() => typeof route.params.id === 'string' ? route.params.id : '')
const registrationJustCreated = computed(() => route.query.registration === 'created' && node.value?.agentAssociationStatus === 'PENDING')
const registrationRequired = computed(() => node.value !== undefined && requiresAgentRegistration(node.value.agentAssociationStatus, node.value.unavailableReasons))
const enrollmentCommand = computed(() => {
  if (!node.value) return ''
  return agentRegistrationCommand(node.value.platform)
})
const registrationCode = computed(() => {
  if (!node.value || !enrollment.value) return ''
  return agentRegistrationCode(node.value.id, enrollment.value.enrollmentId, enrollment.value.enrollmentMaterial)
})
let enrollmentRequestVersion = 0

onMounted(loadNode)
onBeforeUnmount(clearEnrollment)
watch(nodeID, () => {
  clearEnrollment()
  void loadNode()
})

async function loadNode() {
  if (!nodeID.value) {
    failure.value = '未指定执行节点。'
    loading.value = false
    return
  }
  loading.value = true
  failure.value = ''
  try {
    node.value = await api.getExecutionNode(nodeID.value)
  } catch (error) {
    failure.value = executionNodeErrorMessage(error, '执行节点加载失败。')
  } finally {
    loading.value = false
  }
}

async function issueEnrollment() {
  if (!node.value || !registrationRequired.value || enrollmentBusy.value) return
  const requestVersion = ++enrollmentRequestVersion
  enrollmentDialogOpen.value = true
  enrollment.value = undefined
  enrollmentFailure.value = ''
  copyNotice.value = ''
  enrollmentBusy.value = true
  try {
    const result = await api.issueExecutionNodeEnrollment(node.value.id)
    if (requestVersion === enrollmentRequestVersion && enrollmentDialogOpen.value) enrollment.value = result
  } catch (error) {
    if (requestVersion === enrollmentRequestVersion && enrollmentDialogOpen.value) {
      enrollmentFailure.value = executionNodeErrorMessage(error, '关联材料签发失败。')
    }
  } finally {
    if (requestVersion === enrollmentRequestVersion) enrollmentBusy.value = false
  }
}

async function requestEnvironmentCheck() {
  if (!node.value || environmentCheckBusy.value || nodeEnableBusy.value) return
  environmentCheckBusy.value = true
  nodeActionFailure.value = ''
  nodeActionNotice.value = ''
  try {
    node.value = await api.requestExecutionNodeEnvironmentCheck(node.value.id, node.value.revision)
    nodeActionNotice.value = '环境检查已请求，等待当前 Agent 在下一次心跳后回传固定运行时结果。'
    window.setTimeout(() => { void loadNode() }, 2500)
  } catch (error) {
    nodeActionFailure.value = executionNodeErrorMessage(error, '环境检查请求失败。')
  } finally {
    environmentCheckBusy.value = false
  }
}

async function enableNode() {
  if (!node.value || environmentCheckBusy.value || nodeEnableBusy.value) return
  nodeEnableBusy.value = true
  nodeActionFailure.value = ''
  nodeActionNotice.value = ''
  try {
    node.value = await api.enableExecutionNode(node.value.id, node.value.revision)
    nodeActionNotice.value = '执行节点已启用。提交具体任务前仍需完成任务级预检查。'
  } catch (error) {
    nodeActionFailure.value = executionNodeErrorMessage(error, '执行节点启用失败。')
  } finally {
    nodeEnableBusy.value = false
  }
}

function clearEnrollment() {
  enrollmentRequestVersion += 1
  enrollment.value = undefined
  enrollmentFailure.value = ''
  copyNotice.value = ''
  enrollmentBusy.value = false
  enrollmentDialogOpen.value = false
}

async function copyEnrollmentValue(label: string, value: string) {
  copyNotice.value = ''
  try {
    await navigator.clipboard.writeText(value)
    copyNotice.value = `${label}已复制。`
  } catch {
    copyNotice.value = `${label}复制失败，请手动选择复制。`
  }
}

function platformLabel(platform: ExecutionNodeDetail['platform']) {
  if (platform === 'WINDOWS_AMD64') return 'Windows AMD64'
  if (platform === 'LINUX_AMD64') return 'Kylin Linux AMD64'
  return 'Kylin Linux ARM64'
}

function managementLabel(state: ExecutionNodeDetail['managementState']) {
  if (state === 'ENABLED') return '已启用'
  return state === 'MAINTENANCE' ? '维护中' : '已禁用'
}

function associationLabel(status: ExecutionNodeDetail['agentAssociationStatus']) {
  return status === 'ASSOCIATED' ? '已关联' : '待关联'
}

function heartbeatLabel(status: ExecutionNodeDetail['heartbeatStatus']) {
  if (status === 'ONLINE') return '在线'
  return status === 'OFFLINE' ? '离线' : '从未连接'
}

function environmentLabel(status: ExecutionNodeDetail['environmentStatus']) {
  if (status === 'NORMAL') return '工具已就绪'
  if (status === 'ABNORMAL') return '工具异常'
  return status === 'EXPIRED' ? '已过期' : '未检查'
}

function capacityLabel(status: ExecutionNodeDetail['capacityStatus']) {
  if (status === 'AVAILABLE') return '可用'
  return status === 'BUSY' ? '繁忙' : '未知'
}

function statusDotClass(status: string | boolean) {
  if (status === true || status === 'ENABLED' || status === 'ASSOCIATED' || status === 'ONLINE' || status === 'NORMAL' || status === 'AVAILABLE') return 'success'
  if (status === false || status === 'OFFLINE' || status === 'ABNORMAL' || status === 'EXPIRED' || status === 'BUSY') return 'danger'
  return 'muted'
}

function timeLabel(value: string | null | undefined) {
  if (!value) return '尚未收到'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '控制面未提供有效时间' : date.toLocaleString()
}

function capacitySummary(value: ExecutionNodeDetail) {
  if (!value.agentFacts) return value.capacityStatus === 'UNKNOWN' ? '尚未收到容量快照' : '控制面未提供容量快照'
  return `${value.agentFacts.capacityUsed} / ${value.agentFacts.capacityTotal}`
}

function percentageLabel(value: number | undefined) {
  return value === undefined ? '尚未采集' : `${value.toFixed(1)}%`
}

function byteLabel(value: number) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = value
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  return `${size.toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

function bootIdSummary(value: string) {
  if (value.length <= 20) return value
  return `${value.slice(0, 12)}…${value.slice(-4)}`
}

function unavailableReasonLabel(reason: string) {
  const labels: Record<string, string> = {
    AGENT_ASSOCIATION_REQUIRED: '需要完成 Agent 关联',
    HEARTBEAT_REQUIRED: '尚未收到 Agent 心跳',
    HEARTBEAT_OFFLINE: 'Agent 当前离线',
    ENVIRONMENT_CHECK_REQUIRED: '尚未取得环境检查事实',
    ENVIRONMENT_CHECK_IN_PROGRESS: '环境检查正在等待 Agent 回传',
    ENVIRONMENT_CHECK_ABNORMAL: '环境检查发现异常',
    ENVIRONMENT_CHECK_EXPIRED: '环境检查事实已过期',
    RUNTIME_CONFIGURATION_MISMATCH: '登记配置已变更，请在本机重新运行 Agent 注册',
    CAPACITY_UNKNOWN: '任务容量尚未取得',
    CAPACITY_BUSY: '当前任务容量已满',
    NODE_DISABLED: '节点已禁用',
    NODE_MAINTENANCE: '节点处于维护中',
  }
  return labels[reason] ?? reason
}
</script>

<template>
  <section class="page-heading">
    <div>
      <h1>执行节点详情</h1>
      <p>节点详情只展示当前有权读取的登记配置和 Agent 回写事实，不提供远程命令、文件浏览、工具安装或任务级路径检查。</p>
    </div>
    <div class="heading-actions">
      <RouterLink class="button button-secondary" to="/nodes">返回执行节点</RouterLink>
      <a v-if="node?.platform === 'WINDOWS_AMD64'" class="button button-secondary" href="/ob-data-orch-agent-windows-amd64.zip" download>下载 Windows Agent 包</a>
      <button v-if="registrationRequired" type="button" class="button button-primary" :disabled="enrollmentBusy" @click="issueEnrollment">{{ node?.agentAssociationStatus === 'PENDING' ? '生成注册码' : '重新生成注册码' }}</button>
      <button v-if="node?.agentAssociationStatus === 'ASSOCIATED' && node.heartbeatStatus === 'ONLINE' && node.environmentStatus !== 'NORMAL'" type="button" class="button button-primary" :disabled="environmentCheckBusy || nodeEnableBusy" @click="requestEnvironmentCheck">{{ environmentCheckBusy ? '正在请求检查…' : '检查节点环境' }}</button>
      <button v-if="node?.managementState === 'DISABLED' && node.agentAssociationStatus === 'ASSOCIATED' && node.heartbeatStatus === 'ONLINE' && node.environmentStatus === 'NORMAL' && node.capacityStatus === 'AVAILABLE'" type="button" class="button button-primary" :disabled="environmentCheckBusy || nodeEnableBusy" @click="enableNode">{{ nodeEnableBusy ? '正在启用…' : '启用节点' }}</button>
      <RouterLink v-if="node" class="button button-primary" :to="`/nodes/${node.id}/edit`">编辑节点</RouterLink>
    </div>
  </section>

  <section v-if="loading" class="content-card loading-state">正在加载执行节点…</section>
  <section v-else-if="failure" class="content-card empty-state">
    <h2>无法打开执行节点</h2>
    <p>{{ failure }}</p>
    <button type="button" class="button button-secondary" @click="loadNode">重试</button>
  </section>

  <template v-else-if="node">
    <p v-if="registrationJustCreated" class="feedback feedback-notice" role="status">节点记录已创建。生成一次性注册码后，在目标机器启动 Agent 即可完成关联。</p>
    <p v-if="nodeActionNotice" class="feedback feedback-notice" role="status">{{ nodeActionNotice }}</p>
    <p v-if="nodeActionFailure" class="feedback feedback-error" role="alert">{{ nodeActionFailure }}</p>

    <section v-if="node.agentAssociationStatus === 'PENDING'" class="content-card agent-registration">
      <div>
        <p class="detail-kicker">注册步骤 2 / 2</p>
        <h2>生成 Agent 注册码</h2>
        <p>无需填写节点 IP。请在目标机器启动 Agent，再粘贴一次性注册码完成关联；IP 不参与注册、连接或权限判断。本机首版会回写平台、容量与固定运行时检查结论。</p>
      </div>
      <ol>
        <li>在目标机器部署与所选平台匹配的 Agent 二进制。</li>
        <li v-if="node.platform === 'WINDOWS_AMD64'">下载并解压 <a href="/ob-data-orch-agent-windows-amd64.zip" download>Windows Agent 包</a>，首次双击“首次注册并启动Agent.cmd”。</li>
        <li v-else>当前本机 MVP 仅提供 Windows AMD64 Agent 下载包；请勿在此节点使用该安装包。</li>
        <li>生成一次性注册码，并在 Agent 窗口按提示粘贴。</li>
        <li>收到首次心跳后，点击“检查节点环境”；检查通过后才会显示“启用节点”。</li>
      </ol>
      <div class="agent-registration-actions">
        <button type="button" class="button button-primary" :disabled="enrollmentBusy" @click="issueEnrollment">{{ enrollmentBusy ? '正在生成…' : '生成一次性注册码' }}</button>
        <span>注册码仅显示一次，默认 15 分钟内有效。</span>
      </div>
      <details class="agent-registration-more">
        <summary>查看部署和网络要求</summary>
        <p>Agent 需要从目标机器通过 HTTPS 主动连接控制面，并信任控制面证书。本机 Local MVP 固定为回环 HTTPS，可验证实际注册、心跳、固定运行时检查与节点启用；它不提供 SSH、WinRM、远程 Shell 或任意命令。</p>
      </details>
    </section>

    <section class="node-overview">
      <div>
        <p class="detail-kicker">节点名称</p>
        <h2>{{ node.displayName }}</h2>
        <p class="node-id">{{ node.id }}</p>
      </div>
      <div class="availability" :class="{ available: node.acceptsNewTasks }">
        <strong>{{ node.acceptsNewTasks ? '当前可接收新任务' : '当前不可接收新任务' }}</strong>
        <span v-if="node.acceptsNewTasks">仍需在具体任务提交前完成固定预检查。</span>
        <template v-else>
          <span v-if="node.unavailableReasons.length">{{ unavailableReasonLabel(node.unavailableReasons[0] ?? '') }}</span>
          <span v-else>控制面未提供具体阻断原因。</span>
          <ul v-if="node.unavailableReasons.length > 1" class="availability-reasons">
            <li v-for="reason in node.unavailableReasons.slice(1)" :key="reason">{{ unavailableReasonLabel(reason) }}</li>
          </ul>
        </template>
      </div>
    </section>

    <section class="node-status-grid" aria-label="节点状态">
      <article><span>管理状态</span><strong><i class="status-dot" :class="statusDotClass(node.managementState)" />{{ managementLabel(node.managementState) }}</strong><small>由节点管理配置决定</small></article>
      <article><span>Agent 关联</span><strong><i class="status-dot" :class="statusDotClass(node.agentAssociationStatus)" />{{ associationLabel(node.agentAssociationStatus) }}</strong><small>{{ node.agentFacts ? '已收到只读机器事实' : '尚未收到关联后的机器事实' }}</small></article>
      <article><span>心跳状态</span><strong><i class="status-dot" :class="statusDotClass(node.heartbeatStatus)" />{{ heartbeatLabel(node.heartbeatStatus) }}</strong><small>最近心跳：{{ timeLabel(node.lastHeartbeatAt) }}</small></article>
      <article><span>工具运行时</span><strong><i class="status-dot" :class="statusDotClass(node.environmentStatus)" />{{ environmentLabel(node.environmentStatus) }}</strong><small>{{ node.environmentStatus === 'NOT_CHECKED' ? '尚未取得目标机器的工具核验事实' : '控制面投影的工具运行时核验结论' }}</small></article>
      <article><span>容量状态</span><strong><i class="status-dot" :class="statusDotClass(node.capacityStatus)" />{{ capacityLabel(node.capacityStatus) }}</strong><small>{{ capacitySummary(node) }}</small></article>
    </section>

    <section class="node-detail-grid">
      <section class="content-card node-detail-section">
        <h2>登记配置</h2>
        <dl class="summary-definition">
          <div><dt>目标平台</dt><dd>{{ platformLabel(node.platform) }}，实际 OS 与架构以后续 Agent 事实为准</dd></div>
          <div><dt>OB Loader/Dumper 目录</dt><dd><code>{{ node.toolHome }}</code></dd></div>
          <div><dt>工具专用 Java 8</dt><dd><code>{{ node.javaPath }}</code></dd></div>
          <div><dt>导出数据目录白名单</dt><dd><code v-for="root in node.allowedRoots" :key="root">{{ root }}</code></dd></div>
          <div><dt>配置版本</dt><dd>rev-{{ node.revision }}</dd></div>
          <div><dt>最近修改</dt><dd>{{ timeLabel(node.updatedAt) }}</dd></div>
        </dl>
      </section>

      <section class="content-card node-detail-section">
        <h2>导出数据目录空间</h2>
        <dl v-if="node.dataRootUsages?.length" class="summary-definition">
          <div v-for="usage in node.dataRootUsages" :key="usage.root"><dt><code>{{ usage.root }}</code></dt><dd>可用 {{ byteLabel(usage.availableBytes) }} / 总计 {{ byteLabel(usage.totalBytes) }}</dd></div>
        </dl>
        <p v-else class="facts-empty">尚未收到与当前节点配置匹配的数据目录空间采样。</p>
      </section>

      <section class="content-card node-detail-section">
        <h2>Agent 只读事实</h2>
        <dl v-if="node.agentFacts" class="summary-definition">
          <div><dt>操作系统 / 架构</dt><dd>{{ node.agentFacts.os }} / {{ node.agentFacts.arch }}</dd></div>
          <div><dt>Agent 版本</dt><dd>{{ node.agentFacts.agentVersion }}</dd></div>
          <div><dt>启动标识（摘要）</dt><dd>{{ bootIdSummary(node.agentFacts.bootId) }}</dd></div>
          <div><dt>事实采样时间</dt><dd>{{ timeLabel(node.agentFacts.observedAt) }}</dd></div>
          <div><dt>容量快照</dt><dd>{{ node.agentFacts.capacityUsed }} / {{ node.agentFacts.capacityTotal }}</dd></div>
          <div><dt>CPU / 内存</dt><dd>{{ percentageLabel(node.agentFacts.cpuUsagePercent) }} / {{ percentageLabel(node.agentFacts.memoryUsagePercent) }}</dd></div>
        </dl>
        <p v-else class="facts-empty">尚未收到可显示的 Agent 事实。</p>
      </section>
    </section>

    <section class="content-card node-boundary">
      <h2>准入边界</h2>
      <p>Agent 关联和心跳只证明机器身份及在线事实；工具运行时未核验时，节点保持不可接收新任务。即使工具已就绪，具体导出仍必须独立核验数据源、路径、对象和参数。</p>
    </section>
  </template>

  <div v-if="enrollmentDialogOpen" class="enrollment-backdrop" @click.self="clearEnrollment">
    <section class="enrollment-dialog" role="dialog" aria-modal="true" aria-labelledby="enrollment-dialog-title">
      <header class="enrollment-dialog-header">
        <div>
          <h2 id="enrollment-dialog-title">一次性 Agent 注册码</h2>
          <p>{{ node?.displayName }}</p>
        </div>
        <button type="button" class="button button-secondary" @click="clearEnrollment">关闭并清除</button>
      </header>

      <p v-if="enrollmentBusy" class="enrollment-status" role="status">正在生成注册码…</p>
      <template v-else-if="enrollmentFailure">
        <p class="feedback feedback-error" role="alert">{{ enrollmentFailure }}</p>
        <button type="button" class="button button-secondary" @click="issueEnrollment">重新生成</button>
      </template>
      <template v-else-if="enrollment">
        <p class="enrollment-intro">在目标机器启动与所选平台匹配的 Agent。出现注册提示后，粘贴下面的注册码并结束输入。</p>
        <label class="enrollment-material">
          一次性注册码
          <textarea :value="registrationCode" rows="5" readonly spellcheck="false" aria-label="一次性 Agent 注册码" />
        </label>
        <div class="enrollment-copy-actions">
          <button type="button" class="button button-secondary" @click="copyEnrollmentValue('注册码', registrationCode)">复制注册码</button>
          <span v-if="copyNotice" role="status">{{ copyNotice }}</span>
        </div>
        <p class="enrollment-warning">注册码仅本次显示，关闭对话框后会从当前页面内存清除，刷新后无法恢复。本页不安装 Agent、不执行命令。</p>
        <details class="enrollment-advanced">
          <summary>高级部署信息（首次安装或排障时使用）</summary>
          <section v-if="node?.platform === 'WINDOWS_AMD64'" class="enrollment-guide">
            <h3>Windows 本机 Agent 包</h3>
            <p>解压后保持 <code>agent.exe</code>、<code>agent-config.json</code>、<code>control-plane-ca.pem</code> 与两个启动脚本在同一目录。首次双击“首次注册并启动Agent.cmd”；后续双击“启动Agent.cmd”。注册码不会进入命令行参数或环境变量。</p>
            <div class="enrollment-command-heading">
              <h3>注册指令</h3>
              <button type="button" class="button button-secondary" @click="copyEnrollmentValue('注册指令', enrollmentCommand)">复制注册指令</button>
            </div>
            <pre><code>{{ enrollmentCommand }}</code></pre>
            <p>运行指令后，将上方注册码粘贴到标准输入并结束输入。</p>
          </section>
          <p v-else>当前本机 MVP 尚未提供此平台的 Agent 下载包，不能使用 Windows 下载包注册。</p>
        </details>
      </template>
    </section>
  </div>
</template>

<style scoped>
.heading-actions { display: flex; flex-wrap: wrap; gap: 9px; }
.agent-registration { display: grid; gap: 16px; margin-bottom: 16px; padding: 20px; border-left: 3px solid #2f6fd2; }
.agent-registration h2 { margin: 3px 0 7px; color: #334257; font-size: 18px; }
.agent-registration p { margin: 0; color: #66758a; font-size: 13px; line-height: 1.65; }
.agent-registration ol { display: grid; gap: 7px; margin: 0; padding-left: 22px; color: #536276; font-size: 13px; line-height: 1.6; }
.agent-registration-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; }
.agent-registration-actions span { color: #7a899c; font-size: 12px; }
.agent-registration-more, .enrollment-advanced { padding: 11px 12px; border: 1px solid #e0e7f0; background: #f8fafc; }
.agent-registration-more summary, .enrollment-advanced summary { cursor: pointer; color: #536276; font-size: 13px; font-weight: 600; }
.agent-registration-more p { margin-top: 10px; color: #7d5a2f; }
.node-overview { display: flex; align-items: end; justify-content: space-between; gap: 18px; margin-bottom: 16px; padding: 21px 0; border-bottom: 1px solid #e0e7f0; }
.node-overview h2 { margin: 3px 0 0; color: #334257; font-size: 20px; }
.node-id { margin: 5px 0 0; color: #8a98a9; font-size: 12px; }
.availability { display: grid; gap: 5px; max-width: 430px; color: #8d4e28; font-size: 13px; line-height: 1.55; }
.availability strong { color: #9c5229; font-size: 15px; }
.availability.available { color: #267249; }
.availability.available strong { color: #237348; }
.availability-reasons { display: grid; gap: 3px; margin: 2px 0 0; padding-left: 18px; }
.node-status-grid { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 12px; margin-bottom: 16px; }
.node-status-grid article { min-height: 112px; padding: 15px; border-top: 2px solid #c7d3e4; background: #fff; }
.node-status-grid span { display: block; color: #8593a4; font-size: 12px; }
.node-status-grid strong { display: block; margin: 8px 0 6px; color: #405067; font-size: 15px; }
.node-status-grid small { color: #738195; font-size: 12px; line-height: 1.5; }
.node-detail-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.node-detail-section { padding: 20px; }
.node-detail-section h2, .node-boundary h2 { margin: 0 0 16px; color: #405066; font-size: 16px; }
.summary-definition code { display: block; width: fit-content; max-width: 100%; margin: 3px 0; overflow-wrap: anywhere; color: #40566f; font: 12px/1.55 ui-monospace, SFMono-Regular, Consolas, monospace; }
.facts-empty, .node-boundary p { margin: 0; color: #738195; font-size: 13px; line-height: 1.65; }
.node-boundary { margin-top: 16px; padding: 20px; }
.enrollment-backdrop { position: fixed; inset: 0; z-index: 10; display: grid; place-items: center; padding: 18px; background: rgb(24 34 49 / 46%); }
.enrollment-dialog { width: min(640px, 100%); max-height: min(720px, calc(100vh - 36px)); overflow: auto; padding: 22px; border: 1px solid #d7e0eb; border-radius: 6px; background: #fff; box-shadow: 0 18px 44px rgb(15 23 42 / 22%); }
.enrollment-dialog-header { display: flex; align-items: start; justify-content: space-between; gap: 14px; margin-bottom: 18px; }
.enrollment-dialog-header h2 { margin: 0 0 5px; color: #334257; font-size: 17px; }
.enrollment-dialog-header p { margin: 0; color: #7a899c; font-size: 12px; }
.enrollment-status { margin: 0; color: #6f7e91; font-size: 13px; }
.enrollment-intro { margin: 0 0 16px; color: #536276; font-size: 13px; line-height: 1.65; }
.enrollment-meta { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; margin: 0 0 16px; }
.enrollment-meta div { padding: 10px 12px; border: 1px solid #e0e7f0; background: #f8fafc; }
.enrollment-meta dt { margin-bottom: 4px; color: #8290a3; font-size: 12px; }
.enrollment-meta dd { margin: 0; overflow-wrap: anywhere; color: #46566c; font-size: 13px; }
.enrollment-guide { margin-bottom: 16px; padding: 14px; border: 1px solid #e0e7f0; background: #f8fafc; }
.enrollment-guide h3 { margin: 0 0 10px; color: #405066; font-size: 14px; }
.enrollment-guide dl { display: grid; gap: 8px; margin: 0 0 10px; }
.enrollment-guide dl div { display: grid; gap: 4px; }
.enrollment-guide dt { color: #7a899c; font-size: 12px; }
.enrollment-guide dd { margin: 0; }
.enrollment-guide code { display: block; overflow-wrap: anywhere; color: #40566f; font: 12px/1.55 ui-monospace, SFMono-Regular, Consolas, monospace; }
.enrollment-guide p { margin: 8px 0 0; color: #738195; font-size: 12px; line-height: 1.6; }
.enrollment-command-heading { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-top: 14px; }
.enrollment-command-heading h3 { margin: 0; }
.enrollment-guide pre { margin: 8px 0 0; overflow-x: auto; padding: 10px; border: 1px solid #d7e0eb; background: #fff; white-space: pre-wrap; }
.enrollment-material { display: grid; gap: 7px; color: #536276; font-size: 13px; }
.enrollment-material textarea { width: 100%; min-height: 116px; resize: vertical; padding: 10px; border: 1px solid #cfd9e6; border-radius: 4px; color: #334257; background: #f8fafc; font: 12px/1.6 ui-monospace, SFMono-Regular, Consolas, monospace; overflow-wrap: anywhere; }
.enrollment-copy-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; margin-top: 10px; }
.enrollment-copy-actions span { color: #267249; font-size: 12px; }
.enrollment-warning { margin: 14px 0 0; color: #7a899c; font-size: 12px; line-height: 1.65; }
.enrollment-advanced { margin-top: 16px; }
.enrollment-advanced .enrollment-meta { margin: 14px 0 16px; }
@media (max-width: 1080px) { .node-status-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
@media (max-width: 720px) { .node-status-grid, .node-detail-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 620px) { .node-overview { align-items: start; flex-direction: column; }.node-status-grid, .node-detail-grid, .enrollment-meta { grid-template-columns: 1fr; }.enrollment-dialog-header { flex-direction: column; } }
</style>
