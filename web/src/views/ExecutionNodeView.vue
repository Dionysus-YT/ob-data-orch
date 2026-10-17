<script setup lang="ts">
import { CheckCircleOutlined, EllipsisOutlined, PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { Alert as AAlert, Badge as ABadge, Button as AButton, ConfigProvider, Dropdown as ADropdown, Input as AInput, Menu as AMenu, MenuItem as AMenuItem, Select as ASelect, SelectOption as ASelectOption, Skeleton as ASkeleton, Space as ASpace, Tooltip as ATooltip, type TableColumnsType } from 'ant-design-vue'
import { computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import type { ExecutionNodeSummary } from '@/api/browser'
import EmptyState from '@/components/EmptyState.vue'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import OrchOperationalTable from '@/components/OrchOperationalTable.vue'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { managementTheme } from '@/platform/theme'
import { useNodeList } from '@/workbench/nodes/useNodeList'
import { useNodeListActions } from '@/workbench/nodes/useNodeListActions'
import NodeResourceUsage from '@/workbench/nodes/NodeResourceUsage.vue'
import NodeExecutionQuota from '@/workbench/nodes/NodeExecutionQuota.vue'
import { useNodeResourceSnapshots } from '@/workbench/nodes/useNodeResourceSnapshots'
import {
  nodeCategoryStates, nodeEnvironmentLabel, nodePlatformLabel,
  nodeUnavailableReasonLabel,
} from '@/workbench/nodes/executionNodePresentation'

const router = useRouter()
const list = useNodeList()
const { nodes, keyword, management, heartbeat, environment, acceptance, loading, refreshing, refreshCompleted, restricted, failure, lastLoadedAt, refreshInterval, refreshIntervals, hasFilters, visibleNodes, loadNodes, clearFilters } = list
const { actionFailure, notice, actionBusy, deletionTarget, deleteNode } = useNodeListActions(list)
const { MessageContext } = useOperationMessage(notice)
const { disks } = useNodeResourceSnapshots(nodes)
let editorSaving = false
// 抽屉写事务使列表旧读取失效，周期刷新沿用列表唯一所有者，避免写后事实被旧响应覆盖。
function setEditorSaving(value: boolean) {
  if (editorSaving === value) return
  editorSaving = value
  if (value) list.beginMutation()
  else list.endMutation()
}
const middleCell = () => ({ style: { verticalAlign: 'middle' } })
const columns = computed<TableColumnsType<ExecutionNodeSummary>>(() => [
  { key: 'identity', title: '节点', width: 200, align: 'center', customCell: middleCell },
  { key: 'state', title: '节点状态', width: 200, align: 'center', customCell: middleCell },
  { key: 'resources', title: '资源使用率', width: 220, align: 'center', customCell: middleCell },
  { key: 'quota', title: '执行额度', width: 220, align: 'center', customCell: middleCell },
  { key: 'tasks', title: nodes.value.length && nodes.value.every(node => node.taskSummary?.scope === 'ALL') ? '全部任务' : '我的任务', width: 145, align: 'center', customCell: middleCell },
  { key: 'actions', title: '操作', width: 80, align: 'center', customCell: middleCell },
])

onMounted(() => { void loadNodes() })

function rowAction(node: ExecutionNodeSummary, action: string) {
  if (actionBusy.value || restricted.value) return
  if (action === 'edit') {
    // 菜单项随 Dropdown 关闭而隐藏，抽屉返回焦点应落在仍可见的行操作按钮。
    const invoker = Array.from(document.querySelectorAll<HTMLElement>('[data-node-actions]')).find(element => element.dataset.nodeActions === node.id)
    invoker?.focus()
    void router.push({ name: 'node-edit', params: { id: node.id } })
    return
  }
  if (action === 'delete') deletionTarget.value = node
}

</script>

<template>
  <ConfigProvider :theme="managementTheme">
    <MessageContext />
    <div class="orch-ui node-workspace">
      <header class="orch-page-header">
        <div class="orch-page-title"><h1>执行节点</h1><span class="orch-context-label">导入 / 导出</span></div>
      </header>

      <AAlert v-if="failure && nodes.length" class="orch-feedback" type="error" message="刷新未完成" :description="`${failure} 当前仍展示上一次加载的节点。`" show-icon />
      <AAlert v-if="actionFailure" class="orch-feedback" type="error" message="操作未完成" :description="actionFailure" show-icon />

      <section class="orch-management" aria-label="执行节点列表">
        <div class="orch-table-meta">
          <h2>执行节点列表</h2>
          <div class="node-page-actions">
            <label class="node-auto-refresh"><span>自动刷新</span><ASelect v-model:value="refreshInterval" aria-label="自动刷新间隔"><ASelectOption v-for="seconds in refreshIntervals" :key="seconds" :value="seconds">{{ seconds ? `${seconds} 秒` : '关闭' }}</ASelectOption></ASelect></label>
            <AButton type="text" :loading="refreshing" :disabled="loading || refreshing" :aria-busy="refreshing" aria-label="刷新执行节点" @click="loadNodes(true)"><template #icon><CheckCircleOutlined v-if="refreshCompleted" aria-hidden="true" /><ReloadOutlined v-else aria-hidden="true" /></template>{{ refreshing ? '刷新中' : refreshCompleted ? '已刷新' : '刷新' }}</AButton>
            <RouterLink v-if="!restricted" v-slot="{ href, navigate }" to="/nodes/new" custom><AButton :href="href" type="primary" @click="navigate"><template #icon><PlusOutlined aria-hidden="true" /></template>注册节点</AButton></RouterLink>
          </div>
        </div>

        <div class="node-toolbar" aria-label="筛选执行节点">
          <label class="node-search"><span class="orch-sr-only">搜索节点名称或标识</span><AInput v-model:value="keyword" aria-label="搜索节点名称或标识" :disabled="restricted" placeholder="搜索节点名称或标识"><template #prefix><SearchOutlined aria-hidden="true" /></template></AInput></label>
          <label class="node-filter"><span class="orch-sr-only">管理状态</span><ASelect v-model:value="management" aria-label="管理状态" class="node-filter-control" :disabled="restricted"><ASelectOption value="">全部管理状态</ASelectOption><ASelectOption value="ENABLED">已启用</ASelectOption><ASelectOption value="MAINTENANCE">维护中</ASelectOption><ASelectOption value="DISABLED">已禁用</ASelectOption></ASelect></label>
          <label class="node-filter"><span class="orch-sr-only">心跳状态</span><ASelect v-model:value="heartbeat" aria-label="心跳状态" class="node-filter-control" :disabled="restricted"><ASelectOption value="">全部心跳状态</ASelectOption><ASelectOption value="ONLINE">在线</ASelectOption><ASelectOption value="OFFLINE">离线</ASelectOption><ASelectOption value="NEVER_CONNECTED">从未连接</ASelectOption></ASelect></label>
          <label class="node-filter"><span class="orch-sr-only">运行环境</span><ASelect v-model:value="environment" aria-label="运行环境" class="node-filter-control" :disabled="restricted"><ASelectOption value="">全部环境状态</ASelectOption><ASelectOption value="NORMAL">{{ nodeEnvironmentLabel('NORMAL') }}</ASelectOption><ASelectOption value="ABNORMAL">{{ nodeEnvironmentLabel('ABNORMAL') }}</ASelectOption><ASelectOption value="UNCONFIRMED">待授权或待核验</ASelectOption><ASelectOption value="EXPIRED">{{ nodeEnvironmentLabel('EXPIRED') }}</ASelectOption><ASelectOption value="NOT_CHECKED">{{ nodeEnvironmentLabel('NOT_CHECKED') }}</ASelectOption></ASelect></label>
          <label class="node-filter"><span class="orch-sr-only">接收新任务</span><ASelect v-model:value="acceptance" aria-label="接收新任务" class="node-filter-control" :disabled="restricted"><ASelectOption value="">全部任务资格</ASelectOption><ASelectOption value="true">可接收</ASelectOption><ASelectOption value="false">不可接收</ASelectOption></ASelect></label>
          <AButton v-if="hasFilters" type="link" @click="clearFilters">清除筛选</AButton>
        </div>

        <ASkeleton v-if="loading" active :paragraph="{ rows: 7 }" class="node-loading" role="status" aria-label="正在加载执行节点" />
        <AAlert v-else-if="failure && !nodes.length" class="node-state" :type="restricted ? 'warning' : 'error'" :message="restricted ? '当前身份无权访问执行节点' : '暂时无法读取执行节点'" :description="restricted ? '请联系管理员确认节点管理权限。' : failure" show-icon><template #action><AButton v-if="!restricted" @click="loadNodes()">重新加载</AButton></template></AAlert>
        <EmptyState v-else-if="!nodes.length" class="node-state" title="尚无已登记执行节点" description="登记目标平台和节点目录后，再在目标机器上完成 Agent 关联。"><RouterLink v-slot="{ href, navigate }" to="/nodes/new" custom><AButton :href="href" type="primary" @click="navigate">注册节点</AButton></RouterLink></EmptyState>
        <template v-else>
          <OrchOperationalTable :rows="visibleNodes" :columns="columns" :loading="refreshing" row-key="id" :minimum-width="1065" label="当前已加载的授权执行节点">
            <template #bodyCell="{ record: node, column }">
              <template v-if="column.key === 'identity'">
                <ATooltip :title="node.id"><RouterLink class="node-identity" :to="`/nodes/${node.id}`">{{ node.displayName }}</RouterLink></ATooltip>
                <span class="node-secondary">{{ nodePlatformLabel(node.platform) }}</span>
              </template>
              <template v-else-if="column.key === 'state'">
                <ASpace direction="vertical" :size="4" class="node-category-states">
                  <div v-for="state in nodeCategoryStates(node)" :key="state.key" class="node-category-state" :data-state-category="state.key"><span>{{ state.label }}</span><ABadge :status="state.status" :text="state.text" /></div>
                </ASpace>
                <ATooltip v-if="!node.acceptsNewTasks" :title="node.unavailableReasons.map(reason => nodeUnavailableReasonLabel(reason)).join('；')"><span class="node-secondary node-blocked-reason">{{ nodeUnavailableReasonLabel(node.unavailableReasons[0] ?? '') }}</span></ATooltip>
              </template>
              <template v-else-if="column.key === 'resources'"><NodeResourceUsage :node="node" :disk-usage="disks[node.id]?.usage" /></template>
              <template v-else-if="column.key === 'quota'"><NodeExecutionQuota :quota="node.executionQuota" /></template>
              <template v-else-if="column.key === 'tasks'">
                <ASpace direction="vertical" :size="8"><ATooltip :title="`包含启动中、运行中和取消中的${node.taskSummary?.scope === 'ALL' ? '全部' : '本人'}任务`"><ABadge :status="node.taskSummary?.running ? 'processing' : 'default'" :text="`运行中 ${node.taskSummary?.running ?? '—'}`" /></ATooltip><ABadge :status="node.taskSummary?.waiting ? 'warning' : 'default'" :text="`等待中 ${node.taskSummary?.waiting ?? '—'}`" /><ABadge v-if="node.taskSummary?.reconciling" status="warning" :text="`核对中 ${node.taskSummary.reconciling}`" /></ASpace>
                <RouterLink class="node-secondary node-task-link" :to="{ path: '/tasks', query: { nodeId: node.id, scope: node.taskSummary?.scope ?? 'MINE' } }">查看任务</RouterLink>
              </template>
              <template v-else-if="column.key === 'actions'">
                <ADropdown :trigger="['click']" placement="bottomRight" :disabled="Boolean(actionBusy) || restricted">
                  <AButton type="text" title="更多操作" aria-haspopup="menu" :data-node-actions="node.id" :aria-label="`${node.displayName} 的操作`" :disabled="Boolean(actionBusy) || restricted"><template #icon><EllipsisOutlined aria-hidden="true" /></template></AButton>
                  <template #overlay><AMenu @click="({ key }) => rowAction(node, String(key))"><AMenuItem key="edit">编辑</AMenuItem><AMenuItem key="delete" danger>删除</AMenuItem></AMenu></template>
                </ADropdown>
              </template>
            </template>
            <template #empty><EmptyState title="没有匹配的执行节点" description="调整筛选条件后重试。" action="清除筛选" @action="clearFilters" /></template>
          </OrchOperationalTable>
        </template>
        <footer v-if="lastLoadedAt" class="orch-table-footer node-footer"><span>显示 {{ visibleNodes.length }} 条 / 已加载 {{ nodes.length }} 条</span><span class="node-refresh-status" role="status" aria-live="polite"><span v-if="refreshing"><ReloadOutlined spin aria-hidden="true" /> 正在刷新</span><span v-else-if="refreshCompleted" class="node-refresh-completed"><CheckCircleOutlined aria-hidden="true" /> 已刷新</span><span>最新刷新：{{ lastLoadedAt }}</span></span></footer>
      </section>
    </div>

    <OrchDangerConfirm :open="Boolean(deletionTarget)" title="永久删除执行节点" confirm-label="永久删除" destructive :busy="Boolean(actionBusy)" @confirm="deleteNode" @cancel="deletionTarget = undefined">
      <p>永久删除节点“{{ deletionTarget?.displayName }}”的配置，此操作不可恢复。历史任务及执行证据保留。</p>
      <p>删除会撤销当前 Agent 身份和未使用注册码。存在运行或待核对的执行时不能删除；目标机器上的文件不受影响。</p>
    </OrchDangerConfirm>
    <RouterView v-slot="{ Component }"><component :is="Component" :list-restricted="restricted" @saving="setEditorSaving" @saved="loadNodes(true)" /></RouterView>
  </ConfigProvider>
</template>

<style scoped>
.node-workspace { min-width: 0; }
.node-page-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; align-items: center; gap: var(--ob-foundation-space-2); }
.node-auto-refresh { display: flex; align-items: center; gap: var(--ob-foundation-space-2); color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); }
.node-auto-refresh .ant-select { width: 90px; }
.node-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--ob-foundation-space-2); min-height: 72px; padding: 0 var(--ob-foundation-space-6) var(--ob-foundation-space-4); border-top: 1px solid var(--ob-color-border); }
.node-search { flex: 1 1 230px; max-width: 360px; }
.node-filter { flex: 0 1 155px; min-width: 142px; }
.node-filter-control { width: 100%; }
.node-loading { padding: var(--ob-foundation-space-6); }
.node-state { margin: var(--ob-foundation-space-6); }
.node-identity { display: inline-block; max-width: 100%; color: var(--ob-color-primary); font-weight: 500; overflow-wrap: anywhere; }
.node-secondary { display: block; margin-top: var(--ob-foundation-space-1); color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); line-height: var(--ob-component-table-metadata-line-height); overflow-wrap: anywhere; }
.node-category-state { display: grid; grid-template-columns: 60px auto; align-items: center; gap: var(--ob-foundation-space-2); }
.node-category-states { text-align: left; }
.node-category-state > span { color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); }
.node-blocked-reason { max-width: 180px; margin: var(--ob-foundation-space-2) auto 0; }
.node-task-link { margin-top: var(--ob-foundation-space-3); color: var(--ob-color-primary); }
.node-footer { justify-content: space-between; }
.node-refresh-status { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: var(--ob-foundation-space-2); }
.node-refresh-completed { color: var(--ob-color-success); }
@media (max-width: 720px) { .node-toolbar { padding: var(--ob-foundation-space-4); }.node-search { max-width: none; flex-basis: 100%; }.node-filter { flex-grow: 1; }.node-footer { align-items: flex-start; } }
</style>
