<script setup lang="ts">
import type { SummaryMetric } from '../types'

defineProps<{ metrics: SummaryMetric[]; label: string }>()
</script>

<template>
  <section class="status-summary" :aria-label="label">
    <article v-for="metric in metrics" :key="metric.key" class="status-summary-item" :class="`tone-${metric.tone}`">
      <span class="status-summary-label"><span class="status-dot" aria-hidden="true" />{{ metric.label }}</span>
      <strong class="status-summary-value">{{ metric.value }}</strong>
      <small>{{ metric.note }}</small>
      <span class="status-progress" role="progressbar" :aria-label="`${metric.label}占比 ${metric.progress}%`" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="metric.progress"><span :style="{ width: `${metric.progress}%` }" /></span>
    </article>
  </section>
</template>
