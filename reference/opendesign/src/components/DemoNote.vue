<script setup lang="ts">
import AppIcon from './AppIcon.vue'

defineProps<{ text: string }>()
</script>

<template>
  <div class="demo-note"><AppIcon name="help" /><span>{{ text }}</span></div>
</template>
