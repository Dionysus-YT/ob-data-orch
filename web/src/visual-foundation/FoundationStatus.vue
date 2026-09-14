<script setup lang="ts">
withDefaults(defineProps<{ tone?: 'success' | 'danger' | 'warning' | 'info' | 'neutral' }>(), { tone: 'neutral' })
</script>

<template><span class="vf-status" :class="`is-${tone}`"><i aria-hidden="true" /><slot /></span></template>
