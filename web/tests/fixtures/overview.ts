import type { OverviewBucket, OverviewRange, OverviewSnapshot } from '../../src/api/overview'

/** 只用于隔离页面和单元测试的合成事实，不进入正式页面或真实 API。 */
export function overviewFixture(at = new Date(), range: OverviewRange = '24h'): OverviewSnapshot {
  const now = at.toISOString(), day = new Date(`${now.slice(0, 10)}T00:00:00Z`)
  const from = range === '24h' ? new Date(at.getTime() - 86400000) : new Date(day.getTime() - 6 * 86400000)
  const buckets: OverviewBucket[] = []
  let cursor = from.getTime()
  while (cursor < at.getTime()) {
    const step = range === '24h' ? 3600000 : 86400000
    const end = Math.min(Math.floor(cursor / step) * step + step, at.getTime())
    const failed = buckets.length % 5 === 0 ? 1 : 0, succeeded = buckets.length % 4
    buckets.push({ from: new Date(cursor).toISOString(), to: new Date(end).toISOString(), total: failed + succeeded + 1, succeeded, failed })
    cursor = end
  }
  const task = (id: string, type: 'OBDUMPER_EXPORT' | 'OBLOADER_NORMAL_IMPORT' | 'OBLOADER_DIRECT_LOAD', state: 'RUNNING' | 'FAILED' | 'WAITING_SCHEDULE', percent?: number) => ({
    id, displayName: id === 'synthetic-failed' ? '合成失败任务 · 财务明细' : id === 'synthetic-running' ? '合成导入 · 等待收尾与结果核验' : '合成等待任务', type, state,
    reconciliationRequired: state === 'FAILED', stageEvidence: percent === undefined ? 'UNAVAILABLE' as const : 'RELIABLE' as const,
    progressEvidence: percent === undefined ? 'UNAVAILABLE' as const : 'PERCENT' as const,
    ...(percent === undefined ? {} : { progress: { phase: 'IMPORT_DATA' as const, percent, completedTasks: 2, runningTasks: 0, updatedAt: now, incomplete: false } }), updatedAt: now,
  })
  return {
    snapshotAt: now, timeZone: 'UTC', timeZoneSource: 'BUILTIN_UTC', range, todayFrom: day.toISOString(), todayTo: new Date(day.getTime() + 86400000).toISOString(),
    refreshIntervalSeconds: 30, staleAfterSeconds: 90, summary: '当前授权范围内今日失败 2 个、运行中 2 个、等待调度 1 个任务；1 个执行节点当前不能接收新任务。',
    metrics: { status: 'READY', data: { running: 2, waiting: 1, todaySucceeded: 8, todayFailed: 2 } }, trend: { status: 'READY', data: buckets },
    nodes: { status: 'READY', data: { total: 2, unavailable: 1, items: [
      { id: 'synthetic-offline', displayName: '合成离线执行节点', managementState: 'ENABLED', heartbeatStatus: 'OFFLINE', environmentStatus: 'NORMAL', capacityStatus: 'AVAILABLE', acceptsNewTasks: false,
        unavailableReasons: ['HEARTBEAT_OFFLINE'], lastHeartbeatAt: new Date(at.getTime() - 120000).toISOString(), observedAt: new Date(at.getTime() - 120000).toISOString(), capacityTotal: 2, capacityUsed: 0, cpuUsagePercent: 12, memoryUsagePercent: 36, updatedAt: now },
      { id: 'synthetic-ready', displayName: '合成正常执行节点', managementState: 'ENABLED', heartbeatStatus: 'ONLINE', environmentStatus: 'NORMAL', capacityStatus: 'AVAILABLE', acceptsNewTasks: true,
        unavailableReasons: [], lastHeartbeatAt: now, observedAt: now, capacityTotal: 3, capacityUsed: 1, cpuUsagePercent: 8, memoryUsagePercent: 27, updatedAt: now },
    ] } },
    myTasks: { status: 'READY', data: [task('synthetic-failed', 'OBDUMPER_EXPORT', 'FAILED'), task('synthetic-running', 'OBLOADER_NORMAL_IMPORT', 'RUNNING', 100), task('synthetic-waiting', 'OBLOADER_DIRECT_LOAD', 'WAITING_SCHEDULE')] },
    attention: { status: 'READY', data: { from: new Date(at.getTime() - 86400000).toISOString(), to: now, coverage: 'TASK_AND_NODE_FACTS', items: [
      { kind: 'TASK_FAILED', objectId: 'synthetic-failed', displayName: '合成失败任务 · 财务明细', occurredAt: now, source: 'TASK_STATE', reconciliationRequired: true },
      { kind: 'NODE_UNAVAILABLE', objectId: 'synthetic-offline', displayName: '合成离线执行节点', occurredAt: now, source: 'NODE_STATE', reconciliationRequired: false },
    ] } },
  }
}
