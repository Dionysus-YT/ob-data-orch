<script setup lang="ts">
import type { DirectLoadStepModel } from '../directLoadStepModels'
import { directLoadScenarioLabels } from '../directLoadRules'
import { directLoadPrecheckOverview } from '../directLoadPrecheckPresentation'
defineProps<{ model: DirectLoadStepModel }>()
</script>
<template>
  <section class="wizard-summary">
    <section class="wizard-summary-section">
      <h3>任务信息</h3>
      <dl class="wizard-summary-facts wizard-summary-facts-paired">
        <div><dt>当前步骤</dt><dd>{{ model.titles[model.activeStep - 1] }}</dd></div>
        <div><dt>预检查</dt><dd>{{ directLoadPrecheckOverview(model.activePrecheck, model.checkExpired, model.checking).message }}</dd></div>
        <div><dt>数据源</dt><dd>{{ model.selectedSource?.displayName || '尚未选择' }}</dd></div>
        <div><dt>执行节点</dt><dd>{{ model.selectedNode?.displayName || '尚未选择' }}</dd></div>
      </dl>
    </section>
    <section class="wizard-summary-section">
      <h3>连接与目标</h3>
      <dl class="wizard-summary-facts">
        <div><dt>连接场景</dt><dd>{{ model.form.scenario === 'PRIVATE_ODP' || model.form.scenario === 'OBSERVER_DIRECT' ? directLoadScenarioLabels[model.form.scenario] : model.form.scenario ? '当前部署不支持，请重新选择' : '尚未配置' }}</dd></div>
        <div><dt>SQL 端口 / RPC 端口</dt><dd>{{ model.selectedSource?.port || '—' }} / {{ model.form.rpcPort || '—' }}（RPC 为填写值，实际连通以检查为准）</dd></div>
        <div><dt>唯一目标表</dt><dd><code class="wizard-summary-path">{{ model.form.database || '—' }} / {{ model.form.table || '—' }}</code></dd></div>
      </dl>
    </section>
    <section class="wizard-summary-section">
      <h3>输入与执行</h3>
      <dl class="wizard-summary-facts">
        <div><dt>本地路径</dt><dd><code class="wizard-summary-path">{{ model.form.filePath || '尚未配置' }}</code></dd></div>
        <div><dt>文件格式</dt><dd>{{ model.form.format || '尚未配置' }}</dd></div>
        <div><dt>客户端 / 服务端并发</dt><dd>{{ model.form.thread || '官方默认' }} / {{ model.form.parallel || '官方默认 1' }}</dd></div>
        <div><dt>高风险行为</dt><dd>{{ model.form.truncateTable ? '导入前清空目标表' : '未配置清空' }}；{{ model.form.replaceData ? '替换数据（待检查）' : '未配置替换' }}</dd></div>
        <div><dt>模式、结构、版本、RPC</dt><dd>仅以当前完整预检查证据为准</dd></div>
        <div><dt>执行门禁</dt><dd>{{ model.currentDraft?.executionEnabled ? '仍需当前预检与最终风险确认' : '尚未开放' }}</dd></div>
      </dl>
    </section>
    <p class="wizard-summary-note">入口确认与最终风险确认独立。配置变化、引用修订变化或证据过期后均须重新检查。</p>
  </section>
</template>
