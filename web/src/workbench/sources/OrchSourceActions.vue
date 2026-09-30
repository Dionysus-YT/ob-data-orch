<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { Button, Dropdown, Menu, MenuItem } from 'ant-design-vue'
import { DeleteOutlined, EditOutlined, MoreOutlined } from '@ant-design/icons-vue'
import type { DataSourceSummary } from '@/api/browser'

const props = defineProps<{ source: DataSourceSummary; busy: boolean }>()
const emit = defineEmits<{ edit: []; delete: [] }>()
const open = ref(false)
const trigger = ref<{ $el: HTMLButtonElement }>()
const menu = ref<HTMLElement>()
let keyboardOpen = false
function openFromKeyboard() { keyboardOpen = true; open.value = true }
// Ant Vue 4.2.6 MenuItem 只处理 Enter；领域菜单补齐方向导航并跳过服务端禁用动作。
function navigate(event: KeyboardEvent) {
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  event.stopPropagation()
  const items = Array.from(menu.value?.querySelectorAll<HTMLElement>('[role="menuitem"]:not([aria-disabled="true"])') ?? [])
  if (!items.length) return
  const index = items.indexOf(document.activeElement as HTMLElement)
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1
    : (index + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length
  items[next]?.focus()
}
// Ant Dropdown 不自动把方向键打开意图交给 Menu；产品行操作需从首个可用动作开始。
watch([open, menu], ([visible], _previous, onCleanup) => {
  if (!visible) { keyboardOpen = false; return }
  if (!keyboardOpen) return
  const element = menu.value
  if (!element) return
  // 弹层挂载时仍可能 display:none；等待真实布局后聚焦，关闭或卸载即取消观察。
  const observer = new ResizeObserver(() => {
    const item = element.querySelector<HTMLElement>('[role="menuitem"]:not([aria-disabled="true"])')
    if (!item?.getClientRects().length) return
    item.focus()
    keyboardOpen = false
    observer.disconnect()
  })
  observer.observe(element)
  onCleanup(() => observer.disconnect())
}, { flush: 'post' })
async function close() { open.value = false; await nextTick(); trigger.value?.$el.focus() }
async function select(key: string | number) {
  if (props.busy) return
  if (key === 'delete' && !props.source.lifecycleEligibility?.delete.allowed) return
  await close()
  if (key === 'edit') emit('edit')
  if (key === 'delete') emit('delete')
}
</script>

<template>
  <Dropdown v-model:open="open" :trigger="['click']" placement="bottomRight">
    <Button ref="trigger" type="text" class="orch-icon-action" :aria-label="`${source.displayName} 的操作`" aria-haspopup="menu" :aria-expanded="open" @keydown.down.prevent="openFromKeyboard" @keydown.esc.prevent="close"><template #icon><MoreOutlined class="product-icon" aria-hidden="true" /></template></Button>
    <template #overlay>
      <div>
        <div ref="menu" @keydown="navigate">
          <Menu class="orch-source-actions" :aria-label="`${source.displayName} 的操作`" @click="({key}) => select(key)" @keydown.esc.stop.prevent="close">
            <MenuItem key="edit" :disabled="busy"><template #icon><EditOutlined class="product-icon" aria-hidden="true" /></template>编辑</MenuItem>
            <MenuItem key="delete" :disabled="busy || !source.lifecycleEligibility?.delete.allowed" danger><template #icon><DeleteOutlined class="product-icon" aria-hidden="true" /></template>删除<small v-if="busy || !source.lifecycleEligibility?.delete.allowed" class="orch-action-reason">{{ busy ? '正在处理数据源操作，请稍候。' : source.lifecycleEligibility?.delete.reason || '服务端未提供删除资格。' }}</small></MenuItem>
          </Menu>
        </div>
      </div>
    </template>
  </Dropdown>
</template>
