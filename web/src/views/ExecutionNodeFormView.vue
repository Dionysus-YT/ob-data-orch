<script setup lang="ts">
import { Alert as AAlert, Button as AButton, Collapse as ACollapse, CollapsePanel as ACollapsePanel, Drawer as ADrawer, Form as AForm, FormItem as AFormItem, Input as AInput, InputNumber as AInputNumber, Select as ASelect, SelectOption as ASelectOption, Skeleton as ASkeleton, Textarea as ATextarea } from 'ant-design-vue'
import { computed, nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import { useAntDrawerDialog } from '@/composables/useAntDrawerDialog'
import { useOperationMessage } from '@/composables/useOperationMessage'
import { product } from '@/platform/tokens'
import type { NodeFormField } from '@/workbench/nodes/nodeFormRules'
import { useNodeForm } from '@/workbench/nodes/useNodeForm'
import { nodePlatformHelp } from '@/workbench/nodes/nodePlatformRules'

const props = defineProps<{ listRestricted?: boolean }>()
const emit = defineEmits<{ saved: []; saving: [value: boolean] }>()
const route = useRoute()
const router = useRouter()
const errorSummary = ref<HTMLElement>()
const advancedPanels = ref<string[]>([])
const confirmClose = ref(false)
const nodeID = computed(() => typeof route.params.id === 'string' ? route.params.id : '')
const isNew = computed(() => route.name === 'node-new')
let navigatingCreated = false
let resolveNavigation: ((leave: boolean) => void) | undefined
const { session, node, loading, busy, restricted, dirty, failure, notice, formErrors, form, operatingSystem, cpuArchitecture, operatingSystemOptions, cpuArchitectureOptions, errorEntries, loadNode, save, clearError, validateField, revokeAccess } = useNodeForm(
  nodeID, isNew,
  async id => {
    // 仅已创建事务的后续导航可越过保存锁；其他关闭与路由操作仍受未保存确认约束。
    navigatingCreated = true
    try { await router.replace({ name: 'node-detail', params: { id }, query: { registration: 'created' } }) }
    finally { navigatingCreated = false }
  },
  async active => {
    if (formErrors.budgetPolicy) advancedPanels.value = ['budget']
    await nextTick()
    if (active()) { errorSummary.value?.focus(); errorSummary.value?.scrollIntoView({ block: 'nearest' }) }
  },
)
const titleID = useId()
const { MessageContext } = useOperationMessage(notice)
const { heading: drawerHeading, afterOpenChange } = useAntDrawerDialog(titleID, close, () => busy.value)
void drawerHeading
const platformHelp = computed(() => nodePlatformHelp(form.platform))

watch(busy, value => emit('saving', value), { flush: 'sync' })
watch(() => props.listRestricted, denied => { if (denied) revokeAccess() }, { immediate: true, flush: 'sync' })
watch([nodeID, isNew], () => { advancedPanels.value = [] }, { flush: 'sync' })
watch(node, value => {
  if (value?.budgetPolicy?.systemReservedBytes || value?.budgetPolicy?.taskOverheadBytes) advancedPanels.value = ['budget']
})

function close() { if (!busy.value) void router.replace({ name: 'nodes' }) }
function guardNavigation() {
  if (navigatingCreated) return true
  if (busy.value) return false
  if (!dirty.value) return true
  // 新导航替代旧意图，取消旧等待，不能让多个待决导航共享一次确认。
  resolveNavigation?.(false)
  confirmClose.value = true
  return new Promise<boolean>(resolve => { resolveNavigation = resolve })
}
function finishNavigation(leave: boolean) {
  confirmClose.value = false
  const resolve = resolveNavigation
  resolveNavigation = undefined
  resolve?.(leave)
}
onBeforeRouteLeave(guardNavigation)
onBeforeRouteUpdate(guardNavigation)
onBeforeUnmount(() => { finishNavigation(false); emit('saving', false) })

async function saveForm() {
  if (props.listRestricted) return
  const active = session.capture()
  if (await save() && active()) emit('saved')
}
async function focusField(field: NodeFormField, id: string) {
  const active = session.capture()
  if (field === 'budgetPolicy') advancedPanels.value = ['budget']
  await nextTick()
  if (!active()) return
  const input = document.getElementById(id)
  input?.focus()
  input?.scrollIntoView({ block: 'nearest' })
}
</script>

<template>
  <MessageContext />
  <ADrawer :open="true" :width="'min(' + product.source.editorWidth + 'px, 100vw)'" :closable="!busy" :keyboard="!busy" :mask-closable="false" :push="false" root-class-name="orch-inspector" class="orch-ui node-editor" @close="close" @after-open-change="afterOpenChange">
    <template #title><h2 :id="titleID" ref="drawerHeading" class="ant-drawer-title" tabindex="-1">{{ isNew ? '注册执行节点' : '编辑执行节点' }}</h2></template>
    <ASkeleton v-if="loading" active :paragraph="{ rows: 8 }" role="status" aria-label="正在加载执行节点配置" />
    <template v-else>
      <AAlert v-if="failure" class="orch-feedback" type="error" :message="restricted ? '节点已不可访问' : '操作未完成'" :description="failure" show-icon>
        <template #action><AButton v-if="!isNew && !node && !listRestricted" @click="loadNode">重新读取</AButton></template>
      </AAlert>
      <AForm v-if="!restricted && (isNew || node)" id="node-configuration" :model="form" :disabled="busy" layout="vertical" aria-label="执行节点配置" novalidate @submit.prevent="saveForm">
        <p class="node-editor-intro">{{ isNew ? '填写目标机器上的配置，保存后继续 Agent 关联。' : '修改后由 Agent 在空闲时同步配置，无需重新注册。' }}</p>
        <div v-if="errorEntries.length" ref="errorSummary" class="node-error-summary" tabindex="-1">
          <AAlert type="error" message="请检查以下字段" show-icon role="alert">
            <template #description><ul><li v-for="entry in errorEntries" :key="entry.field"><a :href="'#' + entry.id" @click.prevent="focusField(entry.field, entry.id)">{{ entry.label }}：{{ entry.message }}</a></li></ul></template>
          </AAlert>
        </div>
        <fieldset :disabled="busy" class="node-editor-fields">
          <section class="node-editor-section" aria-labelledby="node-basics-title">
            <h3 id="node-basics-title">基本信息</h3>
            <div class="node-editor-group node-editor-grid">
              <AFormItem class="node-editor-name" label="节点名称" name="displayName" html-for="node-display-name" required :validate-status="formErrors.displayName ? 'error' : undefined">
                <AInput id="node-display-name" v-model:value.trim="form.displayName" :aria-invalid="Boolean(formErrors.displayName)" :aria-describedby="formErrors.displayName ? 'node-name-error' : undefined" :maxlength="200" placeholder="便于识别的节点名称" @input="clearError('displayName')" @blur="validateField('displayName')" />
                <template v-if="formErrors.displayName" #help><span id="node-name-error">{{ formErrors.displayName }}</span></template>
              </AFormItem>
              <AFormItem label="操作系统" name="platform" html-for="node-platform" required :validate-status="formErrors.platform ? 'error' : undefined">
                <ASelect id="node-platform" v-model:value="operatingSystem" :aria-invalid="Boolean(formErrors.platform)" :aria-describedby="formErrors.platform ? 'node-platform-error' : undefined">
                  <ASelectOption v-for="option in operatingSystemOptions" :key="option.value" :value="option.value">{{ option.label }}</ASelectOption>
                </ASelect>
                <template v-if="formErrors.platform" #help><span id="node-platform-error">{{ formErrors.platform }}</span></template>
              </AFormItem>
              <!-- Select 不把描述属性传给内部输入框，用字段分组关联选择控件与架构帮助。 -->
              <AFormItem label="CPU 架构" html-for="node-cpu-architecture" role="group" aria-label="CPU 架构" :aria-describedby="formErrors.platform ? 'node-platform-error node-architecture-hint' : 'node-architecture-hint'" required :validate-status="formErrors.platform ? 'error' : undefined">
                <ASelect id="node-cpu-architecture" v-model:value="cpuArchitecture" :aria-invalid="Boolean(formErrors.platform)">
                  <ASelectOption v-for="option in cpuArchitectureOptions" :key="option.value" :value="option.value" :disabled="option.disabled">{{ option.label }}</ASelectOption>
                </ASelect>
                <template #extra><span id="node-architecture-hint">{{ platformHelp.architectureHint }}</span></template>
              </AFormItem>
            </div>
          </section>
          <section class="node-editor-section" aria-labelledby="node-runtime-title">
            <h3 id="node-runtime-title">工具运行时</h3>
            <div class="node-editor-group">
              <AFormItem label="OB Loader/Dumper 安装目录" name="toolHome" html-for="node-tool-home" required :validate-status="formErrors.toolHome ? 'error' : undefined">
                <AInput id="node-tool-home" v-model:value.trim="form.toolHome" class="node-path" :aria-invalid="Boolean(formErrors.toolHome)" :aria-describedby="formErrors.toolHome ? 'node-tool-error node-tool-hint' : 'node-tool-hint'" :placeholder="platformHelp.toolHomePlaceholder" @input="clearError('toolHome')" @blur="validateField('toolHome')" />
                <template v-if="formErrors.toolHome" #help><span id="node-tool-error">{{ formErrors.toolHome }}</span></template>
                <template #extra><span id="node-tool-hint">{{ platformHelp.toolHomeHint }}</span></template>
              </AFormItem>
              <AFormItem label="工具专用 Java 8 路径" name="javaPath" html-for="node-java-path" required :validate-status="formErrors.javaPath ? 'error' : undefined">
                <AInput id="node-java-path" v-model:value.trim="form.javaPath" class="node-path" :aria-invalid="Boolean(formErrors.javaPath)" :aria-describedby="formErrors.javaPath ? 'node-java-error node-java-hint' : 'node-java-hint'" :placeholder="platformHelp.javaPathPlaceholder" @input="clearError('javaPath')" @blur="validateField('javaPath')" />
                <template v-if="formErrors.javaPath" #help><span id="node-java-error">{{ formErrors.javaPath }}</span></template>
                <template #extra><span id="node-java-hint">{{ platformHelp.javaPathHint }}</span></template>
              </AFormItem>
            </div>
          </section>
          <section class="node-editor-section" aria-labelledby="node-roots-title">
            <h3 id="node-roots-title">导出数据目录</h3>
            <AFormItem label="导出数据目录白名单" name="allowedRootsText" html-for="node-allowed-roots" required :validate-status="formErrors.allowedRoots ? 'error' : undefined">
              <ATextarea id="node-allowed-roots" v-model:value="form.allowedRootsText" class="node-path" :aria-invalid="Boolean(formErrors.allowedRoots)" :aria-describedby="formErrors.allowedRoots ? 'node-roots-error node-roots-hint' : 'node-roots-hint'" :rows="3" :placeholder="platformHelp.rootPlaceholder" @input="clearError('allowedRoots')" @blur="validateField('allowedRoots')" />
              <template v-if="formErrors.allowedRoots" #help><span id="node-roots-error">{{ formErrors.allowedRoots }}</span></template>
              <template #extra><span id="node-roots-hint">{{ platformHelp.rootHint }}仅允许导出写入这些目录。</span></template>
            </AFormItem>
          </section>
          <ACollapse v-model:active-key="advancedPanels" ghost class="node-budget-disclosure">
            <ACollapsePanel key="budget" header="执行内存预算（可选）">
              <p class="node-editor-hint">默认 JVM 堆为 4 GiB，任务可覆盖。以下比例是可调整的工程默认值。</p>
              <AFormItem label="系统与 Agent 预留（GiB）" name="budgetPolicy" html-for="node-budget-policy" :validate-status="formErrors.budgetPolicy ? 'error' : undefined">
                <AInputNumber id="node-budget-policy" :value="form.systemReservedGiB ?? undefined" class="node-budget-input" :aria-invalid="Boolean(formErrors.budgetPolicy)" :aria-describedby="formErrors.budgetPolicy ? 'node-budget-error node-system-reserve-hint' : 'node-system-reserve-hint'" :min="0" :step="0.25" :precision="2" placeholder="自动" @update:value="value => form.systemReservedGiB = value == null ? undefined : Number(value)" @change="clearError('budgetPolicy')" @blur="validateField('budgetPolicy')" />
                <template v-if="formErrors.budgetPolicy" #help><span id="node-budget-error">{{ formErrors.budgetPolicy }}</span></template>
                <template #extra><span id="node-system-reserve-hint">留空或填 0：总内存的 20%，至少 2 GiB。</span></template>
              </AFormItem>
              <AFormItem label="每任务堆外预算（GiB）" html-for="node-task-overhead">
                <AInputNumber id="node-task-overhead" :value="form.taskOverheadGiB ?? undefined" class="node-budget-input" :aria-invalid="Boolean(formErrors.budgetPolicy)" :aria-describedby="formErrors.budgetPolicy ? 'node-budget-error node-task-overhead-hint' : 'node-task-overhead-hint'" :min="0" :step="0.25" :precision="2" placeholder="自动" @update:value="value => form.taskOverheadGiB = value == null ? undefined : Number(value)" @change="clearError('budgetPolicy')" @blur="validateField('budgetPolicy')" />
                <template #extra><span id="node-task-overhead-hint">留空或填 0：实际 JVM 堆的 25%，至少 1 GiB。</span></template>
              </AFormItem>
            </ACollapsePanel>
          </ACollapse>
        </fieldset>
        <p class="node-editor-boundary" role="note">{{ isNew ? '新节点保存为已禁用、待关联；完成 Agent 关联与环境检查后才能启用。' : '保存保留当前管理状态；配置变更需要 Agent 同步并复验环境。' }}</p>
      </AForm>
    </template>
    <template #footer>
      <div class="node-editor-actions">
        <AButton :disabled="busy" @click="close">取消</AButton>
        <AButton type="primary" html-type="submit" form="node-configuration" :loading="busy" :disabled="loading || restricted || listRestricted || (!isNew && !node)">{{ isNew ? '保存并继续 Agent 关联' : '保存修改' }}</AButton>
      </div>
    </template>
  </ADrawer>
  <OrchDangerConfirm :open="confirmClose" title="放弃未保存的节点配置？" confirm-label="放弃修改" cancel-label="继续编辑" destructive @cancel="finishNavigation(false)" @confirm="finishNavigation(true)">
    <p>尚有未保存的节点配置。离开后这些修改将丢失。</p>
  </OrchDangerConfirm>
</template>

<style scoped>
.node-editor-intro, .node-editor-hint, .node-editor-boundary { color: var(--ob-color-form-secondary); font-size: var(--ob-component-field-helper-size); line-height: var(--ob-component-field-helper-line-height); overflow-wrap: anywhere; }
.node-editor-intro { margin: 0 0 var(--ob-foundation-space-6); }
.node-editor-fields { min-width: 0; margin: 0; padding: 0; border: 0; }
.node-editor-section { margin-bottom: var(--ob-foundation-space-6); }
.node-editor-section h3 { margin: 0 0 var(--ob-foundation-space-3); color: var(--ob-color-form-text); font-size: var(--ob-product-typography-section-size); font-weight: 500; }
.node-editor-group { min-width: 0; padding: var(--ob-foundation-space-4); background: var(--ob-color-subtle); }
.node-editor-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 var(--ob-foundation-space-4); }
.node-editor-name { grid-column: 1 / -1; }
.node-path { font-family: var(--ob-foundation-font-mono); }
.node-budget-input { width: 100%; }
.node-editor-hint { margin: 0 0 var(--ob-foundation-space-4); }
.node-editor-boundary { margin: var(--ob-foundation-space-4) 0 0; padding-top: var(--ob-foundation-space-4); border-top: 1px solid var(--ob-color-border); }
.node-editor-actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: var(--ob-foundation-space-2); }
.node-error-summary { margin-bottom: var(--ob-foundation-space-4); }
.node-error-summary ul { margin: 0; padding-left: var(--ob-foundation-space-4); }
.node-error-summary a { color: inherit; }
@media (max-width: 480px) { .node-editor-grid { grid-template-columns: minmax(0, 1fr); }.node-editor-actions { flex-wrap: nowrap; } }
</style>
