<script setup lang="ts">
import { CheckCircleOutlined, ReloadOutlined } from '@ant-design/icons-vue'
import { Alert as AAlert, Badge as ABadge, Button as AButton, ConfigProvider, Modal as AModal, Spin as ASpin, Tabs as ATabs, TabPane as ATabPane, Textarea as ATextarea } from 'ant-design-vue'
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { activateModal } from '@/components/modalFocus'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { overlayTheme } from '@/platform/theme'
import { component } from '@/platform/tokens'
import { agentRegistrationCommand } from '@/workbench/nodes/executionNodeEnrollmentInstructions'
import { useNodeDetail } from '@/workbench/nodes/useNodeDetail'
import { useNodeEnrollment } from '@/workbench/nodes/useNodeEnrollment'
import NodeEnvironmentConfiguration from '@/workbench/nodes/NodeEnvironmentConfiguration.vue'
import NodeRuntimeOverview from '@/workbench/nodes/NodeRuntimeOverview.vue'
import { nodeAgentPackageURL, nodePlatformHelp } from '@/workbench/nodes/nodePlatformRules'
import {
  nodeCategoryStates, nodeCapacitySummary, nodePlatformLabel, nodeTimeLabel, nodeUnavailableReasonLabel,
} from '@/workbench/nodes/executionNodePresentation'

const route = useRoute()
const nodeID = computed(() => typeof route.params.id === 'string' ? route.params.id : '')
const { session, node, loading, refreshing, refreshCompleted, lastLoadedAt, failure, actionFailure, actionNotice, actionBusy, primaryAction, loadNode, performPrimaryAction } = useNodeDetail(nodeID)
const { enrollmentDialogOpen, enrollment, enrollmentBusy, enrollmentFailure, copyNotice, copyFailure, registrationRequired, registrationCode, issueEnrollment: requestEnrollment, clearEnrollment, copyEnrollmentValue } = useNodeEnrollment(node, session)
const { MessageContext } = useOperationMessage([{ content: actionNotice }, { content: copyNotice }])
const modalCloseButton = ref<{ $el: HTMLButtonElement }>()
const activeTab = ref('overview')
const states = computed(() => node.value ? nodeCategoryStates(node.value) : [])
const environmentChecking = computed(() => states.value.some(state => state.key === 'environment' && state.status === 'processing'))
// Tab 只属于当前详情工作面；对象或授权失效时回到概览，不恢复旧节点的配置区。
watch(node, value => { if (!value) activeTab.value = 'overview' }, { flush: 'sync' })
let enrollmentInvoker: HTMLElement | null = null
const enrollmentCommand = computed(() => node.value ? agentRegistrationCommand(node.value.platform) : '')
const agentPackageUrl = computed(() => nodeAgentPackageURL(node.value?.platform ?? 'WINDOWS_AMD64'))
const platformHelp = computed(() => node.value ? nodePlatformHelp(node.value.platform) : undefined)
function issueEnrollment() {
  if (!enrollmentDialogOpen.value) enrollmentInvoker = document.activeElement instanceof HTMLElement ? document.activeElement : null
  void requestEnrollment()
}

// 当前 Ant Tabs 仅原生处理 Enter/Space；方向键在本页按可见 Tab 顺序移动并激活。
// 只处理 Tab 自身的按键，避免影响配置文本、资源 Tooltip 或弹层中的键盘操作。
function navigateTabs(event: KeyboardEvent) {
  if (!(event.target instanceof HTMLElement) || event.target.getAttribute('role') !== 'tab') return
  const tabs = [...(event.target.closest('[role="tablist"]')?.querySelectorAll<HTMLElement>('[role="tab"]') ?? [])]
    .filter(tab => tab.getAttribute('aria-disabled') !== 'true')
  const current = tabs.indexOf(event.target)
  if (current < 0 || !tabs.length) return
  let next: number
  if (event.key === 'ArrowRight') next = (current + 1) % tabs.length
  else if (event.key === 'ArrowLeft') next = (current + tabs.length - 1) % tabs.length
  else if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = tabs.length - 1
  else return
  event.preventDefault()
  tabs[next]?.focus()
  tabs[next]?.click()
}

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

</script>

<template>
  <MessageContext />
  <div class="orch-ui node-detail">
    <header class="node-detail-header">
      <div>
        <div class="node-detail-title"><h1>{{ node?.displayName || '执行节点详情' }}</h1><span v-if="node">{{ nodePlatformLabel(node.platform) }}</span></div>
      </div>
      <div class="node-detail-actions" role="group" aria-label="节点操作">
        <RouterLink v-slot="{ href, navigate }" to="/nodes" custom><AButton :href="href" @click="navigate">返回执行节点</AButton></RouterLink>
        <AButton :disabled="loading || refreshing || actionBusy" :loading="refreshing" aria-label="刷新节点详情" @click="loadNode(true)"><template #icon><CheckCircleOutlined v-if="refreshCompleted" aria-hidden="true" /><ReloadOutlined v-else aria-hidden="true" /></template><span role="status" aria-live="polite">{{ refreshing ? '刷新中' : refreshCompleted ? '已刷新' : '刷新' }}</span></AButton>
        <AButton v-if="node" :href="agentPackageUrl" download>下载 Agent 包</AButton>
        <RouterLink v-if="node" v-slot="{ href, navigate }" :to="`/nodes/${node.id}/edit`" custom><AButton :href="href" @click="navigate">编辑节点</AButton></RouterLink>
        <AButton v-if="registrationRequired" type="primary" :loading="enrollmentBusy" @click="issueEnrollment">生成一次性注册码</AButton>
        <AButton v-else-if="primaryAction" type="primary" :loading="actionBusy" @click="performPrimaryAction">{{ primaryAction === 'enable' ? '启用节点' : '检查节点环境' }}</AButton>
        <AButton v-else-if="environmentChecking" disabled>环境检查中</AButton>
      </div>
    </header>

    <div v-if="loading" class="node-loading" role="status">正在加载执行节点…</div>
    <AAlert v-else-if="failure && !node" type="error" message="无法打开执行节点" :description="failure" show-icon><template #action><AButton @click="loadNode()">重试</AButton></template></AAlert>
    <template v-else-if="node">
      <AAlert v-if="failure" class="node-feedback" type="error" message="刷新未完成" :description="`${failure} 当前仍展示上一次读取的事实。`" show-icon />
      <AAlert v-if="actionFailure" class="node-feedback" type="error" message="操作未完成" :description="actionFailure" show-icon />

      <section class="node-facts" aria-label="节点运行状态">
        <div v-for="state in states" :key="state.key" :data-state-category="state.key">
          <span>{{ state.key === 'management' ? '管理状态' : state.key === 'agent' ? 'Agent 状态' : state.label }}</span>
          <ABadge :status="state.status" :text="state.text" />
          <small v-if="state.key === 'agent'">最近心跳：{{ nodeTimeLabel(node.lastHeartbeatAt) }}</small>
          <small v-if="state.key === 'capacity'">槽位快照：{{ nodeCapacitySummary(node) }}</small>
        </div>
      </section>

      <p v-if="node.acceptsNewTasks" class="node-eligibility"><ABadge status="success" text="当前可接收新任务" /><span>具体任务仍需核验数据源、路径、对象和参数。</span></p>
      <AAlert v-else class="node-feedback node-eligibility-alert" type="warning" message="当前不可接收新任务" show-icon>
        <template #description>
          <p class="node-blocked-first">{{ nodeUnavailableReasonLabel(node.unavailableReasons[0] ?? '', node) }}</p>
          <ul v-if="node.unavailableReasons.length > 1" class="node-blocked-reasons"><li v-for="reason in node.unavailableReasons.slice(1)" :key="reason">{{ nodeUnavailableReasonLabel(reason, node) }}</li></ul>
        </template>
      </AAlert>
      <AAlert v-if="primaryAction === 'environment-check' || environmentChecking" class="node-feedback" type="info" message="环境检查期间节点保持禁用" description="读取机器和目录事实，并运行固定 Java / 工具版本查询；检查通过后需单独启用节点。具体任务仍需独立预检查。" show-icon />

      <section v-if="registrationRequired" class="node-registration" aria-labelledby="node-registration-title">
        <div><h2 id="node-registration-title">完成 Agent 关联</h2><p>{{ platformHelp?.agentInstallHint }}按提示粘贴本页生成的一次性注册码。</p></div>
        <span>注册码只显示一次，24 小时内有效；收到心跳并取得环境事实后才能启用。</span>
      </section>

      <section class="node-detail-workspace" aria-label="节点详情工作面">
        <ASpin :spinning="refreshing">
          <ATabs v-model:active-key="activeTab" :destroy-inactive-tab-pane="true" aria-label="节点详情内容" @keydown="navigateTabs">
            <ATabPane key="overview" tab="运行概览"><NodeRuntimeOverview :node="node" /></ATabPane>
            <ATabPane key="configuration" tab="环境与配置"><NodeEnvironmentConfiguration :node="node" /></ATabPane>
          </ATabs>
        </ASpin>
        <footer v-if="lastLoadedAt" class="node-detail-freshness">最新刷新：{{ lastLoadedAt }}</footer>
      </section>
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
          <div class="node-copy-actions"><AButton @click="copyEnrollmentValue('注册码', registrationCode)">复制注册码</AButton></div>
          <AAlert v-if="copyFailure" class="node-feedback" type="error" :message="copyFailure" show-icon />
          <p class="node-enrollment-warning">注册码仅本次显示；关闭后从页面内存清除，刷新后无法恢复。复制后请留意剪贴板中的材料。</p>
          <div class="node-enrollment-advanced"><p>{{ platformHelp?.agentInstallHint }}</p><p>在目标机器的 Agent 解压目录运行 <code>{{ enrollmentCommand }}</code>，按提示粘贴注册码并结束输入。</p><p>安装包包含控制面地址与信任配置，后续正常重启和就地升级保留 Agent 身份。</p></div>
        </template>
        <div class="node-enrollment-footer"><AButton ref="modalCloseButton" @click="clearEnrollment">关闭并清除</AButton></div>
      </div>
    </AModal>
  </ConfigProvider>
</template>

<style scoped>
.node-detail { min-width: 0; }
.node-detail-header { display: flex; align-items: flex-end; justify-content: space-between; flex-wrap: wrap; gap: var(--ob-foundation-space-4); margin-bottom: var(--ob-foundation-space-4); }
.node-detail-header > div:first-child { flex: 1 1 320px; min-width: 0; }
.node-detail-title { display: flex; align-items: baseline; flex-wrap: wrap; gap: var(--ob-foundation-space-3); }
.node-detail-title > span { color: var(--ob-color-secondary); font-size: var(--ob-component-field-label-size); }
.node-detail-header h1 { margin: 0; color: var(--ob-color-text); font-size: var(--ob-product-typography-title-size); line-height: 32px; overflow-wrap: anywhere; }
.node-detail-actions, .node-copy-actions { display: flex; flex-wrap: wrap; align-items: center; gap: var(--ob-foundation-space-2); }
.node-detail-actions { margin-left: auto; justify-content: flex-end; }
.node-loading { padding: var(--ob-foundation-space-8); color: var(--ob-color-secondary); text-align: center; }
.node-feedback { margin-bottom: var(--ob-foundation-space-4); }
.node-eligibility { display: flex; align-items: baseline; flex-wrap: wrap; gap: var(--ob-foundation-space-3); margin: var(--ob-foundation-space-3) 0; }
.node-eligibility > span { color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); }
.node-eligibility-alert { margin-top: var(--ob-foundation-space-3); }
.node-blocked-first { margin: 0; }
.node-blocked-reasons { margin-top: var(--ob-foundation-space-2); margin-bottom: 0; padding-left: var(--ob-foundation-space-4); }
.node-registration { display: grid; gap: var(--ob-foundation-space-3); margin-bottom: var(--ob-foundation-space-4); padding: var(--ob-foundation-space-4) var(--ob-foundation-space-6); border: 1px solid var(--ob-color-border); background: var(--ob-color-surface); }
.node-registration h2 { margin: 0 0 var(--ob-foundation-space-1); font-size: var(--ob-product-typography-section-size); }
.node-registration p { margin: 0; color: var(--ob-color-form-secondary); line-height: 1.6; }
.node-registration > span { color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); }
.node-facts { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 1px; border: 1px solid var(--ob-color-border); background: var(--ob-color-border); }
.node-facts > div { min-width: 0; padding: var(--ob-foundation-space-4); background: var(--ob-color-surface); }
.node-facts > div > span { display: block; margin-bottom: var(--ob-foundation-space-2); color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); }
.node-facts small { display: block; margin-top: var(--ob-foundation-space-1); color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); overflow-wrap: anywhere; }
.node-detail-workspace { min-width: 0; margin-top: var(--ob-foundation-space-4); padding: var(--ob-foundation-space-4) var(--ob-foundation-space-6); border: 1px solid var(--ob-color-border); background: var(--ob-color-surface); }
.node-detail-freshness { margin-top: var(--ob-foundation-space-4); padding-top: var(--ob-foundation-space-3); border-top: 1px solid var(--ob-color-border); color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); text-align: right; }
.node-enrollment-dialog p { color: var(--ob-color-form-secondary); line-height: 1.6; }
.node-enrollment-context { margin-top: 0; color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); }
.node-enrollment-material { display: grid; gap: var(--ob-foundation-space-2); color: var(--ob-color-form-secondary); }
.node-copy-actions { margin-top: var(--ob-foundation-space-3); }
.node-enrollment-warning { font-size: var(--ob-component-field-helper-size); }
.node-enrollment-advanced { padding: var(--ob-foundation-space-3); border: 1px solid var(--ob-color-border); background: var(--ob-color-subtle); }
.node-enrollment-advanced code { font-family: var(--ob-foundation-font-mono); overflow-wrap: anywhere; }
.node-enrollment-footer { display: flex; justify-content: flex-end; margin-top: var(--ob-foundation-space-4); }
@media (max-width: 1080px) { .node-facts { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 720px) { .node-detail-workspace, .node-registration { padding-inline: var(--ob-foundation-space-4); } }
@media (max-width: 480px) { .node-facts { grid-template-columns: 1fr; } }
</style>
