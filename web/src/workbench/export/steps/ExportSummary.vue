<script setup lang="ts">
import { computed, ref, toRefs, useId, watch } from 'vue'
import type { ExportSummaryModel } from '../exportStepModels'
import { Alert as AAlert, Button as AButton, TypographyParagraph as ATypographyParagraph } from 'ant-design-vue'
import { precheckStatusLabel } from '../exportPresentation'

const props = defineProps<{ model: ExportSummaryModel }>()
const { titles, activeStep, displayedSource, displayedDraftConfig, draftScopeSummary, draftContentLabel, draftDeliverySummary, draftFileSummary, database, scopeKind, contentKind, formatKind, displayedNode, activePrecheck } = toRefs(props.model)
const scopeSummary = computed(() => displayedDraftConfig.value ? draftScopeSummary.value : database.value ? `${database.value} · ${scopeKind.value === 'QUERY_RESULT' ? '查询结果集' : scopeKind.value === 'ALL' ? '全部对象' : '指定对象'}` : '尚未配置')
const scopeContentID = useId()
// 展开只影响长文本展示；事实继续消费向导的唯一模型，内容变化后重新收起。
const scopeExpanded = ref(false)
const scopeOverflowing = ref(false)
watch(scopeSummary, () => {
  scopeExpanded.value = false
  scopeOverflowing.value = false
})
</script>

<template>
  <div class="wizard-summary">
    <p v-if="activeStep === 5 && displayedDraftConfig" class="wizard-summary-note">以下信息来自本次已保存的草稿。</p>

    <section class="wizard-summary-section">
      <h3>任务信息</h3>
      <dl class="wizard-summary-facts wizard-summary-facts-paired">
        <div><dt>当前步骤</dt><dd>{{ titles[activeStep - 1] }}</dd></div>
        <div><dt>预检查</dt><dd>{{ precheckStatusLabel(activePrecheck?.status) }}</dd></div>
        <div><dt>数据源</dt><dd>{{ displayedSource?.displayName ?? '尚未选择' }}</dd></div>
        <div><dt>执行节点</dt><dd>{{ displayedNode?.displayName ?? '尚未选择' }}</dd></div>
      </dl>
    </section>

    <section class="wizard-summary-section">
      <h3>导出配置</h3>
      <dl class="wizard-summary-facts">
        <div>
          <dt>导出范围</dt>
          <dd>
            <ATypographyParagraph :id="scopeContentID" class="wizard-summary-scope" :ellipsis="scopeExpanded ? false : { rows: 3, onEllipsis: (overflowing: boolean) => scopeOverflowing = overflowing }" :content="scopeSummary" />
            <AButton v-if="scopeOverflowing" class="wizard-summary-expand" type="link" :aria-expanded="scopeExpanded" :aria-controls="scopeContentID" @click="scopeExpanded = !scopeExpanded">{{ scopeExpanded ? '收起' : '展开全部' }}</AButton>
          </dd>
        </div>
        <div><dt>导出内容</dt><dd>{{ activeStep === 5 && displayedDraftConfig ? draftContentLabel : !contentKind ? '未选择' : scopeKind === 'QUERY_RESULT' ? '按结果集导出' : contentKind === 'DDL_ONLY' ? '仅 DDL' : contentKind === 'DDL_AND_DATA' ? 'DDL + 数据' : '仅数据' }}</dd></div>
        <div><dt>数据格式</dt><dd>{{ activeStep === 5 && displayedDraftConfig ? draftDeliverySummary : !contentKind ? '未选择导出内容' : contentKind === 'DDL_ONLY' ? '无数据格式' : formatKind }}</dd></div>
        <div v-if="displayedDraftConfig?.contentSelection.contentKind && displayedDraftConfig.contentSelection.contentKind !== 'DDL_ONLY'">
          <dt>文件组织与限制</dt>
          <dd><ul class="wizard-summary-file-rules"><li v-for="rule in draftFileSummary.split('；')" :key="rule">{{ rule }}</li></ul></dd>
        </div>
      </dl>
    </section>

    <section v-if="displayedDraftConfig" class="wizard-summary-section">
      <h3>执行与输出</h3>
      <dl class="wizard-summary-facts">
        <div><dt>输出位置</dt><dd><code class="wizard-summary-path">{{ displayedDraftConfig.outputConfig.filePath }}</code></dd></div>
        <div><dt>日志目录</dt><dd><code v-if="displayedDraftConfig.outputConfig.logPath" class="wizard-summary-path">{{ displayedDraftConfig.outputConfig.logPath }}</code><span v-else>由 Agent 受控采集</span></dd></div>
        <div v-if="displayedDraftConfig.outputConfig.outputKind !== 'LOCAL'"><dt>临时分块目录</dt><dd><code v-if="displayedDraftConfig.outputConfig.tmpPath" class="wizard-summary-path">{{ displayedDraftConfig.outputConfig.tmpPath }}</code><span v-else>尚未配置</span></dd></div>
      </dl>
      <AAlert v-if="displayedDraftConfig.outputConfig.skipCheckDir" class="wizard-summary-risk" type="warning" message="已跳过目录空性检查，存在覆盖风险" show-icon />
    </section>

    <p class="wizard-summary-note">保存草稿和查看命令不会启动导出。预检查通过后，点击“提交并启动导出”开始执行。</p>
  </div>
</template>
