<script setup lang="ts">
import { Alert as AAlert, Button as AButton, Dropdown as ADropdown, Menu as AMenu, MenuItem as AMenuItem, Modal as AModal, Radio as ARadio, RadioGroup as ARadioGroup, Spin as ASpin, Textarea as ATextarea } from 'ant-design-vue'
import { DownOutlined, FileSearchOutlined, MenuUnfoldOutlined, RedoOutlined, UndoOutlined } from '@ant-design/icons-vue'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import EditorWorker from 'monaco-editor/editor/editor.worker?worker'
import type { editor as MonacoEditor } from 'monaco-editor/editor/editor.api'
import '../../../../node_modules/monaco-editor/esm/vs/base/browser/ui/codicons/codicon/codicon.css'

const props = defineProps<{
  modelValue: string
  language: 'mysql' | 'sql'
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const container = ref<HTMLElement | null>(null)
const editorReady = ref(false)
const loadFailed = ref(false)
const hasSelection = ref(false)
const formatting = ref(false)
const formatError = ref('')
const editNotice = ref('')
const inValuesOpen = ref(false)
const inValuesText = ref('')
const inValuesType = ref<'text' | 'number'>('text')
const inValuesError = ref('')
let editor: MonacoEditor.IStandaloneCodeEditor | undefined
let disposed = false
let inValuesSelection: ReturnType<MonacoEditor.IStandaloneCodeEditor['getSelection']> = null

const inValues = computed(() => inValuesText.value.split(/[,\r\n\t]+/).map((value) => value.trim()).filter(Boolean))
// 文本值按 SQL 字面量规则转义；数值严格校验后才允许不加引号插入。
const inValuesSql = computed(() => {
  if (!inValues.value.length) return ''
  if (inValuesType.value === 'number') {
    if (inValues.value.some((value) => !/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/.test(value))) return ''
    return `(${inValues.value.join(', ')})`
  }
  return `(${inValues.value.map((value) => `'${value.replaceAll("'", "''")}'`).join(', ')})`
})
const inValuesPreview = computed(() => inValuesSql.value.length > 240 ? `${inValuesSql.value.slice(0, 240)}…` : inValuesSql.value)

// 编辑器仅在结果集输入区挂载后加载；Worker 只处理本地编辑模型，不连接数据库。
onMounted(async () => {
  try {
    globalThis.MonacoEnvironment = { getWorker: () => new EditorWorker() }
    const monaco = await import('monaco-editor/editor/editor.api')
    await Promise.all([
      import('monaco-editor/languages/definitions/sql/register'),
      import('monaco-editor/languages/definitions/mysql/register'),
      import('monaco-editor/editor/contrib/find/browser/findController'),
      import('monaco-editor/editor/contrib/linesOperations/browser/linesOperations'),
      import('monaco-editor/editor/contrib/comment/browser/comment'),
    ])
    if (disposed || !container.value) return
    editor = monaco.editor.create(container.value, {
      value: props.modelValue,
      language: props.language,
      theme: 'vs',
      ariaLabel: '查询 SQL',
      accessibilitySupport: 'on',
      automaticLayout: true,
      lineNumbers: 'on',
      minimap: { enabled: false },
      overviewRulerLanes: 0,
      scrollBeyondLastLine: false,
      wordWrap: 'on',
      tabSize: 2,
      find: { addExtraSpaceOnTop: true },
    })
    editor.onDidChangeModelContent(() => {
      formatError.value = ''
      editNotice.value = ''
      emit('update:modelValue', editor?.getValue() ?? '')
    })
    editor.onDidChangeCursorSelection(() => { hasSelection.value = Boolean(editor?.getSelection() && !editor.getSelection()!.isEmpty()) })
    editorReady.value = true
  } catch {
    if (!disposed) loadFailed.value = true
  }
})

watch(() => props.modelValue, (value) => {
  if (editor && value !== editor.getValue()) editor.setValue(value)
})

watch(() => props.language, async (language) => {
  const model = editor?.getModel()
  if (!model) return
  const monaco = await import('monaco-editor/editor/editor.api')
  if (!disposed) monaco.editor.setModelLanguage(model, language)
})

function editHistory(command: 'undo' | 'redo') {
  if (!editor) return
  editor.trigger('sql-query-toolbar', command, null)
  editor.focus()
}

function editorAction(command: string, refocus = true) {
  if (!editor) return
  editor.trigger('sql-query-toolbar', command, null)
  if (refocus) editor.focus()
}

// 显式选区优先；无选区时转换全文，便于从工具栏直接操作并保持一次撤销可恢复。
function changeCase(mode: 'upper' | 'lower' | 'title') {
  const selection = editor?.getSelection()
  const model = editor?.getModel()
  if (!editor || !model) return
  const range = selection && !selection.isEmpty() ? selection : model.getFullModelRange()
  const original = model.getValueInRange(range)
  if (!original) {
    editNotice.value = '请先输入查询 SQL。'
    return
  }
  const replacement = mode === 'upper' ? original.toLocaleUpperCase() : mode === 'lower' ? original.toLocaleLowerCase() : original.toLocaleLowerCase().replace(/\b[a-z]/g, (letter) => letter.toLocaleUpperCase())
  if (replacement === original) {
    editNotice.value = '当前文本已是所选大小写。'
    return
  }
  model.pushStackElement()
  editor.executeEdits('sql-query-case', [{ range, text: replacement, forceMoveMarkers: true }])
  model.pushStackElement()
  editNotice.value = selection && !selection.isEmpty() ? '已转换选中文本。' : '已转换全文。'
  editor.focus()
}

function openInValues() {
  if (!editor) return
  inValuesSelection = editor.getSelection()
  const model = editor.getModel()
  inValuesText.value = model && inValuesSelection && !inValuesSelection.isEmpty() ? model.getValueInRange(inValuesSelection) : ''
  inValuesType.value = 'text'
  inValuesError.value = ''
  inValuesOpen.value = true
}

function insertInValues() {
  const model = editor?.getModel()
  if (!editor || !model) return
  if (!inValues.value.length) {
    inValuesError.value = '请至少输入一个值。'
    return
  }
  if (!inValuesSql.value) {
    inValuesError.value = '数值类型只能包含整数、小数或科学计数法。'
    return
  }
  const range = inValuesSelection ?? editor.getSelection()
  if (!range) return
  const nextSql = model.getValue().slice(0, model.getOffsetAt(range.getStartPosition())) + inValuesSql.value + model.getValue().slice(model.getOffsetAt(range.getEndPosition()))
  if (new TextEncoder().encode(nextSql).length > 60 * 1024) {
    inValuesError.value = '转换后超过查询 SQL 的 60 KiB 上限，请减少输入值。'
    return
  }
  model.pushStackElement()
  editor.executeEdits('sql-query-in-values', [{ range, text: inValuesSql.value, forceMoveMarkers: true }])
  model.pushStackElement()
  inValuesOpen.value = false
  editor.focus()
}

// 格式化只改写当前编辑模型，不发送 SQL；超出服务端长度边界时保留原文。
async function formatSql() {
  if (!editor || formatting.value) return
  if (!editor.getValue().trim()) {
    editNotice.value = '请先输入查询 SQL。'
    return
  }
  formatting.value = true
  formatError.value = ''
  editNotice.value = ''
  try {
    const { format } = await import('sql-formatter')
    if (disposed || !editor) return
    const model = editor.getModel()
    if (!model) return
    const original = model.getValue()
    const result = format(original, { language: props.language === 'mysql' ? 'mysql' : 'plsql', keywordCase: 'upper', tabWidth: 2 })
    if (new TextEncoder().encode(result.trim()).length > 60 * 1024) {
      formatError.value = '格式化后超过查询 SQL 的 60 KiB 上限，原文已保留。'
      return
    }
    if (result !== original) {
      model.pushStackElement()
      editor.executeEdits('sql-query-format', [{ range: model.getFullModelRange(), text: result, forceMoveMarkers: true }])
      model.pushStackElement()
    }
    editNotice.value = result === original ? '当前 SQL 已是规范格式。' : '已格式化 SQL。'
    editor.focus()
  } catch {
    formatError.value = 'SQL 格式化失败，请检查当前语句。原文已保留。'
  } finally {
    formatting.value = false
  }
}

onBeforeUnmount(() => {
  disposed = true
  editor?.dispose()
})
</script>

<template>
  <div class="sql-query-input">
    <ATextarea v-if="loadFailed" :value="modelValue" aria-label="查询 SQL" :rows="9" spellcheck="false" @update:value="emit('update:modelValue', $event)" />
    <template v-else>
      <div class="sql-query-editor-shell">
        <div class="sql-query-toolbar" role="toolbar" aria-label="SQL 编辑操作">
          <AButton type="text" size="small" aria-label="查找与替换 SQL" title="查找与替换" :disabled="!editorReady" @click="editorAction('editor.action.startFindReplaceAction', false)"><template #icon><FileSearchOutlined aria-hidden="true" /></template></AButton>
          <AButton type="text" size="small" aria-label="撤销 SQL 编辑" title="撤销" :disabled="!editorReady" @click="editHistory('undo')"><template #icon><UndoOutlined aria-hidden="true" /></template></AButton>
          <AButton type="text" size="small" aria-label="重做 SQL 编辑" title="重做" :disabled="!editorReady" @click="editHistory('redo')"><template #icon><RedoOutlined aria-hidden="true" /></template></AButton>
          <span class="sql-query-toolbar-divider" aria-hidden="true" />
          <AButton type="text" size="small" aria-label="格式化 SQL" title="格式化 SQL" :loading="formatting" :disabled="!editorReady" @click="formatSql"><template #icon><svg class="sql-query-toolbar-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.35" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m3 13 8-8M9.6 6.4l-1-1M11 1.5v2M13.5 4h-2M11.8 8.2l1.5 1.5M4.2 3.2 5.5 4.5" /></svg></template></AButton>
          <AButton type="text" size="small" aria-label="IN 值转换" title="将粘贴的值转换为 SQL IN 列表" :disabled="!editorReady" @click="openInValues"><span class="sql-query-in-icon" aria-hidden="true">IN</span></AButton>
          <ADropdown :trigger="['click']" :disabled="!editorReady">
            <AButton type="text" size="small" aria-label="转换 SQL 大小写" :title="hasSelection ? '转换选中文本大小写' : '未选中文本时转换全文'" :disabled="!editorReady">Aa <DownOutlined aria-hidden="true" /></AButton>
            <template #overlay><AMenu @click="({ key }) => changeCase(String(key) as 'upper' | 'lower' | 'title')"><AMenuItem key="upper">全部大写</AMenuItem><AMenuItem key="lower">全部小写</AMenuItem><AMenuItem key="title">首字母大写</AMenuItem></AMenu></template>
          </ADropdown>
          <ADropdown :trigger="['click']" :disabled="!editorReady">
            <AButton type="text" size="small" aria-label="调整 SQL 缩进" title="调整缩进" :disabled="!editorReady"><template #icon><MenuUnfoldOutlined aria-hidden="true" /></template><DownOutlined aria-hidden="true" /></AButton>
            <template #overlay><AMenu @click="({ key }) => editorAction(String(key) === 'add' ? 'editor.action.indentLines' : 'editor.action.outdentLines')"><AMenuItem key="add">添加缩进</AMenuItem><AMenuItem key="remove">删除缩进</AMenuItem></AMenu></template>
          </ADropdown>
          <ADropdown :trigger="['click']" :disabled="!editorReady">
            <AButton type="text" size="small" aria-label="调整 SQL 注释" title="调整注释；提交前需移除" :disabled="!editorReady"><span class="sql-query-toolbar-icon-pair"><svg class="sql-query-toolbar-icon" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3.5 2.5h9A1.5 1.5 0 0 1 14 4v6a1.5 1.5 0 0 1-1.5 1.5H7L4 14v-2.5h-.5A1.5 1.5 0 0 1 2 10V4a1.5 1.5 0 0 1 1.5-1.5Z" /><path d="M8 5v4M6 7h4" /></svg><DownOutlined aria-hidden="true" /></span></AButton>
            <template #overlay><AMenu @click="({ key }) => editorAction(String(key) === 'add' ? 'editor.action.addCommentLine' : 'editor.action.removeCommentLine')"><AMenuItem key="add">添加注释</AMenuItem><AMenuItem key="remove">删除注释</AMenuItem></AMenu></template>
          </ADropdown>
        </div>
        <div class="sql-query-editor-viewport">
          <div ref="container" class="sql-query-editor" role="group" aria-label="查询 SQL 编辑区" />
          <div v-if="!editorReady" class="sql-query-loading" role="status"><ASpin size="small" />正在加载 SQL 编辑器…</div>
        </div>
      </div>
      <span class="sql-query-editor-note">仅编辑导出查询，不在页面执行 SQL；添加的注释须在继续前移除。</span>
      <AAlert v-if="formatError" class="sql-query-format-error" type="warning" show-icon :message="formatError" />
      <AAlert v-else-if="editNotice" class="sql-query-format-error" type="info" show-icon :message="editNotice" />
      <AModal v-model:open="inValuesOpen" title="IN 值转换" ok-text="插入到 SQL" cancel-text="取消" @ok="insertInValues" @cancel="inValuesOpen = false">
        <p class="sql-query-modal-hint">每行、逗号或制表符分隔一个值；转换结果会替换当前选区，未选中时插入光标处。</p>
        <ATextarea v-model:value="inValuesText" aria-label="待转换的 IN 值" :rows="6" :maxlength="60 * 1024" placeholder="例如：北京&#10;上海" />
        <ARadioGroup v-model:value="inValuesType" class="sql-query-in-type" aria-label="IN 值类型"><ARadio value="text">文本（自动加引号）</ARadio><ARadio value="number">数值</ARadio></ARadioGroup>
        <p v-if="inValuesSql" class="sql-query-in-preview" role="status">已识别 {{ inValues.length }} 个值 · 预览：<code>{{ inValuesPreview }}</code></p>
        <AAlert v-if="inValuesError" type="warning" show-icon :message="inValuesError" />
      </AModal>
    </template>
    <span v-if="loadFailed" class="sql-query-editor-note" role="status">编辑器加载失败，已切换为普通多行输入。</span>
  </div>
</template>

<style scoped>
.sql-query-input { inline-size: 100%; min-inline-size: 0; }
.sql-query-editor-shell { border: 1px solid var(--ob-color-border-strong); border-radius: var(--ob-component-control-radius); overflow: hidden; }
.sql-query-editor-shell:focus-within { border-color: var(--ob-color-primary); box-shadow: 0 0 0 2px var(--ob-color-primary-soft); }
.sql-query-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--ob-foundation-space-1); min-block-size: 36px; padding: var(--ob-foundation-space-1) var(--ob-foundation-space-2); border-block-end: 1px solid var(--ob-color-border); background: var(--ob-color-surface); }
.sql-query-toolbar-divider { inline-size: 1px; block-size: 18px; margin-inline: var(--ob-foundation-space-1); background: var(--ob-color-border); }
.sql-query-toolbar-icon { display: block; inline-size: 16px; block-size: 16px; }
.sql-query-toolbar-icon-pair { display: inline-flex; align-items: center; gap: 4px; white-space: nowrap; }
.sql-query-in-icon { display: inline-flex; inline-size: 16px; block-size: 16px; align-items: center; justify-content: center; border: 1px solid currentColor; border-radius: 2px; font-size: 8px; font-weight: 700; line-height: 1; }
.sql-query-editor-viewport { position: relative; }
.sql-query-editor { inline-size: 100%; block-size: 240px; overflow: hidden; }
.sql-query-loading { position: absolute; inset: 1px; display: flex; align-items: center; justify-content: center; gap: var(--ob-foundation-space-2); color: var(--ob-color-secondary); background: var(--ob-color-surface); font-size: var(--ob-component-field-helper-size); }
.sql-query-editor-note { display: block; margin-block-start: var(--ob-foundation-space-1); color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); }
.sql-query-format-error { margin-block-start: var(--ob-foundation-space-2); }
.sql-query-modal-hint { margin-block-end: var(--ob-foundation-space-2); color: var(--ob-color-secondary); }
.sql-query-in-type { display: flex; gap: var(--ob-foundation-space-3); margin-block-start: var(--ob-foundation-space-3); }
.sql-query-in-preview { overflow-wrap: anywhere; margin-block: var(--ob-foundation-space-3); }
</style>

<style>
/* Monaco 查找区增加顶部留白时同步扩展容器，保留原有的代码可见高度。 */
.sql-query-input .sql-query-editor:has(.find-widget.visible) { block-size: 276px; }
.sql-query-input .sql-query-editor:has(.find-widget.visible.replaceToggled) { block-size: 308px; }
</style>
