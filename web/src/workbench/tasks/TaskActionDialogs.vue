<script setup lang="ts">
import { computed, type UnwrapNestedRefs } from 'vue'
import { Alert as AAlert, FormItem as AFormItem, Input as AInput } from 'ant-design-vue'
import OrchDangerConfirm from '@/components/OrchDangerConfirm.vue'
import { taskActionLabels, taskHistoricalName, taskStateLabel, taskTypeLabel } from './taskListPresentation'
import type { useTaskListActions } from './useTaskListActions'

const props = defineProps<{ model: UnwrapNestedRefs<ReturnType<typeof useTaskListActions>> }>()
const target = computed(() => props.model.confirmation?.task)
const kind = computed(() => props.model.confirmation?.kind)
const title = computed(() => kind.value ? taskActionLabels[kind.value] : '')
</script>

<template>
  <OrchDangerConfirm :open="Boolean(kind)" :title="title" :confirm-label="kind === 'CANCEL' ? '确认取消' : kind === 'SAVE_TEMPLATE' ? '保存' : title" :destructive="kind === 'CANCEL'" :busy="Boolean(model.busyTaskId)" @confirm="model.confirm()" @cancel="model.dismiss()">
    <dl v-if="target" class="task-confirm-facts"><div><dt>任务</dt><dd>{{ target.displayName }}（{{ target.id }}）</dd></div><div><dt>类型 / 状态</dt><dd>{{ taskTypeLabel(target.type) }} · {{ taskStateLabel(target) }}</dd></div><div><dt>执行节点</dt><dd>{{ taskHistoricalName(target.nodeName, target.nodeId, target.nodeDeleted) }}</dd></div></dl>
    <template v-if="kind === 'CANCEL'"><p>取消只终止工具进程，不回滚已写入的文件或数据。尚未领取的任务可直接取消；已领取的任务先进入“取消中”，确认工具未启动或已终止后才显示“已取消”。</p><p class="task-confirm-note">受控取消目前只完成合成验证，真实 {{ target?.type === 'OBLOADER_NORMAL_IMPORT' ? 'OBLOADER' : 'OBDUMPER' }} 进程终止取证仍待完成。</p></template>
    <p v-else-if="kind === 'REBUILD_FROM_CONFIG'">创建可编辑的新草稿，不复制凭据、预检查结果、风险确认或日志，提交前重新预检查。</p>
    <p v-else-if="kind === 'RERUN_FROM_SCRATCH'">生成新草稿并直达预检查步骤，不允许修改参数；提交前重新复验数据源、节点与风险。</p>
    <p v-else-if="kind === 'CHECKPOINT_RESUME'">使用原快照和官方 --retry 生成新任务 ID，不允许修改参数；资格由服务端复验。</p>
    <template v-else-if="kind === 'SAVE_TEMPLATE'"><p>保存模板只复制非敏感配置，不复制凭据、节点、预检查、风险确认、日志或结果。</p><AFormItem label="模板名称" required :validate-status="model.templateNameError ? 'error' : undefined" :help="model.templateNameError || undefined"><AInput :value="model.templateName" aria-label="模板名称" :maxlength="256" placeholder="输入模板名称" @update:value="value => model.updateTemplateName(String(value))" /></AFormItem></template>
    <AAlert v-if="model.failure" type="error" message="操作未完成" :description="model.failure" show-icon />
  </OrchDangerConfirm>
</template>

<style scoped>
.task-confirm-facts { display: grid; gap: var(--ob-foundation-space-1); margin: 0 0 var(--ob-foundation-space-3); }
.task-confirm-facts div { display: grid; grid-template-columns: 88px minmax(0, 1fr); }
.task-confirm-facts dt, .task-confirm-note { color: var(--ob-color-secondary); }
.task-confirm-facts dd { margin: 0; overflow-wrap: anywhere; }
.task-confirm-note { font-size: var(--ob-component-table-metadata-size); }
</style>
