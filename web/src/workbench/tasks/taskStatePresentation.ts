import dayjs from 'dayjs'
import type { TaskEnvironment } from '@/api/browser'

// 八种任务状态的唯一文案来源；列表与详情共用，避免同一状态出现两种说法。
const taskStateLabels: Readonly<Record<string, string>> = {
  DRAFT: '草稿',
  WAITING_SCHEDULE: '等待调度',
  STARTING: '启动中',
  RUNNING: '运行中',
  CANCELLING: '取消中',
  SUCCEEDED: '成功',
  FAILED: '失败',
  CANCELLED: '已取消',
}

/** 未知状态不伪装为任何业务状态，统一显示状态核对中。 */
export function taskStateText(state?: string): string {
  return isKnownTaskState(state) ? taskStateLabels[state ?? '']! : '状态核对中'
}

// 只接受白名单自有属性，不能把对象原型成员当作业务状态。
export function isKnownTaskState(state?: string): boolean {
  return state !== undefined && Object.hasOwn(taskStateLabels, state)
}

/** 非终态任务需要持续核对，供自动刷新判断。 */
export function isActiveTaskState(state?: string): boolean {
  return state === 'WAITING_SCHEDULE' || state === 'STARTING' || state === 'RUNNING' || state === 'CANCELLING'
}

/** Ant Badge 状态只作补充，文字始终表达状态本身。 */
export function taskStateBadge(state?: string, reconciliationRequired = false): 'success' | 'error' | 'processing' | 'warning' | 'default' {
  if (reconciliationRequired || !isKnownTaskState(state)) return 'warning'
  if (state === 'SUCCEEDED') return 'success'
  if (state === 'FAILED') return 'error'
  if (state === 'STARTING' || state === 'RUNNING' || state === 'CANCELLING') return 'processing'
  return 'default'
}

export function taskEnvironmentLabel(value?: TaskEnvironment): string {
  return value ? { DEVELOPMENT: '开发', TEST: '测试', STAGING: '预生产', PRODUCTION: '生产' }[value] : ''
}

export function taskTypeText(type: string): string {
  return { OBDUMPER_EXPORT: '导出', OBLOADER_NORMAL_IMPORT: '普通导入', OBLOADER_DIRECT_LOAD: '旁路导入' }[type] ?? '未知类型'
}

/** 耗时按开始/结束或当前时间计算；未开始或时间无效时返回空串，不显示伪造值。 */
export function taskDurationText(startedAt?: string, finishedAt?: string, now = Date.now()): string {
  if (!startedAt) return ''
  const start = Date.parse(startedAt)
  const end = finishedAt ? Date.parse(finishedAt) : now
  if (Number.isNaN(start) || Number.isNaN(end) || end < start) return ''
  const seconds = Math.floor((end - start) / 1000)
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const rest = seconds % 60
  if (hours) return `${hours} 小时 ${minutes} 分`
  if (minutes) return `${minutes} 分 ${rest} 秒`
  return `${rest} 秒`
}

/** 任务时间使用浏览器本地时区与固定 24 小时格式，不受本地化分隔符和补零规则影响。 */
export function taskDateTimeText(value?: string): string {
  if (!value) return '—'
  const parsed = new Date(value)
  return Number.isNaN(parsed.valueOf()) ? '—' : dayjs(parsed).format('YYYY-MM-DD HH:mm:ss')
}
