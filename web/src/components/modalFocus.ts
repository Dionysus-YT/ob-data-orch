type ModalEntry = {
  root: HTMLElement
  initialFocus: HTMLElement
  returnFocus: HTMLElement | null
  onEscape: () => void
}

const stack: ModalEntry[] = []
let previousOverflow = ''
let appWasInert = false

function focusableElements(root: HTMLElement) {
  return Array.from(root.querySelectorAll<HTMLElement>('button, a[href], input, select, textarea, summary, [tabindex]'))
    .filter((element) => {
      const disclosure = element.closest('details:not([open])')
      // 折叠详情在部分 Chromium 版本仍保留布局盒，不能只依赖尺寸判断可聚焦性。
      if (disclosure && !disclosure.querySelector(':scope > summary')?.contains(element)) return false
      return element.tabIndex >= 0 && !element.matches(':disabled')
        && !element.closest('[inert], [hidden]') && element.getClientRects().length > 0
        && getComputedStyle(element).visibility !== 'hidden'
    })
}

function onKeydown(event: KeyboardEvent) {
  const current = stack.at(-1)
  if (!current) return
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    current.onEscape()
    return
  }
  if (event.key !== 'Tab') return
  const elements = focusableElements(current.root)
  const index = elements.indexOf(document.activeElement as HTMLElement)
  if (!elements.length) {
    event.preventDefault()
    current.initialFocus.focus()
  } else if (index < 0 || (event.shiftKey && index === 0) || (!event.shiftKey && index === elements.length - 1)) {
    event.preventDefault()
    elements[event.shiftKey ? elements.length - 1 : 0]?.focus()
  }
}

function onFocusIn(event: FocusEvent) {
  const current = stack.at(-1)
  if (current && event.target instanceof Node && !current.root.contains(event.target)) current.initialFocus.focus()
}

// 嵌套确认框独占键盘；只在最后一个弹层关闭时恢复背景和滚动。
export function activateModal(root: HTMLElement, initialFocus: HTMLElement, onEscape: () => void) {
  const entry: ModalEntry = { root, initialFocus, onEscape, returnFocus: document.activeElement instanceof HTMLElement ? document.activeElement : null }
  if (!stack.length) {
    previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const app = document.getElementById('app')
    appWasInert = app?.inert ?? false
    if (app && !app.contains(root)) app.inert = true
    document.addEventListener('keydown', onKeydown, true)
    document.addEventListener('focusin', onFocusIn)
  }
  stack.push(entry)
  initialFocus.focus({ preventScroll: true })
  let released = false
  return () => {
    if (released) return
    released = true
    const wasTop = stack.at(-1) === entry
    const index = stack.indexOf(entry)
    if (index >= 0) stack.splice(index, 1)
    if (!stack.length) {
      document.body.style.overflow = previousOverflow
      const app = document.getElementById('app')
      if (app) app.inert = appWasInert
      document.removeEventListener('keydown', onKeydown, true)
      document.removeEventListener('focusin', onFocusIn)
    }
    if (!wasTop) return
    const parent = stack.at(-1)
    const target = entry.returnFocus
    if (target?.isConnected && !target.matches(':disabled') && (!parent || parent.root.contains(target))) target.focus({ preventScroll: true })
    else parent?.initialFocus.focus({ preventScroll: true })
  }
}
