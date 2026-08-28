<script setup lang="ts">
import { computed, ref } from 'vue'
import { fixtureLogs } from '../data/fixtures'
import type { Task, Tone } from '../types'
import AppIcon from '../components/AppIcon.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import StatusBadge from '../components/StatusBadge.vue'

const props = defineProps<{ task: Task }>()
const emit = defineEmits<{ back: []; notify: [message: string]; openWizard: [] }>()
const activeLevel = ref('all')
const cancelOpen = ref(false)
const logs = computed(() => fixtureLogs.filter((log) => activeLevel.value === 'all' || log.level === activeLevel.value))
const command = computed(() => `obdumper --host 192.0.2.70 --port 2883 --user orders --password ****** --database fixture_orders --table orders --${props.task.format.toLowerCase()} --file-path /E:/exports/orders-20260822`)

function levelTone(level: string): Tone { return level === 'ERROR' ? 'danger' : level === 'WARN' ? 'warning' : 'info' }
async function copyCommand(): Promise<void> {
  try { await navigator.clipboard?.writeText(command.value) } catch { }
  emit('notify', '已复制脱敏命令。')
}
function cancelTask(): void { cancelOpen.value = false; emit('notify', '取消请求仅在本地原型中模拟，不会改变任务终态。') }
</script>

<template>
  <div class="page page-wide task-detail-page">
    <button class="back-link" type="button" @click="emit('back')"><AppIcon name="arrow-left" />返回任务中心</button>
    <header class="task-head"><div><span class="task-id mono">{{ task.id }}</span><div class="task-title"><h1>{{ task.title }}</h1><StatusBadge :label="task.status" :tone="task.tone" /></div><p>{{ task.evidence }}</p></div><div class="heading-actions"><button class="btn" type="button" @click="copyCommand"><AppIcon name="copy" />复制脱敏命令</button><button v-if="task.statusKey === 'running'" class="btn btn-danger" type="button" @click="cancelOpen = true">取消任务</button><button v-else class="btn btn-primary" type="button" @click="emit('openWizard')"><AppIcon name="upload" />基于配置新建</button></div></header>

    <section class="task-facts" aria-label="任务核心事实"><article><span>数据源</span><strong>{{ task.source }}</strong></article><article><span>执行节点</span><strong class="mono">{{ task.node }}</strong></article><article><span>当前阶段</span><strong>{{ task.stage }}</strong></article><article><span>最后更新</span><strong class="mono">{{ task.updatedAt }}</strong></article></section>
    <section class="fact-track" aria-label="任务证据轨道"><article class="track-node is-complete"><span class="track-dot">1</span><strong>配置冻结</strong><small>草稿已提交</small></article><article class="track-node is-complete"><span class="track-dot">2</span><strong>预检查</strong><small>门禁已核对</small></article><article class="track-node" :class="task.statusKey === 'failed' ? 'is-failed' : 'is-current'"><span class="track-dot">3</span><strong>{{ task.stage }}</strong><small>{{ task.progress }}%</small></article><article class="track-node" :class="{ 'is-complete': task.statusKey === 'success' }"><span class="track-dot">4</span><strong>结果事实</strong><small>{{ task.statusKey === 'success' ? '已确认' : '等待终态' }}</small></article></section>

    <section v-if="task.statusKey === 'failed'" class="failure-panel"><h2>执行前已阻断</h2><p>输出目录不可写，受控 OBDUMPER 进程未被启动；请核对匹配平台 Agent 的目录权限和可用根目录。</p><div class="failure-actions"><button class="btn" type="button" @click="emit('notify', '检查点继续资格：当前失败发生在工具启动前，不具备继续资格。')">检查点资格</button><button class="btn btn-primary" type="button" @click="emit('openWizard')">修正配置并新建</button></div></section>

    <div class="detail-grid"><section class="detail-section"><h2>冻结配置</h2><dl class="definition"><div><dt>导出内容</dt><dd>{{ task.typeLabel }}</dd></div><div><dt>数据格式</dt><dd class="mono">{{ task.format }}</dd></div><div><dt>命令预览</dt><dd class="command-inline mono">{{ command }}</dd></div><div><dt>执行证据</dt><dd>{{ task.evidence }}</dd></div></dl></section><section class="detail-section"><h2>建议动作</h2><p class="muted">本地原型只回放安全合成事实。任何恢复、取消或派生操作都不会连接真实 Agent、工具或数据库。</p><button class="btn" type="button" @click="emit('notify', '任务详情中的恢复路径已保持为本地模拟。')">查看恢复说明</button></section></div>

    <section class="detail-section task-log-section"><div class="log-toolbar"><div><h2>事件日志</h2><p>仅展示控制面已有的合成事件，不提供远程终端。</p></div><div class="segmented" role="group" aria-label="日志级别筛选"><button v-for="option in ['all', 'INFO', 'WARN', 'ERROR']" :key="option" class="segment" :class="{ 'is-active': activeLevel === option }" type="button" @click="activeLevel = option">{{ option === 'all' ? '全部' : option }}</button></div></div><div class="log-list"><div v-for="log in logs" :key="log.id" class="log-row" :data-level="log.level"><span>{{ log.time }}</span><StatusBadge :label="log.level" :tone="levelTone(log.level)" /><span>{{ log.source }}</span><span>{{ log.title }} · {{ log.detail }}</span></div></div></section>
    <ConfirmDialog :open="cancelOpen" title="确认取消此任务？" description="取消请求会由控制面按当前租约和任务状态机判定；本地原型不会发送请求。" :impacts="['不会更改当前的合成任务记录。', '不会停止本机或远端任何进程。']" confirm-label="模拟取消" danger @cancel="cancelOpen = false" @confirm="cancelTask" />
  </div>
</template>
