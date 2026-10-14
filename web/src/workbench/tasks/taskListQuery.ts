import type { LocationQuery, LocationQueryRaw } from 'vue-router'
import { taskListPageSizes, taskListStates, type TaskEnvironment, type TaskListFilter, type TaskListPageSize, type TaskListState } from '@/api/browser'

// 任务中心筛选的唯一路由表示。路由 query 是筛选与条数的唯一来源，组件不持有第二份副本。
export interface TaskListRouteState {
  readonly keyword: string
  readonly types: readonly 'OBDUMPER_EXPORT'[]
  readonly states: readonly TaskListState[]
  // 日期为浏览器本地日期 YYYY-MM-DD，提交时换算为半开区间的 RFC 3339 时刻。
  readonly createdFrom: string
  readonly createdTo: string
  readonly startedFrom: string
  readonly startedTo: string
  readonly dataSourceId: string
  readonly environment: '' | TaskEnvironment
  readonly nodeId: string
  readonly creatorId: string
  readonly scope: '' | 'MINE' | 'ALL'
  readonly limit: TaskListPageSize
}

export const emptyTaskListRouteState: TaskListRouteState = {
  keyword: '', types: [], states: [], createdFrom: '', createdTo: '', startedFrom: '', startedTo: '',
  dataSourceId: '', environment: '', nodeId: '', creatorId: '', scope: '', limit: 10,
}

const environments: readonly TaskEnvironment[] = ['DEVELOPMENT', 'TEST', 'STAGING', 'PRODUCTION']
const datePattern = /^\d{4}-\d{2}-\d{2}$/

function text(query: LocationQuery, key: string, maxLength = 256): string {
  const value = query[key]
  return typeof value === 'string' && value.length <= maxLength ? value : ''
}

function validDate(value: string): string {
  return datePattern.test(value) && !Number.isNaN(new Date(`${value}T00:00:00`).valueOf()) ? value : ''
}

/** 解析路由 query；无效或未知值直接丢弃，避免把旧链接中的异常条件传给服务端。 */
export function parseTaskListRoute(query: LocationQuery): TaskListRouteState {
  const list = (key: string) => text(query, key).split(',').map(value => value.trim()).filter(Boolean)
  const states = [...new Set(list('state'))].filter((value): value is TaskListState => (taskListStates as readonly string[]).includes(value))
  const types = list('type').includes('OBDUMPER_EXPORT') ? ['OBDUMPER_EXPORT' as const] : []
  const environment = text(query, 'environment')
  const scope = text(query, 'scope')
  const limit = Number(text(query, 'limit'))
  return {
    keyword: text(query, 'keyword', 128).trim(),
    types, states,
    createdFrom: validDate(text(query, 'createdFrom')),
    createdTo: validDate(text(query, 'createdTo')),
    startedFrom: validDate(text(query, 'startedFrom')),
    startedTo: validDate(text(query, 'startedTo')),
    dataSourceId: text(query, 'dataSourceId'),
    environment: environments.includes(environment as TaskEnvironment) ? environment as TaskEnvironment : '',
    nodeId: text(query, 'nodeId'),
    creatorId: text(query, 'creatorId'),
    scope: scope === 'MINE' || scope === 'ALL' ? scope : '',
    limit: (taskListPageSizes as readonly number[]).includes(limit) ? limit as TaskListPageSize : 10,
  }
}

/** 生成规范化 query：只写非默认值，保证同一条件得到同一 URL 与同一游标缓存键。 */
export function taskListRouteQuery(state: TaskListRouteState): LocationQueryRaw {
  const query: Record<string, string> = {}
  const set = (key: string, value: string) => { if (value) query[key] = value }
  set('keyword', state.keyword.trim())
  set('type', state.types.join(','))
  set('state', [...state.states].sort().join(','))
  set('createdFrom', state.createdFrom)
  set('createdTo', state.createdTo)
  set('startedFrom', state.startedFrom)
  set('startedTo', state.startedTo)
  set('dataSourceId', state.dataSourceId)
  set('environment', state.environment)
  set('nodeId', state.nodeId)
  set('creatorId', state.creatorId)
  set('scope', state.scope)
  if (state.limit !== 10) query.limit = String(state.limit)
  return query
}

/** 条件签名不含条数之外的分页信息；条件或条数变化都意味着回到首页。 */
export function taskListConditionKey(state: TaskListRouteState): string {
  return JSON.stringify(taskListRouteQuery(state))
}

export function hasTaskListConditions(state: TaskListRouteState): boolean {
  return Object.keys(taskListRouteQuery({ ...state, limit: 10 })).length > 0
}

export function hasExpandedTaskListConditions(state: TaskListRouteState): boolean {
  return Boolean(state.dataSourceId || state.environment || state.nodeId || state.creatorId || state.scope || state.startedFrom || state.startedTo)
}

// 本地日期起点换算为 UTC 时刻；结束日期包含当天，因此上界取次日零点。
function localDayStart(value: string, addDays = 0): string | undefined {
  if (!value) return undefined
  const date = new Date(`${value}T00:00:00`)
  date.setDate(date.getDate() + addDays)
  return Number.isNaN(date.valueOf()) ? undefined : date.toISOString()
}

/** 转为 API 筛选；本人范围与按创建人筛选互斥时以显式创建人为准。 */
export function taskListApiFilter(state: TaskListRouteState): TaskListFilter {
  return {
    nodeId: state.nodeId || undefined,
    scope: state.scope || undefined,
    keyword: state.keyword || undefined,
    types: state.types.length ? state.types : undefined,
    states: state.states.length ? state.states : undefined,
    createdFrom: localDayStart(state.createdFrom),
    createdTo: localDayStart(state.createdTo, 1),
    startedFrom: localDayStart(state.startedFrom),
    startedTo: localDayStart(state.startedTo, 1),
    dataSourceId: state.dataSourceId || undefined,
    environment: state.environment || undefined,
    creatorId: state.creatorId || undefined,
  }
}

export interface TaskListConditionChip {
  readonly key: string
  readonly label: string
  readonly clear: Partial<TaskListRouteState>
}

const stateNames: Readonly<Record<TaskListState, string>> = {
  WAITING_SCHEDULE: '等待调度', STARTING: '启动中', RUNNING: '运行中', CANCELLING: '取消中', SUCCEEDED: '成功', FAILED: '失败', CANCELLED: '已取消',
}
const environmentNames: Readonly<Record<TaskEnvironment, string>> = { DEVELOPMENT: '开发', TEST: '测试', STAGING: '预生产', PRODUCTION: '生产' }

/** 活动筛选摘要：每项可单独移除；候选名称缺失时显示标识，不推断名称。 */
export function taskListConditionChips(state: TaskListRouteState, names: (kind: 'dataSource' | 'node' | 'creator', id: string) => string = (_kind, id) => id): readonly TaskListConditionChip[] {
  const chips: TaskListConditionChip[] = []
  const range = (from: string, to: string) => from && to ? `${from} 至 ${to}` : from ? `${from} 起` : `至 ${to}`
  if (state.keyword) chips.push({ key: 'keyword', label: `关键字：${state.keyword}`, clear: { keyword: '' } })
  if (state.types.length) chips.push({ key: 'type', label: '类型：导出', clear: { types: [] } })
  if (state.states.length) chips.push({ key: 'state', label: `状态：${state.states.map(value => stateNames[value]).join('、')}`, clear: { states: [] } })
  if (state.createdFrom || state.createdTo) chips.push({ key: 'created', label: `创建时间：${range(state.createdFrom, state.createdTo)}`, clear: { createdFrom: '', createdTo: '' } })
  if (state.dataSourceId) chips.push({ key: 'dataSource', label: `数据源：${names('dataSource', state.dataSourceId)}`, clear: { dataSourceId: '' } })
  if (state.environment) chips.push({ key: 'environment', label: `环境：${environmentNames[state.environment]}`, clear: { environment: '' } })
  if (state.nodeId) chips.push({ key: 'node', label: `执行节点：${names('node', state.nodeId)}`, clear: { nodeId: '' } })
  if (state.creatorId) chips.push({ key: 'creator', label: `创建人：${names('creator', state.creatorId)}`, clear: { creatorId: '' } })
  if (state.scope) chips.push({ key: 'scope', label: state.scope === 'MINE' ? '范围：本人创建' : '范围：全部任务', clear: { scope: '' } })
  if (state.startedFrom || state.startedTo) chips.push({ key: 'started', label: `开始时间：${range(state.startedFrom, state.startedTo)}`, clear: { startedFrom: '', startedTo: '' } })
  return chips
}

/** 日期区间倒置时不发送请求，由页面在字段旁说明。 */
export function taskListDateRangeError(state: TaskListRouteState): string {
  if (state.createdFrom && state.createdTo && state.createdFrom > state.createdTo) return '创建时间的开始日期晚于结束日期。'
  if (state.startedFrom && state.startedTo && state.startedFrom > state.startedTo) return '开始时间的开始日期晚于结束日期。'
  return ''
}
