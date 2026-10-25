import { inject, onBeforeUnmount, onScopeDispose, provide, readonly, ref, type InjectionKey } from 'vue'

type Guard = () => boolean | Promise<boolean>

// 退出协调器只属于当前 ProductShell，复用页面已有离开意图，不保存业务表单或建立全局事件通道。
export function createLogoutGuard() {
  const committed = ref(false)
  const guards = new Set<{ check: Guard; removed: Promise<boolean>; cancel: () => void }>()
  let generation = 0
  let disposed = false

  function register(check: Guard) {
    if (disposed) return () => undefined
    let cancel = () => undefined as void
    const removed = new Promise<boolean>(resolve => { cancel = () => resolve(false) })
    const entry = { check, removed, cancel }
    guards.add(entry)
    generation++
    return () => {
      if (!guards.delete(entry)) return
      generation++
      entry.cancel()
    }
  }

  async function confirm() {
    if (disposed || committed.value) return false
    const current = generation
    for (const entry of [...guards].reverse()) {
      try {
        // 弹层/页面卸载立即取消旧等待；迟到确认不能批准另一个编辑会话的退出。
        const allowed = await Promise.race([entry.check(), entry.removed])
        if (!allowed || disposed || current !== generation) return false
      } catch { return false }
    }
    return !disposed && current === generation
  }

  function commit() { if (!disposed) committed.value = true }
  function dispose() {
    disposed = true
    generation++
    guards.forEach(entry => entry.cancel())
    guards.clear()
  }
  return { committed: readonly(committed), register, confirm, commit, dispose }
}

const logoutKey: InjectionKey<ReturnType<typeof createLogoutGuard>> = Symbol('logout-guard')

export function provideLogoutGuard() {
  const guard = createLogoutGuard()
  provide(logoutKey, guard)
  onScopeDispose(guard.dispose)
  return guard
}

export function useLogoutContext() { return inject(logoutKey, null) }

export function useLogoutGuard(check: Guard) {
  const guard = useLogoutContext()
  const unregister = guard?.register(check)
  onBeforeUnmount(() => unregister?.())
  return guard?.committed ?? readonly(ref(false))
}
