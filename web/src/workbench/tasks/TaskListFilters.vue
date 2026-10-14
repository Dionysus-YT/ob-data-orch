<script setup lang="ts">
import { CloseOutlined, SearchOutlined } from '@ant-design/icons-vue'
import { Button as AButton, DatePicker, FormItem as AFormItem, Input as AInput, Select as ASelect, SelectOption as ASelectOption, Tag as ATag } from 'ant-design-vue'
import dayjs from 'dayjs'
import 'dayjs/locale/zh-cn'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { taskListStates, type TaskEnvironment, type TaskFilterOptions, type TaskListState } from '@/api/browser'
import { hasExpandedTaskListConditions, hasTaskListConditions, taskListConditionChips, taskListDateRangeError, type TaskListRouteState } from './taskListQuery'
import { taskEnvironmentLabel, taskStateText } from './taskStatePresentation'

// 纯视图：筛选状态由列表所有者从路由 query 派生，本组件只发出修改意图，不持有第二份条件。
const props = defineProps<{ filters: TaskListRouteState; options: TaskFilterOptions | null; disabled?: boolean; dateRangeError?: string }>()
const emit = defineEmits<{ update: [patch: Partial<TaskListRouteState>]; clear: []; expand: [] }>()
const ARangePicker = DatePicker.RangePicker
// 每次展开刷新本地日期，长时间停留后重新打开不会复用旧的“今天”。
const presetToday = ref(dayjs().startOf('day'))
const datePresets = computed(() => [
  { label: '今天', value: [presetToday.value, presetToday.value] },
  { label: '昨天', value: [presetToday.value.subtract(1, 'day'), presetToday.value.subtract(1, 'day')] },
  { label: '最近 7 天', value: [presetToday.value.subtract(6, 'day'), presetToday.value] },
  { label: '最近 30 天', value: [presetToday.value.subtract(29, 'day'), presetToday.value] },
  { label: '本月', value: [presetToday.value.startOf('month'), presetToday.value] },
])
function refreshDatePresets(open: boolean) { if (open) presetToday.value = dayjs().startOf('day') }
const createdRangeError = computed(() => props.dateRangeError ? taskListDateRangeError({ ...props.filters, startedFrom: '', startedTo: '' }) : '')
const startedRangeError = computed(() => props.dateRangeError ? taskListDateRangeError({ ...props.filters, createdFrom: '', createdTo: '' }) : '')
const dateRangeValue = (from: string, to: string): [string, string] | undefined => from || to ? [from, to] : undefined
function updateDateRange(kind: 'created' | 'started', dates: unknown) {
  const values = Array.isArray(dates) ? dates : []
  const from = typeof values[0] === 'string' ? values[0] : ''
  const to = typeof values[1] === 'string' ? values[1] : ''
  // 两个边界作为一次修改意图提交，避免先发出半个区间的列表请求。
  emit('update', kind === 'created' ? { createdFrom: from, createdTo: to } : { startedFrom: from, startedTo: to })
}

const keywordInput = ref(props.filters.keyword)
const keywordField = ref<{ focus: () => void }>()
const expanded = ref(hasExpandedTaskListConditions(props.filters))
let keywordTimer: ReturnType<typeof setTimeout> | undefined
// 外部条件变化（清除、返回、单项移除）时同步输入框；用户正在输入时防抖提交。
watch(() => props.filters.keyword, value => { if (value !== keywordInput.value.trim()) keywordInput.value = value })
watch(() => hasExpandedTaskListConditions(props.filters), active => { if (active) expanded.value = true })
watch(expanded, value => { if (value) emit('expand') }, { immediate: true })
onBeforeUnmount(() => clearTimeout(keywordTimer))

function onKeyword(value: string) {
  keywordInput.value = value
  clearTimeout(keywordTimer)
  keywordTimer = setTimeout(() => emit('update', { keyword: value.trim() }), 300)
}

const environments: readonly TaskEnvironment[] = ['DEVELOPMENT', 'TEST', 'STAGING', 'PRODUCTION']
const environmentChoices = computed(() => props.options?.environments.length ? props.options.environments : environments)
const label = (list: TaskFilterOptions['dataSources'] | undefined, id: string) => list?.find(option => option.id === id)?.label || id
const chips = computed(() => taskListConditionChips(props.filters, (kind, id) => label(kind === 'dataSource' ? props.options?.dataSources : kind === 'node' ? props.options?.nodes : props.options?.creators, id)))
const active = computed(() => hasTaskListConditions(props.filters))
// 非全局身份不提供创建人候选，只能在“全部可见 / 本人创建”之间切换。
const creatorValue = computed(() => props.filters.creatorId || (props.filters.scope === 'MINE' ? '__MINE__' : ''))
function onCreator(value: unknown) {
  const selected = String(value ?? '')
  if (selected === '__MINE__') emit('update', { creatorId: '', scope: 'MINE' })
  else emit('update', { creatorId: selected, scope: props.filters.scope === 'MINE' ? '' : props.filters.scope })
}
function clearAll() {
  clearTimeout(keywordTimer)
  keywordInput.value = ''
  emit('clear')
  keywordField.value?.focus()
}
</script>

<template>
  <div class="task-filters" aria-label="筛选任务">
    <div class="task-filter-row">
      <label class="task-filter-search"><span class="orch-sr-only">任务名称 / ID 关键字</span><AInput ref="keywordField" :value="keywordInput" aria-label="任务名称 / ID 关键字" placeholder="搜索任务名称或 ID" :maxlength="128" :disabled="disabled" allow-clear @update:value="onKeyword"><template #prefix><SearchOutlined aria-hidden="true" /></template></AInput></label>
      <label class="task-filter"><span class="orch-sr-only">任务类型</span>
        <ASelect :value="filters.types[0] ?? ''" aria-label="任务类型" :disabled="disabled" @change="value => emit('update', { types: value ? ['OBDUMPER_EXPORT'] : [] })">
          <ASelectOption value="">全部类型</ASelectOption>
          <ASelectOption value="OBDUMPER_EXPORT">导出</ASelectOption>
          <ASelectOption value="OBLOADER_NORMAL_IMPORT" disabled>普通导入（尚未开放）</ASelectOption>
          <ASelectOption value="OBLOADER_DIRECT_LOAD" disabled>旁路导入（尚未开放）</ASelectOption>
        </ASelect>
      </label>
      <label class="task-filter task-filter-state"><span class="orch-sr-only">任务状态</span>
        <ASelect :value="[...filters.states]" mode="multiple" aria-label="任务状态" placeholder="全部状态" :max-tag-count="1" :disabled="disabled" @change="value => emit('update', { states: value as TaskListState[] })">
          <ASelectOption v-for="state in taskListStates" :key="state" :value="state">{{ taskStateText(state) }}</ASelectOption>
        </ASelect>
      </label>
      <AFormItem class="task-filter-range" label="创建时间" :validate-status="createdRangeError ? 'error' : undefined" :help="createdRangeError || undefined">
        <ARangePicker :value="dateRangeValue(filters.createdFrom, filters.createdTo)" :presets="datePresets" :allow-empty="[true, true]" :placeholder="['开始日期', '结束日期']" format="YYYY-MM-DD" value-format="YYYY-MM-DD" aria-label="创建时间日期范围" :disabled="disabled" @open-change="refreshDatePresets" @change="dates => updateDateRange('created', dates)" />
      </AFormItem>
      <AButton type="link" :aria-expanded="expanded" aria-controls="task-filter-expanded" @click="expanded = !expanded">{{ expanded ? '收起筛选' : '展开筛选' }}</AButton>
      <AButton v-if="active" type="link" @click="clearAll">清除筛选</AButton>
    </div>
    <div v-show="expanded" id="task-filter-expanded" class="task-filter-row">
      <label class="task-filter"><span class="orch-sr-only">数据源</span>
        <ASelect :value="filters.dataSourceId" aria-label="数据源" show-search option-filter-prop="label" :disabled="disabled" @change="value => emit('update', { dataSourceId: String(value ?? '') })">
          <ASelectOption value="" label="全部数据源">全部数据源</ASelectOption>
          <ASelectOption v-for="option in options?.dataSources ?? []" :key="option.id" :value="option.id" :label="option.label || option.id">{{ option.label || option.id }}</ASelectOption>
        </ASelect>
      </label>
      <label class="task-filter"><span class="orch-sr-only">环境</span>
        <ASelect :value="filters.environment" aria-label="环境" :disabled="disabled" @change="value => emit('update', { environment: (value || '') as TaskEnvironment | '' })">
          <ASelectOption value="">全部环境</ASelectOption>
          <ASelectOption v-for="environment in environmentChoices" :key="environment" :value="environment">{{ taskEnvironmentLabel(environment) }}</ASelectOption>
        </ASelect>
      </label>
      <label class="task-filter"><span class="orch-sr-only">执行节点</span>
        <ASelect :value="filters.nodeId" aria-label="执行节点" show-search option-filter-prop="label" :disabled="disabled" @change="value => emit('update', { nodeId: String(value ?? '') })">
          <ASelectOption value="" label="全部执行节点">全部执行节点</ASelectOption>
          <ASelectOption v-for="option in options?.nodes ?? []" :key="option.id" :value="option.id" :label="option.label || option.id">{{ option.label || option.id }}</ASelectOption>
        </ASelect>
      </label>
      <label class="task-filter"><span class="orch-sr-only">创建人</span>
        <ASelect :value="creatorValue" aria-label="创建人" show-search option-filter-prop="label" :disabled="disabled" @change="onCreator">
          <ASelectOption value="" label="全部可见任务">全部可见任务</ASelectOption>
          <ASelectOption value="__MINE__" label="本人创建">本人创建</ASelectOption>
          <template v-if="options?.creatorFilterAvailable"><ASelectOption v-for="option in options.creators" :key="option.id" :value="option.id" :label="option.label || option.id">{{ option.label || option.id }}</ASelectOption></template>
        </ASelect>
      </label>
      <AFormItem class="task-filter-range" label="开始时间" :validate-status="startedRangeError ? 'error' : undefined" :help="startedRangeError || undefined">
        <ARangePicker :value="dateRangeValue(filters.startedFrom, filters.startedTo)" :presets="datePresets" :allow-empty="[true, true]" :placeholder="['开始日期', '结束日期']" format="YYYY-MM-DD" value-format="YYYY-MM-DD" aria-label="开始时间日期范围" :disabled="disabled" @open-change="refreshDatePresets" @change="dates => updateDateRange('started', dates)" />
      </AFormItem>
    </div>
    <div v-if="chips.length" class="task-filter-chips" aria-label="生效的筛选条件">
      <!-- Tag 原生关闭图标不可聚焦，移除入口使用带可访问名称的按钮。 -->
      <ATag v-for="chip in chips" :key="chip.key" class="task-filter-chip">{{ chip.label }}<AButton type="text" size="small" class="task-filter-chip-remove" :aria-label="`移除筛选 ${chip.label}`" :disabled="disabled" @click="emit('update', chip.clear)"><template #icon><CloseOutlined aria-hidden="true" /></template></AButton></ATag>
    </div>
  </div>
</template>

<style scoped>
.task-filters { display: grid; gap: var(--ob-foundation-space-3); padding: var(--ob-foundation-space-4) var(--ob-foundation-space-6); border-top: 1px solid var(--ob-color-border); }
.task-filter-row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--ob-foundation-space-2); }
.task-filter-search { flex: 1 1 220px; max-width: 320px; }
.task-filter { flex: 0 1 160px; min-width: 140px; }
.task-filter-state { flex-basis: 200px; }
.task-filter :deep(.ant-select) { width: 100%; }
.task-filter-range { flex: 0 1 auto; min-inline-size: 0; margin: 0; }
.task-filter-range :deep(.ant-picker) { inline-size: 280px; max-inline-size: 100%; }
.task-filter-chips { display: flex; flex-wrap: wrap; gap: var(--ob-foundation-space-2); }
.task-filter-chip { display: inline-flex; align-items: center; gap: var(--ob-foundation-space-1); margin: 0; }
.task-filter-chip-remove { min-width: 0; height: auto; padding: 0; }
@media (max-width: 720px) { .task-filters { padding: var(--ob-foundation-space-4); } .task-filter-search { max-width: none; flex-basis: 100%; } .task-filter { flex-grow: 1; } .task-filter-range { flex-basis: 100%; } }
</style>
