import type { RouteLocationRaw } from 'vue-router'
import type { TaskDerivedDraft } from '@/api/browser'

/** 派生草稿的唯一目标：基于原配置新建回到第 1 步可改参，从头重新执行直达第 5 步复验。 */
export function derivedDraftRoute(derived: Pick<TaskDerivedDraft, 'draftId' | 'derivation'>): RouteLocationRaw {
  return { path: '/exports/new', query: { draft: derived.draftId, step: derived.derivation === 'RERUN_FROM_SCRATCH' ? '5' : '1' } }
}

/** 检查点继续生成新任务 ID，跳转到新任务详情。 */
export function derivedTaskRoute(taskId: string): RouteLocationRaw {
  return `/tasks/${encodeURIComponent(taskId)}`
}
