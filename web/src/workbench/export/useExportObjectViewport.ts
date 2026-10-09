import { nextTick, onScopeDispose, shallowRef, watch, type Ref } from 'vue'

// 仅管理对象选择区的可见窗口；目录、搜索和勾选始终使用完整业务集合。
export function useExportObjectViewport(element: Ref<HTMLElement | undefined>, revision: () => unknown) {
  const positions = shallowRef<Record<string, { top: number; height: number }>>({})
  let frame = 0
  let observer: ResizeObserver | undefined
  let disposed = false
  function update() {
    const pane = element.value
    if (!pane || disposed) return
    const origin = pane.getBoundingClientRect().top + pane.clientTop
    const next: Record<string, { top: number; height: number }> = {}
    for (const list of pane.querySelectorAll<HTMLElement>('[data-export-object-window]')) {
      next[list.dataset.exportObjectWindow!] = { top: list.getBoundingClientRect().top - origin, height: pane.clientHeight }
    }
    positions.value = next
  }
  function schedule() {
    if (disposed || frame) return
    frame = requestAnimationFrame(() => { frame = 0; update() })
  }
  watch(element, (pane, previous) => {
    previous?.removeEventListener('scroll', schedule)
    observer?.disconnect()
    pane?.addEventListener('scroll', schedule, { passive: true })
    observer = new ResizeObserver(schedule)
    if (pane) observer.observe(pane)
    schedule()
  }, { flush: 'post' })
  watch(revision, () => { void nextTick(schedule) }, { flush: 'post' })
  onScopeDispose(() => {
    disposed = true
    element.value?.removeEventListener('scroll', schedule)
    observer?.disconnect()
    cancelAnimationFrame(frame)
  })
  function range(key: string, count: number, rowHeight: number) {
    if (count <= 100) return { start: 0, end: count, before: 0, after: 0 }
    const position = positions.value[key] ?? { top: 0, height: 400 }
    const start = Math.min(count, Math.max(0, Math.floor(-position.top / rowHeight) - 5))
    const end = Math.min(count, Math.max(start, Math.ceil((position.height - position.top) / rowHeight) + 5))
    return { start, end, before: start * rowHeight, after: (count - end) * rowHeight }
  }
  return { range }
}
