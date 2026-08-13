<script setup lang="ts">
withDefaults(defineProps<{ tone?: 'success' | 'danger' | 'warning' | 'info' | 'neutral' | 'enabled'; appearance?: 'dot' | 'tag' }>(), { tone: 'neutral', appearance: 'dot' })
</script>

<template>
  <span class="workbench-status" :class="[`is-${tone}`, `is-${appearance}`]"><span class="status-indicator" aria-hidden="true" /><slot /></span>
</template>

<style scoped>
.workbench-status {
  display: inline-flex;
  width: fit-content;
  min-height: 20px;
  align-items: center;
  color: var(--color-neutral);
  font-size: 12px;
  font-weight: 500;
  line-height: 18px;
  white-space: nowrap;
}

.status-indicator {
  width: 7px;
  height: 7px;
  margin-right: 6px;
  border: 1px solid currentColor;
  border-radius: 50%;
  background: currentColor;
}

.is-success { color: var(--color-success); }
.is-danger { color: var(--color-danger); }
.is-warning { color: var(--color-warning); }
.is-info { color: var(--color-primary); }
.is-enabled { color: var(--color-success); }
.is-neutral .status-indicator { background: transparent; }

.is-tag {
  min-height: 22px;
  padding: 1px 7px;
  border: 1px solid #d7dce1;
  border-radius: 3px;
  color: #606872;
  background: #f5f6f7;
}

.is-tag .status-indicator { display: none; }
.is-tag.is-enabled { border-color: #bedac9; color: #247348; background: #f0f8f3; }
.is-tag.is-danger { border-color: var(--color-danger-border); color: var(--color-danger); background: var(--color-danger-bg); }
</style>
