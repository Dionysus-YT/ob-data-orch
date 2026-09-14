<script setup lang="ts">
import { LoaderCircle } from '@lucide/vue'

withDefaults(defineProps<{
  variant?: 'primary' | 'secondary' | 'text' | 'danger'
  type?: 'button' | 'submit' | 'reset'
  disabled?: boolean
  busy?: boolean
}>(), { variant: 'secondary', type: 'button', disabled: false, busy: undefined })
</script>

<template>
  <button :type="type" class="workbench-button" :class="[`is-${variant}`, { 'has-progress': busy !== undefined }]" :disabled="disabled || busy" :aria-busy="busy || undefined">
    <span class="workbench-button-content"><slot name="icon" /><slot /></span>
    <LoaderCircle v-if="busy" class="workbench-button-spinner" :size="18" aria-hidden="true" />
  </button>
</template>

<style scoped>
.workbench-button {
  position: relative;
  display: inline-flex;
  height: var(--size-control);
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  padding: 0 var(--space-3);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-control);
  color: var(--color-text-primary);
  background: var(--color-bg-surface);
  font-size: var(--text-label-table-size);
  font-weight: var(--font-weight-medium);
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
}

.workbench-button:hover:not(:disabled) {
  border-color: #9ca5af;
  background: #f5f6f7;
}

.workbench-button.is-primary {
  border-color: var(--color-primary);
  color: #fff;
  background: var(--color-primary);
}

.workbench-button.is-primary:hover:not(:disabled) {
  border-color: var(--color-primary-hover);
  background: var(--color-primary-hover);
}

.workbench-button.is-text {
  padding-inline: var(--space-2);
  border-color: transparent;
  color: var(--color-primary);
  background: transparent;
}

.workbench-button.is-danger {
  border-color: var(--color-danger);
  color: #fff;
  background: var(--color-danger);
}

.workbench-button.is-danger:hover:not(:disabled) {
  border-color: var(--destructive-hover);
  color: #fff;
  background: var(--destructive-hover);
}

/* 异步按钮预留进度图标的位置，加载时动作名持续可见且按钮宽度不变。 */
.workbench-button-content { display: inline-flex; align-items: center; justify-content: center; gap: var(--space-2); }
.workbench-button.has-progress { padding-inline-end: 40px; }
.workbench-button-spinner { position: absolute; right: 12px; animation: workbench-button-spin 900ms linear infinite; }
@keyframes workbench-button-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .workbench-button-spinner { animation: none; } }

.workbench-button:focus-visible {
  outline: 2px solid rgb(37 103 185 / 30%);
  outline-offset: 2px;
}

.workbench-button:disabled {
  color: #9299a2;
  border-color: #dde1e5;
  background: #f1f2f4;
  cursor: not-allowed;
}

.workbench-button :deep(svg) {
  flex: none;
}
</style>
