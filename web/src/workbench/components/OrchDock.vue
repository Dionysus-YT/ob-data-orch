<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, useId } from 'vue'
import { Check, Database, X } from '@lucide/vue'
import OrchButton from './OrchButton.vue'
import { activateModal } from '@/components/modalFocus'
defineProps<{ title: string; dirty: boolean; isNew: boolean; busy?: boolean; compact?: boolean; hideState?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement>()
const heading = ref<HTMLHeadingElement>()
const id = useId()
let release: (() => void) | undefined
let disposed = false
onMounted(async () => {
  await nextTick()
  if (disposed || !dialog.value || !heading.value) return
  // 先记录触发元素再进入原生顶层弹层，避免原生 autofocus 抢走返回目标。
  release = activateModal(dialog.value, heading.value, () => emit('close'))
  dialog.value.showModal()
  heading.value.focus()
})
onBeforeUnmount(() => {
  disposed = true
  dialog.value?.close()
  release?.()
})
</script>

<template>
  <Teleport to="body">
    <dialog ref="dialog" class="orch-ui orch-dock" :class="{ 'orch-source-editor': compact }" :aria-labelledby="id" @cancel.prevent="emit('close')">
      <header class="orch-dock-header">
        <div class="orch-dock-title"><Database :size="20" aria-hidden="true" /><h2 :id="id" ref="heading" tabindex="-1">{{ title }}</h2></div>
        <OrchButton variant="quiet" icon-only label="关闭编辑" :disabled="busy" @click="emit('close')"><X :size="19" /></OrchButton>
      </header>
      <div v-if="!hideState" class="orch-dock-state" role="status"><span v-if="dirty" class="orch-dirty-dot" /><Check v-else-if="!isNew" :size="13" />{{ dirty ? '有未保存的更改' : isNew ? '新建配置' : '已保存的配置' }}<slot name="state" /></div>
      <div class="orch-dock-body"><slot /></div>
      <footer class="orch-dock-footer"><slot name="footer" /></footer>
    </dialog>
  </Teleport>
</template>
