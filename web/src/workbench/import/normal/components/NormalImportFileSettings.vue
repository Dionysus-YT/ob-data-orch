<script setup lang="ts">
import { computed, toRefs } from 'vue'
import { Alert, Button, Form, FormItem, Input, Select, SelectOption } from 'ant-design-vue'
import { nodePlatformLabel } from '@/workbench/nodes/executionNodePresentation'
import NormalImportParameterHint from './NormalImportParameterHint.vue'
import type { NormalImportFileSettingsModel } from '../normalImportStepModels'

const props = defineProps<{ model: NormalImportFileSettingsModel }>()
const { form, fieldPrefix, nodes, selectedNode, nodeFailure, loadingNodes, loadNodes, fieldError } = toRefs(props.model)
const pathPlaceholder = computed(() => selectedNode.value?.platform === 'WINDOWS_AMD64' ? '例如 E:\\imports\\daily' : '例如 /var/ob-data-orch/imports/daily')
</script>

<template>
  <section class="normal-import-field-group">
    <h3>文件来源与执行节点</h3>
    <Alert v-if="nodeFailure" type="error" show-icon :message="nodeFailure"><template #action><Button type="link" @click="loadNodes">重试</Button></template></Alert>
    <Form layout="vertical" class="normal-import-form">
      <div class="normal-import-fields">
        <FormItem label="执行节点" required :html-for="fieldPrefix + '-node'" :validate-status="fieldError('nodeId') ? 'error' : undefined" :help="fieldError('nodeId')" extra="列表中的节点状态不代表普通导入预检查通过。">
          <div class="normal-import-select-action"><Select :id="fieldPrefix + '-node'" v-model:value="form.nodeId" aria-label="执行节点" :loading="loadingNodes" placeholder="请选择执行节点" @dropdown-visible-change="open => open && !loadingNodes && loadNodes()"><SelectOption v-for="node in nodes" :key="node.id" :value="node.id" :disabled="!node.acceptsNewTasks">{{ node.displayName }} · {{ nodePlatformLabel(node.platform) }}{{ !node.acceptsNewTasks ? ' · 当前不可接收任务' : '' }}</SelectOption></Select><Button :loading="loadingNodes" @click="loadNodes">刷新</Button></div>
          <p v-if="!loadingNodes && !nodeFailure && !nodes.length" class="section-hint">当前授权范围内没有执行节点。</p>
        </FormItem>
      </div>
      <FormItem required :html-for="fieldPrefix + '-file-path'" :validate-status="fieldError('filePath') ? 'error' : undefined" :help="fieldError('filePath')" extra="填写所选执行节点上、位于允许目录内的本地文件或目录路径。路径原样传递，由该节点检查文件是否存在、可读及匹配规则。">
        <template #label><span>文件或目录路径</span><NormalImportParameterHint name="file-path" /></template>
        <Input :id="fieldPrefix + '-file-path'" v-model:value="form.filePath" aria-label="文件或目录路径" :placeholder="pathPlaceholder" />
      </FormItem>
      <FormItem :html-for="fieldPrefix + '-log-path'" :validate-status="fieldError('logPath') ? 'error' : undefined" :help="fieldError('logPath')">
        <template #label><span>日志目录（可选）</span><NormalImportParameterHint name="log-path" /></template>
        <Input :id="fieldPrefix + '-log-path'" v-model:value="form.logPath" aria-label="日志目录（可选）" placeholder="留空沿用工具默认目录" />
      </FormItem>
    </Form>
  </section>
</template>
