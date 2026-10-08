<script setup lang="ts">
import { BLOCK_SIZE_PATTERN, isValidEscapeCharacter } from '../exportDraftInput'
import { toRefs } from 'vue'
import type { ExportFormatStepModel } from '../exportStepModels'
import { Alert as AAlert, Form as AForm, FormItem as AFormItem, Select as ASelect, SelectOption as ASelectOption, Input as AInput, Checkbox as ACheckbox, RadioGroup as ARadioGroup, Radio as ARadio } from 'ant-design-vue'
import ExportOptionHint from '@/components/ExportOptionHint.vue'
import ExportFormatChoice from '@/components/ExportFormatChoice.vue'
import ExportAdvancedSettings from '@/components/ExportAdvancedSettings.vue'
import { fileEncodingChoices, fieldSeparatorChoices, cutSeparatorChoices, quoteChoices, lineSeparatorChoices, datetimeFormatChoices, dateFormatChoices } from '../exportPresentation'

const props = defineProps<{ model: ExportFormatStepModel }>()
const { blockSizeChoices, formatChangeNotice, contentKind, dataOptionsActive, fieldPrefix, formatKind, fileEncoding, ordinaryFormat, draftValidationMessage, columnSeparator, columnSplitter, columnQuote, lineSeparator, columnQuoteMode, nullString, escapeCharacter, dropObject, retainSchema, compactSchemaSupported, compactSchema, scopeKind, querySql, snapshot, flashbackScn, flashbackTimestamp, tableOnlySelection, excludeVirtualColumns, selectedSource, skipHeader, withTrim, trailDelimiter, removeNewline, noNestedDir, retainEmptyFiles, formatAdvancedCount, timestampFormatsSupported, datetimeValueFormat, dateValueFormat, splitUnit, changeSplitUnit, attemptedStep, blockSize, splitThreshold, compress, compressionAlgo, compressionLevel } = toRefs(props.model)
</script>

<template>
  <section class="form-section">
    <h2>文件格式与交付约定</h2>
    <AAlert v-if="formatChangeNotice" type="info" show-icon :message="formatChangeNotice" closable @close="formatChangeNotice = ''" />
    <AAlert v-if="contentKind === 'DDL_ONLY'" type="info" show-icon message="无数据格式" description="仅 DDL 导出不生成数据文件；仍可在其他选项中配置 DDL 文件行为。" />
    <div v-if="dataOptionsActive" class="export-field-group">
      <h3>数据文件设置</h3>
      <AForm layout="vertical" class="export-field-body export-format-panel">
        <div class="export-format-grid">
          <AFormItem :html-for="fieldPrefix + '-format-kind'" required>
            <template #label><span class="export-field-label"><span>数据格式</span><ExportOptionHint label="数据格式" parameter="--csv / --cut / --sql" description="选择数据文件格式：CSV 使用单字符分隔，CUT 使用分隔字符串，SQL 输出数据 SQL 文件。此选择不改变第二步的结构或数据导出内容。" /></span></template>
            <ASelect :id="fieldPrefix + '-format-kind'" v-model:value="formatKind" aria-label="数据格式">
              <ASelectOption value="CSV">CSV 格式</ASelectOption>
              <ASelectOption value="CUT">CUT 格式</ASelectOption>
              <ASelectOption value="SQL">SQL 格式</ASelectOption>
            </ASelect>
          </AFormItem>
          <AFormItem :html-for="fieldPrefix + '-file-encoding'" extra="选择常用编码，或直接输入执行环境支持的编码。">
            <template #label><span class="export-field-label"><span>文件编码</span><ExportOptionHint label="文件编码" parameter="--file-encoding" description="指定导出文件的字符编码，与数据库连接字符集不同。留空继承工具默认 UTF-8；所选编码须由执行环境支持。" /></span></template>
            <ExportFormatChoice :id="fieldPrefix + '-file-encoding'" v-model="fileEncoding" label="文件编码" :options="fileEncodingChoices" custom-placeholder="输入执行环境支持的编码" />
          </AFormItem>
        </div>
        <AAlert v-if="!ordinaryFormat" type="warning" show-icon :message="draftValidationMessage" />
      </AForm>
    </div>
    <div v-if="dataOptionsActive && ordinaryFormat" class="export-field-group export-format-group" role="region" :aria-label="`${formatKind} 常用选项`">
      <h3>{{ formatKind }} 常用选项 <small>（可选，未配置时继承工具默认）</small></h3>
      <AForm layout="vertical" class="export-field-body export-format-panel">
        <div class="export-format-grid">
          <AFormItem v-if="dataOptionsActive && ordinaryFormat && formatKind === 'CSV'" :html-for="fieldPrefix + '-csv-separator'" extra="OBDUMPER 4.3.5 仅支持单字符；留空使用逗号。" :validate-status="Array.from(columnSeparator).length > 1 ? 'error' : undefined" :help="Array.from(columnSeparator).length > 1 ? '请输入单个分隔字符。' : undefined">
            <template #label><span class="export-field-label"><span>字段分隔符</span><ExportOptionHint label="字段分隔符" parameter="--column-separator" description="指定 CSV 字段之间的单字符分隔符，默认逗号。OBDUMPER 4.3.5 仅支持单字符；空格和制表符按选择值保留。" /></span></template>
            <ExportFormatChoice :id="fieldPrefix + '-csv-separator'" v-model="columnSeparator" label="字段分隔符" :options="fieldSeparatorChoices" custom-placeholder="输入单个分隔字符" />
          </AFormItem>
          <AFormItem v-else-if="formatKind === 'CUT'" :html-for="fieldPrefix + '-cut-separator'" extra="CUT 使用分隔字符串，可选择常用字符或输入最多 256 个字符。" :validate-status="columnSplitter.length > 256 ? 'error' : undefined" :help="columnSplitter.length > 256 ? '分隔字符串不能超过 256 个字符。' : undefined">
            <template #label><span class="export-field-label"><span>字段分隔符</span><ExportOptionHint label="字段分隔符" parameter="--column-splitter" description="指定 CUT 字段之间的分隔字符串，当前最长 256 个字符。不同于 CSV，可配置多字符分隔；留空继承工具默认。" /></span></template>
            <ExportFormatChoice :id="fieldPrefix + '-cut-separator'" v-model="columnSplitter" label="字段分隔符" :options="cutSeparatorChoices" custom-placeholder="输入自定义分隔字符串" />
          </AFormItem>
          <AFormItem v-if="dataOptionsActive && ordinaryFormat && formatKind === 'CSV'" :html-for="fieldPrefix + '-column-quote'" extra="OBDUMPER 4.3.5 仅支持单字符；留空使用单引号。" :validate-status="Array.from(columnQuote).length > 1 ? 'error' : undefined" :help="Array.from(columnQuote).length > 1 ? '请输入单个识别字符。' : undefined">
            <template #label><span class="export-field-label"><span>文本识别符</span><ExportOptionHint label="文本识别符" parameter="--column-quote" description="用于包围 CSV 字段内容的单字符，默认单引号。与包围模式共同控制字段输出，不支持多个字符。" /></span></template>
            <ExportFormatChoice :id="fieldPrefix + '-column-quote'" v-model="columnQuote" label="文本识别符" :options="quoteChoices" custom-placeholder="输入单个识别字符" />
          </AFormItem>
          <AFormItem :html-for="fieldPrefix + '-line-separator'" :validate-status="lineSeparator.length > 256 ? 'error' : undefined" :help="lineSeparator.length > 256 ? '换行符号不能超过 256 个字符。' : undefined">
            <template #label><span class="export-field-label"><span>换行符号</span><ExportOptionHint label="换行符号" parameter="--line-separator" description="设置数据文件中记录之间的行分隔符，例如 LF、CRLF 或 CR。未指定时继承平台默认；不同于删除字段内容中的换行。" /></span></template>
            <ExportFormatChoice :id="fieldPrefix + '-line-separator'" v-model="lineSeparator" label="换行符号" :options="lineSeparatorChoices" custom-placeholder="输入官方支持的换行表达式" />
          </AFormItem>
          <AFormItem v-if="dataOptionsActive && ordinaryFormat && formatKind === 'CSV'" :html-for="fieldPrefix + '-quote-mode'">
            <template #label><span class="export-field-label"><span>包围模式</span><ExportOptionHint label="包围模式" parameter="--column-quote-mode" description="控制 CSV 字段的包围规则：全部、非空、最小必要、非数字或不包围。默认 non_numeric；不包围时须确认特殊字符已正确处理。" /></span></template><ASelect :id="fieldPrefix + '-quote-mode'" v-model:value="columnQuoteMode" aria-label="包围模式"><ASelectOption value="">继承默认（非数字包围）</ASelectOption><ASelectOption value="all">全部包围 · all</ASelectOption><ASelectOption value="all_not_null">非空包围 · all_not_null</ASelectOption><ASelectOption value="minimal">最小包围 · minimal</ASelectOption><ASelectOption value="non_numeric">非数字包围 · non_numeric</ASelectOption><ASelectOption value="none">不包围 · none</ASelectOption></ASelect>
          </AFormItem>
          <AFormItem v-if="dataOptionsActive && ordinaryFormat && (formatKind === 'CSV' || formatKind === 'CUT')" extra="留空继承默认 \N；与空字符串不同，字面空格按原值保留。">
            <template #label><span class="export-field-label"><span>NULL 表示</span><ExportOptionHint label="NULL 表示" parameter="--null-string" description="设置 CSV/CUT 中 NULL 的输出字符串，默认 \N。NULL 与空字符串不同；显式输入的首尾空格按原值保留。" /></span></template><AInput v-model:value="nullString" aria-label="NULL 替换" placeholder="默认 \N" />
          </AFormItem>
          <AFormItem v-if="dataOptionsActive && ordinaryFormat && (formatKind === 'CSV' || formatKind === 'CUT')" extra="当前仅支持单个 ASCII 字符；留空继承该格式的工具默认值。" :validate-status="!isValidEscapeCharacter(escapeCharacter) ? 'error' : undefined" :help="!isValidEscapeCharacter(escapeCharacter) ? '请输入单个 ASCII 字符，不能使用换行或 NUL。' : undefined">
            <template #label><span class="export-field-label"><span>转义字符</span><ExportOptionHint label="转义字符" parameter="--escape-character" description="指定 CSV/CUT 用来转义特殊字符的字符。当前仅接受单个 ASCII 字符，不允许换行或 NUL；未填写时继承该格式默认。" /></span></template><AInput v-model:value="escapeCharacter" aria-label="转义字符" placeholder="例如 |" />
          </AFormItem>
        </div>
        <template v-if="dataOptionsActive && ordinaryFormat && formatKind === 'CSV'">
          <p class="section-hint">默认 non_numeric（非数字包围）；all 全部包围、all_not_null 非空包围、minimal 最小包围、none 不包围。</p>
          <AAlert v-if="columnQuoteMode === 'none'" type="warning" show-icon message="不包围模式在数据包含分隔符、包围符或换行时可能生成无法直接导入的 CSV；请确认数据已正确转义。" />
        </template>
        <p v-if="dataOptionsActive && ordinaryFormat && formatKind === 'CSV'" class="section-hint">常用组合：默认逗号 + 单引号 + 系统换行；也可选双引号与固定换行。包围模式、NULL 表示与转义字符在本区配置；空格修剪在下方“其他选项”勾选。</p>
        <p v-else-if="formatKind === 'CUT'" class="section-hint">CUT 使用分隔字符串和数据文件换行符；单字符分隔时，工具会转义数据中的分隔符与换行。NULL 表示与转义在本区配置；文本处理在下方“其他选项”勾选，删除换行会改变数据内容。</p>
        <p v-else class="section-hint">SQL 数据文件与仅导出结构的 DDL 文件不同。此格式仅配置文件编码与换行符号；日期、文件拆分与压缩在高级设置中按适用范围配置。</p>
      </AForm>
    </div>
    <div class="export-field-group" role="region" aria-label="其他选项">
      <h3>其他选项</h3>
      <AForm layout="vertical" class="export-field-body export-format-panel export-other-options-body">
        <section v-if="contentKind !== 'DATA_ONLY'" class="export-option-group" aria-label="结构导出选项">
          <h4>结构导出</h4>
          <div class="export-option-content">
            <div class="export-format-toggle-grid">
              <AFormItem><div class="export-option-control"><ACheckbox v-model:checked="dropObject">前置 DROP</ACheckbox><ExportOptionHint label="前置 DROP" parameter="--drop-object" description="在导出的 DDL 中，先添加删除同名对象的语句，再生成创建语句。执行该 DDL 可能删除目标对象及其数据；导出过程不删除源对象。" /></div></AFormItem>
              <AFormItem><div class="export-option-control"><ACheckbox v-model:checked="retainSchema">保留 Schema 前缀</ACheckbox><ExportOptionHint label="保留 Schema 前缀" parameter="--retain-schema" description="在导出的对象定义中保留 Schema 名限定，例如 schema.table，便于明确对象归属。" /></div></AFormItem>
              <AFormItem v-if="compactSchemaSupported"><div class="export-option-control"><ACheckbox v-model:checked="compactSchema">使用原生建表语句</ACheckbox><ExportOptionHint label="使用原生建表语句" parameter="--compact-schema" description="通过 SHOW CREATE TABLE 获取表定义，而非查询系统视图后反拼 DDL。未勾选时继承工具默认策略，不表示显式关闭。" /></div></AFormItem>
              <AFormItem extra="sys 权限预检查与凭据绑定尚未接入，暂不可用。"><div class="export-option-control"><ACheckbox disabled>附加表定义信息</ACheckbox><ExportOptionHint label="附加表定义信息" parameter="--add-extra-message" description="在导出的表定义中附加表组名称等信息。依赖 sys 租户权限，当前尚未开放。" /></div></AFormItem>
            </div>
            <AAlert v-if="dropObject" class="export-other-options-warning" type="warning" show-icon message="--drop-object 会在导入侧重建对象前删除同名对象，可能造成数据丢失，请确认已了解影响。" />
          </div>
        </section>
        <section v-if="dataOptionsActive && scopeKind !== 'QUERY_RESULT' && !querySql" class="export-option-group" aria-label="数据读取选项">
          <h4>数据读取</h4>
          <div class="export-option-content">
            <div class="export-format-toggle-grid">
              <AFormItem extra="副本可用性与权限预检查尚未接入，暂不可用。"><div class="export-option-control"><ACheckbox disabled>备副本读取</ACheckbox><ExportOptionHint label="备副本读取" parameter="--weak-read" description="从备副本读取数据，采用弱一致性读取，数据可能落后于主副本。副本可用性与权限预检查尚未接入，暂不可用。" /></div></AFormItem>
              <AFormItem><div class="export-option-control"><ACheckbox v-model:checked="snapshot" :disabled="!snapshot && Boolean(flashbackScn || flashbackTimestamp)">一致性快照</ACheckbox><ExportOptionHint label="一致性快照" parameter="--snapshot" description="导出最近一次成功合并的基线数据，以获得全局一致性快照；不等同于当前最新数据。与闪回 SCN、闪回时间互斥。" /></div></AFormItem>
              <AFormItem v-if="scopeKind === 'ALL' || tableOnlySelection" extra="表结构、版本与权限预检查尚未接入，暂不可用。"><div class="export-option-control"><ACheckbox disabled>隐藏主键加速</ACheckbox><ExportOptionHint label="隐藏主键加速" parameter="--enable-hidden-pk" description="利用隐藏主键 __pk_increment 加速无主键表的读取，不表示将隐藏主键列加入导出文件。需确认表结构、数据库版本与权限，当前尚未开放。" /></div></AFormItem>
            </div>
          </div>
        </section>
        <section v-if="dataOptionsActive && (scopeKind !== 'QUERY_RESULT' || ordinaryFormat && (formatKind === 'CSV' || formatKind === 'CUT'))" class="export-option-group" aria-label="字段与文本处理选项">
          <h4>字段与文本处理</h4>
          <div class="export-option-content">
            <div class="export-format-toggle-grid">
              <AFormItem v-if="scopeKind !== 'QUERY_RESULT'"><div class="export-option-control"><ACheckbox v-model:checked="excludeVirtualColumns">排除生成列</ACheckbox><ExportOptionHint label="排除生成列" parameter="--exclude-virtual-columns" description="导出表数据时排除生成列的值；不删除源表列，也不改变导出的表结构定义。" /></div></AFormItem>
              <AFormItem v-if="ordinaryFormat && selectedSource?.compatibilityMode === 'MYSQL' && (formatKind === 'CSV' || formatKind === 'CUT')" extra="尚未完成零日期时间输出验证，暂不可用。"><div class="export-option-control"><ACheckbox disabled>保留零日期时间</ACheckbox><ExportOptionHint label="保留零日期时间" parameter="--preserve-zero-datetime" description="保留 MySQL DATE、DATETIME、TIMESTAMP 零值的原有表达方式。官方说明中，非空日期时间列读出的 NULL 导出为 0，可空列仍保留 NULL。" /></div></AFormItem>
              <AFormItem v-if="ordinaryFormat && formatKind === 'CSV'"><div class="export-option-control"><ACheckbox :checked="!skipHeader" @update:checked="skipHeader = !$event">包含列名表头</ACheckbox><ExportOptionHint label="包含列名表头" parameter="--skip-header（反向映射）" description="勾选时保留 CSV 第一行列名；取消勾选时传入 --skip-header，跳过列名表头。" /></div></AFormItem>
              <AFormItem v-if="ordinaryFormat && (formatKind === 'CSV' || formatKind === 'CUT')"><div class="export-option-control"><ACheckbox v-model:checked="withTrim">去除首尾空格</ACheckbox><ExportOptionHint label="去除首尾空格" parameter="--with-trim" description="去除导出字段值首尾的空格；字段内的空格保持不变。此处理会改变导出值，不修改源表数据。" /></div></AFormItem>
              <AFormItem v-if="ordinaryFormat && formatKind === 'CUT'"><div class="export-option-control"><ACheckbox v-model:checked="trailDelimiter">行尾分隔符处理</ACheckbox><ExportOptionHint label="行尾分隔符处理" parameter="--trail-delimiter" description="控制 CUT 记录末尾的字段分隔符处理。官方 4.3.5 文档对保留或移除方向描述不一致，启用前需确认实际输出。" /></div></AFormItem>
              <AFormItem v-if="ordinaryFormat && formatKind === 'CUT'"><div class="export-option-control"><ACheckbox v-model:checked="removeNewline">移除回车换行</ACheckbox><ExportOptionHint label="移除回车换行" parameter="--remove-newline" description="删除 CUT 字段内容中的回车和换行字符，例如 \r、\n；不用于设置记录分隔符，不修改源表，但会改变导出文本。" /></div></AFormItem>
            </div>
            <AAlert v-if="formatKind === 'CUT' && removeNewline" class="export-other-options-warning" type="warning" show-icon message="删除换行会改变导出数据中的文本内容，请确认符合交付要求。" />
          </div>
        </section>
        <section v-if="dataOptionsActive && ordinaryFormat" class="export-option-group" aria-label="文件组织选项">
          <h4>文件组织</h4>
          <div class="export-option-content">
            <div class="export-format-toggle-grid">
              <AFormItem><div class="export-option-control"><ACheckbox v-model:checked="noNestedDir">扁平输出目录</ACheckbox><ExportOptionHint label="扁平输出目录" parameter="--no-nested-dir" description="所有导出文件直接写入指定输出目录，不再创建子目录。" /></div></AFormItem>
              <AFormItem><div class="export-option-control"><ACheckbox v-model:checked="retainEmptyFiles">保留空文件</ACheckbox><ExportOptionHint label="保留空文件" parameter="--retain-empty-files" description="空表、空分区或条件筛选结果为空时仍生成文件。CSV 默认保留列名表头；不包含表头时生成完全空文件。" /></div></AFormItem>
            </div>
          </div>
        </section>
      </AForm>
    </div>
    <ExportAdvancedSettings v-if="dataOptionsActive && ordinaryFormat" class="export-advanced" title="读取一致性与文件设置" :configured-count="formatAdvancedCount">
      <AForm layout="vertical" class="export-advanced-form">
        <template v-if="scopeKind !== 'QUERY_RESULT' && !querySql">
          <h3>闪回读取</h3>
          <p class="section-hint">可选；未设置快照或闪回参数时读取实时数据，不承诺全局一致性。闪回参数与一致性快照互斥。</p>
          <div class="export-field-grid">
            <AFormItem extra="--flashback-scn；仅接受正整数。" :validate-status="flashbackScn && !/^[1-9][0-9]*$/.test(flashbackScn) ? 'error' : undefined" :help="flashbackScn && !/^[1-9][0-9]*$/.test(flashbackScn) ? '闪回 SCN 必须是正整数。' : undefined">
              <template #label><span class="export-field-label"><span>闪回 SCN</span><ExportOptionHint label="闪回 SCN" parameter="--flashback-scn" description="指定用于导出数据的 SCN 事务点，仅接受正整数。与一致性快照及自定义查询互斥；留空不指定 SCN。" /></span></template><AInput v-model:value.trim="flashbackScn" aria-label="闪回 SCN" :disabled="snapshot" inputmode="numeric" placeholder="留空不指定 SCN" />
            </AFormItem>
            <AFormItem v-if="selectedSource?.compatibilityMode === 'ORACLE'" extra="--flashback-timestamp；仅 Oracle 兼容模式可用。">
              <template #label><span class="export-field-label"><span>闪回时间点</span><ExportOptionHint label="闪回时间点" parameter="--flashback-timestamp" description="指定闪回读取的时间点，仅 Oracle 兼容模式可用。与一致性快照及自定义查询互斥；留空不指定时间点。" /></span></template><AInput v-model:value.trim="flashbackTimestamp" aria-label="闪回时间点" :disabled="snapshot" placeholder="例如 2026-10-08 00:00:00" />
            </AFormItem>
          </div>
        </template>
        <template v-if="dataOptionsActive && ordinaryFormat && (formatKind === 'CSV' || formatKind === 'CUT')">
          <h3>日期时间输出约定</h3>
          <p class="section-hint">当前数据源为 {{ selectedSource?.compatibilityMode === 'MYSQL' ? 'MySQL' : selectedSource?.compatibilityMode === 'ORACLE' ? 'Oracle' : '未确认' }} 兼容模式；日期时间参数按租户模式和已验证范围启用。</p>
          <div v-if="timestampFormatsSupported" class="export-field-grid">
            <AFormItem :html-for="fieldPrefix + '-datetime-format'">
              <template #label><span class="export-field-label"><span>DATETIME 值格式</span><ExportOptionHint label="DATETIME 值格式" parameter="--datetime-value-format" description="设置导出 DATETIME 值的文本格式，当前仅开放 MySQL CSV/CUT 的已验证组合。不改变源字段类型；留空继承工具默认。" /></span></template><ExportFormatChoice :id="fieldPrefix + '-datetime-format'" v-model="datetimeValueFormat" label="DATETIME 值格式" :options="datetimeFormatChoices" custom-placeholder="输入自定义 DATETIME 格式" />
            </AFormItem>
            <AFormItem :html-for="fieldPrefix + '-date-format'">
              <template #label><span class="export-field-label"><span>DATE 值格式</span><ExportOptionHint label="DATE 值格式" parameter="--date-value-format" description="设置导出 DATE 值的文本格式，例如 yyyy-MM-dd 或 yyyyMMdd。当前仅开放 MySQL CSV/CUT 的已验证组合，Oracle 格式尚未开放。" /></span></template><ExportFormatChoice :id="fieldPrefix + '-date-format'" v-model="dateValueFormat" label="DATE 值格式" :options="dateFormatChoices" custom-placeholder="输入自定义 DATE 格式" />
            </AFormItem>
          </div>
          <p v-if="timestampFormatsSupported" class="section-hint">TIME、TIMESTAMP 与零日期保留尚待验证，当前继承工具默认。</p>
          <p v-else-if="selectedSource?.compatibilityMode === 'ORACLE'" class="section-hint">Oracle 日期时间值格式尚待验证，当前继承工具默认；NLS 会话格式不等同于导出值格式。</p>
          <p v-else class="section-hint">当前格式或租户模式没有已验证的日期时间值格式选项。</p>
        </template>
        <h3>文件拆分</h3>
        <AFormItem>
          <template #label><span class="export-field-label"><span>文件拆分方式</span><ExportOptionHint label="文件拆分方式" parameter="--block-size（单位：MB / ROW）" description="选择单个文件按大小或行数拆分。MB 按大小设置阈值，ROW 按行数设置阈值；不限制整个任务的导出总量。" /></span></template><ARadioGroup :value="splitUnit" role="radiogroup" aria-label="文件拆分方式" @update:value="changeSplitUnit"><ARadio value="MB">按大小（MB）</ARadio><ARadio value="ROW">按行数（ROW）</ARadio></ARadioGroup>
        </AFormItem>
        <AFormItem :html-for="fieldPrefix + '-block-size'" extra="达到阈值后拆分，不限制总导出量；不指定时继承工具默认。" :validate-status="attemptedStep === 3 && blockSize.trim() && !BLOCK_SIZE_PATTERN.test(blockSize.trim()) ? 'error' : undefined">
          <template #label><span class="export-field-label"><span>{{ splitUnit === 'MB' ? '单个文件拆分阈值（MB）' : '单个文件拆分阈值（行）' }}</span><ExportOptionHint label="单个文件拆分阈值" parameter="--block-size" description="达到阈值后拆分为多个文件，不限制总导出量。MB 模式填正整数或整数MB，ROW 模式使用整数ROW；未设置时继承工具默认。" /></span></template>
          <ExportFormatChoice :id="fieldPrefix + '-block-size'" v-model="splitThreshold" label="单个文件上限" :options="blockSizeChoices" :custom-placeholder="splitUnit === 'MB' ? '正整数或整数MB' : '正整数行数，例如 256'" />
        </AFormItem>
        <h3>压缩参数</h3>
        <AFormItem><div class="export-option-control"><ACheckbox v-model:checked="compress">启用压缩</ACheckbox><ExportOptionHint label="启用压缩" parameter="--compress" description="压缩导出的 CSV/CUT/SQL 数据文件，以减小文件体积。启用后可指定算法与等级；关闭时清除算法和等级。" /></div></AFormItem>
        <p v-if="!compress" class="section-hint">勾选启用压缩后，可设置压缩算法和等级；未指定算法时使用 zstd。</p>
        <div v-else class="export-field-grid">
          <AFormItem v-if="compress" :html-for="fieldPrefix + '-compression-algo'">
            <template #label><span class="export-field-label"><span>压缩算法</span><ExportOptionHint label="压缩算法" parameter="--compression-algo" description="指定导出压缩算法，可选 zstd、zlib、gzip、snappy。需启用压缩；自动选择 zstd，gzip/snappy 不支持指定压缩等级。" /></span></template><ASelect :id="fieldPrefix + '-compression-algo'" v-model:value="compressionAlgo" aria-label="压缩算法" :disabled="!compress"><ASelectOption value="">自动（zstd）</ASelectOption><ASelectOption value="zstd">zstd</ASelectOption><ASelectOption value="zlib">zlib</ASelectOption><ASelectOption value="gzip">gzip</ASelectOption><ASelectOption value="snappy">snappy</ASelectOption></ASelect>
          </AFormItem>
          <AFormItem v-if="compress && compressionAlgo !== 'gzip' && compressionAlgo !== 'snappy'">
            <template #label><span class="export-field-label"><span>压缩等级</span><ExportOptionHint label="压缩等级" parameter="--compression-level" description="指定压缩算法的等级：zstd 支持 1~22，zlib 支持 -1~9。gzip/snappy 不提供等级；不填写时使用算法默认等级。" /></span></template><AInput v-model:value.trim="compressionLevel" aria-label="压缩等级" :disabled="!compress" :placeholder="compressionAlgo === 'zlib' ? '例如 5（zlib 支持 -1~9）' : '例如 3（zstd 支持 1~22）'" />
          </AFormItem>
        </div>
        <p v-if="compress" class="section-hint">gzip/snappy 不支持指定压缩等级；取消启用压缩时清除算法与等级。</p>
      </AForm>
    </ExportAdvancedSettings>
  </section>
</template>
