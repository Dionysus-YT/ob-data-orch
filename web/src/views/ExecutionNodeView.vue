<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { browserApi, executionNodeErrorMessage, type ExecutionNodeSummary } from '@/api/browser'

const api = browserApi()
const nodes = ref<ExecutionNodeSummary[]>([])
const keyword = ref('')
const loading = ref(true)
const failure = ref('')
const nodeActionBusy = ref('')
const nodeActionNotice = ref('')
const nodeDeletionTarget = ref<ExecutionNodeSummary>()
const nodeDeletionBusy = ref('')

const filteredNodes = computed(() => {
  const query = keyword.value.trim().toLocaleLowerCase()
  if (!query) return nodes.value
  return nodes.value.filter((node) => node.displayName.toLocaleLowerCase().includes(query) || node.id.toLocaleLowerCase().includes(query))
})

onMounted(loadNodes)

async function loadNodes() {
  loading.value = true
  failure.value = ''
  try {
    nodes.value = await api.listExecutionNodes()
  } catch (error) {
    failure.value = executionNodeErrorMessage(error, '执行节点加载失败。')
  } finally {
    loading.value = false
  }
}

function primaryNodeAction(node: ExecutionNodeSummary): 'environment-check' | 'enable' | undefined {
  if (node.managementState !== 'DISABLED' || node.agentAssociationStatus !== 'ASSOCIATED' || node.heartbeatStatus !== 'ONLINE') return undefined
  if (node.unavailableReasons.includes('ENVIRONMENT_CHECK_IN_PROGRESS')) return undefined
  if (node.environmentStatus === 'NORMAL' && node.capacityStatus === 'AVAILABLE') return 'enable'
  return 'environment-check'
}

function primaryNodeActionLabel(node: ExecutionNodeSummary) {
  return primaryNodeAction(node) === 'enable' ? '启用节点' : '检查环境'
}

async function runPrimaryNodeAction(node: ExecutionNodeSummary) {
  const action = primaryNodeAction(node)
  if (!action || nodeActionBusy.value || nodeDeletionBusy.value) return
  nodeActionBusy.value = node.id
  failure.value = ''
  nodeActionNotice.value = ''
  try {
    const updated = action === 'enable'
      ? await api.enableExecutionNode(node.id, node.revision)
      : await api.requestExecutionNodeEnvironmentCheck(node.id, node.revision)
    nodes.value = nodes.value.map((current) => current.id === updated.id ? updated : current)
    nodeActionNotice.value = action === 'enable'
      ? `节点“${updated.displayName}”已启用，可在任务提交时作为候选节点。`
      : `已请求“${updated.displayName}”执行固定环境检查；等待 Agent 回传后可直接启用。`
    if (action === 'environment-check') window.setTimeout(() => { void loadNodes() }, 2500)
  } catch (error) {
    failure.value = executionNodeErrorMessage(error, action === 'enable' ? '执行节点启用失败。' : '环境检查请求失败。')
  } finally {
    nodeActionBusy.value = ''
  }
}

function openNodeDeletionDialog(node: ExecutionNodeSummary) {
  if (nodeActionBusy.value || nodeDeletionBusy.value) return
  failure.value = ''
  nodeActionNotice.value = ''
  nodeDeletionTarget.value = node
}

function closeNodeDeletionDialog() {
  if (nodeDeletionBusy.value) return
  nodeDeletionTarget.value = undefined
}

async function deleteOrArchiveNode() {
  const target = nodeDeletionTarget.value
  if (!target || nodeDeletionBusy.value) return
  nodeDeletionBusy.value = target.id
  failure.value = ''
  try {
    const result = await api.deleteOrArchiveExecutionNode(target.id, target.revision)
    nodeDeletionTarget.value = undefined
    await loadNodes()
    nodeActionNotice.value = result.outcome === 'DELETED'
      ? `节点“${target.displayName}”已删除。`
      : result.agentAccessRevoked
        ? `节点“${target.displayName}”已有历史引用，已归档；Agent 身份和未使用注册码已撤销。`
        : `节点“${target.displayName}”已有历史引用，已归档。`
  } catch (error) {
    failure.value = executionNodeErrorMessage(error, '执行节点删除失败。')
  } finally {
    nodeDeletionBusy.value = ''
  }
}

function platformLabel(platform: ExecutionNodeSummary['platform']) {
  if (platform === 'WINDOWS_AMD64') return 'Windows AMD64'
  if (platform === 'LINUX_AMD64') return 'Kylin Linux AMD64'
  return 'Kylin Linux ARM64'
}

function managementLabel(state: ExecutionNodeSummary['managementState']) {
  if (state === 'ENABLED') return '已启用'
  return state === 'MAINTENANCE' ? '维护中' : '已禁用'
}

function associationLabel(status: ExecutionNodeSummary['agentAssociationStatus']) {
  return status === 'ASSOCIATED' ? '已关联' : '待关联'
}

function heartbeatLabel(status: ExecutionNodeSummary['heartbeatStatus']) {
  if (status === 'ONLINE') return '在线'
  return status === 'OFFLINE' ? '离线' : '从未连接'
}

function environmentLabel(status: ExecutionNodeSummary['environmentStatus']) {
  if (status === 'NORMAL') return '工具已就绪'
  if (status === 'ABNORMAL') return '工具异常'
  return status === 'EXPIRED' ? '已过期' : '未检查'
}

function capacityLabel(status: ExecutionNodeSummary['capacityStatus']) {
  if (status === 'AVAILABLE') return '可用'
  return status === 'BUSY' ? '繁忙' : '未知'
}

function statusDotClass(status: string | boolean) {
  if (status === true || status === 'ENABLED' || status === 'ASSOCIATED' || status === 'ONLINE' || status === 'NORMAL' || status === 'AVAILABLE') return 'success'
  if (status === false || status === 'OFFLINE' || status === 'ABNORMAL' || status === 'EXPIRED' || status === 'BUSY') return 'danger'
  return 'muted'
}

function timeLabel(value: string | null) {
  if (!value) return '尚未收到'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '控制面未提供有效时间' : date.toLocaleString()
}

function capacitySummary(node: ExecutionNodeSummary) {
  if (!node.agentFacts) return node.capacityStatus === 'UNKNOWN' ? '尚未收到容量快照' : '控制面未提供容量快照'
  return `${node.agentFacts.capacityUsed} / ${node.agentFacts.capacityTotal}`
}

function primaryUnavailableReason(node: ExecutionNodeSummary) {
  const reason = node.unavailableReasons[0]
  return reason ? unavailableReasonLabel(reason) : '控制面未提供具体阻断原因'
}

function unavailableReasonLabel(reason: string) {
  const labels: Record<string, string> = {
    AGENT_ASSOCIATION_REQUIRED: '需要完成 Agent 关联',
    HEARTBEAT_REQUIRED: '尚未收到 Agent 心跳',
    HEARTBEAT_OFFLINE: 'Agent 当前离线',
    ENVIRONMENT_CHECK_REQUIRED: '尚未取得环境检查事实',
    ENVIRONMENT_CHECK_IN_PROGRESS: '环境检查正在等待 Agent 回传',
    ENVIRONMENT_CHECK_ABNORMAL: '环境检查发现异常',
    // 环境检查失败码透传（2026-08-11）：区分工具目录缺失与运行时不可用，便于判断故障原因。
    TOOL_RUNTIME_INVALID: '工具运行时无效：登记的 OBDUMPER 工具目录或 Java 在节点上不可用，请检查节点上的工具安装',
    TOOL_RUNTIME_UNAVAILABLE: '工具运行时不可用：节点暂无法核验工具环境，请稍后在节点详情页重新触发环境检查',
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
  <section class="page-heading task-heading">
    <div>
      <h1>执行节点</h1>
      <p>管理节点登记与 Agent 回写的只读事实。未检查环境不会被显示为可接收任务，平台不会通过 SSH、WinRM 或远程 Shell 控制节点。</p>
    </div>
    <RouterLink class="button button-primary" to="/nodes/new">注册节点</RouterLink>
  </section>

  <p v-if="failure" class="feedback feedback-error" role="alert">{{ failure }}</p>
  <p v-if="nodeActionNotice" class="feedback feedback-notice" role="status">{{ nodeActionNotice }}</p>
  <section class="filter-bar" aria-label="执行节点筛选">
    <label>节点名称或标识<input v-model="keyword" placeholder="筛选当前有权节点" /></label>
    <button v-if="keyword" type="button" class="button button-secondary" @click="keyword = ''">清除筛选</button>
    <button type="button" class="button button-secondary" :disabled="loading" @click="loadNodes">{{ loading ? '正在刷新…' : '刷新' }}</button>
  </section>

  <section class="content-card table-card">
    <table>
      <thead>
        <tr><th>节点名称</th><th>目标平台</th><th>管理状态</th><th>Agent 关联</th><th>心跳状态</th><th>工具运行时</th><th>任务容量</th><th>接收新任务</th><th>操作</th></tr>
      </thead>
      <tbody v-if="loading">
        <tr><td colspan="9" class="table-placeholder">正在加载执行节点…</td></tr>
      </tbody>
      <tbody v-else-if="filteredNodes.length">
        <tr v-for="node in filteredNodes" :key="node.id">
          <td><strong>{{ node.displayName }}</strong><small>{{ node.id }}</small></td>
          <td>{{ platformLabel(node.platform) }}</td>
          <td><span class="status-dot" :class="statusDotClass(node.managementState)" />{{ managementLabel(node.managementState) }}</td>
          <td><span class="status-dot" :class="statusDotClass(node.agentAssociationStatus)" />{{ associationLabel(node.agentAssociationStatus) }}<small>{{ node.agentFacts ? `${node.agentFacts.os} / ${node.agentFacts.arch}` : '尚未收到机器事实' }}</small></td>
          <td><span class="status-dot" :class="statusDotClass(node.heartbeatStatus)" />{{ heartbeatLabel(node.heartbeatStatus) }}<small>最近心跳：{{ timeLabel(node.lastHeartbeatAt) }}</small></td>
          <td><span class="status-dot" :class="statusDotClass(node.environmentStatus)" />{{ environmentLabel(node.environmentStatus) }}</td>
          <td><span class="status-dot" :class="statusDotClass(node.capacityStatus)" />{{ capacityLabel(node.capacityStatus) }}<small>{{ capacitySummary(node) }}</small></td>
          <td><span class="status-dot" :class="statusDotClass(node.acceptsNewTasks)" />{{ node.acceptsNewTasks ? '可接收' : '不可接收' }}<small>{{ node.acceptsNewTasks ? '仍需任务级预检查' : primaryUnavailableReason(node) }}</small></td>
          <td class="table-actions">
            <button v-if="primaryNodeAction(node)" type="button" class="table-action-button" :disabled="nodeActionBusy === node.id || nodeDeletionBusy === node.id" @click="runPrimaryNodeAction(node)">{{ nodeActionBusy === node.id ? '正在处理…' : primaryNodeActionLabel(node) }}</button>
            <RouterLink :to="`/nodes/${node.id}`">查看</RouterLink><RouterLink :to="`/nodes/${node.id}/edit`">编辑</RouterLink>
            <button type="button" class="table-action-button" :disabled="nodeActionBusy === node.id || nodeDeletionBusy === node.id" @click="openNodeDeletionDialog(node)">{{ nodeDeletionBusy === node.id ? '正在删除…' : '删除 / 归档' }}</button>
          </td>
        </tr>
      </tbody>
      <tbody v-else>
        <tr>
          <td colspan="9">
            <div class="empty-state">
              <h2>{{ nodes.length ? '没有匹配的执行节点' : '尚无已登记执行节点' }}</h2>
              <p>{{ nodes.length ? '请调整筛选条件后重试。' : '先登记节点名称、目标平台、工具目录、Java 路径和导出数据目录；Agent 关联与工具核验会在后续受控步骤完成。' }}</p>
              <button v-if="nodes.length" type="button" class="button button-secondary" @click="keyword = ''">清除筛选</button>
              <RouterLink v-else class="button button-primary" to="/nodes/new">注册节点</RouterLink>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </section>
  <p class="section-hint">接收新任务是控制面派生的通用结论，不替代任务级路径、数据源、对象和参数预检查。离线只反映节点心跳，不直接判定运行任务失败。</p>

  <div v-if="nodeDeletionTarget" class="node-deletion-backdrop" @click.self="closeNodeDeletionDialog">
    <section class="node-deletion-dialog" role="dialog" aria-modal="true" aria-labelledby="node-deletion-dialog-title">
      <header>
        <div>
          <h2 id="node-deletion-dialog-title">删除或归档执行节点</h2>
          <p>{{ nodeDeletionTarget.displayName }}</p>
        </div>
        <button type="button" class="button button-secondary" :disabled="Boolean(nodeDeletionBusy)" @click="closeNodeDeletionDialog">取消</button>
      </header>
      <p>没有 Agent、草稿、预检查、任务或连接测试引用的节点会被物理删除；存在引用时会归档，历史记录保持不变。归档会撤销当前 Agent 身份和未使用注册码；如有运行任务则拒绝处置，以免中断任务回报。此操作不会远程停止 Agent 或删除目标机器上的文件。</p>
      <div class="node-deletion-actions">
        <button type="button" class="button button-secondary" :disabled="Boolean(nodeDeletionBusy)" @click="closeNodeDeletionDialog">取消</button>
        <button type="button" class="button button-primary" :disabled="Boolean(nodeDeletionBusy)" @click="deleteOrArchiveNode">{{ nodeDeletionBusy ? '正在处理…' : '确认删除 / 归档' }}</button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.table-placeholder { padding: 42px; color: #6f7e91; text-align: center; }
.table-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.table-actions a, .table-action-button { color: #2863bd; text-decoration: none; }
.table-action-button { margin: 0; padding: 0; border: 0; background: transparent; font: inherit; cursor: pointer; }
.table-action-button:disabled { color: #8795a8; cursor: wait; }
.node-deletion-backdrop { position: fixed; inset: 0; z-index: 10; display: grid; place-items: center; padding: 18px; background: rgb(24 34 49 / 46%); }
.node-deletion-dialog { width: min(640px, 100%); padding: 22px; border: 1px solid #d7e0eb; border-radius: 6px; background: #fff; box-shadow: 0 18px 44px rgb(15 23 42 / 22%); }
.node-deletion-dialog header, .node-deletion-actions { display: flex; align-items: start; justify-content: space-between; gap: 14px; }
.node-deletion-dialog h2 { margin: 0 0 5px; color: #334257; font-size: 17px; }
.node-deletion-dialog header p { margin: 0; color: #7a899c; font-size: 12px; }
.node-deletion-dialog > p { margin: 0 0 16px; color: #536276; font-size: 13px; line-height: 1.65; }
.node-deletion-actions { justify-content: end; }
</style>
