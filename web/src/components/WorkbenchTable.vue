<script setup lang="ts">
withDefaults(defineProps<{ label?: string }>(), { label: '数据表格' })
</script>

<template>
  <div class="workbench-table" role="region" tabindex="0" :aria-label="label"><slot /></div>
</template>

<style scoped>
.workbench-table { width: 100%; overflow-x: auto; border-top: 1px solid var(--border-default); border-bottom: 1px solid var(--border-default); background: var(--surface-primary); }
:slotted(table) { width: 100%; border-collapse: collapse; table-layout: fixed; }
</style>
