import { describe, expect, it, vi } from 'vitest'
import { createLogoutGuard } from './useLogoutGuard'

function deferred() {
  let resolve!: (value: boolean) => void
  const promise = new Promise<boolean>(done => { resolve = done })
  return { promise, resolve }
}

describe('退出登录前的页面离开协调', () => {
  it('无编辑会话可以退出，只有明确提交注销结果才放行卸载', async () => {
    const guard = createLogoutGuard()
    expect(await guard.confirm()).toBe(true)
    expect(guard.committed.value).toBe(false)
    guard.commit()
    expect(guard.committed.value).toBe(true)
    expect(await guard.confirm()).toBe(false)
  })

  it('先询问当前弹层，取消或忙碌阻断后不继续询问其他工作面', async () => {
    const guard = createLogoutGuard()
    const page = vi.fn(() => true), editor = vi.fn(() => false)
    guard.register(page)
    const remove = guard.register(editor)
    expect(await guard.confirm()).toBe(false)
    expect(page).not.toHaveBeenCalled()
    remove()
    expect(await guard.confirm()).toBe(true)
    expect(page).toHaveBeenCalledOnce()
    expect(guard.committed.value).toBe(false)
  })

  it('页面卸载立即结束旧确认，迟到同意不能批准退出', async () => {
    const guard = createLogoutGuard(), pending = deferred()
    const remove = guard.register(() => pending.promise)
    const result = guard.confirm()
    remove()
    expect(await result).toBe(false)
    pending.resolve(true)
    expect(guard.committed.value).toBe(false)
  })

  it('确认期间出现新编辑会话，旧确认不能代表新会话', async () => {
    const guard = createLogoutGuard(), pending = deferred()
    guard.register(() => pending.promise)
    const result = guard.confirm()
    guard.register(() => true)
    pending.resolve(true)
    expect(await result).toBe(false)
  })

  it('读取离开资格异常时阻断，不把失败当作同意', async () => {
    const guard = createLogoutGuard()
    guard.register(() => { throw new Error('synthetic guard failure') })
    expect(await guard.confirm()).toBe(false)
    expect(guard.committed.value).toBe(false)
  })

  it('外壳销毁结束待决确认，旧请求不能重新放行卸载', async () => {
    const guard = createLogoutGuard(), pending = deferred()
    guard.register(() => pending.promise)
    const result = guard.confirm()
    guard.dispose()
    expect(await result).toBe(false)
    pending.resolve(true)
    guard.commit()
    expect(guard.committed.value).toBe(false)
    expect(await guard.confirm()).toBe(false)
  })
})
