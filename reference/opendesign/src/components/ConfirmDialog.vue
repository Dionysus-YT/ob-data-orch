<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'

const props = withDefaults(defineProps<{ open: boolean; title: string; description: string; impacts?: string[]; confirmLabel?: string; danger?: boolean }>(), { impacts: () => [], confirmLabel: '确认', danger: false })
const emit = defineEmits<{ cancel: []; confirm: [] }>()
const confirmButton = ref<HTMLButtonElement>()
let returnFocus: HTMLElement | null = null

watch(() => props.open, async (open) => {
  if (open) {
    returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
    await nextTick()
    confirmButton.value?.focus()
    return
  }
  returnFocus?.focus()
  returnFocus = null
})

function handleKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') emit('cancel')
}
</script>

<template>
  <div v-if="open" class="dialog-backdrop" @keydown="handleKeydown">
    <section class="dialog" role="dialog" aria-modal="true" :aria-label="title" tabindex="-1">
      <header class="dialog-head"><h2>{{ title }}</h2><button class="icon-button" type="button" aria-label="关闭确认对话框" @click="emit('cancel')">×</button></header>
      <div class="dialog-body"><p>{{ description }}</p><ul v-if="impacts.length" class="impact-list"><li v-for="impact in impacts" :key="impact">{{ impact }}</li></ul></div>
      <footer class="dialog-foot"><button class="btn" type="button" @click="emit('cancel')">返回</button><button ref="confirmButton" class="btn" :class="danger ? 'btn-danger' : 'btn-primary'" type="button" @click="emit('confirm')">{{ confirmLabel }}</button></footer>
    </section>
  </div>
</template>
