<script setup lang="ts">
import { nextTick, ref, useId, watch } from 'vue'
import { CircleAlert, X } from '@lucide/vue'
import OrchButton from './OrchButton.vue'
import { activateModal } from '@/components/modalFocus'
const props = withDefaults(defineProps<{ open: boolean; title: string; confirmLabel?: string; cancelLabel?: string; destructive?: boolean; busy?: boolean }>(), { confirmLabel: '确认', cancelLabel: '取消', destructive: false, busy: false })
const emit = defineEmits<{ cancel: []; confirm: [] }>()
const dialog = ref<HTMLDialogElement>()
const cancelButton = ref<HTMLElement>()
const id = useId()
watch(() => props.open, async (open, _previous, onCleanup) => {
  if (!open) return
  let cancelled = false
  let release: (() => void) | undefined
  onCleanup(() => { cancelled = true; dialog.value?.close(); release?.() })
  await nextTick()
  const initialFocus = cancelButton.value?.querySelector('button')
  if (cancelled || !dialog.value || !initialFocus) return
  release = activateModal(dialog.value, initialFocus, () => { if (!props.busy) emit('cancel') })
  dialog.value.showModal()
  initialFocus.focus()
}, { immediate: true })
</script>

<template>
  <Teleport to="body">
    <dialog ref="dialog" class="orch-ui orch-confirm" :aria-labelledby="id" :aria-describedby="`${id}-body`" @cancel.prevent="!busy && emit('cancel')">
      <header><CircleAlert v-if="destructive" :size="20" aria-hidden="true" /><h2 :id="id">{{ title }}</h2><OrchButton variant="quiet" icon-only label="关闭确认" :disabled="busy" @click="emit('cancel')"><X :size="17" /></OrchButton></header>
      <div :id="`${id}-body`" class="orch-confirm-body"><slot /></div>
      <footer><span ref="cancelButton"><OrchButton :disabled="busy" @click="emit('cancel')">{{ cancelLabel }}</OrchButton></span><OrchButton :variant="destructive ? 'danger' : 'primary'" :busy="busy" @click="emit('confirm')">{{ confirmLabel }}</OrchButton></footer>
    </dialog>
  </Teleport>
</template>
