<script setup lang="ts">
import type { Tone } from '../types'

withDefaults(defineProps<{ label: string; tone?: Tone }>(), { tone: 'neutral' })
</script>

<template>
  <span class="status" :class="`is-${tone}`"><span class="status-dot" aria-hidden="true" />{{ label }}</span>
</template>
