<script setup lang="ts">
import { ReloadOutlined } from '@ant-design/icons-vue'
import { Alert as AAlert, Badge as ABadge, Button as AButton, ConfigProvider, Modal as AModal, Textarea as ATextarea } from 'ant-design-vue'
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute } from 'vue-router'

import { browserApi, executionNodeErrorMessage, type ApiError, type ExecutionNodeDetail, type ExecutionNodeEnrollment } from '@/api/browser'
import { activateModal } from '@/components/modalFocus'
import { overlayTheme } from '@/platform/theme'
import { component } from '@/platform/tokens'
import { agentRegistrationCode, agentRegistrationCommand, requiresAgentRegistration } from './executionNodeEnrollmentInstructions'
import {
  nodeAssociationLabel, nodeCapacityLabel, nodeCapacitySummary, nodeEnvironmentLabel,
  nodeHeartbeatLabel, nodeManagementLabel, nodePlatformLabel, nodePrimaryAction,
  nodeTimeLabel, nodeUnavailableReasonLabel,
} from './executionNodePresentation'

const api = browserApi()
const route = useRoute()
const node = ref<ExecutionNodeDetail>()
const loading = ref(true)
const refreshing = ref(false)
const failure = ref('')
const actionFailure = ref('')
const actionNotice = ref('')
const actionBusy = ref(false)
const enrollmentDialogOpen = ref(false)
const enrollment = ref<ExecutionNodeEnrollment>()
const enrollmentBusy = ref(false)
const enrollmentFailure = ref('')
const copyNotice = ref('')
const modalCloseButton = ref<{ $el: HTMLButtonElement }>()
const nodeID = computed(() => typeof route.params.id === 'string' ? route.params.id : '')
const registrationJustCreated = computed(() => route.query.registration === 'created' && node.value?.agentAssociationStatus === 'PENDING')
const registrationRequired = computed(() => node.value !== undefined && requiresAgentRegistration(node.value.agentAssociationStatus))
const primaryAction = computed(() => node.value ? nodePrimaryAction(node.value) : undefined)
const enrollmentCommand = computed(() => node.value ? agentRegistrationCommand(node.value.platform) : '')
const registrationCode = computed(() => node.value && enrollment.value ? agentRegistrationCode(node.value.id, enrollment.value.enrollmentId, enrollment.value.enrollmentMaterial) : '')
const agentPackageUrl = computed(() => {
  const platforms = { WINDOWS_AMD64: 'windows-amd64', LINUX_AMD64: 'linux-amd64', LINUX_ARM64: 'linux-arm64' } as const
  return `/ob-data-orch-agent-${platforms[node.value?.platform ?? 'WINDOWS_AMD64']}.zip`
})
let requestSequence = 0
let enrollmentRequestVersion = 0
let refreshTimer: ReturnType<typeof setTimeout> | undefined
let enrollmentInvoker: HTMLElement | null = null

watch(nodeID, () => {
  clearEnrollment()
  node.value = undefined
  void loadNode()
}, { immediate: true })
onBeforeUnmount(() => { requestSequence++; clearTimeout(refreshTimer); clearEnrollment() })

// 注册码弹层用共享焦点栈约束键盘，并在关闭时将焦点还给发起按钮。
watch([enrollmentDialogOpen, modalCloseButton], async ([open], _previous, cleanup) => {
  if (!open) return
  let disposed = false
  let release: (() => void) | undefined
  cleanup(() => { disposed = true; release?.() })
  await nextTick()
  const button = modalCloseButton.value?.$el
  const root = button?.closest<HTMLElement>('[role="dialog"]')
  if (!disposed && root && button) release = activateModal(root, button, clearEnrollment, enrollmentInvoker)
}, { flush: 'post' })

async function loadNode(preserve = false) {
  const id = nodeID.value
  if (!id) { failure.value = '未指定执行节点。'; loading.value = false; return }
  const sequence = ++requestSequence
  if (preserve) refreshing.value = true
  else loading.value = true
  failure.value = ''
  try {
    const result = await api.getExecutionNode(id)
    if (sequence === requestSequence) node.value = result
  } catch (error) {
    if (sequence === requestSequence) {
      if ([401, 403, 404].includes((error as Partial<ApiError>).status ?? 0)) node.value = undefined
      failure.value = executionNodeErrorMessage(error, '执行节点加载失败。')
    }
  } finally {
    if (sequence === requestSequence) { loading.value = false; refreshing.value = false }
  }
}

async function performPrimaryAction() {
  const current = node.value
  const action = primaryAction.value
  if (!current || !action || actionBusy.value) return
  actionBusy.value = true
  actionFailure.value = ''
  actionNotice.value = ''
  try {
    node.value = action === 'enable'
      ? await api.enableExecutionNode(current.id, current.revision)
      : await api.requestExecutionNodeEnvironmentCheck(current.id, current.revision)
    actionNotice.value = action === 'enable'
      ? '节点已启用；提交具体任务前仍需任务级预检查。'
      : '环境检查已请求，等待 Agent 回传固定运行时结果。'
    if (action === 'environment-check') refreshTimer = setTimeout(() => { void loadNode(true) }, 2500)
  } catch (error) {
    actionFailure.value = executionNodeErrorMessage(error, action === 'enable' ? '执行节点启用失败。' : '环境检查请求失败。')
    await loadNode(true)
  } finally {
    actionBusy.value = false
  }
}

async function issueEnrollment() {
  if (!node.value || !registrationRequired.value || enrollmentBusy.value) return
  const requestVersion = ++enrollmentRequestVersion
  if (!enrollmentDialogOpen.value) enrollmentInvoker = document.activeElement instanceof HTMLElement ? document.activeElement : null
  enrollmentDialogOpen.value = true
  enrollment.value = undefined
  enrollmentFailure.value = ''
  copyNotice.value = ''
  enrollmentBusy.value = true
  try {
    const result = await api.issueExecutionNodeEnrollment(node.value.id)
    if (requestVersion === enrollmentRequestVersion && enrollmentDialogOpen.value) enrollment.value = result
  } catch (error) {
    if (requestVersion === enrollmentRequestVersion && enrollmentDialogOpen.value) enrollmentFailure.value = executionNodeErrorMessage(error, '关联材料签发失败。')
  } finally {
    if (requestVersion === enrollmentRequestVersion) enrollmentBusy.value = false
  }
}

function clearEnrollment() {
  enrollmentRequestVersion++
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

function percentageLabel(value: number | undefined) {
  return value === undefined ? '尚未采集' : `${value.toFixed(1)}%`
}

function byteLabel(value: number) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = value
  let index = 0
  while (size >= 1024 && index < units.length - 1) { size /= 1024; index++ }
  return `${size.toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

function bootIdSummary(value: string) {
  return value.length <= 20 ? value : `${value.slice(0, 12)}…${value.slice(-4)}`
}
</script>

<template>
  <div class="orch-ui node-detail">
    <header class="node-detail-header">
      <div>
        <RouterLink to="/nodes" class="node-back">返回执行节点</RouterLink>
        <h1>{{ node?.displayName || '执行节点详情' }}</h1>
        <p v-if="node" class="node-identifier">{{ node.id }} · {{ nodePlatformLabel(node.platform) }}</p>
      </div>
      <div class="node-detail-actions">
        <AButton :disabled="loading || refreshing" aria-label="刷新节点详情" @click="loadNode(true)"><template #icon><ReloadOutlined :spin="refreshing" aria-hidden="true" /></template>刷新</AButton>
        <AButton v-if="node" :href="agentPackageUrl" download>下载 Agent 包</AButton>
        <RouterLink v-if="node" v-slot="{ href, navigate }" :to="`/nodes/${node.id}/edit`" custom><AButton :href="href" @click="navigate">编辑节点</AButton></RouterLink>
        <AButton v-if="primaryAction && !registrationRequired" type="primary" :loading="actionBusy" @click="performPrimaryAction">{{ primaryAction === 'enable' ? '启用节点' : '检查节点环境' }}</AButton>
      </div>
    </header>

    <div v-if="loading" class="node-loading" role="status">正在加载执行节点…</div>
    <AAlert v-else-if="failure && !node" type="error" message="无法打开执行节点" :description="failure" show-icon><template #action><AButton @click="loadNode()">重试</AButton></template></AAlert>
    <template v-else-if="node">
      <AAlert v-if="failure" class="node-feedback" type="error" message="刷新未完成" :description="`${failure} 当前仍展示上一次读取的事实。`" show-icon />
      <AAlert v-if="actionFailure" class="node-feedback" type="error" message="操作未完成" :description="actionFailure" show-icon />
      <AAlert v-if="actionNotice" class="node-feedback" type="success" :message="actionNotice" show-icon />
      <AAlert v-if="registrationJustCreated" class="node-feedback" type="info" message="节点记录已创建" description="生成一次性注册码，在目标机器上启动对应 Agent 并完成关联。" show-icon />

      <section class="node-result" aria-labelledby="node-result-title">
        <div>
          <p class="node-eyebrow">通用任务资格</p>
          <h2 id="node-result-title"><ABadge :status="node.acceptsNewTasks ? 'success' : 'warning'" :text="node.acceptsNewTasks ? '当前可接收新任务' : '当前不可接收新任务'" /></h2>
          <p v-if="node.acceptsNewTasks">具体任务仍需核验数据源、路径、对象和参数。</p>
          <template v-else>
            <p>{{ nodeUnavailableReasonLabel(node.unavailableReasons[0] ?? '') }}</p>
            <ul v-if="node.unavailableReasons.length > 1"><li v-for="reason in node.unavailableReasons.slice(1)" :key="reason">{{ nodeUnavailableReasonLabel(reason) }}</li></ul>
          </template>
        </div>
        <span class="node-result-time">配置更新：{{ nodeTimeLabel(node.updatedAt) }}</span>
      </section>

      <section v-if="registrationRequired" class="node-registration" aria-labelledby="node-registration-title">
        <div><h2 id="node-registration-title">完成 Agent 关联</h2><p>下载与目标平台匹配的包，在目标机器运行唯一“启动”入口，然后粘贴本页生成的一次性注册码。无需填写节点 IP。</p></div>
        <div class="node-registration-actions"><AButton :href="agentPackageUrl" download>下载 Agent 包</AButton><AButton type="primary" :loading="enrollmentBusy" @click="issueEnrollment">生成一次性注册码</AButton></div>
        <span>注册码只显示一次，24 小时内有效；收到心跳并取得环境事实后才能启用。</span>
      </section>

      <section class="node-facts" aria-label="节点运行状态">
        <div><span>管理状态</span><strong><ABadge :status="node.managementState === 'ENABLED' ? 'success' : node.managementState === 'MAINTENANCE' ? 'warning' : 'default'" :text="nodeManagementLabel(node.managementState)" /></strong></div>
        <div><span>Agent 关联</span><strong><ABadge :status="node.agentAssociationStatus === 'ASSOCIATED' ? 'success' : 'default'" :text="nodeAssociationLabel(node.agentAssociationStatus)" /></strong></div>
        <div><span>心跳</span><strong><ABadge :status="node.heartbeatStatus === 'ONLINE' ? 'success' : node.heartbeatStatus === 'OFFLINE' ? 'error' : 'default'" :text="nodeHeartbeatLabel(node.heartbeatStatus)" /></strong><small>最近心跳：{{ nodeTimeLabel(node.lastHeartbeatAt) }}</small></div>
        <div><span>工具运行时</span><strong><ABadge :status="node.environmentStatus === 'NORMAL' ? 'success' : node.environmentStatus === 'ABNORMAL' ? 'error' : node.environmentStatus === 'EXPIRED' ? 'warning' : 'default'" :text="nodeEnvironmentLabel(node.environmentStatus)" /></strong></div>
        <div><span>任务容量</span><strong><ABadge :status="node.capacityStatus === 'AVAILABLE' ? 'success' : node.capacityStatus === 'BUSY' ? 'warning' : 'default'" :text="nodeCapacityLabel(node.capacityStatus)" /></strong><small>{{ nodeCapacitySummary(node) }}</small></div>
      </section>

      <div class="node-evidence">
        <section aria-labelledby="node-config-title">
          <h2 id="node-config-title">登记配置</h2>
          <dl class="node-definition">
            <div><dt>目标平台</dt><dd>{{ nodePlatformLabel(node.platform) }}</dd></div>
            <div><dt>OB Loader/Dumper 目录</dt><dd><code>{{ node.toolHome }}</code></dd></div>
            <div><dt>工具专用 Java 8</dt><dd><code>{{ node.javaPath }}</code></dd></div>
            <div><dt>导出数据目录白名单</dt><dd><code v-for="root in node.allowedRoots" :key="root">{{ root }}</code></dd></div>
            <div><dt>配置版本</dt><dd>rev-{{ node.revision }}</dd></div>
            <div><dt>最近修改</dt><dd>{{ nodeTimeLabel(node.updatedAt) }}</dd></div>
          </dl>
        </section>
        <section aria-labelledby="node-agent-title">
          <h2 id="node-agent-title">Agent 只读事实</h2>
          <dl v-if="node.agentFacts" class="node-definition">
            <div><dt>操作系统 / 架构</dt><dd>{{ node.agentFacts.os }} / {{ node.agentFacts.arch }}</dd></div>
            <div><dt>Agent 版本</dt><dd>{{ node.agentFacts.agentVersion }}</dd></div>
            <div><dt>启动标识摘要</dt><dd><code>{{ bootIdSummary(node.agentFacts.bootId) }}</code></dd></div>
            <div><dt>事实采样时间</dt><dd>{{ nodeTimeLabel(node.agentFacts.observedAt) }}</dd></div>
            <div><dt>容量快照</dt><dd>{{ node.agentFacts.capacityUsed }} / {{ node.agentFacts.capacityTotal }}</dd></div>
            <div><dt>CPU / 内存</dt><dd>{{ percentageLabel(node.agentFacts.cpuUsagePercent) }} / {{ percentageLabel(node.agentFacts.memoryUsagePercent) }}</dd></div>
          </dl>
          <p v-else class="node-empty-fact">尚未收到可显示的 Agent 事实。</p>
        </section>
        <section class="node-root-usage" aria-labelledby="node-roots-title">
          <h2 id="node-roots-title">导出数据目录空间</h2>
          <dl v-if="node.dataRootUsages?.length" class="node-definition">
            <div v-for="usage in node.dataRootUsages" :key="usage.root"><dt><code>{{ usage.root }}</code></dt><dd>可用 {{ byteLabel(usage.availableBytes) }} / 总计 {{ byteLabel(usage.totalBytes) }}</dd></div>
          </dl>
          <p v-else class="node-empty-fact">尚未收到与当前节点配置匹配的目录空间采样。</p>
        </section>
      </div>
      <p class="node-boundary">心跳只证明最近在线事实，旧资源快照不能视为实时数据。节点通用资格也不能代替任务级预检查。</p>
    </template>
  </div>

  <ConfigProvider :theme="overlayTheme">
    <AModal :open="enrollmentDialogOpen" title="一次性 Agent 注册码" :width="component.overlay.confirmWidth" :footer="null" :mask-closable="false" :destroy-on-close="true" :focus-trigger-after-close="false" @cancel="clearEnrollment">
      <div class="node-enrollment-dialog">
        <p class="node-enrollment-context">{{ node?.displayName }} · {{ node ? nodePlatformLabel(node.platform) : '' }}</p>
        <p v-if="enrollmentBusy" role="status">正在生成注册码…</p>
        <template v-else-if="enrollmentFailure"><AAlert type="error" message="注册码签发失败" :description="enrollmentFailure" show-icon /><AButton @click="issueEnrollment">重新生成</AButton></template>
        <template v-else-if="enrollment">
          <p>在目标机器启动对应 Agent，出现注册提示后粘贴以下注册码。注册码不进入命令行或环境变量。</p>
          <label class="node-enrollment-material">一次性注册码<ATextarea :value="registrationCode" :rows="5" readonly spellcheck="false" aria-label="一次性 Agent 注册码" /></label>
          <div class="node-copy-actions"><AButton @click="copyEnrollmentValue('注册码', registrationCode)">复制注册码</AButton><span v-if="copyNotice" role="status">{{ copyNotice }}</span></div>
          <p class="node-enrollment-warning">注册码仅本次显示；关闭后从页面内存清除，刷新后无法恢复。复制后请留意剪贴板中的材料。</p>
          <details class="node-enrollment-advanced"><summary>查看首次安装指引</summary><p>安装包包含控制面地址与信任配置，后续正常重启和就地升级保留 Agent 身份。</p><p>在目标机器运行 <code>{{ enrollmentCommand }}</code>，按提示粘贴注册码并结束输入。</p></details>
        </template>
        <div class="node-enrollment-footer"><AButton ref="modalCloseButton" @click="clearEnrollment">关闭并清除</AButton></div>
      </div>
    </AModal>
  </ConfigProvider>
</template>

<style scoped>
.node-detail { min-width: 0; }
.node-detail-header { display: flex; align-items: flex-end; justify-content: space-between; flex-wrap: wrap; gap: var(--ob-foundation-space-4); margin-bottom: var(--ob-foundation-space-6); }
.node-back { display: inline-block; margin-bottom: var(--ob-foundation-space-2); color: var(--ob-color-primary); }
.node-detail-header h1 { margin: 0; color: var(--ob-color-text); font-size: var(--ob-product-typography-title-size); line-height: 32px; }
.node-identifier { margin: var(--ob-foundation-space-1) 0 0; color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); overflow-wrap: anywhere; }
.node-detail-actions, .node-registration-actions, .node-copy-actions { display: flex; flex-wrap: wrap; align-items: center; gap: var(--ob-foundation-space-2); }
.node-loading { padding: var(--ob-foundation-space-8); color: var(--ob-color-secondary); text-align: center; }
.node-feedback { margin-bottom: var(--ob-foundation-space-4); }
.node-result { display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: var(--ob-foundation-space-4); padding: var(--ob-foundation-space-6); border: 1px solid var(--ob-color-border); background: var(--ob-color-surface); }
.node-eyebrow { margin: 0 0 var(--ob-foundation-space-2); color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); }
.node-result h2 { margin: 0; font-size: var(--ob-product-typography-section-size); }
.node-result p:not(.node-eyebrow), .node-result li { margin: var(--ob-foundation-space-2) 0 0; color: var(--ob-color-form-secondary); font-size: var(--ob-component-field-label-size); line-height: 1.6; }
.node-result ul { margin: var(--ob-foundation-space-1) 0 0; padding-left: var(--ob-foundation-space-6); }
.node-result-time { color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); }
.node-registration { display: grid; gap: var(--ob-foundation-space-3); margin-top: var(--ob-foundation-space-4); padding: var(--ob-foundation-space-4) var(--ob-foundation-space-6); border: 1px solid var(--ob-color-border); border-left: 3px solid var(--ob-color-primary); background: var(--ob-color-surface); }
.node-registration h2 { margin: 0 0 var(--ob-foundation-space-1); font-size: var(--ob-product-typography-section-size); }
.node-registration p { margin: 0; color: var(--ob-color-form-secondary); line-height: 1.6; }
.node-registration > span { color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); }
.node-facts { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); margin-top: var(--ob-foundation-space-4); border: 1px solid var(--ob-color-border); background: var(--ob-color-surface); }
.node-facts > div { min-width: 0; padding: var(--ob-foundation-space-4); border-right: 1px solid var(--ob-color-border); }
.node-facts > div:last-child { border-right: 0; }
.node-facts > div > span { display: block; margin-bottom: var(--ob-foundation-space-2); color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); }
.node-facts strong { display: block; font-weight: 500; }
.node-facts small { display: block; margin-top: var(--ob-foundation-space-1); color: var(--ob-color-secondary); overflow-wrap: anywhere; }
.node-evidence { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--ob-foundation-space-6); margin-top: var(--ob-foundation-space-6); }
.node-evidence section { min-width: 0; padding-top: var(--ob-foundation-space-4); border-top: 1px solid var(--ob-color-border); }
.node-evidence h2 { margin: 0 0 var(--ob-foundation-space-3); color: var(--ob-color-text); font-size: var(--ob-product-typography-section-size); }
.node-root-usage { grid-column: 1 / -1; }
.node-definition { margin: 0; }
.node-definition > div { display: grid; grid-template-columns: minmax(110px, 32%) minmax(0, 1fr); gap: var(--ob-foundation-space-3); padding: var(--ob-foundation-space-2) 0; border-bottom: 1px solid var(--ob-color-border); }
.node-definition dt { color: var(--ob-color-secondary); font-size: var(--ob-component-field-label-size); }
.node-definition dd { min-width: 0; margin: 0; color: var(--ob-color-text); overflow-wrap: anywhere; }
.node-definition code { display: block; max-width: 100%; color: inherit; font: var(--ob-component-field-label-size)/1.6 var(--ob-foundation-font-mono); overflow-wrap: anywhere; }
.node-empty-fact, .node-boundary { color: var(--ob-color-secondary); font-size: var(--ob-component-field-label-size); line-height: 1.6; }
.node-boundary { margin-top: var(--ob-foundation-space-6); }
.node-enrollment-dialog p { color: var(--ob-color-form-secondary); line-height: 1.6; }
.node-enrollment-context { margin-top: 0; color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); }
.node-enrollment-material { display: grid; gap: var(--ob-foundation-space-2); color: var(--ob-color-form-secondary); }
.node-copy-actions { margin-top: var(--ob-foundation-space-3); }
.node-enrollment-warning { font-size: var(--ob-component-field-helper-size); }
.node-enrollment-advanced { padding: var(--ob-foundation-space-3); border: 1px solid var(--ob-color-border); background: var(--ob-color-subtle); }
.node-enrollment-advanced summary { cursor: pointer; }
.node-enrollment-advanced code { font-family: var(--ob-foundation-font-mono); overflow-wrap: anywhere; }
.node-enrollment-footer { display: flex; justify-content: flex-end; margin-top: var(--ob-foundation-space-4); }
@media (max-width: 1080px) { .node-facts { grid-template-columns: repeat(3, minmax(0, 1fr)); }.node-facts > div { border-bottom: 1px solid var(--ob-color-border); } }
@media (max-width: 720px) { .node-facts, .node-evidence { grid-template-columns: 1fr; }.node-facts > div { border-right: 0; }.node-root-usage { grid-column: auto; }.node-definition > div { grid-template-columns: 1fr; gap: var(--ob-foundation-space-1); } }
</style>
