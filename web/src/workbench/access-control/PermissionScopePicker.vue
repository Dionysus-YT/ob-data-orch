<script setup lang="ts">
import { computed } from 'vue'
import { Alert as AAlert, Button as AButton, Select as ASelect } from 'ant-design-vue'
import type { AccessControlApi, PermissionScopeObject, PermissionScopeType } from '@/api/accessControl'
import { scopeLabels } from './permissionRules'
import { usePermissionCandidates } from './usePermissionCandidates'

const props = defineProps<{ api: AccessControlApi; type: PermissionScopeType; session: number; value: readonly string[]; baseline: readonly string[]; objects: readonly PermissionScopeObject[]; disabled: boolean; allowAdd: boolean; productionAllowed: boolean }>()
const emit = defineEmits<{ change: [ids: string[]]; facts: [session: number, items: readonly PermissionScopeObject[]]; denied: [session: number] }>()
const model = usePermissionCandidates(props.api, () => props.session, () => props.type, () => emit('denied', props.session))
const entries = computed(() => {
  const rows = new Map(model.items.value.map(row => [row.id, { type: props.type, objectId: row.id, displayName: row.displayName, available: row.available, ...(row.environment ? { environment: row.environment } : {}) }]))
  for (const row of props.objects) if (row.type === props.type && (props.value.includes(row.objectId) || props.baseline.includes(row.objectId)) && !rows.has(row.objectId)) rows.set(row.objectId, row)
  return [...rows.values()]
})
const options = computed(() => entries.value.map(row => {
  const production = row.environment === 'PROD' || row.environment === 'PRODUCTION'
  const baseline = props.baseline.includes(row.objectId)
  const productionBlocked = production && ['DATA_SOURCE_USE', 'TASK_OPERATE_BY_DATA_SOURCE'].includes(props.type) && !props.productionAllowed
  return { value: row.objectId, label: `${row.displayName}${production ? ' · 生产' : ''}${!row.available ? ' · 不可新增' : ''}`, disabled: !baseline && (!props.allowAdd || !row.available || productionBlocked) }
}))
function change(value: unknown) { if (props.disabled || !Array.isArray(value) || value.some(id => typeof id !== 'string')) return; const ids = value as string[]; emit('facts', props.session, entries.value.filter(row => ids.includes(row.objectId))); emit('change', ids) }
function search(value: string) { if (!props.disabled && props.allowAdd) model.setSearch(value) }
</script>

<template>
  <div class="permission-scope-picker">
    <ASelect :id="`permission-scope-${type}`" :aria-label="scopeLabels[type]" mode="multiple" :value="[...value]" :options="options" :disabled="disabled" :loading="model.loading.value" :filter-option="false" :max-tag-count="3" :show-search="allowAdd" :placeholder="allowAdd ? '搜索并选择明确对象' : '暂无可保留的范围'" :on-search="allowAdd ? search : undefined" @change="change" @dropdown-visible-change="visible => { if (visible && !disabled && allowAdd) model.begin() }" />
    <AAlert v-if="model.failure.value" type="error" show-icon :message="model.failure.value"><template #action><AButton type="text" :disabled="model.loading.value || disabled" @click="() => model.read()">重试候选</AButton></template></AAlert>
    <AButton v-if="model.nextCursor.value" type="link" :disabled="model.loading.value || disabled || model.items.value.length >= 500" @click="() => model.read(true)">{{ model.items.value.length >= 500 ? '请缩小搜索范围' : '加载更多候选' }}</AButton>
  </div>
</template>

<style scoped>
.permission-scope-picker { display: grid; gap: var(--ob-foundation-space-2); min-inline-size: 0; }
</style>
