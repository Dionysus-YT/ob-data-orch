import { computed, ref, watch, type Ref } from 'vue'
import type { DataSourceSummary, ExecutionNodeSummary } from '@/api/browser'
import { initialNormalImportForm, type ImportContent, type NormalImportForm } from './normalImportForm'
import { importContentChoices, importExecutionFields, importFormatChoices, importObjectLabels, normalImportSteps } from './normalImportOptions'
import { importAvailableObjectTypes, importTraits, projectNormalImport, validateNormalImport } from './normalImportRules'

interface ParameterReferences {
  selectedSource: Readonly<Ref<DataSourceSummary | undefined>>
  selectedNode: Readonly<Ref<ExecutionNodeSummary | undefined>>
  eligibleSources: Readonly<Ref<readonly DataSourceSummary[]>>
}

// 参数能力独占活动投影、条件联动、清值确认与本地规则；步骤只编辑唯一表单。
export function useNormalImportParameters(form: NormalImportForm, refs: ParameterReferences) {
  const traits = computed(() => importTraits(form))
  const currentFormat = computed(() => traits.value.format ? form.formats[traits.value.format] : undefined)
  const facts = computed(() => ({ source: refs.selectedSource.value, node: refs.selectedNode.value }))
  const configuration = computed(() => projectNormalImport(form, facts.value))
  const configurationSignature = computed(() => JSON.stringify(configuration.value))
  const acknowledgedSignature = ref('')
  const riskAcknowledged = computed(() => acknowledgedSignature.value === configurationSignature.value)
  const riskDescriptions = computed(() => [
    ...(configuration.value.replaceData ? ['替换重复数据仅适用于有主键或非空唯一键的目标表，适用性尚未检查。'] : []),
    ...(configuration.value.replaceObject ? ['替换已有对象定义可能先删除再创建对象。'] : []),
    ...(configuration.value.cleanup === 'TRUNCATE' ? ['导入前清空目标表，现有数据将丢失。'] : []),
    ...(configuration.value.cleanup === 'DELETE' ? [`导入前删除目标表数据。${form.scope === 'ALL' || form.objects.table.includes('*') ? '当前范围包含全部或通配表，即使没有对应数据文件也可能删除表数据。' : ''}`] : []),
  ])
  const issues = computed(() => {
    const result = validateNormalImport(form, facts.value)
    if (riskDescriptions.value.length && !riskAcknowledged.value) result.push({ step: normalImportSteps.execution, field: 'risk', message: '请阅读并确认当前配置的影响说明；这不替代服务端执行确认。' })
    return result
  })
  const availableObjectTypes = computed(() => importAvailableObjectTypes(form, refs.selectedSource.value?.compatibilityMode))
  const executionFields = computed(() => importExecutionFields.filter(field =>
    (!field.dataOnly || traits.value.hasData) && (!field.oracleOnly || refs.selectedSource.value?.compatibilityMode === 'ORACLE'),
  ))
  const contentLabel = computed(() => importContentChoices.find(item => item.value === form.content)?.label ?? '尚未选择')
  const formatLabel = computed(() => traits.value.format ? importFormatChoices.find(item => item.value === traits.value.format)?.label ?? '尚未选择' : traits.value.hasDdl ? '当前内容不使用数据格式' : '尚未选择')
  const objectSummary = computed(() => form.scope === 'ALL' ? '全部对象' : Object.entries(configuration.value.objects)
    .map(([type, value]) => `${importObjectLabels[type as keyof typeof importObjectLabels]}：${value}`).join('；') || '尚未配置')
  const inputAdvancedCount = computed(() => Number(Boolean(form.fileRegex)) + Object.keys(configuration.value.parser)
    .filter(key => ['escapeCharacter', 'nullString', 'emptyString', 'compress', 'compressionAlgo', 'compressionLevel', 'dateFormat', 'defaultDate'].includes(key)).length)
  const executionAdvancedCount = computed(() => Object.keys(configuration.value.execution)
    .filter(key => !['maxDiscards', 'maxErrors', 'strict'].includes(key)).length
    + Number(configuration.value.compatMysql) + Number(configuration.value.replaceData) + Number(configuration.value.replaceObject) + Number(configuration.value.cleanup !== 'NONE'))

  function acknowledgeRisk(checked: boolean) { acknowledgedSignature.value = checked ? configurationSignature.value : '' }

  // 失效使用唯一活动投影，不单独维护第二份提交字段名单；旧配置恢复也需重新阅读风险。
  watch(configurationSignature, () => { acknowledgedSignature.value = '' }, { flush: 'sync' })
  watch(() => [form.format, currentFormat.value?.compressionAlgo], () => {
    if (currentFormat.value && ['gzip', 'snappy'].includes(currentFormat.value.compressionAlgo)) currentFormat.value.compressionLevel = ''
  }, { flush: 'sync' })

  const pendingSourceId = ref<string | null>(null)
  function applySource(id: string) {
    const source = refs.eligibleSources.value.find(item => item.id === id)
    if (!source) return
    form.dataSourceId = id
    form.database = source.defaultDatabase ?? ''
    for (const type of Object.keys(form.objects) as (keyof typeof form.objects)[]) form.objects[type] = ''
    form.excludeTables = ''
    form.execution.nlsDateFormat = ''
    form.execution.nlsTimestampFormat = ''
    form.execution.nlsTimestampTzFormat = ''
    form.compatMysql = false
    form.replaceData = false
    form.replaceObject = false
    form.cleanup = 'NONE'
  }
  function selectSource(id: string) {
    if (id === form.dataSourceId || pendingSourceId.value !== null) return
    if (form.dataSourceId && (form.database || Object.values(form.objects).some(Boolean) || form.replaceData || form.replaceObject || form.cleanup !== 'NONE')) pendingSourceId.value = id
    else applySource(id)
  }
  function confirmSourceChange() {
    const id = pendingSourceId.value
    pendingSourceId.value = null
    if (id !== null) applySource(id)
  }
  function cancelSourceChange() { pendingSourceId.value = null }

  const pendingScope = ref<NormalImportForm['scope'] | null>(null)
  function selectScope(scope: NormalImportForm['scope']) {
    if (scope === form.scope || pendingScope.value !== null) return
    if (scope === 'ALL' && Object.values(form.objects).some(Boolean)) pendingScope.value = scope
    else form.scope = scope
  }
  function confirmScopeChange() {
    if (pendingScope.value !== null) {
      for (const type of Object.keys(form.objects) as (keyof typeof form.objects)[]) form.objects[type] = ''
      form.scope = pendingScope.value
    }
    pendingScope.value = null
  }
  function cancelScopeChange() { pendingScope.value = null }

  const pendingContent = ref<ImportContent | null>(null)
  function applyContent(content: ImportContent) {
    form.content = content
    const next = importTraits(form)
    if (!next.hasDdl) {
      for (const type of Object.keys(form.objects) as (keyof typeof form.objects)[]) if (type !== 'table') form.objects[type] = ''
      form.replaceObject = false
      form.compatMysql = false
    }
    if (!next.structuredData) { form.replaceData = false; form.cleanup = 'NONE' }
  }
  function selectContent(content: ImportContent) {
    if (content === form.content || pendingContent.value !== null) return
    const next = importTraits({ ...form, content })
    const losesDefinitions = !next.hasDdl && (form.replaceObject || form.compatMysql || Object.entries(form.objects).some(([type, value]) => type !== 'table' && value))
    const losesDataBehavior = !next.structuredData && (form.replaceData || form.cleanup !== 'NONE')
    if (losesDefinitions || losesDataBehavior) pendingContent.value = content
    else applyContent(content)
  }
  function confirmContentChange() { const value = pendingContent.value; pendingContent.value = null; if (value !== null) applyContent(value) }
  function cancelContentChange() { pendingContent.value = null }

  function resetParameters() {
    pendingSourceId.value = null
    pendingScope.value = null
    pendingContent.value = null
    acknowledgedSignature.value = ''
    Object.assign(form, initialNormalImportForm())
  }
  return {
    traits, currentFormat, configuration, configurationSignature, issues, availableObjectTypes, executionFields,
    contentLabel, formatLabel, objectSummary, inputAdvancedCount, executionAdvancedCount, riskDescriptions, riskAcknowledged, acknowledgeRisk,
    pendingSourceId, selectSource, confirmSourceChange, cancelSourceChange,
    pendingScope, selectScope, confirmScopeChange, cancelScopeChange, resetParameters,
    pendingContent, selectContent, confirmContentChange, cancelContentChange,
  }
}

export type NormalImportParameters = ReturnType<typeof useNormalImportParameters>
