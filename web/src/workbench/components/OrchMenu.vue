<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, useId } from 'vue'
import { Ellipsis } from '@lucide/vue'
import type { Component } from 'vue'
const props = defineProps<{ label: string; primary?: boolean; items: readonly { id: string; label: string; icon: Component; disabled?: boolean; reason?: string; danger?: boolean; separator?: boolean }[] }>()
const emit = defineEmits<{ select: [id: string] }>()
const root = ref<HTMLDivElement>()
const trigger = ref<HTMLButtonElement>()
const opened = ref(false)
const position = ref({ top: '0px', left: '0px' })
const id = useId()
let disposed = false
let hoverClose: ReturnType<typeof setTimeout> | undefined

function scheduleClose() { hoverClose = setTimeout(() => close(), 120) }
function cancelScheduledClose() { if (hoverClose) { clearTimeout(hoverClose); hoverClose = undefined } }
function close(restore = false) {
  if (hoverClose) { clearTimeout(hoverClose); hoverClose = undefined }
  opened.value = false
  if (restore) trigger.value?.focus()
  document.removeEventListener('pointerdown', outside)
  window.removeEventListener('resize', dismiss)
  window.removeEventListener('scroll', dismiss, true)
}
function dismiss(event: Event) {
  // 菜单自身滚动用于阅读禁用原因，不能把这次滚动当作离开菜单。
  if (event.type === 'scroll' && event.target instanceof Node && root.value?.contains(event.target)) return
  close()
}
function outside(event: PointerEvent) { if (event.target instanceof Node && !root.value?.contains(event.target) && !trigger.value?.contains(event.target)) close() }
async function open(last = false) {
  if (opened.value) return
  opened.value = true
  await nextTick()
  if (disposed || !opened.value) return
  const bounds = trigger.value?.getBoundingClientRect()
  const menu = root.value
  if (!bounds || !menu) { close(); return }
  position.value = { top: `${Math.max(8, Math.min(bounds.bottom + 6, window.innerHeight - menu.offsetHeight - 8))}px`, left: `${Math.max(8, Math.min(bounds.right - menu.offsetWidth, window.innerWidth - menu.offsetWidth - 8))}px` }
  const items = menu.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
  items[last ? items.length - 1 : 0]?.focus({ preventScroll: true })
  document.addEventListener('pointerdown', outside)
  window.addEventListener('resize', dismiss)
  window.addEventListener('scroll', dismiss, true)
}
// 主操作入口由点击打开，避免鼠标移入先展开、随后点击又立即关闭。
function hoverOpen() { cancelScheduledClose(); if (!props.primary && !opened.value) void open() }
function hoverMenu() { cancelScheduledClose() }
function hoverLeave() { scheduleClose() }
function toggle() { if (opened.value) close(true); else void open() }
function select(id: string) {
  // aria-disabled 保留键盘可达性以便阅读原因，但绝不发出被禁用的操作。
  const item = props.items.find((candidate) => candidate.id === id)
  if (!item || item.disabled) return
  close(true)
  emit('select', id)
}
function onKeydown(event: KeyboardEvent) {
  const items = Array.from(root.value?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [])
  const index = items.indexOf(document.activeElement as HTMLButtonElement)
  if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(true) }
  if (event.key === 'Tab') close(true)
  if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key) && items.length) {
    event.preventDefault()
    const target = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : (index + (event.key === 'ArrowUp' ? -1 : 1) + items.length) % items.length
    items[target]?.focus()
  }
}
onBeforeUnmount(() => { disposed = true; close() })
</script>

<template>
  <button ref="trigger" type="button" :class="['orch-button', primary ? 'orch-button--primary' : 'orch-button--quiet orch-button--icon']" :title="label" :aria-label="label" aria-haspopup="menu" :aria-expanded="opened" :aria-controls="opened ? id : undefined" @mouseenter="hoverOpen" @mouseleave="hoverLeave" @click="toggle" @keydown.down.prevent="open()" @keydown.up.prevent="open(true)"><slot><Ellipsis :size="18" aria-hidden="true" /></slot></button>
  <Teleport to="body">
    <div v-if="opened" :id="id" ref="root" role="menu" class="orch-ui orch-menu" :style="position" :aria-label="label" @mouseenter="hoverMenu" @mouseleave="hoverLeave" @keydown="onKeydown">
      <template v-for="item in items" :key="item.id">
        <hr v-if="item.separator" role="separator" />
        <button type="button" role="menuitem" tabindex="-1" :aria-disabled="item.disabled || undefined" :aria-describedby="item.disabled ? `${id}-${item.id}-reason` : undefined" :class="{ 'is-danger': item.danger }" @click="select(item.id)">
          <component :is="item.icon" :size="16" aria-hidden="true" />
          <span class="orch-menu-copy"><span>{{ item.label }}</span><small v-if="item.disabled" :id="`${id}-${item.id}-reason`" class="sr-only">{{ item.reason || '当前状态不允许此操作。' }}</small></span>
        </button>
      </template>
    </div>
  </Teleport>
</template>
