<script setup lang="ts">
import { computed, toRefs } from 'vue'
import { Alert, Button, Form, FormItem, Input, InputNumber, Radio, RadioGroup, Select, Skeleton } from 'ant-design-vue'
import EmptyState from '@/components/EmptyState.vue'
import { environmentLabel, formatVerifiedTime } from '@/workbench/sources/sourcePresentation'
import { directLoadDefaultRpcPort, directLoadScenarioForSource, directLoadScenarioLabels } from '../directLoadRules'
import type { DirectLoadConfig } from '@/api/directLoad'
import type { DirectLoadStepModel } from '../directLoadStepModels'

const props = defineProps<{ model: DirectLoadStepModel; fieldPrefix: string }>()
const { form, sourceKeyword, sourceEnvironment } = toRefs(props.model)
const environments = [{ value: '', label: '全部环境' }, ...['DEVELOPMENT', 'TEST', 'STAGING', 'PRODUCTION'].map(value => ({ value, label: environmentLabel(value) }))]
const sourceScenario = computed(() => directLoadScenarioForSource(props.model.selectedSource))
const rpcHint = computed(() => {
  const port = directLoadDefaultRpcPort(props.model.form.scenario)
  return port ? `当前场景默认 ${port}，可按实际监听端口或负载均衡映射修改；默认值不代表检查通过。` : '选择实际连接场景后预填默认端口，可按环境修改；SQL 与 RPC 端口分别检查。'
})
</script>

<template>
  <section class="form-section direct-load-section">
    <div class="direct-load-field-group">
      <h3>筛选数据源</h3>
      <Form layout="vertical" class="wizard-source-filter" :disabled="model.rerunLocked">
        <FormItem label="按名称筛选数据源"><Input v-model:value="sourceKeyword" aria-label="按名称筛选数据源" allow-clear placeholder="输入数据源名称" /></FormItem>
        <FormItem label="环境" :html-for="fieldPrefix + '-environment'"><Select :id="fieldPrefix + '-environment'" v-model:value="sourceEnvironment" :options="environments" /></FormItem>
      </Form>
    </div>
    <div class="direct-load-field-group">
      <div class="wizard-section-heading"><h3>可用数据源</h3><Button :loading="model.loadingSources" @click="model.loadSources">刷新数据源</Button></div>
      <Alert v-if="model.sourceFailure" type="error" show-icon :message="model.sourceFailure"><template #action><Button type="link" @click="model.loadSources">重试</Button></template></Alert>
      <Skeleton v-else-if="model.loadingSources" active :paragraph="{ rows: 3 }" aria-label="正在读取数据源" />
      <EmptyState v-else-if="!model.eligibleSources.length" title="没有可选数据源" :description="model.sources.length ? '当前已授权数据源尚未同时满足旁路连接类型、已启用和基础连接测试成功条件。' : '当前授权范围内没有数据源。请在数据源管理中登记并完成基础连接测试。'" action="前往数据源管理" @action="model.goToSources" />
      <EmptyState v-else-if="!model.visibleSources.length" title="当前筛选无可选数据源" description="请清除名称或环境筛选后重试。" />
      <Form v-else layout="vertical" :disabled="model.rerunLocked">
        <FormItem label="目标数据源" required :validate-status="model.fieldError('dataSourceId') ? 'error' : undefined" :help="model.fieldError('dataSourceId')">
          <RadioGroup :value="model.form.dataSourceId" class="wizard-source-list" role="radiogroup" aria-label="目标数据源" @update:value="model.selectSource">
            <Radio v-for="source in model.visibleSources" :key="source.id" :value="source.id" class="wizard-source-choice">
              <span class="wizard-source-facts"><strong>{{ source.displayName }}</strong><span>{{ environmentLabel(source.environment) }} · {{ source.compatibilityMode }} · {{ source.host }}:{{ source.port }}</span><span>{{ source.connectionKind === 'OBSERVER_DIRECT' ? '直连 OBServer' : `集群：${source.clusterName || '未登记'}` }} / 租户：{{ source.tenantName || '未登记' }} · 基础连接测试成功：{{ formatVerifiedTime(source.lastTestedAt) }}</span></span>
            </Radio>
          </RadioGroup>
        </FormItem>
      </Form>
    </div>
    <Alert type="info" show-icon message="旁路适用于大批量单表装载，按表整体提交，失败需从头执行；首次继续时确认适用边界。" />
    <div v-if="model.selectedSource" class="direct-load-field-group">
      <h3>旁路连接</h3>
      <Form layout="vertical" class="direct-load-fields" :disabled="model.rerunLocked">
        <FormItem label="连接场景" :html-for="fieldPrefix + '-scenario'" required :validate-status="model.fieldError('scenario') ? 'error' : undefined" :help="model.fieldError('scenario') || '由数据源连接方式带出。'"><Select :id="fieldPrefix + '-scenario'" :value="model.form.scenario === sourceScenario ? model.form.scenario : undefined" :disabled="model.form.scenario === sourceScenario" placeholder="应用数据源连接方式" :options="Object.entries(directLoadScenarioLabels).filter(([value]) => value === sourceScenario).map(([value,label]) => ({value,label}))" @change="value => model.selectScenario(value as DirectLoadConfig['scenario'])" /></FormItem>
        <FormItem label="RPC 端口" :html-for="fieldPrefix + '-rpc'" required :validate-status="model.fieldError('rpcPort') ? 'error' : undefined" :help="model.fieldError('rpcPort') || rpcHint"><InputNumber :id="fieldPrefix + '-rpc'" aria-label="RPC 端口" :value="form.rpcPort || undefined" :min="1" :max="65535" :precision="0" placeholder="选择场景后预填，可修改" @update:value="value => form.rpcPort = Number(value || 0)" /></FormItem>
      </Form>
      <Alert type="info" show-icon message="SQL、RPC 和版本组合待检查" description="SQL 地址与端口只读复用所选数据源；第 5 步从执行节点检查 SQL/RPC、工具及数据库版本。连接测试成功或默认端口均不表示旁路可用。" />
      <Alert type="info" show-icon message="大批量导入建议使用固定 ODP 地址" description="使用 F5 / VIP 时，所选数据源地址须同时提供 SQL 与 RPC 转发；只开放 SQL 端口无法完成旁路导入。请核对 RPC 映射、后端连接与带宽，实际链路仍需验证。" />
      <Alert v-if="model.form.scenario === 'OBSERVER_DIRECT'" type="warning" show-icon message="直连 OBServer 可能产生跨机事务，提交前必须重新确认。" />
      <Alert v-if="model.selectedSource.environment === 'PRODUCTION'" type="warning" show-icon message="已选择生产环境；实际提交前仍须完成任务检查与独立执行确认。" />
    </div>
  </section>
</template>
