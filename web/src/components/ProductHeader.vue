<script setup lang="ts">
import { Button as AButton } from 'ant-design-vue'
import { ref } from 'vue'
import { RouterLink } from 'vue-router'
import { QuestionCircleOutlined, MenuOutlined, LogoutOutlined, CloseOutlined } from '@ant-design/icons-vue'

defineProps<{ navigationOpen: boolean }>()
defineEmits<{ toggleNavigation: []; help: [] }>()
const toggle = ref<{ focus: () => void }>()
async function logout() {
  const token = document.querySelector('meta[name="ob-data-orch-csrf-token"]')?.getAttribute('content')
  if (!token) return
  const response = await fetch('/logout', { method: 'POST', credentials: 'same-origin', headers: { 'X-CSRF-Token': token } }).catch(() => undefined)
  if (response?.ok || response?.status === 401) window.location.assign('/login')
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
      <a class="enterprise-account" href="/logout" aria-label="退出登录" @click.prevent="logout"><LogoutOutlined class="product-icon" aria-hidden="true" /><span>退出登录</span></a>
    </div>
  </header>
</template>
