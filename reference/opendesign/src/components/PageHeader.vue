<script setup lang="ts">
defineProps<{ title: string; description: string; dataId: string }>()
</script>

<template>
  <header class="page-heading"><div><h1 :data-od-id="dataId">{{ title }}</h1><p>{{ description }}</p></div><div class="heading-actions"><slot name="actions" /></div></header>
</template>
