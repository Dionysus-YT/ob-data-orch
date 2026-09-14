<script setup lang="ts">
import { ref } from 'vue'

withDefaults(defineProps<{ label: string; disabled?: boolean; active?: boolean }>(), { disabled: false, active: false })

const buttonElement = ref<HTMLButtonElement>()

defineExpose({ focus: () => buttonElement.value?.focus() })
</script>

<template>
  <button ref="buttonElement" type="button" class="workbench-icon-button" :class="{ 'is-active': active }" :aria-label="label" :title="label" :disabled="disabled">
    <slot />
  </button>
</template>

<style scoped>
.workbench-icon-button {
  display: grid;
  width: 30px;
  height: 30px;
  place-items: center;
  padding: 0;
  border: 0;
  border-radius: var(--radius-control);
  color: var(--color-text-secondary);
  background: transparent;
  cursor: pointer;
}

.workbench-icon-button:hover:not(:disabled),
.workbench-icon-button.is-active {
  color: var(--color-primary);
  background: var(--color-primary-soft);
}

.workbench-icon-button:focus-visible {
  outline: 2px solid rgb(37 103 185 / 30%);
  outline-offset: 2px;
}

.workbench-icon-button:disabled {
  color: var(--text-disabled);
  cursor: not-allowed;
}
</style>
