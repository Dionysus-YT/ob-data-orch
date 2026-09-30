<script setup lang="ts">
import { RouterView } from 'vue-router'
import { ConfigProvider } from 'ant-design-vue'
import zhCN from 'ant-design-vue/es/locale/zh_CN'
import { platformTheme } from './platform/theme'
import { popupContainer } from './platform/popupContainer'

import ProductShell from '@/components/ProductShell.vue'
</script>

<template>
  <ConfigProvider :theme="platformTheme" :locale="zhCN" :auto-insert-space-in-button="false" :get-popup-container="popupContainer">
    <RouterView v-slot="{ Component }">
      <ProductShell>
        <component :is="Component" />
      </ProductShell>
    </RouterView>
  </ConfigProvider>
</template>
