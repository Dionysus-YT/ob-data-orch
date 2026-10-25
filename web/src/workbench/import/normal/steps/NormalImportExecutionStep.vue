<script setup lang="ts">
import { computed, toRefs } from 'vue'
import { Alert, Checkbox, Form, FormItem, Input, Radio, RadioGroup, Select } from 'ant-design-vue'
import WizardAdvancedSettings from '@/components/WizardAdvancedSettings.vue'
import NormalImportParameterHint from '../components/NormalImportParameterHint.vue'
import type { NormalImportExecutionModel } from '../normalImportStepModels'

const props = defineProps<{ model: NormalImportExecutionModel }>()
const { form, fieldPrefix, traits, selectedSource, executionFields, executionAdvancedCount, riskDescriptions, riskAcknowledged, acknowledgeRisk, fieldError, validationAttempt } = toRefs(props.model)
const commonFields = computed(() => executionFields.value.filter(field => field.group === 'errors'))
const advancedGroups = computed(() => [
  { title: '性能与资源', fields: executionFields.value.filter(field => field.group === 'performance') },
  { title: 'Oracle 会话格式', fields: executionFields.value.filter(field => field.group === 'session') },
].filter(group => group.fields.length))
const advancedFields = computed(() => advancedGroups.value.flatMap(group => group.fields))
const advancedHasError = computed(() => advancedFields.value.some(field => fieldError.value(field.key)) || Boolean(fieldError.value('compatMysqlVersion')))
const strictOptions = [{ value: '', label: '沿用官方默认 true' }, { value: 'true', label: '脏数据导致非零退出码' }, { value: 'false', label: '脏数据不改变退出码' }]
</script>

<template>
  <section class="form-section normal-import-section" aria-label="执行参数">
    <div v-if="commonFields.length" class="normal-import-field-group">
      <h3>错误与重复数据</h3>
      <Form layout="vertical">
        <div class="normal-import-fields">
          <FormItem v-for="field in commonFields" :key="field.key" :html-for="fieldPrefix + '-' + field.key" :validate-status="fieldError(field.key) ? 'error' : undefined" :help="fieldError(field.key)">
            <template #label><span>{{ field.label }}</span><NormalImportParameterHint :name="field.option" /></template>
            <Select v-if="field.key === 'strict'" :id="fieldPrefix + '-' + field.key" v-model:value="form.execution[field.key]" :aria-label="field.label" :options="strictOptions" />
            <Input v-else :id="fieldPrefix + '-' + field.key" v-model:value="form.execution[field.key]" :aria-label="field.label" :placeholder="field.placeholder" />
          </FormItem>
        </div>
      </Form>
    </div>
    <p class="section-hint">未显式设置的参数沿用工具默认；实际资源与错误处理仍须预检查核对。</p>
    <WizardAdvancedSettings title="资源、会话与数据行为" :configured-count="executionAdvancedCount" :force-open="advancedHasError" :open-request="validationAttempt">
      <Form layout="vertical" class="normal-import-advanced-form">
        <template v-for="group in advancedGroups" :key="group.title">
          <h3>{{ group.title }}</h3>
          <div class="normal-import-fields">
            <FormItem v-for="field in group.fields" :key="field.key" :html-for="fieldPrefix + '-' + field.key" :validate-status="fieldError(field.key) ? 'error' : undefined" :help="fieldError(field.key)">
              <template #label><span>{{ field.label }}</span><NormalImportParameterHint :name="field.option" /></template>
              <Input :id="fieldPrefix + '-' + field.key" v-model:value="form.execution[field.key]" :aria-label="field.label" :placeholder="field.placeholder" />
            </FormItem>
          </div>
        </template>
        <h3>数据行为</h3>
        <div class="normal-import-inline-options">
          <div v-if="traits.hasDdl && selectedSource?.compatibilityMode === 'MYSQL'" class="normal-import-option-control"><Checkbox v-model:checked="form.compatMysql">MySQL DDL 兼容导入</Checkbox><NormalImportParameterHint name="compat-mode" /></div>
          <div v-if="traits.structuredData" class="normal-import-option-control"><Checkbox v-model:checked="form.replaceData">替换重复数据</Checkbox><NormalImportParameterHint name="replace-data" /></div>
          <div v-if="traits.hasDdl" class="normal-import-option-control"><Checkbox v-model:checked="form.replaceObject">替换已有对象定义</Checkbox><NormalImportParameterHint name="replace-object" /></div>
        </div>
        <FormItem v-if="traits.hasDdl && selectedSource?.compatibilityMode === 'MYSQL' && form.compatMysql" label="源 MySQL 版本" :html-for="fieldPrefix + '-compatMysqlVersion'" :validate-status="fieldError('compatMysqlVersion') ? 'error' : undefined" :help="fieldError('compatMysqlVersion')">
          <Select :id="fieldPrefix + '-compatMysqlVersion'" v-model:value="form.compatMysqlVersion" aria-label="源 MySQL 版本" placeholder="请选择源版本" :options="[{ value: '', label: '请选择源版本' }, { value: 'mysql56', label: 'MySQL 5.6' }, { value: 'mysql57', label: 'MySQL 5.7' }, { value: 'mysql80', label: 'MySQL 8.0' }]" />
        </FormItem>
        <FormItem v-if="traits.structuredData" label="导入前处理目标表">
          <RadioGroup v-model:value="form.cleanup" class="normal-import-inline-options" role="radiogroup" aria-label="导入前处理目标表"><Radio value="NONE">保留现有数据</Radio><span class="normal-import-option-control"><Radio value="TRUNCATE">清空表</Radio><NormalImportParameterHint name="truncate-table" /></span><span class="normal-import-option-control"><Radio value="DELETE">删除表数据</Radio><NormalImportParameterHint name="delete-from-table" /></span></RadioGroup>
        </FormItem>
        <p v-if="!traits.hasDdl && !traits.structuredData" class="section-hint">选择导入内容后显示适用的数据行为。</p>
      </Form>
    </WizardAdvancedSettings>
    <template v-if="riskDescriptions.length">
      <Alert type="warning" show-icon message="当前配置包含需确认的影响">
        <template #description><ul><li v-for="risk in riskDescriptions" :key="risk">{{ risk }}</li></ul><p>影响数据库：{{ form.database || '尚未配置' }}。最终受影响对象与文件尚未检查。</p></template>
      </Alert>
      <Form layout="vertical"><FormItem :validate-status="fieldError('risk') ? 'error' : undefined" :help="fieldError('risk')"><Checkbox :checked="riskAcknowledged" @update:checked="acknowledgeRisk">已阅读当前配置的影响说明（不替代执行确认）</Checkbox></FormItem></Form>
    </template>
  </section>
</template>
