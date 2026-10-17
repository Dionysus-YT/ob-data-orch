<script setup lang="ts">
import { toRefs } from 'vue'
import { Alert, Button, Form, FormItem, Input, Radio, RadioGroup, Select, Skeleton } from 'ant-design-vue'
import EmptyState from '@/components/EmptyState.vue'
import NormalImportParameterHint from '../components/NormalImportParameterHint.vue'
import { importEnvironmentLabel } from '../normalImportOptions'
import type { NormalImportSourceModel } from '../normalImportStepModels'

const props = defineProps<{ model: NormalImportSourceModel }>()
const { form, fieldPrefix, sourceKeyword, sourceEnvironment, sourceFailure, loadingSources, sources, visibleSources, eligibleSources, selectedSource, loadSources, selectSource, fieldError } = toRefs(props.model)
const environments = [{ value: '', label: '全部环境' }, ...['DEVELOPMENT', 'TEST', 'STAGING', 'PRODUCTION'].map(value => ({ value, label: importEnvironmentLabel(value) }))]
</script>

<template>
  <section class="form-section normal-import-section">
    <div class="normal-import-field-group">
      <h3>筛选数据源</h3>
      <Form layout="vertical" class="normal-import-source-filter">
        <FormItem label="按名称筛选数据源"><Input v-model:value="sourceKeyword" aria-label="按名称筛选数据源" allow-clear placeholder="输入数据源名称" /></FormItem>
        <FormItem label="环境" :html-for="fieldPrefix + '-environment'"><Select :id="fieldPrefix + '-environment'" v-model:value="sourceEnvironment" :options="environments" /></FormItem>
      </Form>
    </div>
    <div class="normal-import-field-group">
      <div class="normal-import-section-heading"><h3>可用数据源</h3><Button :loading="loadingSources" @click="loadSources">刷新数据源</Button></div>
      <Alert v-if="sourceFailure" type="error" show-icon :message="sourceFailure"><template #action><Button type="link" @click="loadSources">重试</Button></template></Alert>
      <Skeleton v-else-if="loadingSources" active :paragraph="{ rows: 3 }" aria-label="正在读取数据源" />
      <EmptyState v-else-if="!eligibleSources.length" title="没有可选数据源" :description="sources.length ? '当前数据源尚未同时满足 ODP、已启用和基础连接测试成功条件。' : '当前授权范围内没有数据源。请在数据源管理中登记并完成基础连接测试。'" />
      <EmptyState v-else-if="!visibleSources.length" title="当前筛选无可选数据源" description="请清除名称或环境筛选后重试。" />
      <Form v-else layout="vertical">
        <FormItem label="目标数据源" required :validate-status="fieldError('dataSourceId') ? 'error' : undefined" :help="fieldError('dataSourceId')">
          <RadioGroup :value="form.dataSourceId" class="normal-import-source-list" role="radiogroup" aria-label="目标数据源" @update:value="selectSource">
            <Radio v-for="source in visibleSources" :key="source.id" :value="source.id" class="normal-import-source-choice">
              <span class="normal-import-source-facts"><strong>{{ source.displayName }}</strong><span>{{ importEnvironmentLabel(source.environment) }} · {{ source.compatibilityMode }} · {{ source.host }}:{{ source.port }}</span><span>{{ source.tenantName }} / {{ source.clusterName || '未登记集群' }} · 基础连接测试成功</span></span>
            </Radio>
          </RadioGroup>
        </FormItem>
      </Form>
    </div>
    <Form layout="vertical" class="normal-import-fields">
      <FormItem required :html-for="fieldPrefix + '-database'" :validate-status="fieldError('database') ? 'error' : undefined" :help="fieldError('database')" extra="可覆盖数据源默认值；不会发起数据库或对象探测。">
        <template #label><span>目标数据库 / Schema</span><NormalImportParameterHint name="database" /></template>
        <Input :id="fieldPrefix + '-database'" v-model:value="form.database" aria-label="目标数据库 / Schema" :disabled="!selectedSource" placeholder="填写目标数据库或 Schema" />
      </FormItem>
    </Form>
    <Alert v-if="selectedSource?.environment === 'PRODUCTION'" type="warning" show-icon message="已选择生产环境；实际提交前仍须完成任务检查与独立执行确认。" />
  </section>
</template>
