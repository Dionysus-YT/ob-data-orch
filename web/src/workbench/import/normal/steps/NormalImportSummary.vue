<script setup lang="ts">
import { computed, toRefs } from 'vue'
import { Alert } from 'ant-design-vue'
import { importEnvironmentLabel, importExecutionFields } from '../normalImportOptions'
import type { NormalImportSummaryModel } from '../normalImportStepModels'

const props = defineProps<{ model: NormalImportSummaryModel }>()
const { form, configuration, selectedSource, selectedNode, contentLabel, formatLabel, objectSummary, riskDescriptions } = toRefs(props.model)
const draftDescription = computed(() => props.model.loadingDraft ? '正在读取服务端草稿。'
  : !props.model.currentDraft ? '配置尚未保存为服务端草稿，刷新后需重新配置。'
    : props.model.draftDirty ? '当前更改尚未保存；刷新将恢复上次保存的配置。'
      : props.model.draftNeedsRebind ? '配置已保存，数据源或节点事实变化后需重新保存并检查。'
        : '服务端草稿已保存，刷新后可恢复当前活动配置。')
const precheckDescription = computed(() => {
  const check = props.model.activePrecheck
  if (!check) return '尚未执行完整预检查'
  if (props.model.checkExpired || ['INVALIDATED', 'EXPIRED'].includes(check.status)) return '已失效，需重新检查'
  if (['PENDING', 'LEASED'].includes(check.status)) return '等待检查'
  if (check.results.some(result => result.status === 'UNKNOWN')) return '存在无法确认项，提交已阻断'
  return check.status === 'SUCCEEDED' ? '全部通过' : '存在未通过项，提交已阻断'
})
const parserLabels: Record<string, string> = {
  columnSeparator: 'CSV 列分隔符', columnQuote: 'CSV 包围符', columnSplitter: 'CUT 分隔字符串', lineSeparator: '行分隔符',
  escapeCharacter: '转义字符', nullString: 'NULL 标识', emptyString: '空字符串标识', skipHeader: '跳过首行', withTrim: '去除左右空格',
  skipFooter: '跳过末行', ignoreEscape: '忽略 CUT 转义', trailDelimiter: '行尾分隔符', compress: '压缩输入',
  compressionAlgo: '压缩算法', compressionLevel: '压缩等级', dateFormat: '原日期格式', defaultDate: '无效日期替代值',
}
const mappingLabels: Record<string, string> = { POSITION: '按文件列顺序', CONTROL: '控制文件', AUTO: '官方自动列映射', EXPLICIT: '显式列名单' }
</script>

<template>
  <section class="wizard-summary">
    <section class="wizard-summary-section">
      <h3>任务信息</h3>
      <dl class="wizard-summary-facts wizard-summary-facts-paired">
        <div><dt>当前步骤</dt><dd>{{ model.titles[model.activeStep - 1] }}</dd></div>
        <div><dt>预检查</dt><dd>{{ precheckDescription }}</dd></div>
        <div><dt>数据源</dt><dd>{{ selectedSource?.displayName ?? '尚未选择或当前不可用' }}</dd></div>
        <div><dt>执行节点</dt><dd>{{ selectedNode?.displayName ?? '尚未选择或当前不可用' }}</dd></div>
      </dl>
      <p class="wizard-summary-note">{{ draftDescription }}</p>
    </section>
    <section class="wizard-summary-section">
      <h3>导入配置</h3>
      <dl class="wizard-summary-facts">
        <div><dt>任务名称</dt><dd>{{ form.taskName || '提交时可选填写，留空自动生成' }}</dd></div>
        <div v-if="selectedSource"><dt>环境与兼容模式</dt><dd>{{ importEnvironmentLabel(selectedSource.environment) }} · {{ selectedSource.compatibilityMode }}</dd></div>
        <div><dt>目标数据库 / Schema</dt><dd><code class="wizard-summary-path">{{ form.database || '尚未填写' }}</code></dd></div>
        <div><dt>导入内容</dt><dd>{{ contentLabel }}</dd></div>
        <div><dt>数据格式</dt><dd>{{ formatLabel }}</dd></div>
        <div><dt>对象范围</dt><dd>{{ objectSummary }}</dd></div>
        <div v-if="configuration.excludeTables"><dt>排除表</dt><dd><code class="wizard-summary-path">{{ configuration.excludeTables }}</code></dd></div>
        <div v-for="(value, key) in configuration.parser" :key="key"><dt>{{ parserLabels[key] || key }}</dt><dd>{{ value === true ? '启用' : value }}</dd></div>
        <template v-if="configuration.mapping">
          <div><dt>列映射</dt><dd>{{ mappingLabels[configuration.mapping.kind] }}</dd></div>
          <div v-if="configuration.mapping.ctlPath"><dt>控制文件目录</dt><dd><code class="wizard-summary-path">{{ configuration.mapping.ctlPath }}</code></dd></div>
          <div v-if="configuration.mapping.includeColumns"><dt>包含列</dt><dd>{{ configuration.mapping.includeColumns }}</dd></div>
          <div v-if="configuration.mapping.excludeColumns"><dt>排除列</dt><dd>{{ configuration.mapping.excludeColumns }}</dd></div>
          <div v-if="configuration.mapping.excludeDataTypes"><dt>排除数据类型</dt><dd>{{ configuration.mapping.excludeDataTypes }}</dd></div>
        </template>
        <div v-if="configuration.compatMysql"><dt>MySQL DDL 兼容导入</dt><dd>{{ configuration.compatMysqlVersion || '尚未选择源版本' }}</dd></div>
      </dl>
    </section>
    <section class="wizard-summary-section">
      <h3>执行与文件</h3>
      <dl class="wizard-summary-facts">
        <div><dt>文件路径</dt><dd><code class="wizard-summary-path">{{ form.filePath || '尚未填写' }}</code></dd></div>
        <div><dt>文件后缀</dt><dd>{{ form.fileSuffix || '沿用官方默认' }}</dd></div>
        <div><dt>文件编码</dt><dd>{{ form.fileEncoding || '沿用官方默认 UTF-8' }}</dd></div>
        <div v-if="form.fileRegex"><dt>文件正则</dt><dd><code class="wizard-summary-path">{{ form.fileRegex }}</code></dd></div>
        <div><dt>日志目录</dt><dd><code v-if="form.logPath" class="wizard-summary-path">{{ form.logPath }}</code><span v-else>沿用 file-path 对应目录行为</span></dd></div>
        <div v-for="field in importExecutionFields.filter(item => configuration.execution[item.key])" :key="field.key"><dt>{{ field.label }}</dt><dd>{{ configuration.execution[field.key] }}</dd></div>
        <div><dt>其余可选参数</dt><dd>沿用官方默认；不主动写出默认参数</dd></div>
      </dl>
    </section>
    <Alert v-if="riskDescriptions.length" type="warning" show-icon message="当前配置的影响"><template #description><ul><li v-for="risk in riskDescriptions" :key="risk">{{ risk }}</li></ul></template></Alert>
  </section>
</template>
