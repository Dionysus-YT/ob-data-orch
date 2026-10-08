<script setup lang="ts">
import { toRefs } from 'vue'
import type { ExportObjectsStepModel } from '../exportStepModels'
import { Form as AForm, FormItem as AFormItem, RadioGroup as ARadioGroup, RadioButton as ARadioButton, Skeleton as ASkeleton, Select as ASelect, SelectOption as ASelectOption, SelectOptGroup as ASelectOptGroup, Button as AButton, Alert as AAlert, Radio as ARadio, Input as AInput, Modal as AModal, Checkbox as ACheckbox, Empty as AEmpty, List as AList, ListItem as AListItem } from 'ant-design-vue'
import { CaretDownOutlined, CaretRightOutlined, DeleteOutlined, SearchOutlined } from '@ant-design/icons-vue'
import SqlQueryEditor from '@/components/SqlQueryEditor.vue'
import ExportAdvancedSettings from '@/components/ExportAdvancedSettings.vue'
import ExportOptionHint from '@/components/ExportOptionHint.vue'

const props = defineProps<{ model: ExportObjectsStepModel }>()
const { exportMode, scopeKind, contentKind, fieldPrefix, nodeLoadFailure, loadingNodes, selectedNodeID, derivedDraftBindingLocked, nodes, attemptedStep, database, selectedSource, databaseDropdownOpen, databaseCatalogLoading, onDatabaseDropdownVisibleChange, scheduleDatabaseCatalog, selectDatabaseOption, databaseOptions, databaseCatalogFailure, databaseCatalogLoaded, manualDatabaseInput, manualDatabaseError, manualDatabaseOpen, loadDatabaseCatalog, databaseCatalogTruncated, querySql, queryResultLimit, objectInputMessage, confirmManualDatabase, enteredObjectCount, allSelectedObjects, candidateTotalCount, candidateObjectKeyword, loadCatalog, catalogLoading, catalogKeywordTooLong, catalogFailure, catalogTruncated, visibleObjectCategories, objectType, candidateGroupExpanded, chooseObjectCategory, selectableCategoryNames, categorySelectedCount, toggleCategorySelection, catalogCount, candidateObjectNames, catalogLoaded, visibleCandidateObjectNames, objectNames, toggleCandidate, clearObjectNameRows, selectedObjectKeyword, visibleSelectedGroups, selectedGroupExpanded, clearObjectCategory, removeObjectNameRow, objectAdvancedCount, dataOptionsActive, excludeTablesSupported, excludeTablesText, includeColumnNames, excludeColumnNames, excludeDataTypes, whereSupported, where, partitionSupported, partition, migrateLegacyQuery } = toRefs(props.model)
</script>

<template>
  <section class="form-section">
    <div class="export-field-group">
      <h3>导出内容</h3>
      <AForm layout="vertical" class="export-content-form">
        <AFormItem required>
          <ARadioGroup v-model:value="exportMode" class="export-content-options" button-style="solid" role="radiogroup" aria-label="导出内容"><ARadioButton value="DDL_AND_DATA">导出结构和数据</ARadioButton><ARadioButton value="DATA_ONLY">仅导出数据</ARadioButton><ARadioButton value="DDL_ONLY">仅导出结构</ARadioButton><ARadioButton value="QUERY_RESULT">按结果集导出</ARadioButton></ARadioGroup>
        </AFormItem>
      </AForm>
      <p class="section-hint">{{ scopeKind === 'QUERY_RESULT' ? '将单条 SELECT 的结果作为数据导出；只支持 CSV、CUT 和 SQL 格式，数据库权限与语句有效性在任务执行时确认。' : contentKind === 'DDL_ONLY' ? '仅生成对象定义；数据格式和数据文件参数不参与任务。' : contentKind === 'DATA_ONLY' ? '仅导出表数据；视图等只支持结构的对象不可选。' : '同时生成对象定义与表数据；只支持结构的对象不会被标记为已导出数据。' }}</p>
    </div>
    <div class="export-field-group">
      <h3>{{ scopeKind === 'QUERY_RESULT' ? '数据库与结果集' : '数据库与导出范围' }}</h3>
      <AForm layout="vertical" class="export-field-body">
        <div class="export-database-grid">
          <AFormItem :html-for="fieldPrefix + '-catalog-node'" required :help="nodeLoadFailure || '选择节点后加载当前数据源的数据库目录；执行任务时使用同一节点。'">
            <template #label>读取元数据的执行节点</template>
            <ASkeleton v-if="loadingNodes" active :paragraph="{ rows: 1 }" aria-label="正在加载已授权执行节点" />
            <ASelect v-else :id="fieldPrefix + '-catalog-node'" v-model:value="selectedNodeID" aria-label="读取元数据的执行节点" :disabled="derivedDraftBindingLocked" placeholder="请选择执行节点"><ASelectOption v-for="node in nodes" :key="node.id" :value="node.id">{{ node.displayName }} · {{ node.platform }}</ASelectOption></ASelect>
          </AFormItem>
          <AFormItem :html-for="fieldPrefix + '-database'" required :validate-status="attemptedStep === 2 && !database.trim() ? 'error' : undefined" :help="attemptedStep === 2 && !database.trim() ? '请选择数据库或 Schema。' : `数据源：${selectedSource?.displayName ?? '尚未选择'}；可搜索已读取的数据库，或手动输入其他名称。`">
            <template #label>数据库</template>
            <div class="export-database-select">
              <ASelect :id="fieldPrefix + '-database'" :value="database || undefined" :open="databaseDropdownOpen" aria-label="数据库 / Schema" show-search :filter-option="false" :loading="databaseCatalogLoading" placeholder="请选择数据库 / Schema" @click="onDatabaseDropdownVisibleChange(true)" @search="scheduleDatabaseCatalog" @change="selectDatabaseOption" @dropdown-visible-change="onDatabaseDropdownVisibleChange">
                <ASelectOption v-if="!selectedNodeID" value="__choose_node__" disabled>请先选择读取元数据的执行节点</ASelectOption>
                <ASelectOptGroup v-if="databaseOptions.length" :label="selectedSource?.displayName ?? '当前数据源'">
                  <ASelectOption v-for="name in databaseOptions" :key="name" :value="name" :label="name">{{ name }}</ASelectOption>
                </ASelectOptGroup>
                <ASelectOption v-if="databaseCatalogLoading" value="__loading__" disabled>正在读取数据库目录…</ASelectOption>
                <ASelectOption v-else-if="databaseCatalogFailure" value="__unavailable__" disabled>目录暂不可用，可重试或手动输入</ASelectOption>
                <ASelectOption v-else-if="selectedNodeID && databaseCatalogLoaded && !databaseOptions.length" value="__empty__" disabled>没有匹配的数据库 / Schema</ASelectOption>
                <ASelectOption value="__manual__" label="手动输入其他数据库 / Schema">手动输入其他数据库 / Schema…</ASelectOption>
              </ASelect>
              <AButton aria-label="展开数据库列表" @click="onDatabaseDropdownVisibleChange(!databaseDropdownOpen)"><template #icon><CaretDownOutlined aria-hidden="true" /></template></AButton>
              <AButton type="link" class="export-database-manual" @click="manualDatabaseInput = database; manualDatabaseError = ''; manualDatabaseOpen = true">手动输入数据库</AButton>
            </div>
          </AFormItem>
        </div>
        <AAlert v-if="databaseCatalogFailure" type="warning" show-icon :message="databaseCatalogFailure"><template #action><AButton type="link" :disabled="!selectedNodeID" @click="loadDatabaseCatalog">重试</AButton></template></AAlert>
        <p v-else-if="databaseCatalogTruncated" class="section-hint" role="status">仅显示前 100 个匹配数据库；输入关键字继续筛选。</p>
        <p v-else-if="!selectedNodeID" class="section-hint">选择执行节点后加载数据库目录；已登记的默认数据库仍可直接选择。</p>
        <AFormItem v-if="scopeKind !== 'QUERY_RESULT'" required>
          <template #label>导出范围</template><ARadioGroup v-model:value="scopeKind" role="radiogroup" aria-label="导出范围"><ARadio value="SPECIFIED">部分导出</ARadio><ARadio value="ALL">整库导出</ARadio></ARadioGroup>
        </AFormItem>
        <p v-if="scopeKind === 'ALL'" class="section-hint">整库导出按当前数据库范围生成 --all；指定对象区已收起。</p>
      </AForm>
    </div>
    <div v-if="scopeKind === 'QUERY_RESULT'" class="export-field-group">
      <h3>导出范围 · 查询结果集</h3>
      <AForm layout="vertical" class="export-field-body">
        <AFormItem required extra="仅支持输入单条 SELECT；此处只配置 OBDUMPER --query-sql，不在页面执行查询。">
          <template #label>查询 SQL</template>
          <SqlQueryEditor v-model="querySql" :language="selectedSource?.compatibilityMode === 'MYSQL' ? 'mysql' : 'sql'" />
        </AFormItem>
        <AFormItem required extra="默认 1000；控制面按 MySQL / Oracle 兼容模式将上限写入查询，不使用独立 OBDUMPER 参数。">
          <template #label>查询结果条数限制</template>
          <AInput v-model:value="queryResultLimit" aria-label="查询结果条数限制" inputmode="numeric" :maxlength="10" class="export-result-limit" />
        </AFormItem>
        <AAlert v-if="objectInputMessage && database.trim()" type="warning" show-icon :message="objectInputMessage" />
      </AForm>
    </div>
    <AModal :open="manualDatabaseOpen" title="手动输入数据库 / Schema" ok-text="使用此名称" cancel-text="取消" @ok="confirmManualDatabase" @cancel="manualDatabaseOpen = false">
      <AInput v-model:value="manualDatabaseInput" aria-label="手动输入数据库 / Schema" autocomplete="off" :maxlength="256" @press-enter="confirmManualDatabase" />
      <AAlert v-if="manualDatabaseError" type="error" show-icon :message="manualDatabaseError" />
      <p class="section-hint">目录不可用时可填写明确名称，最终访问权限由预检查确认。</p>
    </AModal>
    <AAlert v-if="scopeKind === 'SPECIFIED' && !database.trim()" type="info" show-icon message="选择数据库后显示导出对象" description="先从数据库下拉框选择，或手动输入数据库 / Schema。" />
    <div v-else-if="scopeKind === 'SPECIFIED'" class="export-field-group export-object-group">
      <div class="export-group-heading"><h3>导出对象</h3><span class="export-object-count" role="status">{{ database }} · 已选 {{ enteredObjectCount }} 项</span></div>
      <AAlert v-if="contentKind === 'DATA_ONLY' && allSelectedObjects.length > enteredObjectCount" type="info" show-icon message="已选的非表对象暂不参与仅数据导出；切回包含结构的模式后会恢复显示。" />
      <div class="export-object-workspace">
        <section class="export-object-pane" aria-label="选择导出对象">
          <div class="export-object-pane-heading"><h4>选择对象({{ candidateTotalCount }})</h4></div>
          <div class="export-object-search"><AInput v-model:value="candidateObjectKeyword" aria-label="搜索候选对象" placeholder="搜索关键字" :maxlength="100" allow-clear @press-enter="loadCatalog()" /><AButton :loading="catalogLoading" :disabled="catalogLoading || !selectedNodeID || !database.trim() || catalogKeywordTooLong" aria-label="搜索或刷新对象" @click="loadCatalog()"><template #icon><SearchOutlined aria-hidden="true" /></template></AButton></div>
          <div class="export-object-scroll" tabindex="0" role="region" aria-label="候选对象滚动区">
            <AAlert v-if="catalogKeywordTooLong" type="warning" show-icon message="名称关键字最多 100 字节。" />
            <AAlert v-if="catalogFailure" type="error" show-icon :message="catalogFailure" />
            <AAlert v-if="catalogTruncated" type="info" show-icon message="当前目录仅显示前 100 个匹配对象；输入名称关键字继续筛选，已选对象会保留。" />
            <p v-else-if="catalogLoading" class="section-hint" role="status">正在通过执行节点读取对象元数据…</p>
            <div class="export-object-tree" aria-label="候选对象分类">
              <template v-for="category in visibleObjectCategories" :key="category.type">
                <div class="export-object-tree-category" :class="{ 'is-active': category.type === objectType }">
                  <AButton type="text" class="export-object-expand" :aria-label="`${category.type === objectType && candidateGroupExpanded ? '收起' : '展开'}${category.label}分类`" @click="chooseObjectCategory(category.type)"><template #icon><CaretDownOutlined v-if="category.type === objectType && candidateGroupExpanded" aria-hidden="true" /><CaretRightOutlined v-else aria-hidden="true" /></template></AButton>
                  <ACheckbox :checked="selectableCategoryNames(category.type).length > 0 && categorySelectedCount(category.type) === selectableCategoryNames(category.type).length" :indeterminate="categorySelectedCount(category.type) > 0 && categorySelectedCount(category.type) < selectableCategoryNames(category.type).length" :disabled="selectableCategoryNames(category.type).length === 0" :aria-label="category.type === objectType ? `选择全部可见${category.label}` : `选择全部${category.label}`" @change="toggleCategorySelection(category.type)" />
                  <span v-if="category.glyph" class="export-object-kind-icon export-object-kind-glyph" aria-hidden="true">{{ category.glyph }}</span><component :is="category.icon" v-else class="export-object-kind-icon" aria-hidden="true" />
                  <AButton type="link" class="export-object-category-name" @click="chooseObjectCategory(category.type)">{{ category.label }}（{{ catalogCount(category.type) }}）</AButton>
                  <span v-if="category.type !== 'TABLE'" class="export-object-kind-note">仅结构</span>
                </div>
                <template v-if="category.type === objectType && candidateGroupExpanded">
                  <AEmpty v-if="candidateObjectNames.length === 0 && !catalogLoading" class="export-object-tree-empty" :description="catalogFailure ? '自动读取失败，请重试' : catalogLoaded ? '当前条件没有匹配对象' : '选择执行节点后加载对象'" />
                  <AEmpty v-else-if="visibleCandidateObjectNames.length === 0" class="export-object-tree-empty" description="没有匹配的候选对象" />
                  <AList v-else size="small" class="export-object-tree-children" :data-source="visibleCandidateObjectNames" aria-label="候选对象列表">
                    <template #renderItem="{ item }"><AListItem><ACheckbox :checked="objectNames.includes(item)" @change="toggleCandidate(item)"><span v-if="category.glyph" class="export-object-kind-icon export-object-kind-glyph" aria-hidden="true">{{ category.glyph }}</span><component :is="category.icon" v-else class="export-object-kind-icon" aria-hidden="true" />{{ item }}</ACheckbox></AListItem></template>
                  </AList>
                </template>
              </template>
            </div>
          </div>
        </section>
        <section class="export-object-pane export-object-selected" aria-label="已选导出对象">
          <div class="export-object-pane-heading"><h4>已选 {{ enteredObjectCount }} 项</h4><AButton type="link" :disabled="enteredObjectCount === 0" @click="clearObjectNameRows">清空</AButton></div>
          <AInput v-model:value="selectedObjectKeyword" aria-label="搜索已选对象" placeholder="搜索关键字" allow-clear :disabled="enteredObjectCount === 0"><template #suffix><SearchOutlined aria-hidden="true" /></template></AInput>
          <div class="export-object-scroll" tabindex="0" role="region" aria-label="已选对象滚动区">
            <AEmpty v-if="enteredObjectCount === 0" description="尚未选择对象" />
            <AEmpty v-else-if="visibleSelectedGroups.length === 0" description="没有匹配的已选对象" />
            <template v-else>
              <template v-for="group in visibleSelectedGroups" :key="group.type">
                <div class="export-object-tree-category export-object-selected-category"><AButton type="text" class="export-object-expand" :aria-label="`${selectedGroupExpanded[group.type] ? '收起' : '展开'}已选${group.label}`" @click="selectedGroupExpanded[group.type] = !selectedGroupExpanded[group.type]"><template #icon><CaretDownOutlined v-if="selectedGroupExpanded[group.type]" aria-hidden="true" /><CaretRightOutlined v-else aria-hidden="true" /></template></AButton><span v-if="group.glyph" class="export-object-kind-icon export-object-kind-glyph" aria-hidden="true">{{ group.glyph }}</span><component :is="group.icon" v-else class="export-object-kind-icon" aria-hidden="true" /><span>{{ group.label }}（{{ group.rows.length }}）</span><AButton type="text" class="export-object-delete" :aria-label="`清空已选${group.label}`" @click="clearObjectCategory(group.type)"><template #icon><DeleteOutlined aria-hidden="true" /></template></AButton></div>
                <AList v-if="selectedGroupExpanded[group.type]" size="small" class="export-object-tree-children" :data-source="group.rows" :aria-label="`已选${group.label}列表`">
                  <template #renderItem="{ item }"><AListItem><span v-if="group.glyph" class="export-object-kind-icon export-object-kind-glyph" aria-hidden="true">{{ group.glyph }}</span><component :is="group.icon" v-else class="export-object-kind-icon" aria-hidden="true" /><span>{{ item.name }}</span><AButton type="text" class="export-object-delete" :aria-label="`移除已选对象 ${item.name}`" @click="removeObjectNameRow(item.index, group.type)"><template #icon><DeleteOutlined aria-hidden="true" /></template></AButton></AListItem></template>
                </AList>
              </template>
            </template>
          </div>
        </section>
      </div>
    </div>
    <ExportAdvancedSettings v-if="scopeKind !== 'QUERY_RESULT'" class="export-advanced" title="DDL、对象与数据筛选" :configured-count="objectAdvancedCount">
      <AForm layout="vertical" class="export-advanced-form">
        <template v-if="contentKind !== 'DATA_ONLY'">
          <h3>DDL 与对象处理</h3>
          <AFormItem :html-for="fieldPrefix + '-sequence-policy'" extra="--sequence-policy 待验证，当前保持关闭。">
            <template #label><span class="export-field-label">序列策略<ExportOptionHint label="序列策略" parameter="--sequence-policy" description="选择保留序列当前值或重置序列；当前未完成能力验证，保持禁用。" /></span></template><ASelect :id="fieldPrefix + '-sequence-policy'" default-value="preserve（默认）" disabled><ASelectOption value="preserve（默认）">preserve（默认）</ASelectOption><ASelectOption value="restart">restart</ASelectOption></ASelect>
          </AFormItem>
        </template>
        <h3 v-if="dataOptionsActive || excludeTablesSupported">对象排除与数据筛选</h3>
        <AFormItem v-if="excludeTablesSupported" extra="多个表名以逗号分隔。">
          <template #label><span class="export-field-label">排除表（可选）<ExportOptionHint label="排除表" parameter="--exclude-table" description="排除指定表，多个表名以逗号分隔；仅在当前对象范围支持时生效。" /></span></template><AInput v-model:value.trim="excludeTablesText" aria-label="排除表" autocomplete="off" placeholder="例如 tmp_a,tmp_b" />
        </AFormItem>
        <template v-if="dataOptionsActive">
          <p class="section-hint">以下筛选仅在包含数据时生效；服务端将复核参数互斥和对象资格。</p>
          <h3>列与类型筛选</h3>
          <AFormItem extra="多个列名以逗号分隔。">
            <template #label><span class="export-field-label">包含列<ExportOptionHint label="包含列" parameter="--include-column-names" description="只导出指定列，多个列名以逗号分隔；与排除列互斥。" /></span></template><AInput v-model:value.trim="includeColumnNames" aria-label="包含列" :disabled="Boolean(excludeColumnNames)" placeholder="例如 col_a,col_b" />
          </AFormItem>
          <AFormItem extra="多个列名以逗号分隔。">
            <template #label><span class="export-field-label">排除列<ExportOptionHint label="排除列" parameter="--exclude-column-names" description="不导出指定列，多个列名以逗号分隔；与包含列互斥。" /></span></template><AInput v-model:value.trim="excludeColumnNames" aria-label="排除列" :disabled="Boolean(includeColumnNames)" placeholder="例如 col_c" />
          </AFormItem>
          <AFormItem>
            <template #label><span class="export-field-label">排除数据类型<ExportOptionHint label="排除数据类型" parameter="--exclude-data-types" description="不导出指定类型的列，多个类型名以逗号分隔；仅包含数据时生效。" /></span></template><AInput v-model:value.trim="excludeDataTypes" aria-label="排除数据类型" placeholder="例如 BLOB,TEXT" />
          </AFormItem>
          <h3>数据筛选</h3>
          <AFormItem :extra="whereSupported ? '仅指定表范围可用，与自定义查询互斥。' : '仅指定表范围可用；整库及包含其他类型的对象范围不适用。'">
            <template #label><span class="export-field-label">条件筛选<ExportOptionHint label="条件筛选" parameter="--where" description="按 WHERE 条件筛选指定表的数据；不填写 WHERE 关键字，与结果集查询互斥。" /></span></template><AInput v-model:value.trim="where" aria-label="条件筛选" :disabled="!whereSupported || Boolean(querySql)" placeholder="例如 id &gt; 100" />
          </AFormItem>
          <AFormItem :extra="partitionSupported ? '仅指定表范围可用，多个分区以逗号分隔；与自定义查询互斥。' : '仅指定表范围可用；整库及包含其他类型的对象范围不适用。'">
            <template #label><span class="export-field-label">分区筛选<ExportOptionHint label="分区筛选" parameter="--partition" description="仅导出指定表的所选分区，多个分区以逗号分隔；与结果集查询互斥。" /></span></template><AInput v-model:value.trim="partition" aria-label="分区筛选" :disabled="!partitionSupported || Boolean(querySql)" placeholder="例如 p0,p2" />
          </AFormItem>
          <AAlert v-if="querySql" type="info" show-icon message="历史草稿包含自定义查询" description="原查询继续保留；转到结果集入口后只导出查询数据，并清除不适用的筛选。"><template #action><AButton type="link" @click="migrateLegacyQuery">转到结果集编辑</AButton></template></AAlert>
        </template>
      </AForm>
    </ExportAdvancedSettings>
    <p class="section-hint">{{ scopeKind === 'QUERY_RESULT' ? '固定预检查仅验证连接等基础条件；查询语法、被引用对象和实际权限在 OBDUMPER 执行时确认。' : '对象存在性和实际权限不在此页推断，仍由后续固定预检查确认。' }}</p>
  </section>
</template>
