<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { fixtureNodes, fixtureTasks } from './data/fixtures'
import type { ViewKey } from './types'
import AppIcon from './components/AppIcon.vue'
import DashboardView from './views/DashboardView.vue'
import DataSourcesView from './views/DataSourcesView.vue'
import ExportWizardView from './views/ExportWizardView.vue'
import LogsView from './views/LogsView.vue'
import NodeDetailView from './views/NodeDetailView.vue'
import NodesView from './views/NodesView.vue'
import TaskDetailView from './views/TaskDetailView.vue'
import TasksView from './views/TasksView.vue'

interface NavigationItem {
  label: string
  icon: string
  view?: ViewKey
  disabled?: boolean
}

const currentView = ref<ViewKey>('dashboard')
const selectedTaskId = ref('demo-task-042')
const selectedNodeId = ref('win-agent-01')
const toast = ref('')
let toastTimer: ReturnType<typeof window.setTimeout> | undefined

const navGroups: { label: string; items: NavigationItem[] }[] = [
  { label: '运行概览', items: [{ label: '首页', icon: 'home', view: 'dashboard' }] },
  { label: '任务配置', items: [{ label: '数据源管理', icon: 'database', view: 'sources' }, { label: '导出任务', icon: 'upload', view: 'wizard' }, { label: '普通导入', icon: 'download', disabled: true }, { label: '旁路导入', icon: 'bolt', disabled: true }] },
  { label: '运行与支持', items: [{ label: '任务中心', icon: 'tasks', view: 'tasks' }, { label: '模板中心', icon: 'database', disabled: true }, { label: '执行节点', icon: 'server', view: 'nodes' }, { label: '日志中心', icon: 'log', view: 'logs' }] },
  { label: '平台设置', items: [{ label: '系统设置', icon: 'settings', disabled: true }, { label: '存储凭据', icon: 'key', disabled: true }] },
]
const selectedTask = computed(() => fixtureTasks.find((task) => task.id === selectedTaskId.value) ?? fixtureTasks[0]!)
const selectedNode = computed(() => fixtureNodes.find((node) => node.id === selectedNodeId.value) ?? fixtureNodes[0]!)

function navigate(view: ViewKey): void {
  currentView.value = view
  window.scrollTo({ top: 0, behavior: 'auto' })
}
function openTask(taskId: string): void { selectedTaskId.value = taskId; navigate('task') }
function openNode(nodeId: string): void { selectedNodeId.value = nodeId; navigate('node') }
function showToast(message: string): void {
  toast.value = message
  if (toastTimer) window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => { toast.value = '' }, 3000)
}

onBeforeUnmount(() => { if (toastTimer) window.clearTimeout(toastTimer) })
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar"><button class="brand" type="button" data-od-id="brand-home" @click="navigate('dashboard')"><span class="brand-lockup"><strong>OB</strong><small>ORCH</small></span><span class="brand-name">Data Orchestration</span></button><nav aria-label="主导航"><section v-for="group in navGroups" :key="group.label" class="nav-group"><p class="nav-label">{{ group.label }}</p><button v-for="item in group.items" :key="item.label" class="nav-item" :class="{ 'is-active': item.view === currentView || (currentView === 'task' && item.view === 'tasks') || (currentView === 'node' && item.view === 'nodes') }" type="button" :disabled="item.disabled" :aria-current="item.view === currentView ? 'page' : undefined" :aria-disabled="item.disabled || undefined" @click="item.view && navigate(item.view)"><AppIcon :name="item.icon" />{{ item.label }}</button></section></nav><footer class="sidebar-foot">设计原型 · 基于 V1.0 代码与契约</footer></aside>
    <section class="workspace"><header class="topbar"><button class="icon-button" type="button" aria-label="帮助" @click="showToast('帮助入口在本地原型中仅提供状态反馈。')"><AppIcon name="help" /></button><span class="top-divider" /><span class="avatar" aria-hidden="true">管</span><span class="user-name">管理员</span></header><main>
      <DashboardView v-if="currentView === 'dashboard'" @navigate="navigate" @open-task="openTask" />
      <DataSourcesView v-else-if="currentView === 'sources'" />
      <ExportWizardView v-else-if="currentView === 'wizard'" @open-task="openTask" @notify="showToast" />
      <TasksView v-else-if="currentView === 'tasks'" @open-task="openTask" @open-wizard="navigate('wizard')" />
      <TaskDetailView v-else-if="currentView === 'task'" :task="selectedTask" @back="navigate('tasks')" @notify="showToast" @open-wizard="navigate('wizard')" />
      <NodesView v-else-if="currentView === 'nodes'" @open-node="openNode" @notify="showToast" />
      <NodeDetailView v-else-if="currentView === 'node'" :node="selectedNode" @back="navigate('nodes')" @notify="showToast" />
      <LogsView v-else-if="currentView === 'logs'" />
    </main></section>
    <div v-if="toast" class="toast" role="status" aria-live="polite">{{ toast }}</div>
  </div>
</template>
