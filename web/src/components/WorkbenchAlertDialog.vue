<script setup lang="ts">
import { nextTick, ref, useId, watch } from 'vue'

import { CircleAlert } from '@lucide/vue'

import WorkbenchButton from './WorkbenchButton.vue'
import { activateModal } from './modalFocus'
import { useWorkbenchFoundation } from './workbenchFoundation'

const props = withDefaults(defineProps<{
  open: boolean
  title: string
  description: string
  confirmLabel: string
  objectName?: string
  objectContext?: string
  impacts?: readonly string[]
  destructive?: boolean
  busy?: boolean
}>(), { objectName: '', objectContext: '', impacts: () => [], destructive: false, busy: false })

const emit = defineEmits<{ cancel: []; confirm: [] }>()
const dialogElement = ref<HTMLElement>()
const cancelButton = ref<InstanceType<typeof WorkbenchButton>>()
const titleID = `workbench-alert-title-${useId()}`
const descriptionID = `workbench-alert-description-${useId()}`
const foundation = useWorkbenchFoundation()

watch(() => props.open, async (open, _previous, onCleanup) => {
  if (!open) return
  let cancelled = false
  let release: (() => void) | undefined
  onCleanup(() => { cancelled = true; release?.() })
  await nextTick()
  const initialFocus = cancelButton.value?.$el
  if (!cancelled && dialogElement.value && initialFocus instanceof HTMLElement) {
    release = activateModal(dialogElement.value, initialFocus, () => { if (!props.busy) emit('cancel') })
  }
}, { immediate: true })
</script>

<template>
  <Teleport to="body">
    <Transition name="workbench-alert-dialog-fade">
      <div v-if="open" class="workbench-alert-dialog-backdrop" :class="{ 'workbench-foundation': foundation }" @click.self="!busy && emit('cancel')">
        <section ref="dialogElement" class="workbench-alert-dialog" role="alertdialog" aria-modal="true" :aria-labelledby="titleID" :aria-describedby="descriptionID">
          <header><CircleAlert v-if="destructive" :size="20" aria-hidden="true" /><h2 :id="titleID">{{ title }}</h2></header>
          <div class="workbench-alert-dialog-body">
            <p :id="descriptionID">{{ description }}</p>
            <p v-if="objectName" class="workbench-alert-dialog-object"><strong>{{ objectName }}</strong><span v-if="objectContext">{{ objectContext }}</span></p>
            <ul v-if="impacts.length"><li v-for="impact in impacts" :key="impact">{{ impact }}</li></ul>
          </div>
          <footer>
            <WorkbenchButton ref="cancelButton" :disabled="busy" @click="emit('cancel')">取消</WorkbenchButton>
            <WorkbenchButton :variant="destructive ? 'danger' : 'primary'" :busy="busy" @click="emit('confirm')">{{ confirmLabel }}</WorkbenchButton>
          </footer>
        </section>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.workbench-alert-dialog-backdrop { position: fixed; inset: 0; z-index: 140; display: grid; place-items: center; padding: var(--space-6); background: rgb(20 26 34 / 40%); }
.workbench-alert-dialog { display: flex; flex-direction: column; width: min(460px, 100%); max-height: calc(100dvh - 48px); overflow: hidden; border: 1px solid var(--border-default); border-radius: var(--radius-overlay); background: var(--surface-primary); box-shadow: 0 18px 44px rgb(20 26 34 / 20%); }
.workbench-alert-dialog header, .workbench-alert-dialog footer { flex: none; }
.workbench-alert-dialog .workbench-alert-dialog-body { min-height: 0; overflow-y: auto; overflow-wrap: anywhere; }
.workbench-alert-dialog header, .workbench-alert-dialog footer { padding: var(--space-4) 20px; }.workbench-alert-dialog header { border-bottom: 1px solid var(--border-default); }.workbench-alert-dialog h2 { margin: 0; color: var(--text-primary); font-size: var(--text-section-title-size); font-weight: var(--font-weight-semibold); line-height: var(--text-section-title-line-height); }
.workbench-alert-dialog-body { padding: 18px 20px 20px; color: var(--text-secondary); font-size: var(--text-body-size); line-height: var(--text-body-line-height); }.workbench-alert-dialog-body p { margin: 0; }.workbench-alert-dialog-object { display: grid; gap: 3px; margin-top: var(--space-3) !important; padding: 9px 11px; border-left: 3px solid var(--border-default); background: var(--surface-subtle); color: var(--text-primary); }.workbench-alert-dialog-object strong { font-weight: var(--font-weight-semibold); }.workbench-alert-dialog-object span { color: var(--text-secondary); font-size: var(--text-metadata-size); font-weight: 400; }.workbench-alert-dialog-body ul { display: grid; gap: var(--space-1); margin: var(--space-3) 0 0; padding-left: 20px; }
.workbench-alert-dialog footer { display: flex; justify-content: flex-end; gap: var(--space-2); border-top: 1px solid var(--border-default); background: var(--surface-subtle); }
.workbench-alert-dialog-fade-enter-active, .workbench-alert-dialog-fade-leave-active { transition: opacity 160ms ease; }.workbench-alert-dialog-fade-enter-from, .workbench-alert-dialog-fade-leave-to { opacity: 0; }
@media (prefers-reduced-motion: reduce) { .workbench-alert-dialog-fade-enter-active, .workbench-alert-dialog-fade-leave-active { transition: none; } }
</style>
