<script setup lang="ts">
import { Inbox } from '@lucide/vue'
defineProps<{ title: string; description: string; action?: string }>()

defineEmits<{ action: [] }>()
</script>

<template>
  <section class="empty-state">
    <div class="empty-mark" aria-hidden="true"><Inbox :size="28" :stroke-width="1.5" /></div>
    <h2>{{ title }}</h2>
    <p>{{ description }}</p>
    <button v-if="action" type="button" class="button button-primary" @click="$emit('action')">{{ action }}</button>
  </section>
</template>
