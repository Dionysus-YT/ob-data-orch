<script setup lang="ts">
import { Alert as AAlert, Badge as ABadge } from 'ant-design-vue'
import { computed } from 'vue'
import type { NodeRuntimeInspection } from '@/api/browser'
import { nodeTimeLabel } from './executionNodePresentation'
import { inspectionItemGuidance, inspectionLabels, inspectionStatusLabels } from './nodeInspectionPresentation'

const props = defineProps<{ inspection: NodeRuntimeInspection; currentCheckId?: string }>()
const rows = computed(() => props.inspection.items.map(item => ({ item, guidance: inspectionItemGuidance(item) })))
</script>

<template>
  <div class="node-inspection-details">
    <AAlert v-if="inspection.checkId !== currentCheckId" type="info" message="正在展示上次检查结果" description="本次检查尚无新的逐项结果；以下旧结果不能作为本次检查通过的依据。" show-icon />
    <p class="node-inspection-note">检查时间：{{ nodeTimeLabel(inspection.observedAt) }}</p>
    <ul class="node-inspection-items">
      <li v-for="{ item, guidance } in rows" :key="item.kind">
        <div class="node-inspection-heading"><strong>{{ inspectionLabels[item.kind] }}<small v-if="!item.required">（可选）</small></strong><ABadge :status="item.status === 'PASSED' ? 'success' : item.status === 'FAILED' ? 'error' : 'default'" :text="inspectionStatusLabels[item.status]" /></div>
        <template v-if="guidance">
          <p class="node-inspection-note">{{ guidance.reason }}</p>
          <p class="node-inspection-advice">处理建议：{{ guidance.action }}</p>
          <p v-if="!item.required" class="node-inspection-note">可选项，不影响当前导出环境检查。</p>
        </template>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.node-inspection-details { min-width: 0; }
.node-inspection-items { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); column-gap: var(--ob-foundation-space-6); }
.node-inspection-items > li { padding: var(--ob-foundation-space-3) 0; border-bottom: 1px solid var(--ob-color-border); }
.node-inspection-heading { display: flex; align-items: baseline; justify-content: space-between; gap: var(--ob-foundation-space-2); }
.node-inspection-heading strong { font-weight: var(--ob-product-typography-weight); }
.node-inspection-heading small { font-weight: normal; color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); }
.node-inspection-note, .node-inspection-advice { margin: var(--ob-foundation-space-2) 0; font-size: var(--ob-component-field-helper-size); line-height: 1.6; overflow-wrap: anywhere; }
.node-inspection-note { color: var(--ob-color-secondary); }
.node-inspection-advice { color: var(--ob-color-text); }
@media (max-width: 900px) { .node-inspection-items { grid-template-columns: 1fr; } }
</style>
