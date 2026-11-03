<script setup lang="ts">
import { computed, type UnwrapNestedRefs } from 'vue'
import { Alert as AAlert, Button as AButton, Collapse as ACollapse, CollapsePanel as ACollapsePanel, Descriptions as ADescriptions, DescriptionsItem as ADescriptionsItem, Drawer as ADrawer, Form as AForm, FormItem as AFormItem, Select as ASelect, SelectOption as ASelectOption } from 'ant-design-vue'
import { ReloadOutlined } from '@ant-design/icons-vue'
import { useAntDrawerDialog } from '@/composables/useAntDrawerDialog'
import { product } from '@/platform/tokens'
import { nodePlatformLabel } from '@/workbench/nodes/executionNodePresentation'
import { taskTypeText } from '@/workbench/tasks/taskStatePresentation'
import type { useTemplateDraft } from './useTemplateDraft'

const props = defineProps<{ model: UnwrapNestedRefs<ReturnType<typeof useTemplateDraft>>; busy: boolean }>()
const emit = defineEmits<{ close: []; inspectDrafts: [] }>()
const open = computed(() => Boolean(props.model.selectedTemplate))
const locked = computed(() => props.busy || props.model.resultUnknown || Boolean(props.model.createdDraft))
const { heading, afterOpenChange } = useAntDrawerDialog('template-draft-title', () => emit('close'), () => props.busy, () => open.value)
function setHeading(element: unknown) { heading.value = element instanceof HTMLElement ? element : undefined }
</script>

<template>
  <ADrawer :open="open" :width="'min(' + product.inspector.width + 'px, 100vw)'" :closable="!busy" :keyboard="!busy" :mask-closable="false" :push="false" :destroy-on-close="true" root-class-name="orch-inspector" class="orch-ui" @close="!busy && emit('close')" @after-open-change="afterOpenChange">
    <template #title><h2 id="template-draft-title" :ref="setHeading" tabindex="-1" class="template-drawer-title">使用模板</h2></template>
    <template v-if="model.selectedTemplate">
      <div class="template-draft-identity"><strong>{{ model.selectedTemplate.displayName }}</strong><span>{{ taskTypeText(model.selectedTemplate.taskType) }}</span></div>
      <AAlert v-if="model.createDraftFailure" class="orch-feedback" :type="model.resultUnknown ? 'warning' : model.createdDraft ? 'info' : 'error'" :message="model.resultUnknown ? '创建结果待确认' : model.createdDraft ? '草稿已创建' : '操作未完成'" :description="model.createDraftFailure" show-icon>
        <template v-if="model.resultUnknown" #action><AButton @click="emit('inspectDrafts')">查看我的草稿</AButton></template>
      </AAlert>
      <AForm name="template-binding" :model="{ dataSource: model.draftSourceID, node: model.draftNodeID }" layout="vertical" :aria-busy="model.sourcesLoading">
        <AFormItem name="dataSource" label="数据源" required><ASelect :value="model.draftSourceID || undefined" placeholder="请选择数据源" :loading="model.sourcesLoading" :disabled="locked || model.sourcesLoading" show-search option-filter-prop="label" :options="model.eligibleSources.map(source => ({ value: source.id, label: source.displayName }))" @update:value="value => model.selectSource(String(value))" /></AFormItem>
        <AFormItem name="node" label="执行节点" required><ASelect :value="model.draftNodeID || undefined" placeholder="请选择执行节点" :loading="model.sourcesLoading" :disabled="locked || model.sourcesLoading && !model.nodes.length" show-search option-filter-prop="label" @update:value="value => model.selectNode(String(value))" @dropdown-visible-change="model.onNodeDropdownVisibleChange"><ASelectOption v-for="node in model.nodes" :key="node.id" :value="node.id" :label="node.displayName + ' · ' + nodePlatformLabel(node.platform)">{{ node.displayName }} · {{ nodePlatformLabel(node.platform) }}</ASelectOption></ASelect></AFormItem>
        <div class="template-candidate-actions"><AButton :loading="model.sourcesLoading" :disabled="locked" @click="model.loadChoices(true)"><template #icon><ReloadOutlined class="product-icon" aria-hidden="true" /></template>刷新候选</AButton></div>
        <AAlert v-if="!model.sourcesLoading && !model.createDraftFailure && (!model.eligibleSources.length || !model.nodes.length)" class="orch-feedback" type="info" message="缺少可选绑定" :description="[!model.eligibleSources.length ? '当前没有可用数据源。' : '', !model.nodes.length ? '当前没有可用执行节点。' : ''].join('')" show-icon />
      </AForm>
      <p class="template-draft-hint">创建后进入向导重新检查。</p>
      <ACollapse ghost><ACollapsePanel key="technical" header="技术信息"><ADescriptions :column="1" size="small"><ADescriptionsItem label="模板 ID"><code>{{ model.selectedTemplate.id }}</code></ADescriptionsItem><ADescriptionsItem label="能力版本"><code>{{ model.selectedTemplate.capabilityVersion }}</code></ADescriptionsItem><ADescriptionsItem label="修订">{{ model.selectedTemplate.revision }}</ADescriptionsItem><ADescriptionsItem label="创建时间">{{ model.selectedTemplate.createdAt }}</ADescriptionsItem></ADescriptions></ACollapsePanel></ACollapse>
    </template>
    <template #footer><div class="template-drawer-footer"><AButton :disabled="busy" @click="emit('close')">取消</AButton><AButton v-if="model.createdDraft" type="primary" :loading="busy" @click="model.openCreatedDraft()">打开草稿</AButton><AButton v-else-if="model.resultUnknown" type="primary" :loading="busy" @click="model.retryCreate()">用原请求重试</AButton><AButton v-else type="primary" :loading="busy" :disabled="!model.canCreate" @click="model.createDraft()">{{ busy ? '创建中…' : '创建草稿' }}</AButton></div></template>
  </ADrawer>
</template>

<style scoped>
.template-drawer-title { margin: 0; font-size: var(--ob-component-overlay-title-size); line-height: var(--ob-component-overlay-title-line-height); }
.template-draft-identity { display: grid; gap: var(--ob-foundation-space-1); margin-block-end: var(--ob-foundation-space-4); overflow-wrap: anywhere; }
.template-draft-identity span, .template-draft-hint { color: var(--ob-color-secondary); }
.template-candidate-actions { margin-block-end: var(--ob-foundation-space-3); }
.template-drawer-footer { display: flex; justify-content: flex-end; gap: var(--ob-foundation-space-2); }
code { overflow-wrap: anywhere; }
</style>
