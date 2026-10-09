import type { BrowserApi } from '@/api/browser'

export type TaskDetailApi = Pick<BrowserApi,
  'getTaskOverview' | 'getTaskExecution' | 'getTaskSnapshot' | 'getTaskCommandEvidence' |
  'getTaskLogs' | 'streamTaskLogs' | 'rebuildTaskDraft' | 'resumeTaskFromCheckpoint' | 'saveTaskTemplate'>

// 每次任务 ID 变化创建独立会话；读写回调必须验证会话仍有效，不能只比较 ID（A → B → A）。
export interface TaskDetailSession {
  readonly id: string
  readonly api: TaskDetailApi
  readonly active: () => boolean
}
