<script setup lang="ts">
import { useId } from 'vue'
withDefaults(defineProps<{ label: string; required?: boolean; hint?: string; error?: string; technical?: boolean }>(), { required: false, hint: '', error: '', technical: false })
const id = useId()
</script>

<template>
  <div class="orch-field" :class="{ 'orch-field--technical': technical }">
    <label :for="id"><span>{{ label }}</span><span v-if="required" class="orch-required" aria-hidden="true">*</span></label>
    <slot :id="id" :described-by="error || hint ? `${id}-hint` : undefined" :invalid="Boolean(error)" />
    <p v-if="error || hint" :id="`${id}-hint`" :class="{ 'orch-field-error': error }" :role="error ? 'alert' : undefined">{{ error || hint }}</p>
  </div>
</template>
