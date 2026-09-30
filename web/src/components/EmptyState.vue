<script setup lang="ts">
import { Button as AButton, Empty } from 'ant-design-vue'
defineProps<{ title: string; description: string; action?: string }>()
defineEmits<{ action: [] }>()
</script>
<template>
  <Empty>
    <template #description><h2>{{ title }}</h2><p>{{ description }}</p></template>
    <AButton v-if="action" type="primary" @click="$emit('action')">{{ action }}</AButton>
    <slot />
  </Empty>
</template>
