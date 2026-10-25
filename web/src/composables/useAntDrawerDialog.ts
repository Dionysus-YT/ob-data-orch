import { onBeforeUnmount, ref, watch } from 'vue'
import { activateModal } from '@/components/modalFocus'

export function useAntDrawerDialog(id: string, close: () => void, busy: () => boolean = () => false, isOpen?: () => boolean) {
  const heading = ref<HTMLElement>()
  let invoker = document.activeElement instanceof HTMLElement ? document.activeElement : null
  let release: (() => void) | undefined

  function captureInvoker() {
    invoker = document.activeElement instanceof HTMLElement ? document.activeElement : null
  }

  function afterOpenChange(open: boolean) {
    // 快速关闭或重开时，Ant 的上一轮动画回调不能激活隐藏抽屉或释放新一轮焦点栈。
    if (isOpen && open !== isOpen()) return
    if (!open) {
      if (release) release()
      else if (invoker?.isConnected && !invoker.matches(':disabled')) invoker.focus({ preventScroll: true })
      release = undefined
      return
    }
    const root = heading.value?.closest<HTMLElement>('.ant-drawer-content')
    if (!root || !heading.value) return
    root.setAttribute('role', 'dialog')
    root.setAttribute('aria-modal', 'true')
    root.setAttribute('aria-labelledby', id)
    release?.()
    release = activateModal(root, heading.value, () => { if (!busy()) close() }, invoker)
  }

  if (isOpen) watch(isOpen, open => {
    if (open) captureInvoker()
    else afterOpenChange(false)
  }, { flush: 'sync' })

  onBeforeUnmount(() => release?.())

  return { heading, afterOpenChange, captureInvoker }
}
