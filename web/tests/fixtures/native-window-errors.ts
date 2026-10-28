import { expect, type Page } from '@playwright/test'

// 收集原生窗口错误并跨页面导航保留；不改写观察器、过滤错误或阻止浏览器处理。
export async function monitorNativeWindowErrors(page: Page): Promise<() => Promise<void>> {
  const errors: string[] = []
  await page.exposeFunction('recordNativeWindowError', (message: string) => { errors.push(message) })
  await page.addInitScript(() => {
    window.addEventListener('error', event => {
      void (window as unknown as { recordNativeWindowError(message: string): Promise<void> }).recordNativeWindowError(event.message)
    })
  })
  return async () => {
    await page.evaluate(async () => { await new Promise(requestAnimationFrame); await new Promise(requestAnimationFrame) })
    expect(errors, '原生窗口错误包含 ResizeObserver 异常，不能只检查 pageerror').toEqual([])
  }
}
