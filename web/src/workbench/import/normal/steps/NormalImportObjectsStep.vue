<script setup lang="ts">
import { toRefs } from 'vue'
import { Alert, Form, FormItem, Input, Radio, RadioGroup } from 'ant-design-vue'
import WizardAdvancedSettings from '@/components/WizardAdvancedSettings.vue'
import NormalImportParameterHint from '../components/NormalImportParameterHint.vue'
import { importObjectLabels } from '../normalImportOptions'
import type { NormalImportObjectsModel } from '../normalImportStepModels'

const props = defineProps<{ model: NormalImportObjectsModel }>()
const { form, fieldPrefix, traits, currentFormat, availableObjectTypes, selectScope, fieldError } = toRefs(props.model)
</script>

<template>
  <section class="form-section normal-import-section">
    <div class="normal-import-field-group">
      <h3>目标对象</h3>
      <Form layout="vertical">
        <FormItem label="导入范围" required :validate-status="fieldError('scope') ? 'error' : undefined" :help="fieldError('scope')">
          <RadioGroup :value="form.scope" class="normal-import-inline-options" role="radiogroup" aria-label="导入范围" @update:value="selectScope"><span class="normal-import-option-control"><Radio value="ALL">全部对象</Radio><NormalImportParameterHint name="all" /></span><Radio value="SPECIFIC">指定对象</Radio></RadioGroup>
        </FormItem>
        <div v-if="form.scope === 'SPECIFIC'" class="normal-import-fields">
          <FormItem v-for="type in availableObjectTypes" :key="type" :html-for="fieldPrefix + '-object-' + type" :validate-status="fieldError('objects') ? 'error' : undefined" :help="type === availableObjectTypes[0] ? fieldError('objects') : undefined">
            <template #label><span>{{ importObjectLabels[type] }}表达式</span><NormalImportParameterHint :name="type" /></template>
            <Input :id="fieldPrefix + '-object-' + type" v-model:value="form.objects[type]" :aria-label="importObjectLabels[type] + '表达式'" placeholder="填写官方对象名称或匹配表达式，留空不选择" />
          </FormItem>
        </div>
        <p v-if="form.scope === 'SPECIFIC'" class="section-hint">至少填写一种适用对象。仅数据时只选择表；Oracle 专属定义仅在对应兼容模式下提供。</p>
        <FormItem v-if="form.scope === 'ALL' || form.scope === 'SPECIFIC' && form.objects.table" :html-for="fieldPrefix + '-exclude-tables'"><template #label><span>排除表</span><NormalImportParameterHint name="exclude-table" /></template><Input :id="fieldPrefix + '-exclude-tables'" v-model:value="form.excludeTables" aria-label="排除表" placeholder="留空不排除" /></FormItem>
      </Form>
    </div>
    <div v-if="traits.structuredData && currentFormat" class="normal-import-field-group">
      <h3>列映射</h3>
      <Form layout="vertical">
        <FormItem label="列映射方式" required :validate-status="fieldError('mapping') ? 'error' : undefined" :help="fieldError('mapping')">
          <RadioGroup v-model:value="currentFormat.mapping" class="normal-import-inline-options" role="radiogroup" aria-label="列映射方式">
            <Radio value="POSITION">按文件列顺序</Radio>
            <span class="normal-import-option-control"><Radio value="CONTROL">控制文件</Radio><NormalImportParameterHint name="ctl-path" /></span>
            <span v-if="traits.autoMapping" class="normal-import-option-control"><Radio value="AUTO">官方自动列映射</Radio><NormalImportParameterHint name="auto-column-mapping" /></span>
            <Radio value="EXPLICIT">显式列名单</Radio>
          </RadioGroup>
        </FormItem>
        <FormItem v-if="currentFormat.mapping === 'CONTROL'" required :html-for="fieldPrefix + '-ctl'" :validate-status="fieldError('ctlPath') ? 'error' : undefined" :help="fieldError('ctlPath')"><template #label><span>控制文件目录</span><NormalImportParameterHint name="ctl-path" /></template><Input :id="fieldPrefix + '-ctl'" v-model:value="currentFormat.ctlPath" aria-label="控制文件目录" placeholder="执行节点上的绝对路径" /></FormItem>
        <div v-if="currentFormat.mapping === 'EXPLICIT'" class="normal-import-fields">
          <FormItem :html-for="fieldPrefix + '-include-columns'" :validate-status="fieldError('includeColumns') ? 'error' : undefined" :help="fieldError('includeColumns')"><template #label><span>包含列</span><NormalImportParameterHint name="include-column-names" /></template><Input :id="fieldPrefix + '-include-columns'" v-model:value="currentFormat.includeColumns" aria-label="包含列" :disabled="!!currentFormat.excludeColumns.trim() && !currentFormat.includeColumns.trim()" placeholder="按文件列顺序填写，大小写精确匹配" /></FormItem>
          <FormItem :html-for="fieldPrefix + '-exclude-columns'"><template #label><span>排除列</span><NormalImportParameterHint name="exclude-column-names" /></template><Input :id="fieldPrefix + '-exclude-columns'" v-model:value="currentFormat.excludeColumns" aria-label="排除列" :disabled="!!currentFormat.includeColumns.trim() && !currentFormat.excludeColumns.trim()" placeholder="填写不导入的目标列" /></FormItem>
        </div>
        <p v-if="currentFormat.mapping === 'EXPLICIT'" class="section-hint">包含列和排除列择一配置，清空当前值后可切换。</p>
      </Form>
      <WizardAdvancedSettings title="列条件" :configured-count="currentFormat.excludeDataTypes ? 1 : 0"><Form layout="vertical" class="normal-import-advanced-form"><FormItem :html-for="fieldPrefix + '-exclude-types'"><template #label><span>排除数据类型</span><NormalImportParameterHint name="exclude-data-types" /></template><Input :id="fieldPrefix + '-exclude-types'" v-model:value="currentFormat.excludeDataTypes" aria-label="排除数据类型" placeholder="沿用官方规则，留空不排除" /></FormItem></Form></WizardAdvancedSettings>
    </div>
    <Alert v-else type="info" show-icon :message="traits.hasDdl ? '当前内容不使用独立数据格式的列映射。' : '请先在导入文件与格式步骤选择 CSV、CUT 或 SQL。'" />
  </section>
</template>
