<script setup lang="ts">
import { Badge as ABadge } from 'ant-design-vue'
import { computed } from 'vue'
import type { ExecutionNodeDetail, InstalledRuntimeVersion, NodeInspectionItem } from '@/api/browser'
import { byteLabel, nodePlatformLabel, nodeTimeLabel } from './executionNodePresentation'
import { nodeAgentPlatformLabel } from './nodePlatformRules'

// 配置页只展示直观环境、已配置目录与日志摘要；标识、协议、哈希及检查绑定不进入页面。
const props = defineProps<{ node: ExecutionNodeDetail }>()
const host = computed(() => props.node.agentDiagnostics?.host)
const queue = computed(() => props.node.agentDiagnostics?.logQueue)
// 仅匹配当前登记目录的容量，不混入旧采样目录，也不累加可能属于同一磁盘的目录。
const diskCapacities = computed(() => props.node.allowedRoots.map(root => ({
  root, totalBytes: props.node.dataRootUsages?.find(usage => usage.root === root)?.totalBytes,
})))
const reportedPlatformDifference = computed(() => {
  const facts = props.node.agentFacts
  if (!facts) return undefined
  const reported = nodeAgentPlatformLabel(facts.os, facts.arch)
  return reported === nodePlatformLabel(props.node.platform) ? undefined : reported
})
const uploadLabels = { UNKNOWN: '尚无上传结果', SUCCEEDED: '上传已确认', FAILED: '上传失败，等待重试' } as const

function versionLabel(kind: NodeInspectionItem['kind'], installed: InstalledRuntimeVersion | undefined): string {
  const report = props.node.environmentCheck?.inspection
  const actual = report?.items.find(item => item.kind === kind)?.version
  if (actual) return actual + (report?.checkId !== props.node.environmentCheck?.checkId ? '（上次检查）' : '')
  return installed ? installed.version + '（待检查）' : '尚未取得版本'
}
function budgetLabel(value: number | undefined): string {
  return value ? (value / 1024 ** 3).toLocaleString('zh-CN', { maximumFractionDigits: 2 }) + ' GiB' : '自动'
}
function memoryLabel(value: string | undefined): string {
  const match = /^([1-9]\d*)([GM])$/i.exec(value ?? '')
  return match ? match[1] + (match[2]?.toUpperCase() === 'G' ? ' GiB' : ' MiB') : value ?? '尚未取得'
}
function capacityLabel(totalBytes: number | undefined): string {
  if (totalBytes === undefined || !Number.isSafeInteger(totalBytes) || totalBytes <= 0) return '尚未取得'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']
  const index = Math.min(Math.floor(Math.log2(totalBytes) / 10), units.length - 1)
  return `${(totalBytes / 1024 ** index).toFixed(index ? 1 : 0)} ${units[index]}`
}
</script>

<template>
  <div class="node-environment-configuration">
    <section aria-labelledby="node-host-facts-title">
      <h2 id="node-host-facts-title">执行环境</h2>
      <dl class="node-definition">
        <div><dt>主机名</dt><dd>{{ host ? host.hostname ?? '尚未取得' : 'Agent 尚未上报' }}</dd></div>
        <div><dt>操作系统</dt><dd>{{ host?.osName ?? '尚未取得' }}<span v-if="host?.osVersion"> · {{ host.osVersion }}</span></dd></div>
        <div><dt>处理器型号</dt><dd>{{ host?.cpuModel ?? '尚未取得' }}</dd></div>
        <div><dt>CPU 物理核</dt><dd>{{ host?.physicalCores !== undefined ? host.physicalCores + ' 核' : '尚未取得' }}</dd></div>
        <div><dt>CPU 逻辑核</dt><dd>{{ host?.logicalCpus !== undefined ? host.logicalCpus + ' 核' : '尚未取得' }}</dd></div>
        <div><dt>内存大小</dt><dd>{{ capacityLabel(node.agentFacts?.memoryTotalBytes) }}</dd></div>
        <div>
          <dt>磁盘大小</dt>
          <dd>
            <ul v-if="diskCapacities.length" class="node-path-list" aria-label="磁盘大小">
              <li v-for="disk in diskCapacities" :key="disk.root">{{ disk.root }} · {{ capacityLabel(disk.totalBytes) }}</li>
            </ul>
            <span v-else>尚未取得</span>
          </dd>
        </div>
        <div v-if="reportedPlatformDifference"><dt>实际上报平台</dt><dd>{{ reportedPlatformDifference }}（与登记不同）</dd></div>
        <div><dt>Agent 版本</dt><dd>{{ node.agentFacts?.agentVersion ?? '尚未取得' }}</dd></div>
        <div><dt>启动时间</dt><dd>{{ node.agentDiagnostics?.startedAt ? nodeTimeLabel(node.agentDiagnostics.startedAt) : '尚未取得' }}</dd></div>
      </dl>
    </section>
    <section aria-labelledby="node-config-title">
      <h2 id="node-config-title">工具与内存配置</h2>
      <dl class="node-definition">
        <div><dt>Java 版本</dt><dd>{{ versionLabel('JAVA_RUNTIME', node.agentDiagnostics?.runtimeVersions?.java) }}</dd></div>
        <div><dt>导入工具版本</dt><dd>{{ versionLabel('OBLOADER_RUNTIME', node.agentDiagnostics?.runtimeVersions?.obloader) }}</dd></div>
        <div><dt>导出工具版本</dt><dd>{{ versionLabel('OBDUMPER_RUNTIME', node.agentDiagnostics?.runtimeVersions?.obdumper) }}</dd></div>
        <div><dt>工具安装目录</dt><dd>{{ node.toolHome }}</dd></div>
        <div><dt>Java 位置</dt><dd>{{ node.javaPath }}</dd></div>
        <div>
          <dt>任务数据目录</dt>
          <dd>
            <ul v-if="node.allowedRoots.length" class="node-path-list" aria-label="任务数据目录">
              <li v-for="root in node.allowedRoots" :key="root">{{ root }}</li>
            </ul>
            <span v-else>尚未配置</span>
          </dd>
        </div>
        <div><dt>默认任务内存</dt><dd>{{ memoryLabel(node.defaultJvmMemory) }}</dd></div>
        <div><dt>系统预留内存</dt><dd>{{ node.budgetPolicy ? budgetLabel(node.budgetPolicy.systemReservedBytes) : '尚未取得' }}</dd></div>
        <div><dt>每任务额外预留</dt><dd>{{ node.budgetPolicy ? budgetLabel(node.budgetPolicy.taskOverheadBytes) : '尚未取得' }}</dd></div>
      </dl>
    </section>
    <section class="node-log-queue-section" aria-labelledby="node-log-queue-title">
      <h2 id="node-log-queue-title">日志上传队列</h2>
      <dl v-if="queue" class="node-definition node-log-definition">
        <div><dt>最近上传结果</dt><dd><ABadge :status="queue.lastUploadResult === 'FAILED' ? 'error' : queue.lastUploadResult === 'SUCCEEDED' ? 'success' : 'default'" :text="uploadLabels[queue.lastUploadResult]" /></dd></div>
        <div><dt>待确认日志</dt><dd>{{ queue.pendingBatches }} 份</dd></div>
        <div><dt>缓存占用</dt><dd>{{ byteLabel(queue.pendingBytes) }}</dd></div>
        <div><dt>最近上传时间</dt><dd>{{ queue.lastUploadAt ? nodeTimeLabel(queue.lastUploadAt) : '尚无结果' }}</dd></div>
      </dl>
      <p v-else class="node-environment-note">Agent 尚未提供日志上传信息。</p>
      <p v-if="queue && queue.pendingGaps > 0" class="node-environment-note">有 {{ queue.pendingGaps }} 处日志缺口等待上传确认。</p>
    </section>
  </div>
</template>

<style scoped>
.node-environment-configuration { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--ob-foundation-space-6); padding-top: var(--ob-foundation-space-4); }
.node-environment-configuration section { min-width: 0; }
.node-environment-configuration h2 { margin: 0 0 var(--ob-foundation-space-3); font-size: var(--ob-product-typography-section-size); }
.node-definition { margin: 0; }
.node-definition > div { display: grid; grid-template-columns: minmax(100px, 36%) minmax(0, 1fr); gap: var(--ob-foundation-space-3); padding: var(--ob-foundation-space-2) 0; border-bottom: 1px solid var(--ob-color-border); }
.node-definition dt { color: var(--ob-color-secondary); font-size: var(--ob-component-field-label-size); }
.node-definition dd { min-width: 0; margin: 0; color: var(--ob-color-text); font-size: var(--ob-component-field-label-size); overflow-wrap: anywhere; }
.node-log-queue-section { grid-column: 1 / -1; }
.node-path-list { margin: 0; padding: 0; list-style: none; }
.node-path-list li + li { margin-top: var(--ob-foundation-space-1); }
.node-log-definition { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); column-gap: var(--ob-foundation-space-6); }
.node-environment-note { margin: var(--ob-foundation-space-2) 0; color: var(--ob-color-secondary); font-size: var(--ob-component-field-helper-size); line-height: 1.6; }
@media (max-width: 900px) { .node-environment-configuration, .node-log-definition { grid-template-columns: 1fr; } }
@media (max-width: 480px) { .node-definition > div { grid-template-columns: 1fr; gap: var(--ob-foundation-space-1); } }
</style>
