<script setup lang="ts">
import { computed } from 'vue'
import { Alert, Badge, Button, Spin, Table } from 'ant-design-vue'
import type { DirectLoadStepModel } from '../directLoadStepModels'
import { directLoadCheckItems, directLoadCheckPresentation, directLoadCheckRecovery, directLoadPrecheckOverview } from '../directLoadPrecheckPresentation'
const props = defineProps<{ model: DirectLoadStepModel }>()
const rows = computed(() => directLoadCheckItems.map(item => { const check = props.model.activePrecheck, result = check?.results.find(result => result.check === item.check); return { ...item, result, recovery: directLoadCheckRecovery(check, result, props.model.checkExpired), ...directLoadCheckPresentation(check, result, props.model.checkExpired) } }))
const columns = [{ title: '预检查项', key: 'name', width: 180 }, { title: '状态', key: 'status', width: 150 }, { title: '检查结果', key: 'detail' }, { title: '操作', key: 'action', width: 110 }]
const overview = computed(() => directLoadPrecheckOverview(props.model.activePrecheck, props.model.checkExpired, props.model.checking))
const needsNodeSetup = computed(() => rows.value.some(row => row.recovery === 'NODE_SETUP'))
</script>
<template>
  <section class="form-section direct-load-section">
    <h2>预检查、完整命令与风险确认</h2>
    <Alert v-if="model.issues.length" type="warning" show-icon message="配置尚未完整，不能预检查"><template #description><ul class="direct-load-issues"><li v-for="issue in model.issues" :key="issue.field"><Button type="link" @click="model.moveToStep(issue.step)">{{ model.titles[issue.step - 1] }}：{{ issue.message }}</Button></li></ul></template></Alert>
    <Alert v-if="model.reviewFailure" type="error" show-icon :message="model.reviewFailure" />
    <Alert :type="overview.type" show-icon :message="overview.message">
      <template v-if="needsNodeSetup || !model.currentDraft?.executionEnabled" #description><template v-if="needsNodeSetup">请联系管理员配置旁路检查能力后重新预检查。<code>DIRECT_LOAD_PROBE_NOT_CONFIGURED</code></template><template v-if="!model.currentDraft?.executionEnabled"> 旁路执行许可尚未开启，请联系管理员核对。</template></template>
      <template v-if="needsNodeSetup" #action><Button type="link" @click="model.goToNode">查看执行节点</Button></template>
    </Alert>
    <Table class="wizard-precheck-table" :columns="columns" :data-source="rows" row-key="check" :pagination="false" :scroll="{x: 690}" size="small">
      <template #bodyCell="{column,record}"><strong v-if="column.key === 'name'">{{ record.name }}</strong><Badge v-else-if="column.key === 'status'" :status="record.status" :text="record.text" /><div v-else-if="column.key === 'detail'" class="wizard-precheck-result"><span>{{ record.detail }}</span><small v-if="record.result && record.result.status !== 'PASSED' && record.recovery !== 'NODE_SETUP'">原因码：<code>{{ record.result.evidenceCode }}</code></small></div><Button v-else-if="column.key === 'action' && record.recovery === 'CONFIGURATION' && !model.rerunLocked" type="link" size="small" @click="model.moveToStep(record.step)">检查配置</Button></template>
    </Table>
    <p v-if="model.activePrecheck?.inputManifest" class="section-hint">已核对 {{ model.activePrecheck.inputManifest.fileCount }} 个同表输入文件，共 {{ model.activePrecheck.inputManifest.totalBytes }} 字节；文件变化后必须重新检查。</p>
    <p v-if="model.activePrecheck?.directLoadConfigFingerprint" class="section-hint">节点 Direct Load 安全配置指纹：<code>{{ model.activePrecheck.directLoadConfigFingerprint }}</code>；提交时由服务端冻结并复核。</p>
    <section class="wizard-command" aria-label="完整脱敏命令"><div class="wizard-section-heading"><h3>完整命令（敏感信息已脱敏）</h3><Button :loading="model.copying" :disabled="!model.commandPreview" @click="model.copyCommand">复制脱敏命令</Button></div><Spin v-if="model.previewing" tip="正在生成命令预览…"><span class="wizard-loading-space" /></Spin><pre v-else-if="model.commandPreview"><code>{{ model.commandPreview.redactedCommand }}</code></pre><p v-else>配置完整并保存后，由控制面唯一命令生成器准备预览。</p><p>命令仅供核对；密码已脱敏，不能直接作为可执行命令使用。</p><Alert v-if="model.copyFailure" type="error" show-icon :message="model.copyFailure" /></section>
    <Alert type="warning" show-icon message="单表整体提交；失败只能从头执行" description="旁路不提供 --retry、检查点继续或普通导入错误阈值。取消在服务端提交阶段的安全语义未验证，当前不承诺安全取消。" />
  </section>
</template>
