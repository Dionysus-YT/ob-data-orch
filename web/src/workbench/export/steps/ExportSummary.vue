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
  <div class="configuration-summary export-summary">
    <p v-if="activeStep === 5 && displayedDraftConfig" class="export-summary-note">以下信息来自本次已保存的草稿。</p>

    <section class="export-summary-section">
      <h3>任务信息</h3>
      <dl class="export-summary-facts export-summary-facts-paired">
        <div><dt>当前步骤</dt><dd>{{ titles[activeStep - 1] }}</dd></div>
        <div><dt>预检查</dt><dd>{{ precheckStatusLabel(activePrecheck?.status) }}</dd></div>
        <div><dt>数据源</dt><dd>{{ displayedSource?.displayName ?? '尚未选择' }}</dd></div>
        <div><dt>执行节点</dt><dd>{{ displayedNode?.displayName ?? '尚未选择' }}</dd></div>
      </dl>
    </section>

    <section class="export-summary-section">
      <h3>导出配置</h3>
      <dl class="export-summary-facts">
        <div>
          <dt>导出范围</dt>
          <dd>
            <ATypographyParagraph :id="scopeContentID" class="export-summary-scope" :ellipsis="scopeExpanded ? false : { rows: 3, onEllipsis: (overflowing: boolean) => scopeOverflowing = overflowing }" :content="scopeSummary" />
            <AButton v-if="scopeOverflowing" class="export-summary-expand" type="link" :aria-expanded="scopeExpanded" :aria-controls="scopeContentID" @click="scopeExpanded = !scopeExpanded">{{ scopeExpanded ? '收起' : '展开全部' }}</AButton>
          </dd>
        </div>
        <div><dt>导出内容</dt><dd>{{ activeStep === 5 && displayedDraftConfig ? draftContentLabel : scopeKind === 'QUERY_RESULT' ? '按结果集导出' : contentKind === 'DDL_ONLY' ? '仅 DDL' : contentKind === 'DDL_AND_DATA' ? 'DDL + 数据' : '仅数据' }}</dd></div>
        <div><dt>数据格式</dt><dd>{{ activeStep === 5 && displayedDraftConfig ? draftDeliverySummary : contentKind === 'DDL_ONLY' ? '无数据格式' : formatKind }}</dd></div>
        <div v-if="displayedDraftConfig && displayedDraftConfig.contentSelection.contentKind !== 'DDL_ONLY'">
          <dt>文件组织与限制</dt>
          <dd><ul class="export-summary-file-rules"><li v-for="rule in draftFileSummary.split('；')" :key="rule">{{ rule }}</li></ul></dd>
        </div>
      </dl>
    </section>

    <section v-if="displayedDraftConfig" class="export-summary-section">
      <h3>执行与输出</h3>
      <dl class="export-summary-facts">
        <div><dt>输出位置</dt><dd><code class="export-summary-path">{{ displayedDraftConfig.outputConfig.filePath }}</code></dd></div>
        <div><dt>日志目录</dt><dd><code v-if="displayedDraftConfig.outputConfig.logPath" class="export-summary-path">{{ displayedDraftConfig.outputConfig.logPath }}</code><span v-else>继承工具默认</span></dd></div>
        <div v-if="displayedDraftConfig.outputConfig.outputKind !== 'LOCAL'"><dt>临时分块目录</dt><dd><code v-if="displayedDraftConfig.outputConfig.tmpPath" class="export-summary-path">{{ displayedDraftConfig.outputConfig.tmpPath }}</code><span v-else>尚未配置</span></dd></div>
      </dl>
      <AAlert v-if="displayedDraftConfig.outputConfig.skipCheckDir" class="export-summary-risk" type="warning" message="已跳过目录空性检查，存在覆盖风险" show-icon />
    </section>

    <p class="export-summary-note">保存草稿和查看命令不会启动导出。预检查通过后，点击“提交并启动导出”开始执行。</p>
  </div>
</template>

<style scoped>
.export-summary {
  display: grid;
  gap: var(--ob-foundation-space-4);
  min-inline-size: 0;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  overflow-wrap: anywhere;
}
.export-summary-section {
  min-inline-size: 0;
  padding-block-end: var(--ob-foundation-space-4);
  border-block-end: 1px solid var(--ob-color-border);
}
.export-summary-section h3 {
  margin: 0 0 var(--ob-foundation-space-3);
  color: var(--ob-color-text);
  font-size: var(--ob-product-typography-section-size);
  font-weight: var(--ob-product-typography-weight);
}
.export-summary-note {
  margin: 0;
  color: var(--ob-color-secondary);
  font-size: var(--ob-component-field-helper-size);
  line-height: var(--ob-component-field-helper-line-height);
}
.export-summary-scope {
  margin: 0;
}
.export-summary-facts {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: var(--ob-foundation-space-3) var(--ob-foundation-space-6);
  margin: 0;
}
.export-summary-facts-paired {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
.export-summary-facts > div {
  min-inline-size: 0;
}
.export-summary-facts dt {
  margin-block-end: var(--ob-foundation-space-1);
  color: var(--ob-color-secondary);
  font-size: var(--ob-component-field-label-size);
  line-height: var(--ob-component-field-label-line-height);
}
.export-summary-facts dd {
  margin: 0;
  font-size: var(--ob-component-control-font-size);
  line-height: var(--ob-component-control-line-height);
}
.export-summary-expand {
  padding-inline: 0;
}
.export-summary-risk {
  margin-block-start: var(--ob-foundation-space-4);
}
.export-summary-path {
  font-family: var(--ob-foundation-font-mono);
  font-size: var(--ob-component-control-font-size);
  white-space: pre-wrap;
}
.export-summary-file-rules {
  display: grid;
  gap: var(--ob-foundation-space-1);
  margin: 0;
  padding-inline-start: var(--ob-foundation-space-4);
}
@media (max-width: 575px) {
  .export-summary-facts-paired {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
