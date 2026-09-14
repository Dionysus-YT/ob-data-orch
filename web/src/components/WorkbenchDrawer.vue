<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { X } from '@lucide/vue'

import WorkbenchIconButton from './WorkbenchIconButton.vue'
import { activateModal } from './modalFocus'
import { useWorkbenchFoundation } from './workbenchFoundation'

const props = withDefaults(defineProps<{
  open: boolean
  title: string
  subtitle?: string
  labelledBy?: string
}>(), { subtitle: '', labelledBy: '' })

const emit = defineEmits<{ close: [] }>()
const drawer = ref<HTMLElement>()
const heading = ref<HTMLHeadingElement>()
const foundation = useWorkbenchFoundation()

watch(() => props.open, async (open, _previous, onCleanup) => {
  if (!open) return
  let cancelled = false
  let release: (() => void) | undefined
  onCleanup(() => { cancelled = true; release?.() })
  await nextTick()
  if (!cancelled && drawer.value && heading.value) release = activateModal(drawer.value, heading.value, () => emit('close'))
}, { immediate: true })
</script>

<template>
  <Teleport to="body">
    <Transition name="workbench-overlay">
      <div v-if="open" class="workbench-drawer-backdrop" :class="{ 'workbench-foundation': foundation }" @click.self="emit('close')">
        <aside ref="drawer" class="workbench-drawer" role="dialog" aria-modal="true" :aria-label="labelledBy ? undefined : title" :aria-labelledby="labelledBy || undefined">
          <header class="workbench-drawer-header">
            <div class="workbench-drawer-heading"><h2 ref="heading" tabindex="-1">{{ title }}</h2><p v-if="subtitle">{{ subtitle }}</p></div>
            <div class="workbench-drawer-header-actions"><slot name="header-meta" /><WorkbenchIconButton ref="closeButton" label="关闭" @click="emit('close')"><slot name="close-icon"><X :size="18" :stroke-width="1.75" aria-hidden="true" /></slot></WorkbenchIconButton></div>
          </header>
          <div class="workbench-drawer-body"><slot /></div>
          <footer v-if="$slots.footer" class="workbench-drawer-footer"><slot name="footer" /></footer>
        </aside>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.workbench-drawer-backdrop { position: fixed; inset: 0; z-index: 100; background: rgb(20 26 34 / 40%); }
.workbench-drawer { position: absolute; inset: 0 0 0 auto; display: flex; width: min(var(--app-drawer-width), calc(100vw - 72px)); min-height: 0; flex-direction: column; background: var(--surface-primary); box-shadow: -12px 0 32px rgb(20 26 34 / 16%); }
.workbench-drawer-header { display: flex; flex: none; align-items: flex-start; justify-content: space-between; gap: var(--space-4); min-height: 72px; padding: var(--space-4) var(--space-6) var(--space-3); border-bottom: 1px solid var(--border-default); }
.workbench-drawer-header-actions { display: flex; flex: none; align-items: center; gap: var(--space-2); }
.workbench-drawer-heading { min-width: 0; }.workbench-drawer-heading h2 { margin: 0; color: var(--text-primary); font-size: var(--text-drawer-title-size); font-weight: var(--font-weight-semibold); line-height: var(--text-drawer-title-line-height); }.workbench-drawer-heading p { margin: var(--space-1) 0 0; color: var(--text-muted); font-size: var(--text-metadata-size); line-height: var(--text-metadata-line-height); }
.workbench-drawer-body { min-height: 0; flex: 1; overflow: hidden; }.workbench-drawer-footer { display: flex; flex: none; align-items: center; gap: var(--space-2); min-height: 72px; padding: var(--space-3) var(--space-6); border-top: 1px solid var(--border-default); background: var(--surface-primary); }
.workbench-overlay-enter-active, .workbench-overlay-leave-active { transition: opacity 180ms ease; }.workbench-overlay-enter-active .workbench-drawer, .workbench-overlay-leave-active .workbench-drawer { transition: transform 200ms ease; }.workbench-overlay-enter-from, .workbench-overlay-leave-to { opacity: 0; }.workbench-overlay-enter-from .workbench-drawer, .workbench-overlay-leave-to .workbench-drawer { transform: translateX(100%); }
@media (max-width: 760px) { .workbench-drawer { width: 100vw; } }
@media (prefers-reduced-motion: reduce) { .workbench-overlay-enter-active, .workbench-overlay-leave-active, .workbench-overlay-enter-active .workbench-drawer, .workbench-overlay-leave-active .workbench-drawer { transition: none; } }
</style>
