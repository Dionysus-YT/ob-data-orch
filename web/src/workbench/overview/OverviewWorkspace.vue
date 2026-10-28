<script setup lang="ts">
import { CheckCircleOutlined, ReloadOutlined } from '@ant-design/icons-vue'
import { Alert as AAlert, Badge as ABadge, Button as AButton, Skeleton as ASkeleton } from 'ant-design-vue'
import { computed } from 'vue'
import OverviewAttention from './OverviewAttention.vue'
import OverviewMyTasks from './OverviewMyTasks.vue'
import OverviewNodes from './OverviewNodes.vue'
import OverviewMetrics from './OverviewMetrics.vue'
import OverviewTrend from './OverviewTrend.vue'
import { overviewRangeLabels, overviewTime } from './overviewPresentation'
import { useOverview } from './useOverview'
import './overview.css'

const { range, snapshot, loading, refreshing, refreshCompleted, restricted, failure, stale, partial, currentTime, refresh, invalidateAuthorization } = useOverview()
// 读取期间所有重试入口一并禁用；方法入口仍由 useOverview 防重。
const busy = computed(() => loading.value || refreshing.value)
const refreshLabel = computed(() => busy.value ? '刷新中' : refreshCompleted.value ? '已刷新' : '刷新')
</script>

<template>
  <div class="orch-ui overview-workspace" :aria-busy="busy">
    <header class="orch-page-header overview-header">
      <div class="orch-page-title"><h1>首页</h1></div>
      <div class="overview-header-actions">
        <div class="overview-freshness" aria-label="快照范围与时间" role="status">
          <span>当前授权范围</span><span>UTC</span>
          <span v-if="snapshot">快照时间 {{ overviewTime(snapshot.snapshotAt) }}</span><span v-else>尚未取得可信快照</span>
          <ABadge v-if="stale" status="warning" text="数据已过期" /><ABadge v-else-if="refreshing" status="processing" text="更新中" />
          <span v-if="!restricted">每 30 秒自动刷新</span>
        </div>
        <AButton type="text" :loading="busy" :disabled="busy" @click="refresh"><template #icon><CheckCircleOutlined v-if="refreshCompleted" aria-hidden="true" /><ReloadOutlined v-else aria-hidden="true" /></template>{{ refreshLabel }}</AButton>
      </div>
    </header>

    <div v-if="failure || stale || partial || (snapshot && snapshot.range !== range)" class="overview-feedback">
      <AAlert v-if="failure" :type="restricted ? 'warning' : 'error'" show-icon :message="failure" :description="snapshot ? `保留最后可信快照（${overviewTime(snapshot.snapshotAt)} UTC），当前情况需要刷新核对。` : undefined">
        <template #action><AButton :disabled="busy" @click="refresh">{{ restricted ? '刷新权限' : '重试' }}</AButton></template>
      </AAlert>
      <AAlert v-else-if="stale" type="warning" show-icon message="快照已过期，以下为最后可信记录。" description="任务与节点当前情况需要刷新核对；旧值不会被当作实时正常状态。"><template #action><AButton :disabled="busy" @click="refresh">刷新快照</AButton></template></AAlert>
      <AAlert v-else-if="partial" type="warning" show-icon message="快照数据不完整" description="部分分区读取失败，已单独标记；可用分区来自同一份快照。" />
      <AAlert v-if="snapshot && snapshot.range !== range" type="info" show-icon :message="`所选时间范围尚未取得，当前仍显示${overviewRangeLabels[snapshot.range]}的完整快照。`" />
    </div>

    <ASkeleton v-if="loading && !snapshot" class="content-card overview-section" active :paragraph="{ rows: 8 }" title aria-label="正在加载首页快照" />
    <template v-if="snapshot && !restricted">
      <!-- OCP 参考布局：四项指标 → 趋势 / 关注并列 → 任务与节点通栏；窄屏按 DOM 顺序阅读。 -->
      <OverviewMetrics :snapshot="snapshot" :busy="busy" @retry="refresh" />
      <div class="overview-analysis">
        <OverviewTrend v-model:range="range" :section="snapshot.trend" :snapshot-range="snapshot.range" :snapshot-at="snapshot.snapshotAt" :busy="busy" @retry="refresh" @authorization-lost="invalidateAuthorization()" />
        <OverviewAttention :section="snapshot.attention" :nodes="snapshot.nodes" :summary="snapshot.summary" :busy="busy" @retry="refresh" />
      </div>
      <OverviewMyTasks :section="snapshot.myTasks" :busy="busy" @retry="refresh" />
      <OverviewNodes :section="snapshot.nodes" :busy="busy" :current-time="currentTime" @retry="refresh" />
    </template>
  </div>
</template>
