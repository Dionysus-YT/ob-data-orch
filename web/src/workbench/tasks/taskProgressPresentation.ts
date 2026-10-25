import type { TaskProgress } from '@/api/browser'
import { isKnownTaskState, taskDateTimeText } from './taskStatePresentation'

const phases: Readonly<Record<TaskProgress['phase'], string>> = {
  EXPORT_DATA: '导出数据', IMPORT_DATA: '导入数据', EXPORT_SCHEMA_COMPLETE: '结构导出完成', IMPORT_SCHEMA_COMPLETE: '结构导入完成',
  EXPORT_RECORD_COMPLETE: '数据导出完成', IMPORT_RECORD_COMPLETE: '数据导入完成',
}

// 进度事实只决定展示；成功/失败/取消及核对状态始终由原执行事实所有者提供。
export function taskProgressPresentation(progress: TaskProgress | undefined, state?: string, reconciliation = false, type?: string, timeZone?: 'UTC') {
  if (progress && type && (type === 'OBLOADER_DIRECT_LOAD' || !progress.phase.startsWith(type === 'OBLOADER_NORMAL_IMPORT' ? 'IMPORT_' : 'EXPORT_'))) progress = undefined
  const phase = progress ? phases[progress.phase] : ''
  const historical = Boolean(progress && (progress.incomplete || reconciliation || state !== undefined && !isKnownTaskState(state) || ['FAILED', 'CANCELLED'].includes(state ?? '')))
  const amount = progress?.percent === undefined ? '工具仅报告阶段，尚无可靠百分比。' : `数据子任务 ${progress.percent}%（已完成 ${progress.completedTasks}，运行中 ${progress.runningTasks}）。百分比不表示记录数、字节数或数据库结果。`
  const tail = state === 'SUCCEEDED' ? '任务已完成。' : historical ? '仅为最后可信记录，当前执行情况须核对。' : progress?.percent === 100 ? '工具报告数据进度 100%，任务仍在等待收尾与结果核验。' : ''
  // 首页声明 UTC；其他调用继续使用既有本地时区，不改变进度证据或终态语义。
  const updatedAt = progress ? timeZone === 'UTC' ? `${new Date(progress.updatedAt).toISOString().slice(0, 19).replace('T', ' ')} UTC` : taskDateTimeText(progress.updatedAt) : ''
  return {
    phase,
    text: state === 'SUCCEEDED' ? '已完成' : progress?.percent !== undefined ? `${progress.percent}%` : phase || '—',
    percent: state === 'SUCCEEDED' ? undefined : progress?.percent,
    status: state === 'FAILED' ? 'exception' as const : historical || state === 'CANCELLING' ? 'normal' as const : 'active' as const,
    historical,
    description: progress ? `${phase}。${amount}进度更新于 ${updatedAt}。${tail}` : state === 'SUCCEEDED' ? '任务已完成；没有可用的历史进度记录。' : '工具未提供可靠进度；可查看运行状态、耗时与日志。',
  }
}
