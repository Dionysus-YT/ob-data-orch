<script setup lang="ts">
import { LoaderCircle } from '@lucide/vue'

withDefaults(defineProps<{
  variant?: 'primary' | 'secondary' | 'quiet' | 'danger'
  iconOnly?: boolean
  label?: string
  busy?: boolean
  disabled?: boolean
  type?: 'button' | 'submit'
}>(), { variant: 'secondary', iconOnly: false, label: '', busy: undefined, disabled: false, type: 'button' })
</script>

<template>
  <button :type="type" class="orch-button" :class="[`orch-button--${variant}`, { 'orch-button--icon': iconOnly, 'has-progress': busy !== undefined && !iconOnly }]" :disabled="disabled || busy" :aria-label="label || undefined" :aria-busy="busy || undefined" :title="iconOnly ? label : undefined">
    <span class="orch-button-content" :class="{ 'is-busy-icon': busy && iconOnly }"><slot name="icon" /><slot /></span>
    <LoaderCircle v-if="busy" class="orch-button-spinner orch-spin" :size="18" aria-hidden="true" />
  </button>
</template>

<style scoped>
.orch-button { position: relative; }
/* 异步文字按钮始终保留动作名和进度图标空位，纯图标按钮才替换原图标。 */
.orch-button-content { display: inline-flex; align-items: center; justify-content: center; gap: 8px; }
.orch-button.has-progress { padding-inline-end: 40px; }
.orch-button-content.is-busy-icon { opacity: 0; }
.orch-button-spinner { position: absolute; }
.has-progress .orch-button-spinner { right: 12px; }
</style>
