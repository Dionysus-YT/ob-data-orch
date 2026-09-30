import { onBeforeUnmount, ref } from 'vue'
import { activateModal } from '@/components/modalFocus'

export function useAntDrawerDialog(id: string, close: () => void, busy: () => boolean = () => false) {
  const heading = ref<HTMLElement>()
  let invoker = document.activeElement instanceof HTMLElement ? document.activeElement : null
  let release: (() => void) | undefined

  function captureInvoker() {
    invoker = document.activeElement instanceof HTMLElement ? document.activeElement : null
  }

  function afterOpenChange(open: boolean) {
    if (!open) {
      release?.()
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

  onBeforeUnmount(() => release?.())

  return { heading, afterOpenChange, captureInvoker }
}
