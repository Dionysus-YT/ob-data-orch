import type { BrowserApi } from '@/api/browser'

export type TaskDetailApi = Pick<BrowserApi,
  'getTaskOverview' | 'getTaskExecution' | 'getTaskSnapshot' | 'getTaskCommandEvidence' |
  'getTaskLogs' | 'streamTaskLogs' | 'rebuildTaskDraft' | 'resumeTaskFromCheckpoint' | 'saveTaskTemplate'> & Partial<Pick<BrowserApi, 'getTaskOperations'>>

// 每次任务 ID 变化创建独立会话；读写回调必须验证会话仍有效，不能只比较 ID（A → B → A）。
export interface TaskDetailSession {
  readonly id: string
  readonly api: TaskDetailApi
  readonly active: () => boolean
}

/** 受限响应使整个任务会话失败关闭，不保留此前读取的对象事实。 */
export function isTaskAuthorizationError(error: unknown): boolean {
  return typeof error === 'object' && error !== null && 'status' in error && [401, 403, 404].includes(Number(error.status))
}
