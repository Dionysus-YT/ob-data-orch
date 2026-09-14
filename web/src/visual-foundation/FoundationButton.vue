<script setup lang="ts">
withDefaults(defineProps<{ variant?: 'primary' | 'secondary' | 'utility'; disabled?: boolean; icon?: boolean }>(), { variant: 'secondary', disabled: false, icon: false })
</script>

<template><button class="vf-button" :class="[`is-${variant}`, { 'is-icon': icon }]" :disabled="disabled"><slot /></button></template>
