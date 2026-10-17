<script setup lang="ts">
import { computed, toRefs } from 'vue'
import { Alert, Descriptions, DescriptionsItem } from 'ant-design-vue'
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
  <section class="normal-import-summary">
    <h2>当前配置总览</h2>
    <Alert type="info" show-icon :message="draftDescription" />
    <Descriptions :column="1" size="small">
      <DescriptionsItem label="任务名称">{{ form.taskName || '提交时可选填写，留空自动生成' }}</DescriptionsItem>
      <DescriptionsItem label="数据源">{{ selectedSource?.displayName ?? '尚未选择或当前不可用' }}</DescriptionsItem>
      <DescriptionsItem v-if="selectedSource" label="环境与兼容模式">{{ importEnvironmentLabel(selectedSource.environment) }} · {{ selectedSource.compatibilityMode }}</DescriptionsItem>
      <DescriptionsItem label="目标数据库 / Schema">{{ form.database || '尚未填写' }}</DescriptionsItem>
      <DescriptionsItem label="执行节点">{{ selectedNode?.displayName ?? '尚未选择或当前不可用' }}</DescriptionsItem>
      <DescriptionsItem label="文件路径">{{ form.filePath || '尚未填写' }}</DescriptionsItem>
      <DescriptionsItem label="文件后缀">{{ form.fileSuffix || '沿用官方默认' }}</DescriptionsItem>
      <DescriptionsItem label="文件编码">{{ form.fileEncoding || '沿用官方默认 UTF-8' }}</DescriptionsItem>
      <DescriptionsItem v-if="form.fileRegex" label="文件正则">{{ form.fileRegex }}</DescriptionsItem>
      <DescriptionsItem label="日志目录">{{ form.logPath || '沿用 file-path 对应目录行为' }}</DescriptionsItem>
      <DescriptionsItem label="导入内容">{{ contentLabel }}</DescriptionsItem>
      <DescriptionsItem label="数据格式">{{ formatLabel }}</DescriptionsItem>
      <DescriptionsItem label="对象范围">{{ objectSummary }}</DescriptionsItem>
      <DescriptionsItem v-if="configuration.excludeTables" label="排除表">{{ configuration.excludeTables }}</DescriptionsItem>
      <DescriptionsItem v-for="(value, key) in configuration.parser" :key="key" :label="parserLabels[key] || key">{{ value === true ? '启用' : value }}</DescriptionsItem>
      <template v-if="configuration.mapping">
        <DescriptionsItem label="列映射">{{ mappingLabels[configuration.mapping.kind] }}</DescriptionsItem>
        <DescriptionsItem v-if="configuration.mapping.ctlPath" label="控制文件目录">{{ configuration.mapping.ctlPath }}</DescriptionsItem>
        <DescriptionsItem v-if="configuration.mapping.includeColumns" label="包含列">{{ configuration.mapping.includeColumns }}</DescriptionsItem>
        <DescriptionsItem v-if="configuration.mapping.excludeColumns" label="排除列">{{ configuration.mapping.excludeColumns }}</DescriptionsItem>
        <DescriptionsItem v-if="configuration.mapping.excludeDataTypes" label="排除数据类型">{{ configuration.mapping.excludeDataTypes }}</DescriptionsItem>
      </template>
      <DescriptionsItem v-for="field in importExecutionFields.filter(item => configuration.execution[item.key])" :key="field.key" :label="field.label">{{ configuration.execution[field.key] }}</DescriptionsItem>
      <DescriptionsItem v-if="configuration.compatMysql" label="MySQL DDL 兼容导入">启用</DescriptionsItem>
      <DescriptionsItem label="其余可选参数">沿用官方默认；不主动写出默认参数</DescriptionsItem>
      <DescriptionsItem label="任务预检查">{{ precheckDescription }}</DescriptionsItem>
    </Descriptions>
    <Alert v-if="riskDescriptions.length" type="warning" show-icon message="当前配置的影响"><template #description><ul><li v-for="risk in riskDescriptions" :key="risk">{{ risk }}</li></ul></template></Alert>
  </section>
</template>
