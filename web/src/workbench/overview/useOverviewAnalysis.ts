import { computed, ref } from 'vue'
import { useTaskList, type TaskListApiFactory } from '@/workbench/tasks/useTaskList'
import { overviewTrendQuery, type OverviewTrendSession } from './overviewTrend'

/** 把弹窗的固定窗口和选择意图绑定到既有任务列表；请求、游标及迟到隔离仍只有 useTaskList 一个所有者。 */
export function useOverviewAnalysis(session: OverviewTrendSession, createApi?: TaskListApiFactory) {
  const selectedFrom = ref(session.selectedFrom ?? '')
  const state = ref('')
  const query = computed(() => overviewTrendQuery(session, selectedFrom.value, state.value))
  const valid = computed(() => query.value !== null)
  // 无效窗口绝不退化为无筛选读取；关闭弹窗时由组件卸载同步取消等待。
  const list = useTaskList(computed(() => query.value ?? {}), () => undefined, { enabled: valid, createApi, autoRefreshMs: 0 })
  return { selectedFrom, state, query, valid, list }
}
