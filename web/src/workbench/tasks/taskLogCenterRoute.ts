import type { RouteLocationRaw } from 'vue-router'

/** 只携带当前有权任务和时间范围；日志中心仍独立复验权限，不携带正文或内部游标。 */
export function taskLogCenterRoute(taskId: string, submittedAt: string, now = Date.now()): RouteLocationRaw {
  const submitted = Date.parse(submittedAt)
  return { name: 'logs', query: { taskId, ...(Number.isFinite(submitted) ? { from: new Date(submitted - 60000).toISOString(), to: new Date(Math.max(submitted, now) + 1000).toISOString() } : {}) } }
}
