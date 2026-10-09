<script setup lang="ts">
import { Button as AButton, Tooltip as ATooltip } from 'ant-design-vue'
import { QuestionCircleOutlined } from '@ant-design/icons-vue'

defineProps<{ label: string; parameter: string; description: string }>()
</script>

<template>
  <ATooltip :trigger="['hover', 'focus']">
    <template #title>
      <div class="export-option-hint-content">
        <div class="export-option-hint-parameter">参数：<code>{{ parameter }}</code></div>
        <div>含义：{{ description }}</div>
      </div>
    </template>
    <AButton type="text" size="small" class="export-option-hint" :aria-label="`${label}参数说明`">
      <template #icon><QuestionCircleOutlined aria-hidden="true" /></template>
    </AButton>
  </ATooltip>
</template>

<style scoped>
.export-option-hint { flex: none; color: var(--ob-color-secondary); }
.export-option-hint-content { overflow-wrap: anywhere; }
.export-option-hint-parameter { margin-block-end: var(--ob-foundation-space-2); }
</style>
