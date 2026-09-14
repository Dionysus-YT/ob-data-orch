<script setup lang="ts">
import { ref } from 'vue'
import { RouterLink } from 'vue-router'
import { CircleHelp, Menu, UserRound, X } from '@lucide/vue'
import WorkbenchIconButton from './WorkbenchIconButton.vue'

defineProps<{ navigationOpen: boolean }>()
defineEmits<{ toggleNavigation: []; help: [] }>()
const toggle = ref<{ focus: () => void }>()
defineExpose({ focus: () => toggle.value?.focus() })
</script>

<template>
  <header class="topbar product-header">
    <WorkbenchIconButton ref="toggle" class="navigation-toggle" :label="navigationOpen ? '关闭导航' : '打开导航'" :aria-expanded="navigationOpen" aria-controls="product-navigation" @click="$emit('toggleNavigation')"><X v-if="navigationOpen" :size="18" /><Menu v-else :size="18" /></WorkbenchIconButton>
    <RouterLink class="product-header-brand" to="/">OB Data Orch</RouterLink>
    <div class="topbar-actions">
      <WorkbenchIconButton label="帮助" @click="$emit('help')"><CircleHelp :size="18" :stroke-width="1.75" aria-hidden="true" /></WorkbenchIconButton>
      <span class="topbar-divider" aria-hidden="true" />
      <RouterLink class="enterprise-account" to="/settings/access-control" aria-label="账户与权限配置"><UserRound :size="16" aria-hidden="true" /><span>账户</span></RouterLink>
    </div>
  </header>
</template>
