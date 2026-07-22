<script setup lang="ts">
defineProps<{ title: string; description: string; action?: string }>()

defineEmits<{ action: [] }>()
</script>

<template>
  <section class="empty-state">
    <div class="empty-mark" aria-hidden="true">◇</div>
    <h2>{{ title }}</h2>
    <p>{{ description }}</p>
    <button v-if="action" type="button" class="button button-primary" @click="$emit('action')">{{ action }}</button>
  </section>
</template>
