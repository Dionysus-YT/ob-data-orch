<script setup lang="ts">
import { toRefs } from 'vue'
import { Checkbox, FormItem, Input, Select } from 'ant-design-vue'
import NormalImportParameterHint from './NormalImportParameterHint.vue'
import type { NormalImportFormatSettingsModel } from '../normalImportStepModels'

const props = defineProps<{ model: NormalImportFormatSettingsModel }>()
const { form, fieldPrefix, traits, currentFormat, selectedSource, fieldError } = toRefs(props.model)
</script>

<template>
  <template v-if="traits.structuredData && currentFormat">
    <h3>文本解析</h3>
    <div class="normal-import-fields">
      <FormItem v-if="traits.text" :html-for="fieldPrefix + '-escape'" :validate-status="fieldError('escapeCharacter') ? 'error' : undefined" :help="fieldError('escapeCharacter')"><template #label><span>转义字符</span><NormalImportParameterHint name="escape-character" /></template><Input :id="fieldPrefix + '-escape'" v-model:value="currentFormat.escapeCharacter" aria-label="转义字符" :disabled="form.format === 'CUT' && currentFormat.ignoreEscape" placeholder="沿用当前格式官方默认" /></FormItem>
      <FormItem :html-for="fieldPrefix + '-null'"><template #label><span>NULL 标识</span><NormalImportParameterHint name="null-string" /></template><Input :id="fieldPrefix + '-null'" v-model:value="currentFormat.nullString" aria-label="NULL 标识" placeholder="沿用官方默认 \N" /></FormItem>
      <FormItem v-if="traits.text" :html-for="fieldPrefix + '-empty'" :validate-status="fieldError('emptyString') ? 'error' : undefined" :help="fieldError('emptyString')"><template #label><span>空字符串标识</span><NormalImportParameterHint name="empty-string" /></template><Input :id="fieldPrefix + '-empty'" v-model:value="currentFormat.emptyString" aria-label="空字符串标识" placeholder="沿用官方默认 \E" /></FormItem>
    </div>
    <h3>压缩输入</h3>
    <FormItem extra="只表示读取 OBDUMPER 压缩输出，不会再次压缩数据。"><div class="normal-import-option-control"><Checkbox v-model:checked="currentFormat.compress">输入文件已压缩</Checkbox><NormalImportParameterHint name="compress" /></div></FormItem>
    <div v-if="currentFormat.compress" class="normal-import-fields">
      <FormItem :html-for="fieldPrefix + '-algorithm'" :validate-status="fieldError('compressionAlgo') ? 'error' : undefined" :help="fieldError('compressionAlgo')"><template #label><span>压缩算法</span><NormalImportParameterHint name="compression-algo" /></template><Select :id="fieldPrefix + '-algorithm'" v-model:value="currentFormat.compressionAlgo" aria-label="压缩算法" :options="[{ value: '', label: '沿用官方默认 zstd' }, ...['zstd', 'zlib', 'gzip', 'snappy'].map(value => ({ value, label: value }))]" /></FormItem>
      <FormItem v-if="['zstd', 'zlib'].includes(currentFormat.compressionAlgo || 'zstd')" :html-for="fieldPrefix + '-level'" :validate-status="fieldError('compressionLevel') ? 'error' : undefined" :help="fieldError('compressionLevel')"><template #label><span>压缩等级</span><NormalImportParameterHint name="compression-level" /></template><Input :id="fieldPrefix + '-level'" v-model:value="currentFormat.compressionLevel" aria-label="压缩等级" :placeholder="currentFormat.compressionAlgo !== 'zlib' ? '沿用默认 3；1～22' : '沿用默认 -1；-1～9'" /></FormItem>
    </div>
    <template v-if="traits.text && selectedSource?.compatibilityMode === 'MYSQL'">
      <h3>MySQL 日期纠错</h3>
      <div class="normal-import-fields">
        <FormItem :html-for="fieldPrefix + '-date-format'" :validate-status="fieldError('dateFormat') ? 'error' : undefined" :help="fieldError('dateFormat')"><template #label><span>原日期格式</span><NormalImportParameterHint name="date-format" /></template><Input :id="fieldPrefix + '-date-format'" v-model:value="currentFormat.dateFormat" aria-label="原日期格式" placeholder="例如 yyyy-MM-dd HH:mm:ss" /></FormItem>
        <FormItem :html-for="fieldPrefix + '-default-date'"><template #label><span>无效日期替代值</span><NormalImportParameterHint name="default-date" /></template><Input :id="fieldPrefix + '-default-date'" v-model:value="currentFormat.defaultDate" aria-label="无效日期替代值" placeholder="仅适用于 MySQL DATE / DATETIME" /></FormItem>
      </div>
      <p class="section-hint">目标列类型与实际日期解析仍须由任务预检查核对。</p>
    </template>
  </template>
</template>
