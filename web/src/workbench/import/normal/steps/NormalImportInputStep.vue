<script setup lang="ts">
import { computed, toRefs } from 'vue'
import { Form, FormItem, Input, RadioButton, RadioGroup } from 'ant-design-vue'
import WizardAdvancedSettings from '@/components/WizardAdvancedSettings.vue'
import NormalImportFileSettings from '../components/NormalImportFileSettings.vue'
import NormalImportFormatSettings from '../components/NormalImportFormatSettings.vue'
import NormalImportFormatAdvanced from '../components/NormalImportFormatAdvanced.vue'
import NormalImportParameterHint from '../components/NormalImportParameterHint.vue'
import { importContentChoices } from '../normalImportOptions'
import type { NormalImportInputModel } from '../normalImportStepModels'

const props = defineProps<{ model: NormalImportInputModel }>()
const { form, fieldPrefix, selectContent, inputAdvancedCount, fieldError, validationAttempt } = toRefs(props.model)
const advancedHasError = computed(() => ['fileRegex', 'escapeCharacter', 'emptyString', 'compressionAlgo', 'compressionLevel', 'dateFormat'].some(field => fieldError.value(field)))
</script>

<template>
  <section class="form-section normal-import-section" aria-label="导入文件与格式">
    <div class="normal-import-field-group">
      <h3>导入内容</h3>
      <Form layout="vertical">
        <FormItem required :validate-status="fieldError('content') ? 'error' : undefined" :help="fieldError('content')">
          <RadioGroup :value="form.content" button-style="solid" class="normal-import-content-options" role="radiogroup" aria-label="导入内容" @update:value="selectContent">
            <RadioButton v-for="choice in importContentChoices" :key="choice.value" :value="choice.value">{{ choice.label }}</RadioButton>
          </RadioGroup>
        </FormItem>
      </Form>
    </div>
    <NormalImportFileSettings :model="model" />
    <NormalImportFormatSettings :model="model" />
    <WizardAdvancedSettings title="文件与解析" :configured-count="inputAdvancedCount" :force-open="advancedHasError" :open-request="validationAttempt">
      <Form layout="vertical" class="normal-import-advanced-form">
        <h3>文件匹配</h3>
        <FormItem :html-for="fieldPrefix + '-regex'" :validate-status="fieldError('fileRegex') ? 'error' : undefined" :help="fieldError('fileRegex')" extra="仅对目录匹配有意义；实际匹配由 OBLOADER 检查。"><template #label><span>文件正则</span><NormalImportParameterHint name="file-regular-expression" /></template><Input :id="fieldPrefix + '-regex'" v-model:value="form.fileRegex" aria-label="文件正则" placeholder="留空按路径与后缀匹配" /></FormItem>
        <NormalImportFormatAdvanced :model="model" />
      </Form>
    </WizardAdvancedSettings>
  </section>
</template>
