<script setup lang="ts">
import { computed, ref } from 'vue'
import { fixtureTasks } from '../data/fixtures'
import type { ExecutionNode, Tone } from '../types'
import AppIcon from '../components/AppIcon.vue'
import StatusBadge from '../components/StatusBadge.vue'

const props = defineProps<{ node: ExecutionNode }>()
const emit = defineEmits<{ back: []; notify: [message: string] }>()
const tab = ref<'overview' | 'tasks' | 'events'>('overview')
const tasks = computed(() => fixtureTasks.filter((task) => task.node === props.node.id))
function eventTone(level: string): Tone { return level === 'ERROR' ? 'danger' : level === 'WARN' ? 'warning' : 'info' }
</script>

<template>
  <div class="page page-wide node-detail-page">
    <button class="back-link" type="button" @click="emit('back')"><AppIcon name="arrow-left" />返回执行节点</button>
    <header class="node-detail-hero"><div><span class="task-id mono">{{ node.id }}</span><h1>{{ node.displayName }}</h1><p>{{ node.description }}</p></div><div class="node-detail-commandbar"><StatusBadge :label="node.schedule === 'schedulable' ? '可调度' : '不可调度'" :tone="node.schedule === 'schedulable' ? 'success' : 'warning'" /><button class="btn" type="button" @click="emit('notify', '环境检查在本地原型中仅反馈界面状态。')"><AppIcon name="refresh" />重新检查环境</button></div></header>
    <section class="node-status-grid" aria-label="节点状态层次"><article><span>管理状态</span><StatusBadge :label="node.managementState === 'ENABLED' ? '已启用' : node.managementState" :tone="node.managementState === 'ENABLED' ? 'success' : 'neutral'" /></article><article><span>Agent 关联</span><StatusBadge label="已关联" tone="success" /></article><article><span>运行时</span><StatusBadge :label="node.runtime === 'online' ? '心跳在线' : '心跳过期'" :tone="node.runtime === 'online' ? 'success' : 'warning'" /></article><article><span>工具环境</span><StatusBadge :label="node.environment === 'normal' ? '工具已就绪' : '事实已过期'" :tone="node.environment === 'normal' ? 'success' : 'warning'" /></article></section>
    <p class="node-reason"><AppIcon name="help" />{{ node.reason }}</p>
    <nav class="node-detail-tabs" aria-label="节点详情分区"><button v-for="item in [{ key: 'overview', label: '概览' }, { key: 'tasks', label: '关联任务' }, { key: 'events', label: '事件与审计' }]" :key="item.key" type="button" :class="{ 'is-active': tab === item.key }" @click="tab = item.key as 'overview' | 'tasks' | 'events'">{{ item.label }}</button></nav>
    <section v-if="tab === 'overview'" class="node-overview-grid"><div class="detail-section"><h2>运行环境</h2><dl class="definition"><div><dt>目标平台</dt><dd>{{ node.platform }}</dd></div><div><dt>OBDUMPER</dt><dd>{{ node.tool }}</dd></div><div><dt>Java</dt><dd>{{ node.java }}</dd></div><div><dt>最后心跳</dt><dd class="mono">{{ node.lastHeartbeat }}</dd></div></dl></div><div class="detail-section"><h2>受控目录与容量</h2><dl class="definition"><div><dt>允许数据根目录</dt><dd class="mono">{{ node.outputRoot }}</dd></div><div><dt>默认日志目录</dt><dd class="mono">{{ node.logRoot }}</dd></div><div><dt>当前任务容量</dt><dd class="mono">{{ node.activeTasks }} / {{ node.capacity }}</dd></div></dl></div></section>
    <section v-else-if="tab === 'tasks'" class="detail-section"><h2>关联任务</h2><div class="table-wrap"><table><thead><tr><th>任务</th><th>状态</th><th>阶段</th><th>更新时间</th></tr></thead><tbody><tr v-for="task in tasks" :key="task.id"><td><span class="cell-primary">{{ task.title }}</span><span class="cell-meta mono">{{ task.id }}</span></td><td><StatusBadge :label="task.status" :tone="task.tone" /></td><td>{{ task.stage }}</td><td class="mono">{{ task.updatedAt }}</td></tr></tbody></table></div><p v-if="!tasks.length" class="empty-inline">当前没有关联到此节点的合成任务。</p></section>
    <section v-else class="detail-section"><h2>节点事件</h2><div class="record-list"><article v-for="event in node.events" :key="`${event.time}-${event.title}`" class="node-record-row"><span class="mono">{{ event.time }}</span><StatusBadge :label="event.level" :tone="eventTone(event.level)" /><div><strong>{{ event.title }}</strong><p>{{ event.detail }}</p></div></article></div></section>
  </div>
</template>
