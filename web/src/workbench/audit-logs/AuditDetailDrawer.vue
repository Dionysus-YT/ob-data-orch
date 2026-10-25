<script setup lang="ts">
import { Alert as AAlert, Button as AButton, Drawer as ADrawer, Spin as ASpin } from 'ant-design-vue'
import type { UnwrapNestedRefs } from 'vue'
import { useAntDrawerDialog } from '@/composables/useAntDrawerDialog'
import type { useAuditLogs } from './useAuditLogs'
import { auditFieldLabels, auditObjectLabels, auditResultLabels } from './presentation'
import { logTime } from '@/workbench/log-query/presentation'
const props = defineProps<{ model: UnwrapNestedRefs<ReturnType<typeof useAuditLogs>> }>()
const dialog = useAntDrawerDialog('audit-detail-title', () => props.model.closeDetail(), () => false, () => props.model.detailOpen)
</script>
<template>
  <ADrawer :open="model.detailOpen" placement="right" width="min(760px, 100vw)" :destroy-on-close="true" @close="model.closeDetail" @after-open-change="dialog.afterOpenChange">
    <template #title><h2 id="audit-detail-title" :ref="dialog.heading" tabindex="-1">审计事件详情</h2></template>
    <ASpin v-if="model.detailLoading" />
    <AAlert v-if="model.detailFailure" type="error" show-icon :message="model.detailFailure" />
    <template v-if="model.detail">
      <p class="log-query-task">{{ logTime(model.detail.occurredAt) }} · 平台审计事件 · 已脱敏</p>
      <dl class="audit-detail-facts">
        <dt>事件 ID</dt><dd>{{ model.detail.id }}</dd><dt>操作人</dt><dd>{{ model.detail.actorType }} · {{ model.detail.actorId }}</dd>
        <dt>操作类型</dt><dd>{{ model.detail.action }}</dd><dt>对象</dt><dd>{{ auditObjectLabels[model.detail.objectType] }} · {{ model.detail.objectId || '未关联具体对象' }}</dd>
        <dt>结果</dt><dd>{{ auditResultLabels[model.detail.result] }}</dd><dt>请求标识</dt><dd>{{ model.detail.requestId }}</dd>
        <template v-for="field in model.detail.fields" :key="field.name"><dt>{{ auditFieldLabels[field.name] }}</dt><dd>{{ field.value === 'true' ? '是' : field.value === 'false' ? '否' : field.value }}</dd></template>
      </dl>
      <AAlert v-if="model.detail.detailsOmitted" type="info" show-icon message="部分历史变更详情暂不展示，查询与下载范围相同。" />
      <p class="log-query-footnote">按平台记录时间展示；未取得独立来源时间或日志级别。事件相邻不代表调用链或因果关系。</p>
      <AButton @click="model.closeDetail">关闭详情</AButton>
    </template>
  </ADrawer>
</template>
