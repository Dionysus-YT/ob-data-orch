import type { RouteLocationRaw } from 'vue-router'
import type { OverviewAttentionItem, OverviewBucket, OverviewMetrics, OverviewRange, OverviewSnapshot } from '@/api/overview'

/** 首页统一按快照声明的 UTC 展示；不使用浏览器本地时区重解释统计窗口。 */
export function overviewTime(value?: string | null): string {
  if (!value || !Number.isFinite(Date.parse(value))) return '尚未取得'
  return new Date(value).toISOString().slice(0, 19).replace('T', ' ')
}

export function overviewTasksLink(state?: string, from?: string, to?: string): RouteLocationRaw {
  return { name: 'tasks', query: { ...(state ? { state } : {}), ...(from ? { finishedFrom: from } : {}), ...(to ? { finishedTo: to } : {}) } }
}

export function overviewMetricLink(key: keyof OverviewMetrics, snapshot: OverviewSnapshot): RouteLocationRaw {
  if (key === 'running') return overviewTasksLink('RUNNING')
  if (key === 'waiting') return overviewTasksLink('WAITING_SCHEDULE')
  return overviewTasksLink(key === 'todaySucceeded' ? 'SUCCEEDED' : 'FAILED', snapshot.todayFrom, snapshot.todayTo)
}

export function overviewAttentionLink(item: OverviewAttentionItem): RouteLocationRaw {
  return item.kind === 'TASK_FAILED' ? { name: 'task-detail', params: { id: item.objectId } } : { name: 'node-detail', params: { id: item.objectId } }
}

export const overviewMetricLabels: Readonly<Record<keyof OverviewMetrics, string>> = {
  running: '运行中任务', waiting: '等待调度任务', todaySucceeded: '今日成功任务', todayFailed: '今日失败任务',
}

export const overviewRangeLabels: Readonly<Record<OverviewRange, string>> = { '24h': '最近 24 小时', '7d': '最近 7 天' }

/** 趋势分桶的可读区间；24 小时含首尾部分小时桶，因此按真实起止展示，不补齐整点。 */
export function overviewBucketLabel(bucket: OverviewBucket, range: OverviewRange): string {
  const from = overviewTime(bucket.from), to = overviewTime(bucket.to)
  return range === '24h' ? `${from.slice(5, 16)} – ${to.slice(11, 16)}` : `${from.slice(5, 10)}${to.slice(11) === '00:00:00' ? '' : ` 至 ${to.slice(11, 16)}`}`
}
