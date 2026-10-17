import { onScopeDispose, watch, type Ref } from 'vue'

// 第五步的自动准备只协调既有命令和检查事务；每次配置变化最多尝试一次，失败保留手动重试。
export function useNormalImportReviewAutomation(deps: {
  active: Readonly<Ref<boolean>>; binding: Readonly<Ref<string>>; ready: Readonly<Ref<boolean>>; busy: Readonly<Ref<boolean>>
  hasCommand: Readonly<Ref<boolean>>; hasCheck: Readonly<Ref<boolean>>
  prepareCommand(): Promise<void>; startCheck(): Promise<void>
}) {
  let disposed = false, version = 0, attempted = -1, navigation = 0
  watch(deps.binding, () => { version++ }, { flush: 'sync' })
  watch(deps.active, active => { navigation++; if (!active) attempted = -1 }, { flush: 'sync' })
  async function prepareReview(force = false) {
    if (disposed || !deps.active.value || !deps.ready.value || deps.busy.value || !force && (attempted === version || deps.hasCheck.value)) return
    const current = version, visit = navigation
    attempted = current
    if (!deps.hasCommand.value) await deps.prepareCommand()
    if (disposed || current !== version || visit !== navigation || !deps.active.value || !deps.hasCommand.value || deps.busy.value) return
    await deps.startCheck()
  }
  watch([deps.active, deps.ready, deps.busy, deps.binding], () => { void prepareReview() }, { flush: 'post', immediate: true })
  onScopeDispose(() => { disposed = true; version++; navigation++ })
  return { retryReview: () => prepareReview(true) }
}
