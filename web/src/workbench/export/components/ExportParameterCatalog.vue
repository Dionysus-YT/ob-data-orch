<script setup lang="ts">
import { computed, type UnwrapNestedRefs } from 'vue'
import WizardParameterCatalog from '@/components/WizardParameterCatalog.vue'
import { exportParameterRows } from '../exportParameterCatalog'
import type { ExportWizard } from '../useExportWizard'

const props = defineProps<{ open: boolean; model: Pick<UnwrapNestedRefs<ExportWizard>, 'titles' | 'contentKind' | 'formatKind' | 'scopeKind' | 'outputKind' | 'compress' | 'compressionAlgo' | 'selectedSource' | 'moveToStep'> }>()
const emit = defineEmits<{ close: [] }>()
const rows = computed(() => exportParameterRows({ content: props.model.contentKind, format: props.model.formatKind, scope: props.model.scopeKind, output: props.model.outputKind, compressed: props.model.compress, algorithm: props.model.compressionAlgo, mode: props.model.selectedSource?.compatibilityMode }))
</script>

<template>
  <WizardParameterCatalog :open="open" title="导出全部参数" :entries="rows" :step-titles="model.titles" @close="emit('close')" @locate="model.moveToStep" />
</template>
