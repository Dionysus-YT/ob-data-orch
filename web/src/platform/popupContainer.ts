// 弹层内的 Select/Menu 挂在同一产品焦点边界中，避免 Teleport 到 body 后被抽屉焦点约束拦截。
export function popupContainer(trigger?: HTMLElement): HTMLElement {
  return trigger?.closest<HTMLElement>('[role="dialog"]') ?? document.body
}
