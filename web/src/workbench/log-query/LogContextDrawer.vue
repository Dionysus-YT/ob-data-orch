<script setup lang="ts">
import { Alert as AAlert, Button as AButton, Drawer as ADrawer, Space as ASpace, Spin as ASpin } from 'ant-design-vue'
import type { UnwrapNestedRefs } from 'vue'
import { useAntDrawerDialog } from '@/composables/useAntDrawerDialog'
import type { useLogQuery } from './useLogQuery'
import { logSourceLabel, logTime } from './presentation'
const props = defineProps<{ model: UnwrapNestedRefs<ReturnType<typeof useLogQuery>> }>()
const dialog = useAntDrawerDialog('log-context-title', () => props.model.closeContext(), () => false, () => props.model.contextOpen)
</script>
<template>
  <ADrawer :open="model.contextOpen" placement="right" width="min(760px, 100vw)" :destroy-on-close="true" @close="model.closeContext" @after-open-change="dialog.afterOpenChange">
    <template #title><h2 id="log-context-title" :ref="dialog.heading" tabindex="-1">同来源日志上下文</h2></template>
    <ASpin v-if="model.contextLoading" />
    <AAlert v-if="model.contextFailure" type="error" show-icon :message="model.contextFailure"><template #action><AButton v-if="model.selected" @click="model.openContext(model.selected)">重试</AButton></template></AAlert>
    <template v-if="model.context && model.selected">
      <p class="log-query-task">{{ logSourceLabel[model.selected.source] }} · {{ model.selected.taskId }}<br>同一来源 epoch {{ model.selected.epoch }} · 当前序号 #{{ model.context.selectedSequence }}<br>前后各最多 50 条，实际 {{ model.context.items.length }} 条；关键字与时间筛选不限制同来源窗口。</p>
      <p class="log-query-task">来源标识：{{ model.selected.streamId }}<br>原始来源时间未取得 · 原始级别未取得 · 已脱敏</p>
      <AAlert v-if="model.context.incomplete" type="warning" show-icon message="窗口内存在日志缺口或截断。" />
      <ASpace wrap><AButton :loading="model.copying" @click="model.copyContext(true)">复制当前消息</AButton><AButton :loading="model.copying" @click="model.copyContext()">复制上下文</AButton><RouterLink :to="{ name: 'task-detail', params: { id: model.selected.taskId }, query: { tab: 'logs' } }">查看任务日志</RouterLink></ASpace>
      <AAlert v-if="model.actionFailure" type="error" show-icon :message="model.actionFailure" />
      <ol class="log-context-list" aria-label="同来源上下文记录"><li v-for="item in model.context.items" :key="item.sequence" :class="{ 'log-context-selected': item.sequence === model.context.selectedSequence }"><span class="log-query-secondary">#{{ item.sequence }} · {{ logTime(item.receivedAt) }} · {{ item.kind }} {{ item.integrityCode }}</span><pre>{{ item.message }}</pre></li></ol>
    </template>
  </ADrawer>
</template>
