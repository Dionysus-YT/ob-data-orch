<script setup lang="ts">
import { nextTick, ref, useId, watch } from 'vue'
import { Button, ConfigProvider, Modal } from 'ant-design-vue'
import { overlayTheme } from '@/platform/theme'
import { component } from '@/platform/tokens'
import { activateModal } from './modalFocus'

const props = withDefaults(defineProps<{ open: boolean; title: string; confirmLabel?: string; cancelLabel?: string; destructive?: boolean; busy?: boolean }>(), { confirmLabel: '确认', cancelLabel: '取消', destructive: false, busy: false })
const emit = defineEmits<{ cancel: []; confirm: [] }>()
const cancelButton = ref<{ $el: HTMLButtonElement }>()
const id = useId()
let invoker: HTMLElement | null = null
watch(() => props.open, (open) => { if (open) invoker = document.activeElement instanceof HTMLElement ? document.activeElement : null }, { immediate: true, flush: 'sync' })
// 嵌套确认与编辑抽屉共享产品焦点栈，防止背景抽屉把焦点抢回；安全动作始终优先。
watch([() => props.open, cancelButton], async ([open], _previous, cleanup) => {
  if (!open) return
  let disposed = false
  let release: (() => void) | undefined
  cleanup(() => { disposed = true; release?.() })
  await nextTick()
  const button = cancelButton.value?.$el
  const root = button?.closest<HTMLElement>('[role="dialog"]')
  if (!disposed && root && button) release = activateModal(root, button, () => { if (!props.busy) emit('cancel') }, invoker)
}, { flush: 'post' })
</script>

<template>
  <ConfigProvider :theme="overlayTheme">
    <Modal :open="open" :title="title" :width="component.overlay.confirmWidth" :closable="!busy" :mask-closable="false" :keyboard="!busy" :focus-trigger-after-close="true" :destroy-on-close="true" class="orch-confirmation" :aria-describedby="id" @cancel="!busy && emit('cancel')">
      <div :id="id" class="orch-confirmation-body"><slot /></div>
      <template #footer><Button ref="cancelButton" :disabled="busy" @click="emit('cancel')">{{ cancelLabel }}</Button><Button :type="destructive ? 'default' : 'primary'" :danger="destructive" :loading="busy" :disabled="busy" @click="emit('confirm')">{{ confirmLabel }}</Button></template>
    </Modal>
  </ConfigProvider>
</template>
