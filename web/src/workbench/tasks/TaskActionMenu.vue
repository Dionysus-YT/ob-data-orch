<script setup lang="ts">
import { DownOutlined, EllipsisOutlined } from '@ant-design/icons-vue'
import { Button as AButton, Dropdown as ADropdown, Menu as AMenu, MenuItem as AMenuItem } from 'ant-design-vue'
import { nextTick, onScopeDispose, ref, watch } from 'vue'
import type { TaskListAction } from '@/api/browser'
import { taskActionLabels } from './taskListPresentation'

type Action = TaskListAction | 'SUBMIT' | 'DELETE' | 'CREATE_EXPORT' | 'CREATE_NORMAL_IMPORT' | 'CREATE_DIRECT_LOAD'
const props = defineProps<{ actions: readonly Action[]; label: string; busy?: boolean; creation?: boolean; sessionKey?: string }>()
const emit = defineEmits<{ select: [action: Action] }>()
const open = ref(false), menu = ref<HTMLElement>(), trigger = ref<{ $el: HTMLButtonElement }>()
let frame: number | undefined, sequence = 0, disposed = false, openIntent = false
function actionLabel(action: Action) { return action === 'SUBMIT' ? '提交执行' : action === 'DELETE' ? '删除草稿' : action === 'CREATE_EXPORT' ? '导出' : action === 'CREATE_NORMAL_IMPORT' ? '普通导入' : action === 'CREATE_DIRECT_LOAD' ? '旁路导入（尚未开放）' : taskActionLabels[action] }
function disabled(action: Action) { return action === 'CREATE_DIRECT_LOAD' }
function entries() { return Array.from(menu.value?.querySelectorAll<HTMLElement>('[role="menuitem"]:not([aria-disabled="true"])') ?? []) }
async function close(restore = true) { const current = ++sequence; openIntent = false; if (frame !== undefined) cancelAnimationFrame(frame); open.value = false; await nextTick(); if (!disposed && restore && current === sequence) trigger.value?.$el.focus() }
function toggle() { if (props.busy) return; if (open.value) void close(); else { openIntent = true; open.value = true } }
// Ant 延迟发送的打开事件只能消费当前打开意图，不能在 Escape、对象切换或关闭后恢复旧菜单。
function changeOpen(value: boolean) { if (!value) { openIntent = false; open.value = false } else if (openIntent && !disposed) open.value = true }
async function openFromKeyboard() {
  if (props.busy) return
  const current = ++sequence; openIntent = true; open.value = true; await nextTick()
  // 等待 Ant 弹层布局后聚焦；关闭、对象变化和卸载撤销本次焦点意图。
  function focusWhenReady(attempt: number) {
    if (disposed || !open.value || current !== sequence) return
    const item = entries()[0]
    if (item?.getClientRects().length) { item.focus(); return }
    if (attempt < 12) frame = requestAnimationFrame(() => focusWhenReady(attempt + 1))
  }
  frame = requestAnimationFrame(() => focusWhenReady(0))
}
function navigate(event: KeyboardEvent) {
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  event.preventDefault(); event.stopPropagation()
  const items = entries(); if (!items.length) return
  const index = items.indexOf(document.activeElement as HTMLElement)
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : (index + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length
  items[next]?.focus()
}
async function select(key: string | number) { const action = String(key) as Action; if (props.busy || !props.actions.includes(action) || disabled(action)) return; const current = sequence + 1; await close(); if (!disposed && current === sequence) emit('select', action) }
watch(() => `${props.sessionKey ?? ''}|${props.label}|${props.actions.join(',')}`, () => { void close(false) }, { flush: 'sync' })
onScopeDispose(() => { disposed = true; sequence++; if (frame !== undefined) cancelAnimationFrame(frame) })
</script>

<template>
  <ADropdown :open="open" :trigger="['click']" placement="bottomRight" :disabled="busy" :destroy-popup-on-hide="true" @open-change="changeOpen">
    <AButton ref="trigger" :type="creation ? 'primary' : 'text'" :size="creation ? undefined : 'small'" :aria-label="label" aria-haspopup="menu" :aria-expanded="open" :loading="busy" :disabled="busy" @click="toggle" @keydown.down.prevent="openFromKeyboard" @keydown.esc.stop.prevent="close()"><template v-if="!creation" #icon><EllipsisOutlined aria-hidden="true" /></template><template v-if="creation">{{ label }}<DownOutlined aria-hidden="true" /></template></AButton>
    <template #overlay><div ref="menu" @keydown="navigate"><AMenu :aria-label="label" @click="({ key }) => select(key)" @keydown.esc.stop.prevent="close()"><AMenuItem v-for="action in actions" :key="action" :disabled="disabled(action)" :danger="action === 'CANCEL' || action === 'DELETE'">{{ actionLabel(action) }}</AMenuItem></AMenu></div></template>
  </ADropdown>
</template>
