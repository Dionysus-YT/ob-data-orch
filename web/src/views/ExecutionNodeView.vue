<script setup lang="ts">
import { EllipsisOutlined, PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { Alert as AAlert, Badge as ABadge, Button as AButton, ConfigProvider, Dropdown as ADropdown, Input as AInput, Menu as AMenu, MenuItem as AMenuItem, Select as ASelect, SelectOption as ASelectOption, Skeleton as ASkeleton, type TableColumnsType } from 'ant-design-vue'
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import type { ExecutionNodeSummary } from '@/api/browser'
import EmptyState from '@/components/EmptyState.vue'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import OrchOperationalTable from '@/components/OrchOperationalTable.vue'
import { managementTheme } from '@/platform/theme'
import { useNodeList } from '@/workbench/nodes/useNodeList'
import { useNodeListActions } from '@/workbench/nodes/useNodeListActions'
import {
  nodeAssociationLabel, nodeCapacityLabel, nodeCapacitySummary, nodeEnvironmentLabel,
  nodeHeartbeatLabel, nodeManagementLabel, nodePlatformLabel, nodePrimaryAction,
  nodeTimeLabel, nodeUnavailableReasonLabel,
} from '@/workbench/nodes/executionNodePresentation'

const router = useRouter()
const list = useNodeList()
const { nodes, keyword, management, heartbeat, environment, acceptance, loading, refreshing, restricted, failure, lastLoadedAt, hasFilters, visibleNodes, loadNodes, clearFilters } = list
const { actionFailure, notice, actionBusy, deletionTarget, runPrimaryAction, deleteOrArchiveNode } = useNodeListActions(list)
const columns: TableColumnsType<ExecutionNodeSummary> = [
  { key: 'identity', title: '节点名称', width: 210 },
  { key: 'platform', title: '目标平台', width: 150 },
  { key: 'management', title: '管理状态', width: 105 },
  { key: 'agent', title: 'Agent / 心跳', width: 170 },
  { key: 'environment', title: '工具运行时', width: 125 },
  { key: 'capacity', title: '任务容量', width: 115 },
  { key: 'acceptance', title: '接收新任务', width: 215 },
  { key: 'actions', title: '操作', width: 68 },
]

onMounted(() => { void loadNodes() })

function rowAction(node: ExecutionNodeSummary, action: string) {
  if (actionBusy.value) return
  if (action === 'edit') { void router.push({ name: 'node-edit', params: { id: node.id } }); return }
  if (action === 'delete') { deletionTarget.value = node; return }
  if (action === nodePrimaryAction(node)) void runPrimaryAction(node, action)
}

</script>

<template>
  <ConfigProvider :theme="managementTheme">
    <div class="orch-ui node-workspace">
      <header class="orch-page-header">
        <div class="orch-page-title"><h1>执行节点</h1><span class="orch-context-label">节点登记与 Agent 运行事实</span></div>
      </header>

      <AAlert v-if="failure && nodes.length" class="orch-feedback" type="error" message="刷新未完成" :description="`${failure} 当前仍展示上一次加载的节点。`" show-icon />
      <AAlert v-if="actionFailure" class="orch-feedback" type="error" message="操作未完成" :description="actionFailure" show-icon />
      <AAlert v-if="notice" class="orch-feedback" type="success" :message="notice" show-icon />

      <section class="orch-management" aria-label="执行节点列表">
        <div class="orch-table-meta">
          <h2>执行节点列表</h2>
          <div class="node-page-actions">
            <AButton type="text" :disabled="loading || refreshing" aria-label="刷新执行节点" @click="loadNodes(true)"><template #icon><ReloadOutlined :spin="refreshing" aria-hidden="true" /></template></AButton>
            <RouterLink v-if="!restricted" v-slot="{ href, navigate }" to="/nodes/new" custom><AButton :href="href" type="primary" @click="navigate"><template #icon><PlusOutlined aria-hidden="true" /></template>注册节点</AButton></RouterLink>
          </div>
        </div>

        <div class="node-toolbar" aria-label="筛选执行节点">
          <label class="node-search"><span class="orch-sr-only">搜索节点名称或标识</span><AInput v-model:value="keyword" aria-label="搜索节点名称或标识" :disabled="restricted" placeholder="搜索节点名称或标识"><template #prefix><SearchOutlined aria-hidden="true" /></template></AInput></label>
          <label class="node-filter"><span class="orch-sr-only">管理状态</span><ASelect v-model:value="management" aria-label="管理状态" class="node-filter-control" :disabled="restricted"><ASelectOption value="">全部管理状态</ASelectOption><ASelectOption value="ENABLED">已启用</ASelectOption><ASelectOption value="MAINTENANCE">维护中</ASelectOption><ASelectOption value="DISABLED">已禁用</ASelectOption></ASelect></label>
          <label class="node-filter"><span class="orch-sr-only">心跳状态</span><ASelect v-model:value="heartbeat" aria-label="心跳状态" class="node-filter-control" :disabled="restricted"><ASelectOption value="">全部心跳状态</ASelectOption><ASelectOption value="ONLINE">在线</ASelectOption><ASelectOption value="OFFLINE">离线</ASelectOption><ASelectOption value="NEVER_CONNECTED">从未连接</ASelectOption></ASelect></label>
          <label class="node-filter"><span class="orch-sr-only">工具运行时</span><ASelect v-model:value="environment" aria-label="工具运行时" class="node-filter-control" :disabled="restricted"><ASelectOption value="">全部环境状态</ASelectOption><ASelectOption value="NORMAL">工具已就绪</ASelectOption><ASelectOption value="ABNORMAL">工具异常</ASelectOption><ASelectOption value="EXPIRED">已过期</ASelectOption><ASelectOption value="NOT_CHECKED">未检查</ASelectOption></ASelect></label>
          <label class="node-filter"><span class="orch-sr-only">接收新任务</span><ASelect v-model:value="acceptance" aria-label="接收新任务" class="node-filter-control" :disabled="restricted"><ASelectOption value="">全部任务资格</ASelectOption><ASelectOption value="true">可接收</ASelectOption><ASelectOption value="false">不可接收</ASelectOption></ASelect></label>
          <AButton v-if="hasFilters" type="link" @click="clearFilters">清除筛选</AButton>
        </div>

        <ASkeleton v-if="loading" active :paragraph="{ rows: 7 }" class="node-loading" role="status" aria-label="正在加载执行节点" />
        <AAlert v-else-if="failure && !nodes.length" class="node-state" :type="restricted ? 'warning' : 'error'" :message="restricted ? '当前身份无权访问执行节点' : '暂时无法读取执行节点'" :description="restricted ? '请联系管理员确认节点管理权限。' : failure" show-icon><template #action><AButton v-if="!restricted" @click="loadNodes()">重新加载</AButton></template></AAlert>
        <EmptyState v-else-if="!nodes.length" class="node-state" title="尚无已登记执行节点" description="登记目标平台和节点目录后，再在目标机器上完成 Agent 关联。"><RouterLink v-slot="{ href, navigate }" to="/nodes/new" custom><AButton :href="href" type="primary" @click="navigate">注册节点</AButton></RouterLink></EmptyState>
        <template v-else>
          <OrchOperationalTable :rows="visibleNodes" :columns="columns" row-key="id" :minimum-width="1158" label="当前已加载的授权执行节点">
            <template #bodyCell="{ record: node, column }">
              <template v-if="column.key === 'identity'"><RouterLink class="node-identity" :to="`/nodes/${node.id}`">{{ node.displayName }}</RouterLink><span class="node-secondary node-id">{{ node.id }}</span></template>
              <template v-else-if="column.key === 'platform'">{{ nodePlatformLabel(node.platform) }}</template>
              <template v-else-if="column.key === 'management'"><ABadge :status="node.managementState === 'ENABLED' ? 'success' : node.managementState === 'MAINTENANCE' ? 'warning' : 'default'" :text="nodeManagementLabel(node.managementState)" /></template>
              <template v-else-if="column.key === 'agent'"><ABadge :status="node.heartbeatStatus === 'ONLINE' ? 'success' : node.heartbeatStatus === 'OFFLINE' ? 'error' : 'default'" :text="nodeHeartbeatLabel(node.heartbeatStatus)" /><span class="node-secondary">{{ nodeAssociationLabel(node.agentAssociationStatus) }} · {{ nodeTimeLabel(node.lastHeartbeatAt) }}</span></template>
              <template v-else-if="column.key === 'environment'"><ABadge :status="node.environmentStatus === 'NORMAL' ? 'success' : node.environmentStatus === 'ABNORMAL' ? 'error' : node.environmentStatus === 'EXPIRED' ? 'warning' : 'default'" :text="nodeEnvironmentLabel(node.environmentStatus)" /></template>
              <template v-else-if="column.key === 'capacity'"><ABadge :status="node.capacityStatus === 'AVAILABLE' ? 'success' : node.capacityStatus === 'BUSY' ? 'warning' : 'default'" :text="nodeCapacityLabel(node.capacityStatus)" /><span class="node-secondary">{{ nodeCapacitySummary(node) }}</span></template>
              <template v-else-if="column.key === 'acceptance'"><ABadge :status="node.acceptsNewTasks ? 'success' : 'warning'" :text="node.acceptsNewTasks ? '可接收' : '不可接收'" /><span class="node-secondary">{{ node.acceptsNewTasks ? '仍需任务级预检查' : nodeUnavailableReasonLabel(node.unavailableReasons[0] ?? '') }}</span></template>
              <template v-else-if="column.key === 'actions'"><ADropdown :trigger="['click']"><AButton type="text" :aria-label="`${node.displayName} 的操作`" :disabled="Boolean(actionBusy)"><template #icon><EllipsisOutlined aria-hidden="true" /></template></AButton><template #overlay><AMenu @click="({ key }) => rowAction(node, String(key))"><AMenuItem v-if="nodePrimaryAction(node)" :key="nodePrimaryAction(node)">{{ nodePrimaryAction(node) === 'enable' ? '启用节点' : '检查环境' }}</AMenuItem><AMenuItem key="edit">编辑节点</AMenuItem><AMenuItem key="delete" danger>删除 / 归档</AMenuItem></AMenu></template></ADropdown></template>
            </template>
            <template #empty><EmptyState title="没有匹配的执行节点" description="调整筛选条件后重试。" action="清除筛选" @action="clearFilters" /></template>
          </OrchOperationalTable>
          <footer class="orch-table-footer node-footer"><span>显示 {{ visibleNodes.length }} 条 / 已加载 {{ nodes.length }} 条</span><span>最近刷新：{{ lastLoadedAt || '尚未刷新' }}</span></footer>
        </template>
      </section>
      <p class="node-boundary-note">接收新任务是控制面派生的通用结论；具体任务仍需独立预检查。离线只表示心跳状态，不直接判定运行任务失败。</p>
    </div>

    <OrchDangerConfirm :open="Boolean(deletionTarget)" title="删除或归档执行节点" confirm-label="确认删除 / 归档" destructive :busy="Boolean(actionBusy)" @confirm="deleteOrArchiveNode" @cancel="deletionTarget = undefined">
      <p>将处置节点“{{ deletionTarget?.displayName }}”。没有历史引用时物理删除；有引用时归档并保留历史。</p>
      <p>归档会撤销当前 Agent 身份和未使用注册码；存在运行任务时服务端拒绝操作。平台不会远程停止 Agent 或删除目标机器上的文件。</p>
    </OrchDangerConfirm>
  </ConfigProvider>
</template>

<style scoped>
.node-workspace { min-width: 0; }
.node-page-actions { display: flex; align-items: center; gap: var(--ob-foundation-space-2); }
.node-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--ob-foundation-space-2); min-height: 72px; padding: 0 var(--ob-foundation-space-6) var(--ob-foundation-space-4); border-top: 1px solid var(--ob-color-border); }
.node-search { flex: 1 1 230px; max-width: 360px; }
.node-filter { flex: 0 1 155px; min-width: 142px; }
.node-filter-control { width: 100%; }
.node-loading { padding: var(--ob-foundation-space-6); }
.node-state { margin: var(--ob-foundation-space-6); }
.node-identity { display: inline-block; max-width: 100%; color: var(--ob-color-primary); font-weight: 500; overflow-wrap: anywhere; }
.node-secondary { display: block; margin-top: var(--ob-foundation-space-1); color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); line-height: var(--ob-component-table-metadata-line-height); overflow-wrap: anywhere; }
.node-id { font-family: var(--ob-foundation-font-mono); }
.node-footer { justify-content: space-between; }
.node-boundary-note { margin: var(--ob-foundation-space-3) 0 0; color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); line-height: 1.6; }
@media (max-width: 720px) { .node-toolbar { padding: var(--ob-foundation-space-4); }.node-search { max-width: none; flex-basis: 100%; }.node-filter { flex-grow: 1; }.node-footer { align-items: flex-start; } }
</style>
