import { inject, type ComputedRef, type InjectionKey } from 'vue'

// 路由显式接入新视觉；Teleport 保留注入上下文，避免弹层回落到旧主题。
export const workbenchFoundationKey: InjectionKey<ComputedRef<boolean>> = Symbol('workbench-foundation')

export function useWorkbenchFoundation() {
  return inject(workbenchFoundationKey, undefined)
}
