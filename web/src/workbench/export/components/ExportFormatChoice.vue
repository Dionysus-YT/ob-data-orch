<script setup lang="ts">
import { Input as AInput, Select as ASelect, SelectOption as ASelectOption } from 'ant-design-vue'
import { computed, ref } from 'vue'

type Choice = { value: string; label: string }

const props = defineProps<{
  modelValue: string
  id: string
  label: string
  options: readonly Choice[]
  customPlaceholder: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const customSelected = ref(false)

const selectedValue = computed({
  get: () => customSelected.value || !props.options.some((option) => option.value === props.modelValue) ? '__custom__' : props.modelValue,
  set: (value: string) => {
    if (value === '__custom__') {
      customSelected.value = true
      emit('update:modelValue', '')
      return
    }
    customSelected.value = false
    emit('update:modelValue', value)
  },
})
const customValue = computed({
  get: () => props.modelValue,
  // 分隔符和 NULL 表示中的空格属于交付格式，不能作为表单空白删除。
  set: (value: string) => emit('update:modelValue', value),
})
</script>

<template>
  <div class="export-format-choice">
    <ASelect :id="id" v-model:value="selectedValue" :aria-label="label">
      <ASelectOption v-for="option in options" :key="option.value || 'default'" :value="option.value">{{ option.label }}</ASelectOption>
      <ASelectOption value="__custom__">自定义…</ASelectOption>
    </ASelect>
    <AInput v-if="selectedValue === '__custom__'" v-model:value="customValue" :aria-label="`${label}自定义值`" :placeholder="customPlaceholder" />
  </div>
</template>

<style scoped>
.export-format-choice { display: grid; gap: var(--ob-foundation-space-2); min-inline-size: 0; }
.export-format-choice :deep(.ant-select) { inline-size: 100%; }
</style>
