<script setup lang="ts">
import { toRefs } from 'vue'
import { EditOutlined, HddOutlined } from '@ant-design/icons-vue'
import type { ExportOutputStepModel } from '../exportStepModels'
import { Alert as AAlert, Form as AForm, FormItem as AFormItem, Button as AButton, Skeleton as ASkeleton, RadioGroup as ARadioGroup, Radio as ARadio, Input as AInput, Select as ASelect, SelectOption as ASelectOption, Checkbox as ACheckbox } from 'ant-design-vue'
import ExportOptionHint from '@/workbench/export/components/ExportOptionHint.vue'
import ExportAdvancedSettings from '@/workbench/export/components/ExportAdvancedSettings.vue'

const props = defineProps<{ model: ExportOutputStepModel }>()
const { draftNotice, attemptedStep, selectedNodeID, nodeLoadFailure, loadNodeCandidates, loadingNodes, selectedNode, derivedDraftBindingLocked, moveToStep, outputKind, storageBucket, storagePath, storageEndpoint, storageRegion, fieldPrefix, storageCredentialLoadFailure, loadStorageCredentials, loadingStorageCredentials, storageCredentialID, matchingStorageCredentials, filePath, outputPathPlaceholder, logPath, skipCheckDir, executionAdvancedCount, dataOptionsActive, maxFileSize, thread, pageSize, parallelMacro, selectedSource, fetchSize, jvmMemory, tmpPath, draftFailure } = toRefs(props.model)
</script>

<template>
  <section class="form-section">
    <AAlert v-if="draftNotice" type="info" show-icon :message="draftNotice" />
    <div class="export-field-group">
      <h3>输出设置</h3>
      <AForm layout="vertical" class="export-form">
        <AFormItem required :validate-status="attemptedStep === 4 && !selectedNodeID ? 'error' : undefined" :help="attemptedStep === 4 && !selectedNodeID ? '请返回导出内容与对象选择执行节点。' : undefined">
          <template #label>执行节点</template>
          <AAlert v-if="nodeLoadFailure" type="error" show-icon :message="nodeLoadFailure"><template #action><AButton type="link" @click="loadNodeCandidates">重试</AButton></template></AAlert>
          <ASkeleton v-else-if="loadingNodes" active :paragraph="{ rows: 1 }" aria-label="正在加载已授权执行节点" />
          <div v-else class="export-output-node" role="group" aria-label="执行节点信息">
            <div class="export-output-node-facts">
              <HddOutlined class="export-output-node-icon" aria-hidden="true" />
              <div class="export-output-node-identity">
                <strong class="export-output-node-name">{{ selectedNode?.displayName ?? '尚未选择执行节点' }}</strong>
                <span v-if="selectedNode" class="export-output-node-platform">平台 · {{ selectedNode.platform }}</span>
              </div>
            </div>
            <AButton v-if="!derivedDraftBindingLocked" type="link" class="export-output-node-action" @click="moveToStep(2)">
              <template #icon><EditOutlined /></template>{{ selectedNode ? '更换节点' : '选择节点' }}
            </AButton>
          </div>
        </AFormItem>
        <AAlert v-if="selectedNode && derivedDraftBindingLocked" type="info" show-icon message="派生草稿固定使用来源任务的执行节点；节点状态仍由预检查确认。" />
        <AFormItem required>
          <template #label>输出目的地</template>
          <ARadioGroup v-model:value="outputKind" role="radiogroup" aria-label="输出类型"><ARadio value="LOCAL">本地路径</ARadio><ARadio value="OSS">OSS</ARadio><ARadio value="S3">S3</ARadio><ARadio value="COS">COS</ARadio><ARadio value="OBS">OBS</ARadio></ARadioGroup>
        </AFormItem>
        <template v-if="outputKind !== 'LOCAL'">
          <div class="export-field-grid">
            <AFormItem required>
              <template #label>Bucket</template><AInput v-model:value.trim="storageBucket" aria-label="Bucket" autocomplete="off" />
            </AFormItem>
            <AFormItem required>
              <template #label>对象路径</template><AInput v-model:value.trim="storagePath" aria-label="对象路径" placeholder="/exports/daily" autocomplete="off" />
            </AFormItem>
            <AFormItem>
              <template #label>Endpoint</template><AInput v-model:value.trim="storageEndpoint" aria-label="Endpoint" autocomplete="off" />
            </AFormItem>
            <AFormItem>
              <template #label>Region</template><AInput v-model:value.trim="storageRegion" aria-label="Region" autocomplete="off" />
            </AFormItem>
          </div>
          <AFormItem :html-for="fieldPrefix + '-storage-credential'" extra="只保存凭据标识与修订，不在页面读取密钥。">
            <template #label>存储凭据（可选）</template>
            <AAlert v-if="storageCredentialLoadFailure" type="error" show-icon :message="storageCredentialLoadFailure"><template #action><AButton type="link" @click="loadStorageCredentials">重试</AButton></template></AAlert>
            <ASkeleton v-else-if="loadingStorageCredentials" active :paragraph="{ rows: 1 }" aria-label="正在加载存储凭据" />
            <ASelect v-else :id="fieldPrefix + '-storage-credential'" v-model:value="storageCredentialID"><ASelectOption value="">不指定（依赖执行节点 Hadoop 配置）</ASelectOption><ASelectOption v-for="credential in matchingStorageCredentials" :key="credential.id" :value="credential.id">{{ credential.displayName }} · 修订 {{ credential.currentRevision }}</ASelectOption></ASelect>
          </AFormItem>
        </template>
        <div class="export-format-panel export-parameter-field">
          <template v-if="outputKind === 'LOCAL'">
            <AFormItem required :validate-status="attemptedStep === 4 && !filePath.trim() ? 'error' : undefined" :help="attemptedStep === 4 && !filePath.trim() ? '请填写与节点平台匹配的完整绝对路径。' : '平台原样传递此路径，不追加目录或转换平台格式。'">
              <template #label><span class="export-field-label">导出路径<ExportOptionHint label="导出路径" parameter="--file-path" description="执行节点上的完整绝对输出路径；原样传递，不追加目录或转换平台格式。" /></span></template>
              <AInput v-model:value.trim="filePath" aria-label="导出路径" :placeholder="outputPathPlaceholder" autocomplete="off" />
            </AFormItem>
          </template>
          <AAlert v-if="outputKind !== 'LOCAL'" type="warning" show-icon message="对象存储输出仍需端点连通性与凭据有效性预检查；未取得通过证据时不能提交。" />
          <AFormItem extra="留空时继承 OBDUMPER 默认日志目录。">
            <template #label><span class="export-field-label">日志路径（可选）<ExportOptionHint label="日志路径" parameter="--log-path" description="指定工具日志目录；可选，留空继承工具默认目录，填写后检查可写性。" /></span></template><AInput v-model:value.trim="logPath" aria-label="日志路径" :placeholder="outputPathPlaceholder" autocomplete="off" />
          </AFormItem>
        </div>
      </AForm>
    </div>
    <section v-if="outputKind === 'LOCAL'" class="export-field-group" aria-label="其他选项">
      <h3>其他选项</h3>
      <div class="export-format-panel">
        <div class="export-option-control"><ACheckbox v-model:checked="skipCheckDir">跳过导出目录空性检查</ACheckbox><ExportOptionHint label="跳过导出目录空性检查" parameter="--skip-check-dir" description="允许向已有文件的导出目录写入，可能覆盖同名文件；仍检查路径可写性与可用空间。" /></div>
        <AAlert v-if="skipCheckDir" class="export-other-options-warning" type="warning" show-icon message="可能覆盖同名文件；路径可写性与可用空间仍会检查。" />
      </div>
    </section>
    <ExportAdvancedSettings class="export-advanced" title="执行限制与资源" :configured-count="executionAdvancedCount">
      <AForm layout="vertical" class="export-advanced-form">
        <h3>执行限制</h3>
        <AFormItem v-if="dataOptionsActive" extra="限制整个进程的导出量，达到后停止；与第三步的单个文件拆分不同。">
          <template #label><span class="export-field-label">导出总量上限（Byte）<ExportOptionHint label="导出总量上限（Byte）" parameter="--max-file-size" description="限制整个导出进程的输出总量，单位 Byte，达到上限后停止；不同于第三步单个文件拆分。" /></span></template><AInput v-model:value.trim="maxFileSize" aria-label="导出总量上限" placeholder="正整数，例如 1048576" />
        </AFormItem>
        <AAlert v-if="dataOptionsActive && maxFileSize" type="warning" show-icon message="已设置导出总量限制，输出可能不包含全部所选数据。" />
        <h3>性能与资源</h3>
        <div v-if="dataOptionsActive" class="export-field-grid">
          <AFormItem>
            <template #label><span class="export-field-label">导出线程<ExportOptionHint label="导出线程" parameter="--thread" description="设置并行导出线程数；留空继承工具默认，需结合节点资源和源库负载配置。" /></span></template><AInput v-model:value.trim="thread" aria-label="导出线程" placeholder="继承官方默认" />
          </AFormItem>
          <AFormItem>
            <template #label><span class="export-field-label">分页大小<ExportOptionHint label="分页大小" parameter="--page-size" description="设置分页查询每次读取的行数；留空继承工具默认。" /></span></template><AInput v-model:value.trim="pageSize" aria-label="分页大小" placeholder="继承 1,000,000" />
          </AFormItem>
          <AFormItem>
            <template #label><span class="export-field-label">每线程宏块数<ExportOptionHint label="每线程宏块数" parameter="--parallel-macro" description="设置每个导出线程并行处理的宏块数；留空继承工具默认。" /></span></template><AInput v-model:value.trim="parallelMacro" aria-label="每线程宏块数" placeholder="继承 8" />
          </AFormItem>
          <AFormItem v-if="selectedSource?.compatibilityMode === 'ORACLE'" extra="仅 Oracle 兼容模式适用；MySQL 租户不能配置。">
            <template #label><span class="export-field-label">游标抓取行数（Oracle）<ExportOptionHint label="游标抓取行数（Oracle）" parameter="--fetch-size" description="设置 JDBC 游标每次抓取的行数，仅 Oracle 兼容模式适用。" /></span></template><AInput v-model:value.trim="fetchSize" aria-label="游标抓取行数" :disabled="selectedSource?.compatibilityMode !== 'ORACLE'" placeholder="继承 1000" />
          </AFormItem>
          <AFormItem>
            <template #label><span class="export-field-label">JVM 内存<ExportOptionHint label="JVM 内存" parameter="--mem" description="设置运行工具的 JVM 内存，使用数字及 K/M/G/T 单位，例如 4G；留空继承默认。" /></span></template><AInput v-model:value.trim="jvmMemory" aria-label="JVM 内存" placeholder="例如 4G" />
          </AFormItem>
        </div>
        <AFormItem v-if="outputKind !== 'LOCAL'" extra="Multipart 上传的本地分块目录；留空则继承官方默认。">
          <template #label><span class="export-field-label">本地临时分块目录（可选）<ExportOptionHint label="本地临时分块目录" parameter="--tmp-path" description="对象存储 Multipart 上传使用的执行节点本地临时目录；留空继承工具默认。" /></span></template><AInput v-model:value.trim="tmpPath" aria-label="本地临时分块目录" :placeholder="outputPathPlaceholder" autocomplete="off" />
        </AFormItem>
      </AForm>
    </ExportAdvancedSettings>
    <AAlert v-if="draftFailure" type="error" show-icon :message="draftFailure" />
  </section>
</template>
