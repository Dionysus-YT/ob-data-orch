<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { DownOutlined, RightOutlined, SettingFilled } from '@ant-design/icons-vue'
import { Button, Dropdown, Menu, MenuItem, Tooltip } from 'ant-design-vue'

const props = defineProps<{ collapsed: boolean }>()
const route = useRoute(), router = useRouter()
const items = [
  { label: '平台设置', to: '/settings/platform', paths: ['/settings/platform', '/settings/scheduling', '/settings/security', '/settings/tools'] },
  { label: '权限配置', to: '/settings/access-control' },
  { label: '存储凭据', to: '/settings/storage-credentials' },
]
const activeItem = computed(() => items.find(item => (item.paths ?? [item.to]).some(path => route.path === path || route.path.startsWith(`${path}/`))))
const expanded = ref(true), popupOpen = ref(false)
const trigger = ref<{ $el: HTMLButtonElement }>(), menu = ref<HTMLElement>(), children = ref<HTMLElement>()
let openIntent = false, disposed = false, sequence = 0, frame: number | undefined

// 关闭、路由和布局变化撤销旧焦点意图，避免迟到的弹层事件重新打开导航。
async function closePopup(restoreFocus = true) {
  const current = ++sequence
  openIntent = false
  popupOpen.value = false
  if (frame !== undefined) cancelAnimationFrame(frame)
  await nextTick()
  if (!disposed && restoreFocus && current === sequence) trigger.value?.$el.focus()
}

async function focusPopup() {
  const current = ++sequence
  await nextTick()
  function focusWhenReady(attempt: number) {
    if (disposed || !popupOpen.value || current !== sequence) return
    const entry = menu.value?.querySelector<HTMLElement>('[role="menuitem"][aria-current="page"]') ?? menu.value?.querySelector<HTMLElement>('[role="menuitem"]')
    if (entry?.getClientRects().length) { entry.focus(); return }
    if (attempt < 12) frame = requestAnimationFrame(() => focusWhenReady(attempt + 1))
  }
  frame = requestAnimationFrame(() => focusWhenReady(0))
}

function openFromKeyboard() { openIntent = true; popupOpen.value = true; void focusPopup() }
function togglePopup(event: MouseEvent) {
  if (popupOpen.value) void closePopup()
  else { openIntent = true; popupOpen.value = true; if (event.detail === 0) void focusPopup() }
}
function changeOpen(open: boolean) { if (!open) void closePopup(false); else if (openIntent && !disposed) popupOpen.value = true }
function moveFocus(event: KeyboardEvent) {
  // Ant 菜单已在 Enter 的冒泡前选中条目；阻止默认动作落到恢复焦点后的触发按钮。
  if (event.key === 'Enter') { event.preventDefault(); event.stopPropagation(); return }
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  event.preventDefault(); event.stopPropagation()
  const entries = Array.from(menu.value?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])
  const index = entries.indexOf(document.activeElement as HTMLElement)
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? entries.length - 1 : (index + (event.key === 'ArrowDown' ? 1 : -1) + entries.length) % entries.length
  entries[next]?.focus()
}
async function selectRoute(key: string | number) {
  const item = items.find(entry => entry.to === String(key))
  if (!item) return
  await closePopup()
  if (!disposed) void router.push(item.to)
}
async function focusChild() {
  expanded.value = true
  await nextTick()
  const entry = children.value?.querySelector<HTMLElement>('[aria-current="page"]') ?? children.value?.querySelector<HTMLElement>('a')
  entry?.focus()
}
function closeChildren() { expanded.value = false; trigger.value?.$el.focus() }
watch(() => route.fullPath, () => { if (activeItem.value) expanded.value = true; void closePopup(false) })
watch(() => props.collapsed, () => { void closePopup(false) })
onBeforeUnmount(() => { disposed = true; sequence++; if (frame !== undefined) cancelAnimationFrame(frame) })
</script>

<template>
  <section class="nav-group system-navigation" aria-label="系统管理分组">
    <Dropdown v-if="collapsed" :open="popupOpen" :trigger="['click']" placement="topLeft" :align="{ points: ['bl', 'tr'] }" :destroy-popup-on-hide="true" @open-change="changeOpen">
      <Tooltip :title="popupOpen ? undefined : activeItem ? `系统管理 · ${activeItem.label}` : '系统管理'" placement="right">
        <Button ref="trigger" type="text" class="nav-item nav-disclosure" :class="{ active: activeItem }" aria-label="系统管理" aria-haspopup="menu" :aria-expanded="popupOpen" aria-controls="system-management-menu" @click="togglePopup" @keydown.down.stop.prevent="openFromKeyboard" @keydown.esc.stop.prevent="closePopup()"><SettingFilled class="product-icon nav-icon" aria-hidden="true" /><span class="nav-label">系统管理</span></Button>
      </Tooltip>
      <template #overlay><div ref="menu" class="orch-ui" @keydown="moveFocus"><Menu id="system-management-menu" aria-label="系统管理子菜单" :selected-keys="activeItem ? [activeItem.to] : []" @click="({ key }) => selectRoute(key)" @keydown.esc.stop.prevent="closePopup()" @keydown.left.stop.prevent="closePopup()"><MenuItem v-for="item in items" :key="item.to" :aria-current="activeItem?.to === item.to ? 'page' : undefined">{{ item.label }}</MenuItem></Menu></div></template>
    </Dropdown>
    <template v-else>
      <Button ref="trigger" type="text" class="nav-item nav-disclosure" :class="{ 'nav-section-current': activeItem }" aria-label="系统管理" :aria-expanded="expanded" aria-controls="system-management-navigation" @click="expanded = !expanded" @keydown.right.prevent="focusChild" @keydown.left.prevent="closeChildren"><SettingFilled class="product-icon nav-icon" aria-hidden="true" /><span class="nav-label">系统管理</span><DownOutlined v-if="expanded" class="product-icon nav-chevron" aria-hidden="true" /><RightOutlined v-else class="product-icon nav-chevron" aria-hidden="true" /></Button>
      <nav v-show="expanded" id="system-management-navigation" ref="children" class="nav-children" aria-label="系统管理子导航" @keydown.left.stop.prevent="closeChildren" @keydown.esc.stop.prevent="closeChildren"><RouterLink v-for="item in items" :key="item.to" :to="item.to" class="nav-item nav-child" :aria-current="activeItem?.to === item.to ? 'page' : undefined" :class="{ active: activeItem?.to === item.to }"><span class="nav-label">{{ item.label }}</span></RouterLink></nav>
    </template>
  </section>
</template>
