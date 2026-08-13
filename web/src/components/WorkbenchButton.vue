<script setup lang="ts">
withDefaults(defineProps<{
  variant?: 'primary' | 'secondary' | 'text' | 'danger'
  type?: 'button' | 'submit' | 'reset'
  disabled?: boolean
}>(), { variant: 'secondary', type: 'button', disabled: false })
</script>

<template>
  <button :type="type" class="workbench-button" :class="`is-${variant}`" :disabled="disabled">
    <slot name="icon" />
    <slot />
  </button>
</template>

<style scoped>
.workbench-button {
  display: inline-flex;
  height: var(--size-control);
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  padding: 0 var(--space-3);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-control);
  color: #343b44;
  background: var(--color-bg-surface);
  font-size: 13px;
  font-weight: 500;
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
