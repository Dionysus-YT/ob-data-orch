<script setup lang="ts">
import { toRefs } from 'vue'
import type { ExportReviewStepModel } from '../exportStepModels'
import { Alert as AAlert, Spin as ASpin, Button as AButton, Descriptions as ADescriptions, DescriptionsItem as ADescriptionsItem, Tag as ATag, List as AList, ListItem as AListItem, Badge as ABadge } from 'ant-design-vue'
import { precheckStatusLabel } from '../exportPresentation'
import { precheckCheckLabel, precheckResultDetail, precheckResultLabel, precheckResultBlocksSubmission } from '../exportPrecheckPresentation'

const props = defineProps<{ model: ExportReviewStepModel }>()
const { draftNotice, activePrecheck, loadingDraft, draftLoadFailure, loadCreatedDraft, displayedSource, currentDraft, displayedDraftConfig, draftScopeSummary, draftContentLabel, draftDeliverySummary, draftFileSummary, displayedNode, storageOutput, precheckFailure, precheckRunning, blockingPrecheckRows, precheckRows, commandPreview, copyCommand, previewingCommand, commandPreviewFailure, loadCommandPreview, copyNotice, submissionFailure } = toRefs(props.model)
</script>

<template>
  <section class="form-section">
    <h2>参数预检查与完整命令</h2>
    <AAlert v-if="draftNotice" :type="activePrecheck?.status === 'FAILED' ? 'error' : 'info'" show-icon :message="draftNotice" />
    <ASpin v-if="loadingDraft" tip="正在读取已创建草稿的服务端配置快照…"><span class="export-loading-space" /></ASpin>
    <AAlert v-if="draftLoadFailure" type="error" show-icon :message="draftLoadFailure"><template #action><AButton type="link" @click="loadCreatedDraft()">重新读取草稿</AButton></template></AAlert>
    <section class="configuration-summary">
      <h3>任务配置摘要</h3>
      <ADescriptions class="export-facts" size="small" :column="1">
        <ADescriptionsItem><template #label>数据源</template>{{ displayedSource?.displayName ?? (currentDraft ? '草稿数据源当前不可用' : '尚未读取草稿') }}</ADescriptionsItem>
        <ADescriptionsItem><template #label>对象与内容</template>{{ displayedDraftConfig ? `${draftScopeSummary} · ${draftContentLabel}` : '尚未读取草稿' }}</ADescriptionsItem>
        <ADescriptionsItem><template #label>交付格式</template>{{ displayedDraftConfig ? draftDeliverySummary : '尚未读取草稿' }}</ADescriptionsItem>
        <ADescriptionsItem v-if="displayedDraftConfig?.contentSelection.contentKind !== 'DDL_ONLY'"><template #label>文件组织与限制</template>{{ draftFileSummary }}</ADescriptionsItem>
        <ADescriptionsItem><template #label>导出、日志与节点</template>{{ displayedDraftConfig && displayedNode ? `${displayedNode.displayName} · 导出：${displayedDraftConfig.outputConfig.filePath}${displayedDraftConfig.outputConfig.logPath ? ` · 日志：${displayedDraftConfig.outputConfig.logPath}` : ''}${displayedDraftConfig.outputConfig.skipCheckDir ? ' · 已跳过目录空性检查' : ''}` : '尚未读取草稿' }}</ADescriptionsItem>
      </ADescriptions>
    </section>
    <section class="precheck-list">
      <h3>预检查结果</h3>
      <p class="section-hint">预检查由已选择的 Agent 执行：确认数据源连接、当前草稿所选对象可读取（全部对象或结果集范围按数据库级可达性投影）、OB Loader/Dumper 与专用 Java 8 配置、导出目录及已填写日志目录可写、导出目录空性，以及至少 1 GiB 可用空间。对象存储输出额外检查端点连通性与凭据有效性（未授权探测保持未完成）。勾选跳过选项时，仅目录空性检查会被跳过。它不会启动 OBDUMPER 或创建导出文件。</p>
      <AAlert v-if="displayedDraftConfig?.objectScope.scopeKind === 'QUERY_RESULT'" type="info" show-icon message="结果集预检查只确认连接" description="对象访问项是数据库级连接投影；不会执行查询 SQL，也不能证明被引用对象、语法和权限有效。这些由 OBDUMPER 执行时确认。" />
      <AAlert v-if="storageOutput" type="warning" show-icon message="对象存储的端点连通性与凭据有效性检查必须通过才能提交；未授权探测保持未完成。" />
      <AAlert v-if="precheckFailure" type="error" show-icon :message="precheckFailure" />
      <ASpin v-if="precheckRunning" tip="预检查进行中…"><span class="export-loading-space" /></ASpin>
      <p v-if="activePrecheck" role="status">当前状态：<ATag :color="activePrecheck.status === 'FAILED' ? 'error' : activePrecheck.status === 'SUCCEEDED' ? 'success' : 'processing'">{{ precheckStatusLabel(activePrecheck.status) }}</ATag></p>
      <AAlert v-if="activePrecheck?.status === 'FAILED'" type="error" show-icon message="预检查未通过" :description="`以下 ${blockingPrecheckRows.length} 项检查未通过或未完成。请修正后重新执行预检查。`" />
      <AList v-if="activePrecheck?.status === 'FAILED'" size="small" :data-source="blockingPrecheckRows" aria-label="预检查阻断原因">
        <template #renderItem="{ item: row }"><AListItem><div class="export-precheck-detail"><strong>{{ displayedDraftConfig?.objectScope.scopeKind === 'QUERY_RESULT' && row.check === 'OBJECT_ACCESS' ? '结果集访问（连接级）' : precheckCheckLabel(row.check) }}</strong><span>{{ displayedDraftConfig?.objectScope.scopeKind === 'QUERY_RESULT' && row.check === 'OBJECT_ACCESS' ? '仅核对数据库连接，未执行查询 SQL。' : precheckResultDetail(row.result) }}</span><code>原因码：{{ row.result?.evidenceCode }}</code></div></AListItem></template>
      </AList>
      <AList size="small" :data-source="precheckRows" aria-label="预检查结果">
        <template #renderItem="{ item: row }">
          <AListItem><div class="export-precheck-row"><ABadge :status="!row.result ? 'default' : row.result.status === 'PASSED' ? 'success' : row.result.status === 'FAILED' ? 'error' : 'default'" /><strong>{{ displayedDraftConfig?.objectScope.scopeKind === 'QUERY_RESULT' && row.check === 'OBJECT_ACCESS' ? '结果集访问（连接级）' : precheckCheckLabel(row.check) }}</strong><span><b>{{ precheckResultLabel(row.result, precheckRunning) }}</b><small v-if="precheckResultDetail(row.result)">{{ displayedDraftConfig?.objectScope.scopeKind === 'QUERY_RESULT' && row.check === 'OBJECT_ACCESS' ? '仅核对数据库连接，未执行查询 SQL。' : precheckResultDetail(row.result) }}</small><code v-if="precheckResultBlocksSubmission(row.result)">原因码：{{ row.result?.evidenceCode }}</code></span></div></AListItem>
        </template>
      </AList>
    </section>
    <section class="command-empty">
      <div><h3>完整命令（仅隐藏密码）</h3><AButton :disabled="!commandPreview" @click="copyCommand">复制脱敏命令</AButton></div>
      <ASpin v-if="previewingCommand" tip="控制面正在重算命令预览…"><span class="export-loading-space" /></ASpin>
      <AAlert v-else-if="commandPreviewFailure" type="error" show-icon :message="commandPreviewFailure"><template #action><AButton type="link" @click="loadCommandPreview()">重新生成命令</AButton></template></AAlert>
      <pre v-else-if="commandPreview"><code>{{ commandPreview.command }}</code></pre>
      <pre v-else><code>命令只会由控制面根据已读取的草稿快照生成；密码始终不会显示或由浏览器自行拼接。</code></pre>
      <p v-if="commandPreview">`-p ******` 仅为密码占位；实际运行从官方安全文件读取密码，不把密码放入进程参数。</p>
      <p v-if="commandPreview">草稿版本 rev-{{ currentDraft?.revision }} · 配置指纹 {{ commandPreview.configFingerprint }}</p>
      <AAlert v-if="copyNotice" type="success" show-icon :message="copyNotice" />
    </section>
    <AAlert v-if="submissionFailure" type="error" show-icon :message="submissionFailure" />
  </section>
</template>
