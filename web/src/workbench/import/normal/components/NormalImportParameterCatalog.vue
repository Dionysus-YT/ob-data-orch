<script setup lang="ts">
import { computed, type UnwrapNestedRefs } from 'vue'
import WizardParameterCatalog from '@/components/WizardParameterCatalog.vue'
import { normalImportTitles } from '../normalImportOptions'
import { normalImportParameterRows } from '../normalImportParameterCatalog'
import type { NormalImportWizard } from '../useNormalImportWizard'

const props = defineProps<{ open: boolean; model: Pick<UnwrapNestedRefs<NormalImportWizard>, 'form' | 'selectedSource' | 'moveToStep'> }>()
const emit = defineEmits<{ close: [] }>()
const rows = computed(() => normalImportParameterRows(props.model.form, props.model.selectedSource?.compatibilityMode))
</script>

<template>
  <WizardParameterCatalog :open="open" title="普通导入全部参数" :entries="rows" :step-titles="normalImportTitles" placeholder="例如 ctl-path、压缩、Oracle" @close="emit('close')" @locate="model.moveToStep" />
</template>
