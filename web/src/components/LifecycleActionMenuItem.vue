<script setup lang="ts">
import type { DataSourceLifecycleActionEligibility } from '@/api/browser'

const props = withDefaults(defineProps<{
  eligibility?: DataSourceLifecycleActionEligibility
  label: string
  destructive?: boolean
  busy?: boolean
}>(), { eligibility: undefined, destructive: false, busy: false })

const emit = defineEmits<{ execute: [] }>()
const available = () => Boolean(props.eligibility?.allowed)
const reason = () => props.eligibility?.reason || '当前身份未获得此操作的服务端资格。'
</script>

<template>
  <div class="lifecycle-action">
    <button type="button" role="menuitem" :class="{ 'is-destructive': destructive }" :disabled="busy || !available()" :title="reason()" @click="emit('execute')">
      <slot name="icon" />
      {{ label }}
    </button>
    <p v-if="!available()" class="lifecycle-action-reason">{{ reason() }}</p>
  </div>
</template>

<style scoped>
.lifecycle-action { display: grid; gap: var(--space-1); }.lifecycle-action button { display: flex; width: 100%; align-items: center; gap: var(--space-2); min-height: 32px; padding: 0 var(--space-2); border: 0; border-radius: var(--radius-control); color: var(--text-primary); background: transparent; font-size: var(--text-label-table-size); text-align: left; cursor: pointer; }.lifecycle-action button:hover:not(:disabled) { background: var(--surface-subtle); }.lifecycle-action button.is-destructive { color: var(--destructive); }.lifecycle-action button:disabled { color: var(--text-muted); cursor: not-allowed; }.lifecycle-action-reason { margin: 0; padding: 0 var(--space-2) var(--space-1); color: var(--text-muted); font-size: var(--text-metadata-size); line-height: var(--text-metadata-line-height); }
</style>
