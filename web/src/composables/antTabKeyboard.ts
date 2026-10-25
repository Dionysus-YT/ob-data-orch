// 只接管 Tab 自身的导航键，避免影响面板输入；两类详情使用相同的 Ant Tab 键盘约定。
export function navigateAntTabs(event: KeyboardEvent) {
  if (!(event.target instanceof HTMLElement) || event.target.getAttribute('role') !== 'tab') return
  const tabs = [...(event.target.closest('[role="tablist"]')?.querySelectorAll<HTMLElement>('[role="tab"]') ?? [])]
    .filter(tab => tab.getAttribute('aria-disabled') !== 'true')
  const current = tabs.indexOf(event.target)
  if (current < 0 || !tabs.length) return
  let next: number
  if (event.key === 'ArrowRight') next = (current + 1) % tabs.length
  else if (event.key === 'ArrowLeft') next = (current + tabs.length - 1) % tabs.length
  else if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = tabs.length - 1
  else return
  event.preventDefault()
  tabs[next]?.focus()
  tabs[next]?.click()
}
