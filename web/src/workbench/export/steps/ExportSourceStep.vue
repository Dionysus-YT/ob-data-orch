<script setup lang="ts">
import { toRefs } from 'vue'
import type { ExportSourceStepModel } from '../exportStepModels'
import { Form as AForm, FormItem as AFormItem, Input as AInput, Select as ASelect, SelectOption as ASelectOption, Alert as AAlert, Button as AButton, Skeleton as ASkeleton, RadioGroup as ARadioGroup, Radio as ARadio } from 'ant-design-vue'
import EmptyState from '@/components/EmptyState.vue'
import { environmentLabel } from '../exportPresentation'
import { formatVerifiedTime } from '@/workbench/sources/sourcePresentation'

const props = defineProps<{ model: ExportSourceStepModel }>()
const { sourceKeyword, fieldPrefix, sourceEnvironment, sourceLoadFailure, loadSources, loadingSources, eligibleSources, sources, router, visibleSources, selectedDataSourceID, derivedDraftBindingLocked, selectedSource, selectSource } = toRefs(props.model)
</script>

<template>
  <section class="form-section">
    <div class="export-field-group">
      <h3>筛选数据源</h3>
      <AForm layout="vertical" class="export-source-filter wizard-source-filter">
        <AFormItem>
          <template #label>按名称筛选数据源</template><AInput v-model:value="sourceKeyword" aria-label="按名称筛选数据源" placeholder="输入数据源名称" allow-clear />
        </AFormItem>
        <AFormItem :html-for="fieldPrefix + '-source-environment'">
          <template #label>环境</template><ASelect :id="fieldPrefix + '-source-environment'" v-model:value="sourceEnvironment"><ASelectOption value="">全部环境</ASelectOption><ASelectOption value="DEVELOPMENT">开发</ASelectOption><ASelectOption value="TEST">测试</ASelectOption><ASelectOption value="STAGING">预生产</ASelectOption><ASelectOption value="PRODUCTION">生产</ASelectOption></ASelect>
        </AFormItem>
      </AForm>
    </div>
    <div class="export-field-group">
      <div class="wizard-section-heading"><h3>可用数据源</h3><AButton :loading="loadingSources" @click="loadSources">刷新数据源</AButton></div>
      <AAlert v-if="sourceLoadFailure" type="error" show-icon :message="sourceLoadFailure"><template #action><AButton type="link" @click="loadSources">重试</AButton></template></AAlert>
      <ASkeleton v-else-if="loadingSources" active :paragraph="{ rows: 3 }" aria-label="正在加载已授权数据源" />
      <EmptyState v-else-if="eligibleSources.length === 0" title="没有可选数据源" :description="sources.length === 0 ? '当前授权范围内没有数据源。请先登记数据源并完成一次成功的基础连接测试。' : '当前已授权数据源均未同时满足已启用和成功测试条件。请在数据源管理中完成受控测试并启用数据源。'" action="前往数据源管理" @action="router.push('/data-sources')" />
      <EmptyState v-else-if="visibleSources.length === 0" title="当前筛选无可选数据源" description="清除名称或环境筛选后重试。" />
      <ARadioGroup v-else :value="selectedDataSourceID" class="export-source-list wizard-source-list" aria-label="选择数据源" :disabled="derivedDraftBindingLocked" @update:value="selectSource">
        <ARadio v-for="source in visibleSources" :key="source.id" :value="source.id" class="export-source-choice wizard-source-choice">
          <span class="export-source-facts wizard-source-facts"><strong>{{ source.displayName }}</strong><span>{{ environmentLabel(source.environment) }} · {{ source.compatibilityMode }} · {{ source.host }}:{{ source.port }}</span><span>集群：{{ source.clusterName || '未登记' }} / 租户：{{ source.tenantName }} · 基础连接测试成功：{{ formatVerifiedTime(source.lastTestedAt) }}</span></span>
        </ARadio>
      </ARadioGroup>
      <AAlert v-if="selectedSource && derivedDraftBindingLocked" type="info" show-icon message="派生草稿固定使用来源任务的数据源；如需更换数据源，请退出派生流程后新建草稿。" />
      <AAlert v-else-if="selectedSource" type="info" show-icon :message="`已选择 ${selectedSource.displayName}。更换数据源会清除当前对象选择；未完成配置也可保存草稿。`" />
      <p v-else-if="eligibleSources.length > 0" class="section-hint">请选择一个数据源后继续。</p>
    </div>
  </section>
</template>
