import { afterEach, describe, expect, it, vi } from 'vitest'
import { withTemplateWriteDeadline } from './templateWrites'

afterEach(() => { vi.useRealTimers() })
describe('模板写入的有界等待', () => {
  it('超时只返回未知结果，迟到成功不重新结算并清理计时器', async () => {
    vi.useFakeTimers()
    let complete!: (value: string) => void
    const pending = withTemplateWriteDeadline(() => new Promise<string>(resolve => { complete = resolve }), undefined, 10)
    const checked = expect(pending).rejects.toMatchObject({ code: 'TEMPLATE_WRITE_UNCONFIRMED' })
    await vi.advanceTimersByTimeAsync(10); await checked
    complete('late-created'); await Promise.resolve()
    expect(vi.getTimerCount()).toBe(0)
  })
  it('卸载信号停止等待并清理计时器，已失效会话不再发出操作', async () => {
    vi.useFakeTimers()
    const controller = new AbortController()
    const pending = withTemplateWriteDeadline(() => new Promise<never>(() => {}), controller.signal)
    const checked = expect(pending).rejects.toMatchObject({ code: 'TEMPLATE_WRITE_UNCONFIRMED' })
    controller.abort(); await checked
    expect(vi.getTimerCount()).toBe(0)
    const operation = vi.fn(async () => 'unexpected')
    await expect(withTemplateWriteDeadline(operation, controller.signal)).rejects.toMatchObject({ code: 'TEMPLATE_WRITE_UNCONFIRMED' })
    expect(operation).not.toHaveBeenCalled()
  })
})
