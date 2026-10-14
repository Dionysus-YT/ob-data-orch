import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { useOperationMessage } from './useOperationMessage'

const holders = vi.hoisted(() => [] as { open: ReturnType<typeof vi.fn>; destroy: ReturnType<typeof vi.fn> }[])
const useMessage = vi.hoisted(() => vi.fn(() => {
  const api = { open: vi.fn(), destroy: vi.fn() }
  holders.push(api)
  return [api, () => null]
}))
vi.mock('ant-design-vue', () => ({ message: { useMessage } }))

const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()); holders.length = 0; vi.clearAllMocks() })
function scope() { const value = effectScope(); scopes.push(value); return value }

describe('操作 Message 的会话边界', () => {
  it('读取已有结果不重放，显式成功采用短时、非打断式确认', async () => {
    const notice = ref('上次操作已完成。')
    scope().run(() => useOperationMessage(notice))
    const api = holders[0]!
    expect(api.open).not.toHaveBeenCalled()
    notice.value = ''
    notice.value = '节点已启用。'
    await nextTick()
    expect(useMessage).toHaveBeenCalledWith({ duration: 3, maxCount: 1 })
    const result = api.open.mock.calls[0]![0]
    expect(result).toMatchObject({ type: 'success', duration: 3 })
    expect(result.content.children).toBe('节点已启用。')
    expect(result.content.props).toMatchObject({ role: 'status', 'aria-live': 'polite' })
  })

  it('重试前的同步清空关闭旧提示，相同成功文案仍可再次确认', async () => {
    const notice = ref('')
    scope().run(() => useOperationMessage(notice))
    const api = holders[0]!
    notice.value = '配置已保存。'
    await nextTick()
    notice.value = ''
    notice.value = '配置已保存。'
    await nextTick()
    expect(api.open).toHaveBeenCalledTimes(2)
    expect(api.destroy).toHaveBeenCalledWith()
    expect(api.open.mock.calls[1]![0].key).not.toBe(api.open.mock.calls[0]![0].key)
    expect(api.destroy).not.toHaveBeenCalledWith(api.open.mock.calls[1]![0].key)
  })

  it('无更改为 info，清除其他结果不能关闭当前来源的提示', async () => {
    const saved = ref('')
    const copied = ref('')
    const type = ref<'info' | 'success'>('info')
    scope().run(() => useOperationMessage([{ content: saved, type }, { content: copied }]))
    const api = holders[0]!
    saved.value = '没有需要保存的更改。'
    await nextTick()
    expect(api.open.mock.calls[0]![0].type).toBe('info')
    copied.value = '脱敏命令已复制。'
    await nextTick()
    const closes = api.destroy.mock.calls.length
    saved.value = ''
    expect(api.destroy).toHaveBeenCalledTimes(closes)
    copied.value = ''
    expect(api.destroy).toHaveBeenCalledTimes(closes + 1)
  })

  it('各工作面隔离，卸载销毁上下文并拒绝迟到的命令式回调', async () => {
    const oldScope = scope()
    const old = oldScope.run(() => useOperationMessage())!
    const current = scope().run(() => useOperationMessage())!
    const later = Promise.resolve().then(() => old.success('迟到的旧结果。'))
    oldScope.stop()
    await current.success('当前结果。')
    await later
    expect(holders[0]!.destroy).toHaveBeenCalledWith()
    expect(holders[0]!.open).not.toHaveBeenCalled()
    expect(holders[1]!.open).toHaveBeenCalledTimes(1)
    expect(holders[1]!.destroy).toHaveBeenCalledTimes(1)
  })

  it('等待原生上下文期间清空、替换或卸载均拒绝补放旧结果', async () => {
    const notice = ref('')
    const owner = scope()
    owner.run(() => useOperationMessage(notice))
    notice.value = '旧成功。'
    notice.value = ''
    await nextTick()
    expect(holders[0]!.open).not.toHaveBeenCalled()
    notice.value = '被替换的成功。'
    notice.value = '当前成功。'
    await nextTick()
    expect(holders[0]!.open).toHaveBeenCalledTimes(1)
    expect(holders[0]!.open.mock.calls[0]![0].content.children).toBe('当前成功。')
    notice.value = '卸载前的迟到成功。'
    owner.stop()
    await nextTick()
    expect(holders[0]!.open).toHaveBeenCalledTimes(1)
  })
})
