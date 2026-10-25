<script setup lang="ts">
import { Alert, Button, Checkbox, Form, FormItem, Input, Select, Skeleton } from 'ant-design-vue'
import { toRefs } from 'vue'
import type { DirectLoadStepModel } from '../directLoadStepModels'
import WizardAdvancedSettings from '@/components/WizardAdvancedSettings.vue'
import DirectLoadSourceStep from './DirectLoadSourceStep.vue'
import { nodePlatformLabel } from '@/workbench/nodes/executionNodePresentation'
const props = defineProps<{ model: DirectLoadStepModel; fieldPrefix: string }>()
const { form } = toRefs(props.model)
const formatOptions = [{ value: 'CSV', label: 'CSV' }]
const error = (field: string) => props.model.fieldError(field)
</script>

<template>
  <DirectLoadSourceStep v-if="model.activeStep === 1" :model="model" :field-prefix="fieldPrefix" />
  <section v-else class="form-section direct-load-section">
    <h2>{{ model.titles[model.activeStep - 1] }}</h2>
    <Form layout="vertical" :disabled="model.rerunLocked">
      <template v-if="model.activeStep === 2">
        <div class="direct-load-fields">
          <FormItem label="目标数据库 / Schema" :html-for="fieldPrefix + '-database'" required :validate-status="error('database') ? 'error' : undefined" :help="error('database')"><Input :id="fieldPrefix + '-database'" v-model:value="form.database" aria-label="目标数据库 / Schema" :maxlength="256" placeholder="填写目标数据库或 Schema" /></FormItem>
          <FormItem label="唯一目标表" :html-for="fieldPrefix + '-table'" required :validate-status="error('table') ? 'error' : undefined" :help="error('table') || '只允许一个明确目标表，不支持星号、多表、DDL 或 MIX。'"><Input :id="fieldPrefix + '-table'" v-model:value="form.table" aria-label="唯一目标表" :maxlength="256" placeholder="填写唯一目标表名称" /></FormItem>
        </div>
        <Alert type="info" show-icon message="结构与全量/增量模式待检查" description="模式由目标表空状态派生。BIT、虚拟生成列、二进制类型、索引、LOB 和唯一索引风险必须取证；页面不假设目标表可写。" />
      </template>
      <template v-else-if="model.activeStep === 3">
        <div class="wizard-section-heading"><h3>节点本地文件</h3><Button type="text" :loading="model.loadingNodes" @click="model.loadNodes">刷新执行节点</Button></div>
        <Alert v-if="model.nodeFailure" type="error" show-icon :message="model.nodeFailure" />
        <Skeleton v-if="model.loadingNodes" active :paragraph="{ rows: 2 }" />
        <FormItem v-else label="执行节点" :html-for="fieldPrefix + '-node'" required :validate-status="error('nodeId') ? 'error' : undefined" :help="error('nodeId')"><Select :id="fieldPrefix + '-node'" v-model:value="form.nodeId" placeholder="选择可接收新任务的执行节点" :options="model.nodes.map(item => ({ value: item.id, label: `${item.displayName} · ${nodePlatformLabel(item.platform)}${!item.acceptsNewTasks ? ' · 当前不可接收任务' : ''}`, disabled: !item.acceptsNewTasks }))" /></FormItem>
        <FormItem label="文件或目录路径" :html-for="fieldPrefix + '-path'" required :validate-status="error('filePath') ? 'error' : undefined" :help="error('filePath') || '原样传给匹配平台的 Agent；可填写同一目标表的多个分片所在目录。'"><Input :id="fieldPrefix + '-path'" v-model:value="form.filePath" aria-label="文件或目录路径" :maxlength="2048" placeholder="执行节点本地绝对路径" /></FormItem>
        <div class="direct-load-fields">
          <FormItem label="文件格式" :html-for="fieldPrefix + '-format'" required :validate-status="error('format') ? 'error' : undefined" :help="error('format') || '当前本地部署旁路仅开放 CSV。'"><Select :id="fieldPrefix + '-format'" :value="form.format === 'CSV' ? form.format : undefined" :options="formatOptions" placeholder="选择已确认格式" @change="value => form.format = value as 'CSV'" /></FormItem>
        </div>
        <WizardAdvancedSettings v-if="form.format === 'CSV'" title="CSV 解析" :configured-count="Number(!!form.columnSeparator) + Number(form.externalData)" :force-open="!!error('columnSeparator')">
          <FormItem label="CSV 列分隔符（--column-separator）" :html-for="fieldPrefix + '-separator'" :validate-status="error('columnSeparator') ? 'error' : undefined" :help="error('columnSeparator') || '未设置时沿用官方默认，不额外生成参数。'"><Input :id="fieldPrefix + '-separator'" v-model:value="form.columnSeparator" aria-label="CSV 列分隔符" :maxlength="16" placeholder="沿用官方默认" /></FormItem>
          <FormItem><Checkbox v-model:checked="form.externalData">第三方 CSV 文件（--external-data）</Checkbox><p class="section-hint">精确 MANIFEST 行为仍受控验证；文件集合未知时不能提交。</p></FormItem>
        </WizardAdvancedSettings>
        <Alert type="info" show-icon message="文件可读性、匹配数量和唯一目标表待第 5 步检查" description="零匹配、多表或无法判断均阻断提交。不提供浏览器上传、文件浏览、远程存储或跨节点复制。" />
      </template>
      <template v-else-if="model.activeStep === 4">
        <p><code>--direct</code> 由旁路命令生成器固定派生，不能关闭。</p>
        <WizardAdvancedSettings title="并发与高风险行为" :configured-count="Number(!!form.thread) + Number(!!form.parallel) + Number(form.truncateTable) + Number(form.replaceData)" :force-open="!!error('thread') || !!error('parallel')">
          <div class="direct-load-fields">
            <FormItem label="客户端连接池（--thread）" :html-for="fieldPrefix + '-thread'" :validate-status="error('thread') ? 'error' : undefined" :help="error('thread') || '客户端到服务端的连接池规模。'"><Input :id="fieldPrefix + '-thread'" v-model:value="form.thread" aria-label="客户端连接池" inputmode="numeric" placeholder="沿用官方默认" /></FormItem>
            <FormItem label="服务端并行度（--parallel）" :html-for="fieldPrefix + '-parallel'" :validate-status="error('parallel') ? 'error' : undefined" :help="error('parallel') || 'OBServer 写入与排序线程，官方默认 1。'"><Input :id="fieldPrefix + '-parallel'" v-model:value="form.parallel" aria-label="服务端并行度" inputmode="numeric" placeholder="沿用官方默认 1" /></FormItem>
          </div>
          <Alert v-if="form.thread && form.parallel && form.thread !== form.parallel" type="warning" show-icon message="thread 与 parallel 不一致" description="官方建议结合租户 CPU 配置并保持一致。此处只提示，不自动填充或修改数值。" />
          <h3>高风险数据行为</h3><FormItem><Checkbox v-model:checked="form.truncateTable">导入前清空目标表（--truncate-table）</Checkbox></FormItem><Alert v-if="form.truncateTable" type="warning" show-icon message="清空目标表会删除原数据，必须绑定当前快照再次确认。" />
          <FormItem><Checkbox v-model:checked="form.replaceData">替换数据（--replace-data）</Checkbox><p class="section-hint">此处只记录替换意图；第 5 步按实际表空状态、索引和 LOB 检查，不满足条件或无法确认时阻断提交。替换数据不能解决唯一索引冲突。</p></FormItem>
          <Alert v-if="form.replaceData" type="warning" show-icon message="已请求替换数据，需重新预检查并确认当前配置的写入影响。" />
        </WizardAdvancedSettings>
        <Alert type="info" show-icon message="Direct Load 节点配置只读" description="第 5 步核对 session.config.json 实际值与指纹。向导不修改全局配置，不使用官网示例值替代节点事实，也不开放未确认共享参数。" />
      </template>
    </Form>
  </section>
</template>
