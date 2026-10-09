<script setup lang="ts">
import { Alert, Spin } from 'ant-design-vue'
defineProps<{ state: 'pending'|'testing'|'success'|'error'|'warning'|'invalidated'|'neutral'|'info'; title: string }>()
</script>
<template>
  <Alert :type="state === 'pending' || state === 'testing' || state === 'neutral' ? 'info' : state === 'invalidated' ? 'warning' : state" :message="title" show-icon role="status" aria-live="polite" :aria-busy="state === 'pending' || state === 'testing'">
    <template v-if="state === 'pending' || state === 'testing'" #icon><Spin size="small" /></template>
    <template #description><slot /></template>
  </Alert>
</template>
