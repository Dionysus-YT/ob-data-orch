<script setup lang="ts">
import { RouterView } from 'vue-router'

import ProductShell from '@/components/ProductShell.vue'
</script>

<template>
  <RouterView v-slot="{ Component, route }">
    <component :is="Component" v-if="route.meta.visualFoundationLab" />
    <ProductShell v-else>
      <component :is="Component" />
    </ProductShell>
  </RouterView>
</template>
