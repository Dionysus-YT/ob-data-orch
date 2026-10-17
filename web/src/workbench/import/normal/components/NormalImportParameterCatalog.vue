<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import { Button, Drawer, Input } from 'ant-design-vue'
import { useAntDrawerDialog } from '@/composables/useAntDrawerDialog'
import { normalImportTitles } from '../normalImportOptions'
import { normalImportParameterCatalog, normalImportParameterRows } from '../normalImportParameterCatalog'
import type { UnwrapNestedRefs } from 'vue'
import type { NormalImportWizard } from '../useNormalImportWizard'

const props = defineProps<{ open: boolean; model: Pick<UnwrapNestedRefs<NormalImportWizard>, 'form' | 'selectedSource' | 'moveToStep'> }>()
const emit = defineEmits<{ close: [] }>()
const keyword = ref('')
const titleId = useId()
const { heading, afterOpenChange } = useAntDrawerDialog(titleId, () => emit('close'))
void heading
const rows = computed(() => {
  const query = keyword.value.trim().toLocaleLowerCase()
  return normalImportParameterRows(props.model.form, props.model.selectedSource?.compatibilityMode)
    .filter(row => `${row.name} ${row.label} ${row.description} ${row.reason}`.toLocaleLowerCase().includes(query))
})
function locate(step: number) { emit('close'); void props.model.moveToStep(step) }
</script>

<template>
  <Drawer :open="open" width="min(100vw, 720px)" :destroy-on-close="true" @close="emit('close')" @after-open-change="afterOpenChange">
    <template #title><h2 :id="titleId" ref="heading" class="ant-drawer-title" tabindex="-1">普通导入全部参数</h2></template>
    <div class="normal-import-catalog">
      <label :for="titleId + '-search'">搜索参数、名称或适用条件</label><Input :id="titleId + '-search'" v-model:value="keyword" allow-clear placeholder="例如 ctl-path、压缩、Oracle" />
      <p role="status">共 {{ normalImportParameterCatalog.length }} 个参数，当前显示 {{ rows.length }} 项。仅列本轮适用配置与受管派生项；目录覆盖不代表功能已验证。</p>
      <ul><li v-for="row in rows" :key="row.name"><div><code>--{{ row.name }}</code><strong>{{ row.label }}</strong><Button type="link" @click="locate(row.step)">{{ normalImportTitles[row.step - 1] }}</Button></div><p>{{ row.description }}</p><p>{{ row.reason }}</p></li></ul>
      <p v-if="!rows.length">没有匹配参数，请调整搜索条件。</p>
    </div>
  </Drawer>
</template>

<style scoped>
.normal-import-catalog { display: grid; gap: var(--ob-foundation-space-3); min-inline-size: 0; }
.normal-import-catalog ul { list-style: none; padding: 0; margin: 0; }
.normal-import-catalog li { padding-block: var(--ob-foundation-space-3); border-block-end: 1px solid var(--ob-color-border); overflow-wrap: anywhere; }
.normal-import-catalog li > div { display: flex; flex-wrap: wrap; gap: var(--ob-foundation-space-3); align-items: center; }
.normal-import-catalog p { margin: 0; color: var(--ob-color-secondary); }
</style>
