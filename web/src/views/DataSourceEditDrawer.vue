<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Check, Circle } from '@lucide/vue'

import WorkbenchDrawer from '@/components/WorkbenchDrawer.vue'
import WorkbenchAlertDialog from '@/components/WorkbenchAlertDialog.vue'
import DataSourceFormView from './DataSourceFormView.vue'

const props = withDefaults(
  defineProps<{ modelValue: boolean; dataSourceId?: string | null; focusTest?: boolean }>(),
  { dataSourceId: null, focusTest: false },
)
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; saved: [id: string] }>()

const dirty = ref(false)
const discardConfirmVisible = ref(false)
const testCloseConfirmVisible = ref(false)
const testRunning = ref(false)
const title = computed(() => (props.dataSourceId ? '编辑数据源' : '新增数据源'))
const subtitle = computed(() => (props.dataSourceId ? '已保存配置可在本抽屉内单独测试连接。' : '保存后才能由所选在线执行节点发起真实连接测试。'))
const editState = computed(() => (dirty.value ? '未保存更改' : props.dataSourceId ? '已保存' : '新建配置'))

watch(
  () => props.modelValue,
  (visible) => {
    if (visible) return
    dirty.value = false
    discardConfirmVisible.value = false
    testCloseConfirmVisible.value = false
    testRunning.value = false
  },
)

function requestClose() {
  if (testRunning.value) {
    testCloseConfirmVisible.value = true
    return
  }
  if (dirty.value) {
    discardConfirmVisible.value = true
    return
  }
  close()
}

function closeWhileTestContinues() {
  testCloseConfirmVisible.value = false
  close()
}

function close() {
  emit('update:modelValue', false)
}

function discardAndClose() {
  dirty.value = false
  discardConfirmVisible.value = false
  close()
}

function onSaved(id: string) {
  dirty.value = false
  emit('saved', id)
}
</script>

<template>
  <WorkbenchDrawer
    :open="modelValue"
    :title="title"
    :subtitle="subtitle"
    @close="requestClose"
  >
    <template #header-meta><span class="drawer-edit-state" :class="{ 'is-dirty': dirty }" role="status"><Circle v-if="dirty" :size="8" fill="currentColor" aria-hidden="true" /><Check v-else-if="dataSourceId" :size="14" aria-hidden="true" />{{ editState }}</span></template>
    <DataSourceFormView
      :data-source-id="dataSourceId"
      :focus-test="focusTest"
      @saved="onSaved"
      @cancel="requestClose"
      @dirty-change="dirty = $event"
      @test-running-change="testRunning = $event"
    />
  </WorkbenchDrawer>

  <WorkbenchAlertDialog
    :open="discardConfirmVisible"
    title="放弃未保存的更改？"
    description="当前配置存在未保存的更改。关闭后，这些更改将无法恢复。"
    confirm-label="放弃更改"
    destructive
    @cancel="discardConfirmVisible = false"
    @confirm="discardAndClose"
  />
  <WorkbenchAlertDialog
    :open="testCloseConfirmVisible"
    title="连接测试仍在进行"
    description="关闭不会取消已提交的连接测试；执行节点会继续处理。稍后重新打开数据源可查看测试状态。"
    confirm-label="仍然关闭"
    @cancel="testCloseConfirmVisible = false"
    @confirm="closeWhileTestContinues"
  />
</template>

<style scoped>
.drawer-edit-state {
  display: inline-flex;
  min-height: 32px;
  align-items: center;
  gap: 6px;
  color: var(--color-text-tertiary);
  font-size: var(--text-metadata-size);
  white-space: nowrap;
}

.drawer-edit-state.is-dirty {
  color: var(--color-warning);
}
</style>
