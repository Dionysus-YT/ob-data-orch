<script setup lang="ts">
import { Alert as AAlert, Badge as ABadge, Button as AButton, Empty as AEmpty, Tag as ATag } from 'ant-design-vue'
import { useRouter } from 'vue-router'
import type { OverviewAttention, OverviewAttentionItem, OverviewNodes, OverviewSection } from '@/api/overview'
import { nodeUnavailableReasonLabel } from '@/workbench/nodes/executionNodePresentation'
import { overviewAttentionLink, overviewTime } from './overviewPresentation'

const props = defineProps<{ section: OverviewSection<OverviewAttention>; nodes: OverviewSection<OverviewNodes>; summary: string; busy: boolean }>()
const emit = defineEmits<{ retry: [] }>()
const router = useRouter()
// 节点异常只从同一快照的可见节点事实补充原因；找不到对应节点时不补写或猜测原因。
function nodeReason(item: OverviewAttentionItem): string {
  if (item.kind !== 'NODE_UNAVAILABLE' || props.nodes.status !== 'READY') return ''
  return props.nodes.data.items.find(node => node.id === item.objectId)?.unavailableReasons.map(reason => nodeUnavailableReasonLabel(reason)).join('；') ?? ''
}
</script>

<template>
  <section class="content-card overview-section" aria-labelledby="overview-attention-title">
    <div class="overview-section-heading">
      <div><h2 id="overview-attention-title">需要关注</h2><p>最近 24 小时任务失败与节点当前接收状态</p></div>
    </div>
    <p class="overview-attention-summary">{{ summary }}</p>
    <template v-if="section.status === 'READY'">
      <ul v-if="section.data.items.length" class="overview-attention-list">
        <li v-for="item in section.data.items" :key="`${item.kind}:${item.objectId}`">
          <span class="overview-attention-kind"><ABadge :status="item.kind === 'TASK_FAILED' ? 'error' : 'warning'" :text="item.kind === 'TASK_FAILED' ? '任务失败' : '节点不可接收任务'" /></span>
          <div class="overview-attention-object">
            <AButton type="link" class="overview-object-link" :title="item.displayName" @click="router.push(overviewAttentionLink(item))">{{ item.displayName }}</AButton><ATag v-if="item.reconciliationRequired" color="warning">需要核对</ATag>
            <p class="overview-meta">{{ item.source === 'TASK_STATE' ? '任务终态事实' : '节点当前事实' }}<template v-if="nodeReason(item)"> · {{ nodeReason(item) }}</template></p>
          </div>
          <span class="overview-meta overview-attention-time">{{ item.kind === 'TASK_FAILED' ? '终态时间' : '本次核对时间' }} {{ overviewTime(item.occurredAt) }} UTC</span>
        </li>
      </ul>
      <AEmpty v-else :image="AEmpty.PRESENTED_IMAGE_SIMPLE" description="当前可见事实中未发现最近任务失败或节点接收异常" />
      <p class="overview-meta overview-coverage">{{ section.data.coverage === 'TASK_FACTS_ONLY' ? '节点事实未纳入当前可见范围；' : '' }}仅覆盖已记录的任务状态与可见节点事实，独立调度和工具日志检索尚未接入。可进入对象详情核对已有日志。</p>
    </template>
    <AEmpty v-else-if="section.status === 'RESTRICTED'" :image="AEmpty.PRESENTED_IMAGE_SIMPLE" description="当前没有可查看的异常事实，请核对任务授权范围。" />
    <AAlert v-else type="warning" show-icon message="最近异常事实暂不可用，不能据此认定没有异常。"><template #action><AButton :disabled="busy" @click="emit('retry')">重试异常事实</AButton></template></AAlert>
  </section>
</template>

<style scoped>
.overview-attention-summary { margin: calc(-1 * var(--ob-foundation-space-2)) 0 var(--ob-foundation-space-4); color: var(--ob-color-text); overflow-wrap: anywhere; }
.overview-attention-list { margin: 0; padding: 0; list-style: none; }
.overview-attention-list > li { display: grid; grid-template-columns: minmax(9rem, auto) minmax(0, 1fr) auto; gap: var(--ob-foundation-space-2) var(--ob-foundation-space-4); align-items: start; padding-block: var(--ob-foundation-space-3); border-block-start: 1px solid var(--ob-color-border); }
.overview-attention-list > li:first-child { padding-block-start: 0; border-block-start: 0; }
.overview-attention-object { min-inline-size: 0; }
.overview-attention-time { margin: 0; white-space: nowrap; }
.overview-coverage { margin-top: var(--ob-foundation-space-3); }
@container overview (max-width: 760px) {
  .overview-attention-list > li { grid-template-columns: minmax(0, 1fr); }
  .overview-attention-time { white-space: normal; }
}
</style>
