<script setup lang="ts">
import { toRefs } from 'vue'
import type { ExportSummaryModel } from '../exportStepModels'
import { Descriptions as ADescriptions, DescriptionsItem as ADescriptionsItem } from 'ant-design-vue'
import { precheckStatusLabel } from '../exportPresentation'

const props = defineProps<{ model: ExportSummaryModel }>()
const { titles, activeStep, displayedSource, displayedDraftConfig, draftScopeSummary, database, scopeKind, contentKind, formatKind, displayedNode, activePrecheck } = toRefs(props.model)
</script>

<template>
  <h2>配置总览</h2>
  <ADescriptions class="export-facts" size="small" :column="1">
    <ADescriptionsItem label="当前步骤">{{ titles[activeStep - 1] }}</ADescriptionsItem>
    <ADescriptionsItem label="数据源">{{ displayedSource?.displayName ?? '尚未选择' }}</ADescriptionsItem>
    <ADescriptionsItem label="导出范围">{{ displayedDraftConfig ? draftScopeSummary : (database ? `${database} · ${scopeKind === 'QUERY_RESULT' ? '查询结果集' : scopeKind === 'ALL' ? '全部对象' : '指定对象'}` : '尚未配置') }}</ADescriptionsItem>
    <ADescriptionsItem label="导出内容">{{ scopeKind === 'QUERY_RESULT' ? '按结果集导出' : contentKind === 'DDL_ONLY' ? '仅 DDL' : contentKind === 'DDL_AND_DATA' ? 'DDL + 数据' : '仅数据' }}</ADescriptionsItem>
    <ADescriptionsItem label="数据格式">{{ contentKind === 'DDL_ONLY' ? '无数据格式' : formatKind }}</ADescriptionsItem>
    <ADescriptionsItem label="执行节点">{{ displayedNode?.displayName ?? '尚未选择' }}</ADescriptionsItem>
    <ADescriptionsItem label="预检查">{{ precheckStatusLabel(activePrecheck?.status) }}</ADescriptionsItem>
  </ADescriptions>
  <p class="aside-note">草稿创建和命令预览不启动 Agent、工具或数据库连接。预检查通过后，点击“提交并启动导出”才会由 Agent 领取冻结任务并启动 OBDUMPER。</p>
</template>
