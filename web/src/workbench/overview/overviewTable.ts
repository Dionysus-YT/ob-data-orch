import { h, type FunctionalComponent } from 'vue'

/** 通过 Ant 原生表格渲染端口关联名称，避免 ARIA 只落在外层容器；原表格属性和插槽完整透传。 */
export function overviewTableComponents(label: string) {
  const table: FunctionalComponent = (attributes, { slots }) => h('table', { ...attributes, 'aria-label': label }, slots.default?.())
  return { table }
}
