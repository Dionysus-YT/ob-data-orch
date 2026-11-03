<script setup lang="ts">
import { Button as AButton } from 'ant-design-vue'
import { ref } from 'vue'
import { RouterLink } from 'vue-router'
import { QuestionCircleOutlined, MenuOutlined, CloseOutlined } from '@ant-design/icons-vue'
import ProductAccountMenu from './ProductAccountMenu.vue'
import ldoMark from './assets/ldo-mark.png'

defineProps<{ navigationOpen: boolean }>()
defineEmits<{ toggleNavigation: []; help: [] }>()
const toggle = ref<{ focus: () => void }>()

defineExpose({ focus: () => toggle.value?.focus() })
</script>

<template>
  <header class="topbar product-header">
    <AButton ref="toggle" :aria-expanded="navigationOpen" aria-controls="product-navigation" type="text" :aria-label="navigationOpen ? '关闭导航' : '打开导航'" class="navigation-toggle orch-icon-action" @click="$emit('toggleNavigation')"><template #icon><CloseOutlined v-if="navigationOpen" class="product-icon" aria-hidden="true" /><MenuOutlined v-else class="product-icon" aria-hidden="true" /></template></AButton>
    <RouterLink class="product-header-brand" to="/"><img class="product-header-mark" :src="ldoMark" width="32" height="32" alt="" aria-hidden="true"><span>LDP 导数平台</span></RouterLink>
    <div class="topbar-actions">
      <AButton type="text" aria-label="帮助" class="orch-icon-action" @click="$emit('help')"><template #icon><QuestionCircleOutlined class="product-icon" aria-hidden="true" /></template></AButton>
      <span class="topbar-divider" aria-hidden="true" />
      <ProductAccountMenu />
    </div>
  </header>
</template>
