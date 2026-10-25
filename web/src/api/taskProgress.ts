export const taskProgressPhases = ['EXPORT_DATA', 'IMPORT_DATA', 'EXPORT_SCHEMA_COMPLETE', 'IMPORT_SCHEMA_COMPLETE', 'EXPORT_RECORD_COMPLETE', 'IMPORT_RECORD_COMPLETE'] as const
export type TaskProgressPhase = typeof taskProgressPhases[number]

// 百分比仅指工具数据子任务，和产品终态、记录数及数据库结果分别表达。
export interface TaskProgress {
  readonly phase: TaskProgressPhase
  readonly percent?: number
  readonly completedTasks?: number
  readonly runningTasks?: number
  readonly updatedAt: string
  readonly incomplete: boolean
}

// 旧服务/Agent 的 UNAVAILABLE 保持兼容；新证据必须带完整且相互一致的安全摘要。
export function parseTaskProgressEvidence(value: Record<string, unknown>, taskType?: string, invalid?: () => never): {
  stageEvidence: 'UNAVAILABLE' | 'RELIABLE'; progressEvidence: 'UNAVAILABLE' | 'PERCENT'; progress?: TaskProgress
} {
  const fail = (): never => { if (invalid) return invalid(); throw new Error('任务进度不符合响应契约。') }
  const stageEvidence = value['stageEvidence'], progressEvidence = value['progressEvidence'], raw = value['progress']
  if (stageEvidence !== 'UNAVAILABLE' && stageEvidence !== 'RELIABLE' || progressEvidence !== 'UNAVAILABLE' && progressEvidence !== 'PERCENT') fail()
  if (raw === undefined) {
    if (stageEvidence !== 'UNAVAILABLE' || progressEvidence !== 'UNAVAILABLE') fail()
    return { stageEvidence: 'UNAVAILABLE', progressEvidence: 'UNAVAILABLE' }
  }
  // 旁路尚无独立阶段契约，不能借用导出或普通导入的工具进度语义。
  if (taskType === 'OBLOADER_DIRECT_LOAD') fail()
  if (raw === null || typeof raw !== 'object' || Array.isArray(raw) || stageEvidence !== 'RELIABLE') fail()
  const item = raw as Record<string, unknown>
  if (Object.keys(item).some(key => !['phase', 'percent', 'completedTasks', 'runningTasks', 'updatedAt', 'incomplete'].includes(key))) fail()
  const phase = item['phase'] as TaskProgressPhase
  if (!taskProgressPhases.includes(phase) || taskType && !phase.startsWith(taskType === 'OBLOADER_NORMAL_IMPORT' ? 'IMPORT_' : 'EXPORT_')) fail()
  if (typeof item['updatedAt'] !== 'string' || !Number.isFinite(Date.parse(item['updatedAt'])) || typeof item['incomplete'] !== 'boolean') fail()
  const percent = item['percent']
  if (percent === undefined) {
    if (progressEvidence !== 'UNAVAILABLE' || item['completedTasks'] !== undefined || item['runningTasks'] !== undefined || phase.endsWith('_DATA')) fail()
  } else {
    if (progressEvidence !== 'PERCENT' || !phase.endsWith('_DATA') || typeof percent !== 'number' || !Number.isFinite(percent) || percent < 0 || percent > 100 || Math.abs(percent * 100 - Math.round(percent * 100)) > 1e-8) fail()
    for (const key of ['completedTasks', 'runningTasks']) if (!Number.isSafeInteger(item[key]) || (item[key] as number) < 0) fail()
  }
  const progress: TaskProgress = { phase, updatedAt: item['updatedAt'] as string, incomplete: item['incomplete'] as boolean, ...(percent === undefined ? {} : { percent: percent as number, completedTasks: item['completedTasks'] as number, runningTasks: item['runningTasks'] as number }) }
  return { stageEvidence: 'RELIABLE', progressEvidence: progressEvidence as 'UNAVAILABLE' | 'PERCENT', progress }
}
