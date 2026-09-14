<script setup lang="ts">
import { Check, CircleHelp, LoaderCircle } from '@lucide/vue'
withDefaults(defineProps<{ tone?: 'neutral' | 'success' | 'warning' | 'danger' | 'running'; label: string; icon?: 'check' | 'unknown' | 'none'; dotOnly?: boolean }>(), { tone: 'neutral', icon: 'none', dotOnly: false })
</script>

<template>
  <span class="orch-status" :class="`orch-status--${tone}`">
    <span v-if="dotOnly" class="orch-status-dot" aria-hidden="true" />
    <LoaderCircle v-else-if="tone === 'running'" :size="14" class="orch-spin" aria-hidden="true" />
    <Check v-else-if="icon === 'check'" :size="14" aria-hidden="true" />
    <CircleHelp v-else-if="icon === 'unknown'" :size="14" aria-hidden="true" />
    <span v-else class="orch-status-dot" aria-hidden="true" />
    {{ label }}
  </span>
</template>
