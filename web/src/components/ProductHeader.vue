<script setup lang="ts">
import { Alert as AAlert, Button as AButton } from 'ant-design-vue'
import { onBeforeUnmount, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { QuestionCircleOutlined, MenuOutlined, LogoutOutlined, CloseOutlined, LoadingOutlined } from '@ant-design/icons-vue'
import OrchDangerConfirm from './OrchDangerConfirm.vue'
import { useLogoutContext } from '@/composables/useLogoutGuard'

defineProps<{ navigationOpen: boolean }>()
defineEmits<{ toggleNavigation: []; help: [] }>()
const toggle = ref<{ focus: () => void }>()
const logoutContext = useLogoutContext()
const logoutPending = ref(false)
const logoutFailure = ref('')
let disposed = false
onBeforeUnmount(() => { disposed = true })
async function logout() {
  if (disposed || logoutPending.value) return
  logoutPending.value = true
  logoutFailure.value = ''
  try {
    if (!logoutContext) { logoutFailure.value = '当前页面无法核对离开状态，未发送退出请求。请重新打开页面。'; return }
    if (!await logoutContext.confirm() || disposed) return
    const token = document.querySelector('meta[name="ob-data-orch-csrf-token"]')?.getAttribute('content')
    if (!token) { logoutFailure.value = '当前页面缺少请求安全令牌，未发送退出请求。请保存配置后刷新重试。'; return }
    const response = await fetch('/logout', { method: 'POST', credentials: 'same-origin', headers: { 'X-CSRF-Token': token } }).catch(() => undefined)
    if (disposed) return
    if (response?.ok || response?.status === 401) {
      // 服务端已注销或确认无有效会话后才放行整页跳转，避免第二次未保存提示。
      logoutContext.commit()
      window.location.assign('/login')
    } else logoutFailure.value = '暂无法确认退出登录结果，请重试。当前页面输入仍保留，未保存更改不会自动保存。'
  } finally { if (!disposed) logoutPending.value = false }
}

defineExpose({ focus: () => toggle.value?.focus() })
</script>

<template>
  <header class="topbar product-header">
    <AButton ref="toggle" :aria-expanded="navigationOpen" aria-controls="product-navigation" type="text" :aria-label="navigationOpen ? '关闭导航' : '打开导航'" class="navigation-toggle orch-icon-action" @click="$emit('toggleNavigation')"><template #icon><CloseOutlined v-if="navigationOpen" class="product-icon" aria-hidden="true" /><MenuOutlined v-else class="product-icon" aria-hidden="true" /></template></AButton>
    <RouterLink class="product-header-brand" to="/">OB Data Orch</RouterLink>
    <div class="topbar-actions">
      <AButton type="text" aria-label="帮助" class="orch-icon-action" @click="$emit('help')"><template #icon><QuestionCircleOutlined class="product-icon" aria-hidden="true" /></template></AButton>
      <span class="topbar-divider" aria-hidden="true" />
      <a class="enterprise-account" href="/logout" aria-label="退出登录" :aria-disabled="logoutPending" :aria-busy="logoutPending" @click.prevent="logout"><LoadingOutlined v-if="logoutPending" class="product-icon" spin aria-hidden="true" /><LogoutOutlined v-else class="product-icon" aria-hidden="true" /><span>退出登录</span></a>
    </div>
  </header>
  <OrchDangerConfirm :open="Boolean(logoutFailure)" title="退出登录未完成" confirm-label="重试退出" cancel-label="留在当前页" :busy="logoutPending" @cancel="logoutFailure = ''" @confirm="logout"><AAlert type="error" show-icon :message="logoutFailure" /></OrchDangerConfirm>
</template>
