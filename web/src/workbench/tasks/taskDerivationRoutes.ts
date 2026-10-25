import type { RouteLocationRaw } from 'vue-router'
import type { SupportedTaskType, TaskDerivedDraft } from '@/api/browser'

/** 派生草稿的唯一目标：新建回到第 1 步可改参；从头执行直达自身最终复验步。 */
export function derivedDraftRoute(derived: Pick<TaskDerivedDraft, 'draftId' | 'derivation'>, taskType: SupportedTaskType = 'OBDUMPER_EXPORT'): RouteLocationRaw {
  return { path: taskType === 'OBLOADER_DIRECT_LOAD' ? '/imports/direct/new' : taskType === 'OBLOADER_NORMAL_IMPORT' ? '/imports/normal/new' : '/exports/new', query: { draft: derived.draftId, step: derived.derivation === 'RERUN_FROM_SCRATCH' ? '5' : '1' } }
}

/** 检查点继续生成新任务 ID，跳转到新任务详情。 */
export function derivedTaskRoute(taskId: string): RouteLocationRaw {
  return `/tasks/${encodeURIComponent(taskId)}`
}
