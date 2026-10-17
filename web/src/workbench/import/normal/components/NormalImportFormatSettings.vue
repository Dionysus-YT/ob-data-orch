<script setup lang="ts">
import { computed, toRefs } from 'vue'
import { Alert, Checkbox, Form, FormItem, Input, Select } from 'ant-design-vue'
import { importFormatChoices } from '../normalImportOptions'
import NormalImportParameterHint from './NormalImportParameterHint.vue'
import type { NormalImportFormatSettingsModel } from '../normalImportStepModels'

const props = defineProps<{ model: NormalImportFormatSettingsModel }>()
const { form, fieldPrefix, traits, currentFormat, fieldError } = toRefs(props.model)
const formatDescription = computed(() => importFormatChoices.find(choice => choice.value === form.value.format)?.description)
const encodings = [{ value: '', label: '沿用官方默认 UTF-8' }, ...['UTF-8', 'GBK', 'GB18030', 'UTF-16', 'ISO-8859-1'].map(value => ({ value, label: value }))]
const lineSeparators = [{ value: '', label: '沿用工具平台默认' }, { value: '\\n', label: 'LF' }, { value: '\\r\\n', label: 'CRLF' }, { value: '\\r', label: 'CR' }]
</script>

<template>
  <section class="normal-import-field-group" aria-label="文件设置">
    <h3>{{ traits.structuredData ? '数据文件设置' : '文件设置' }}</h3>
    <Form layout="vertical" class="normal-import-panel">
      <div class="normal-import-format-grid">
        <FormItem v-if="traits.structuredData" required :html-for="fieldPrefix + '-format'" :validate-status="fieldError('format') ? 'error' : undefined" :help="fieldError('format')" :extra="formatDescription || '请选择 CSV、CUT 或 SQL，不自动替你选择格式。'">
          <template #label><span>数据格式</span><NormalImportParameterHint v-if="traits.format" :name="traits.format.toLowerCase()" /></template>
          <Select :id="fieldPrefix + '-format'" v-model:value="form.format" aria-label="数据格式" placeholder="请选择文件格式" :options="importFormatChoices.map(choice => ({ value: choice.value, label: choice.label }))" />
        </FormItem>
        <FormItem :html-for="fieldPrefix + '-encoding'"><template #label><span>文件编码</span><NormalImportParameterHint name="file-encoding" /></template><Select :id="fieldPrefix + '-encoding'" v-model:value="form.fileEncoding" aria-label="文件编码" :options="encodings" /></FormItem>
        <FormItem :html-for="fieldPrefix + '-suffix'" :validate-status="fieldError('fileSuffix') ? 'error' : undefined" :help="fieldError('fileSuffix')" extra="留空按当前内容与格式沿用工具默认。"><template #label><span>文件后缀</span><NormalImportParameterHint name="file-suffix" /></template><Input :id="fieldPrefix + '-suffix'" v-model:value="form.fileSuffix" aria-label="文件后缀" placeholder="沿用官方默认" /></FormItem>
      </div>
    </Form>
  </section>
  <section v-if="traits.structuredData && currentFormat" class="normal-import-field-group" :aria-label="form.format + ' 常用选项'">
    <h3>{{ form.format }} 常用选项</h3>
    <Form layout="vertical" class="normal-import-panel">
      <div class="normal-import-format-grid">
        <FormItem v-if="form.format === 'CSV'" :html-for="fieldPrefix + '-column-separator'"><template #label><span>CSV 列分隔符</span><NormalImportParameterHint name="column-separator" /></template><Input :id="fieldPrefix + '-column-separator'" v-model:value="currentFormat.columnSeparator" aria-label="CSV 列分隔符" placeholder="沿用官方默认逗号，支持多字符" /></FormItem>
        <FormItem v-if="form.format === 'CSV'" :html-for="fieldPrefix + '-column-quote'" :validate-status="fieldError('columnQuote') ? 'error' : undefined" :help="fieldError('columnQuote')"><template #label><span>CSV 字符串包围符</span><NormalImportParameterHint name="column-quote" /></template><Input :id="fieldPrefix + '-column-quote'" v-model:value="currentFormat.columnQuote" aria-label="CSV 字符串包围符" placeholder="沿用官方默认英文单引号" /></FormItem>
        <FormItem v-if="form.format === 'CUT'" :html-for="fieldPrefix + '-column-splitter'"><template #label><span>CUT 列分隔字符串</span><NormalImportParameterHint name="column-splitter" /></template><Input :id="fieldPrefix + '-column-splitter'" v-model:value="currentFormat.columnSplitter" aria-label="CUT 列分隔字符串" placeholder="沿用官方默认" /></FormItem>
        <FormItem :html-for="fieldPrefix + '-line-separator'"><template #label><span>行分隔符</span><NormalImportParameterHint name="line-separator" /></template><Select :id="fieldPrefix + '-line-separator'" v-model:value="currentFormat.lineSeparator" aria-label="行分隔符" :options="lineSeparators" /></FormItem>
      </div>
    </Form>
  </section>
  <section v-if="traits.text && currentFormat" class="normal-import-field-group" aria-label="其他选项">
    <h3>其他选项</h3>
    <Form layout="vertical" class="normal-import-panel">
      <div class="normal-import-toggle-grid">
        <FormItem><div class="normal-import-option-control"><Checkbox v-model:checked="currentFormat.skipHeader">跳过首行</Checkbox><NormalImportParameterHint name="skip-header" /></div></FormItem>
        <FormItem><div class="normal-import-option-control"><Checkbox v-model:checked="currentFormat.withTrim">去除左右空格</Checkbox><NormalImportParameterHint name="with-trim" /></div></FormItem>
        <FormItem v-if="form.format === 'CUT'"><div class="normal-import-option-control"><Checkbox v-model:checked="currentFormat.skipFooter">跳过末行</Checkbox><NormalImportParameterHint name="skip-footer" /></div></FormItem>
        <FormItem v-if="form.format === 'CUT'"><div class="normal-import-option-control"><Checkbox v-model:checked="currentFormat.ignoreEscape">忽略 CUT 转义</Checkbox><NormalImportParameterHint name="ignore-escape" /></div></FormItem>
        <FormItem v-if="form.format === 'CUT'"><div class="normal-import-option-control"><Checkbox v-model:checked="currentFormat.trailDelimiter">行尾包含分隔符</Checkbox><NormalImportParameterHint name="trail-delimiter" /></div></FormItem>
      </div>
    </Form>
  </section>
  <Alert v-if="form.content === 'DDL_ONLY'" type="info" show-icon message="仅对象定义不使用数据格式、列映射或压缩输入参数。" />
</template>
