<script setup lang="ts">
import { Alert, Button, Dropdown, Menu, MenuItem } from 'ant-design-vue'
import { LoadingOutlined, UserOutlined } from '@ant-design/icons-vue'
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { AccountError, accountApi } from '@/api/account'
import { useLogoutContext } from '@/composables/useLogoutGuard'
import OrchDangerConfirm from './OrchDangerConfirm.vue'
import ProductPasswordDialog from './ProductPasswordDialog.vue'

const route = useRoute(), logoutContext = useLogoutContext()
const controller = new AbortController(), api = accountApi(controller.signal)
const injectedUsername = document.querySelector('meta[name="ob-data-orch-username"]')?.getAttribute('content')
const username = ref(injectedUsername && injectedUsername !== 'local-mvp-username-v1' ? injectedUsername : '')
const loading = ref(false), menuOpen = ref(false), passwordOpen = ref(false)
const logoutPending = ref(false), logoutFailure = ref('')
const trigger = ref<{ $el: HTMLButtonElement }>(), menu = ref<HTMLElement>()
let disposed = false, sequence = 0, frame: number | undefined

async function loadUsername() {
  if (disposed || loading.value || username.value) return
  loading.value = true
  try { const value = await api.session(); if (!disposed) username.value = value }
  catch { /* 未核实身份时不猜测用户名；重新打开菜单可重试读取。 */ }
  finally { if (!disposed) loading.value = false }
}
onBeforeUnmount(() => { disposed = true; sequence++; if (frame !== undefined) cancelAnimationFrame(frame); controller.abort() })

async function closeMenu(restoreFocus = true) {
  const current = ++sequence
  menuOpen.value = false
  if (frame !== undefined) cancelAnimationFrame(frame)
  await nextTick()
  if (!disposed && restoreFocus && current === sequence) trigger.value?.$el.focus()
}
async function openMenu(keyboard = false) {
  if (logoutPending.value || passwordOpen.value || disposed) return
  menuOpen.value = true
  const current = ++sequence
  void loadUsername()
  await nextTick()
  // Ant 浮层在独立渲染轮次挂载；关闭或切页立即撤销未完成的键盘焦点意图。
  function focusWhenReady(attempt: number) {
    if (!keyboard || !menuOpen.value || disposed || current !== sequence) return
    const entry = menu.value?.querySelector<HTMLElement>('[role="menuitem"]:not(.ant-dropdown-menu-item-disabled)')
    if (entry?.getClientRects().length) { entry.focus(); return }
    if (attempt < 12) frame = requestAnimationFrame(() => focusWhenReady(attempt + 1))
  }
  if (keyboard) frame = requestAnimationFrame(() => focusWhenReady(0))
}
function moveFocus(event: KeyboardEvent) {
  if (event.key === 'Enter') { event.preventDefault(); event.stopPropagation(); return }
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  event.preventDefault(); event.stopPropagation()
  const entries = Array.from(menu.value?.querySelectorAll<HTMLElement>('[role="menuitem"]:not(.ant-dropdown-menu-item-disabled)') ?? [])
  const index = entries.indexOf(document.activeElement as HTMLElement)
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? entries.length - 1 : (index + (event.key === 'ArrowDown' ? 1 : -1) + entries.length) % entries.length
  entries[next]?.focus()
}
async function selectAction(key: string | number) {
  await closeMenu()
  if (disposed) return
  if (key === 'password' && username.value && !logoutPending.value) passwordOpen.value = true
  if (key === 'logout') void logout()
}
async function logout() {
  if (disposed || logoutPending.value) return
  logoutPending.value = true
  logoutFailure.value = ''
  try {
    if (!logoutContext) { logoutFailure.value = '当前页面无法核对离开状态，未发送退出请求。请重新打开页面。'; return }
    if (!await logoutContext.confirm() || disposed) return
    await api.logout()
    if (disposed) return
    logoutContext.commit()
    window.location.assign('/login')
  } catch (error) {
    if (!disposed) logoutFailure.value = error instanceof AccountError && error.code === 'CSRF_TOKEN_UNAVAILABLE'
      ? '当前页面缺少请求安全令牌，未发送退出请求。请保存配置后刷新重试。'
      : '暂无法确认退出登录结果，请重试。当前页面输入仍保留，未保存更改不会自动保存。'
  } finally { if (!disposed) logoutPending.value = false }
}
watch(() => route.fullPath, () => { void closeMenu(false); passwordOpen.value = false })
</script>

<template>
  <Dropdown :open="menuOpen" :trigger="['click']" placement="bottomRight" :destroy-popup-on-hide="true" @open-change="open => !open && closeMenu(false)">
    <Button ref="trigger" size="small" shape="round" class="enterprise-account" :aria-label="username ? `当前用户 ${username}` : '账户菜单'" aria-haspopup="menu" :aria-expanded="menuOpen" aria-controls="product-account-menu" :aria-busy="logoutPending || loading" :aria-disabled="logoutPending" @click="menuOpen ? closeMenu() : openMenu($event.detail === 0)" @keydown.down.stop.prevent="openMenu(true)" @keydown.esc.stop.prevent="closeMenu()">
      <LoadingOutlined v-if="logoutPending || loading" class="product-icon" spin aria-hidden="true" /><UserOutlined v-else class="product-icon" aria-hidden="true" />
      <span class="enterprise-account-name" :title="username || undefined">{{ username || (loading ? '加载中' : '账户') }}</span>
    </Button>
    <template #overlay><div ref="menu" class="orch-ui" @keydown="moveFocus"><Menu id="product-account-menu" aria-label="账户操作" :selectable="false" @click="({ key }) => selectAction(key)" @keydown.esc.stop.prevent="closeMenu()" @keydown.tab="closeMenu()"><MenuItem key="password" :disabled="!username || logoutPending" :title="username ? undefined : '无法确认当前账户，请刷新页面后重试'">修改密码</MenuItem><MenuItem key="logout" :disabled="logoutPending">退出登录</MenuItem></Menu></div></template>
  </Dropdown>
  <ProductPasswordDialog v-if="passwordOpen" @close="passwordOpen = false" />
  <OrchDangerConfirm :open="Boolean(logoutFailure)" title="退出登录未完成" confirm-label="重试退出" cancel-label="留在当前页" :busy="logoutPending" @cancel="logoutFailure = ''" @confirm="logout"><Alert type="error" show-icon :message="logoutFailure" /></OrchDangerConfirm>
</template>
