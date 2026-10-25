<script setup lang="ts">
import { computed, ref } from 'vue'
import { Progress, Tooltip } from 'ant-design-vue'
import type { TaskProgress } from '@/api/browser'
import { taskProgressPresentation } from './taskProgressPresentation'

const props = defineProps<{ progress?: TaskProgress; state?: string; reconciliation?: boolean; type?: string; timeZone?: 'UTC' }>()
const presentation = computed(() => taskProgressPresentation(props.progress, props.state, props.reconciliation, props.type, props.timeZone))
const detailsOpen = ref(false)
</script>

<template>
  <Tooltip v-model:open="detailsOpen" :title="presentation.description" :trigger="['hover', 'focus']">
    <div :aria-label="presentation.description" class="task-progress" tabindex="0" @keydown.esc="detailsOpen = false">
      <Progress v-if="presentation.percent !== undefined" size="small" :percent="presentation.percent" :status="presentation.status" :show-info="false" aria-label="工具数据子任务进度" :aria-valuemin="0" :aria-valuemax="100" :aria-valuenow="presentation.percent" :aria-valuetext="presentation.description" />
      <span>{{ presentation.text }}</span>
      <small v-if="presentation.historical">最后记录</small>
    </div>
  </Tooltip>
</template>

<style scoped>
.task-progress { min-width: 0; width: 100%; }
.task-progress small { display: block; color: var(--ob-color-secondary); font-size: var(--ob-component-table-metadata-size); white-space: nowrap; }
</style>
