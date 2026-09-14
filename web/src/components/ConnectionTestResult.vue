<script setup lang="ts">
import { CheckCircle2, CircleAlert, CircleHelp, LoaderCircle, TriangleAlert } from '@lucide/vue'

defineProps<{
  state: 'pending' | 'success' | 'error' | 'warning' | 'invalidated' | 'neutral'
  title: string
}>()
</script>

<template>
  <section class="connection-test-result" :class="`is-${state}`" role="status" aria-live="polite">
    <div class="connection-test-result-heading">
      <LoaderCircle v-if="state === 'pending'" class="is-pending" :size="18" aria-hidden="true" />
      <CheckCircle2 v-else-if="state === 'success'" :size="18" aria-hidden="true" />
      <TriangleAlert v-else-if="state === 'warning' || state === 'invalidated'" :size="18" aria-hidden="true" />
      <CircleHelp v-else-if="state === 'neutral'" :size="18" aria-hidden="true" />
      <CircleAlert v-else :size="18" aria-hidden="true" />
      <strong>{{ title }}</strong>
    </div>
    <div class="connection-test-result-content"><slot /></div>
  </section>
</template>

<style scoped>
.connection-test-result { display: grid; gap: var(--space-3); margin: 0 0 var(--space-6); padding: 14px 16px; border: 1px solid var(--color-neutral-border); border-left: 3px solid var(--color-neutral); border-radius: 4px; color: var(--text-secondary); background: var(--surface-primary); }.connection-test-result-heading { display: flex; align-items: flex-start; gap: var(--space-2); color: inherit; }.connection-test-result-heading strong { color: inherit; font-size: var(--text-body-size); font-weight: var(--font-weight-semibold); line-height: var(--text-body-line-height); }.connection-test-result-heading svg { flex: none; margin-top: 1px; }.connection-test-result.is-pending { border-color: var(--color-border-strong); border-left-color: var(--color-primary); color: var(--interactive-primary); background: var(--surface-primary); }.connection-test-result.is-success { border-color: var(--color-success-border); border-left-color: #22a565; color: var(--status-success); background: var(--surface-primary); }.connection-test-result.is-error { border-color: var(--color-danger-border); border-left-color: var(--color-danger); color: var(--status-error); background: var(--surface-primary); }.connection-test-result.is-warning, .connection-test-result.is-invalidated { border-color: var(--color-warning-border); border-left-color: var(--color-warning); color: var(--status-warning); background: var(--surface-primary); }.connection-test-result-content { color: var(--text-secondary); display: grid; gap: var(--space-3); }.connection-test-result-heading > .is-pending { animation: connection-test-spin 1s linear infinite; } @keyframes connection-test-spin { to { transform: rotate(360deg); } } @media (prefers-reduced-motion: reduce) { .connection-test-result-heading > .is-pending { animation: none; } }
</style>
