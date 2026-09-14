<template>
  <section class="filter-toolbar" aria-label="筛选条件"><slot /></section>
</template>

<style scoped>
.filter-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); padding: 0 0 var(--space-4); border-bottom: 1px solid var(--border-default); }
</style>
