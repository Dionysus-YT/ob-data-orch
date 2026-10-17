import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { useNormalImportReviewAutomation } from './useNormalImportReviewAutomation'

const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => scopes.splice(0).forEach(scope => scope.stop()))
function deferred() { let resolve!: () => void; const promise = new Promise<void>(yes => { resolve = yes }); return { promise, resolve } }
function setup() {
  const scope = effectScope(); scopes.push(scope)
  const deps = { active: ref(false), binding: ref('A'), ready: ref(true), busy: ref(false), hasCommand: ref(false), hasCheck: ref(false),
    prepareCommand: vi.fn(async () => { deps.hasCommand.value = true }), startCheck: vi.fn(async () => { deps.hasCheck.value = true }) }
  const automation = scope.run(() => useNormalImportReviewAutomation(deps))!
  return { scope, deps, automation }
}
describe('普通导入第五步自动准备', () => {
  it('配置与引用就绪后先生成命令再检查，同一版本与重复进入不重放检查', async () => {
    const { deps } = setup(); deps.ready.value = false; deps.active.value = true; await nextTick()
    expect(deps.prepareCommand).not.toHaveBeenCalled()
    deps.ready.value = true
    await vi.waitFor(() => expect(deps.startCheck).toHaveBeenCalledTimes(1))
    expect(deps.prepareCommand.mock.invocationCallOrder[0]).toBeLessThan(deps.startCheck.mock.invocationCallOrder[0]!)
    deps.busy.value = true; await nextTick(); deps.busy.value = false; deps.active.value = false; await nextTick(); deps.active.value = true; await nextTick()
    expect(deps.startCheck).toHaveBeenCalledTimes(1)
  })
  it('自动生成失败只尝试一次，手动重试可恢复且不重复生成现有命令', async () => {
    const { deps, automation } = setup(); deps.prepareCommand.mockImplementationOnce(async () => undefined); deps.active.value = true; await nextTick()
    expect(deps.startCheck).not.toHaveBeenCalled()
    deps.busy.value = true; await nextTick(); deps.busy.value = false; await nextTick()
    expect(deps.prepareCommand).toHaveBeenCalledTimes(1)
    await automation.retryReview(); expect(deps.startCheck).toHaveBeenCalledTimes(1)
    await automation.retryReview(); expect(deps.prepareCommand).toHaveBeenCalledTimes(2); expect(deps.startCheck).toHaveBeenCalledTimes(2)
  })
  it.each(['binding', 'navigation', 'dispose'] as const)('命令等待期间的 %s 变化隔离迟到检查，绑定使用版本处理 A → B → A', async kind => {
    const { deps, scope } = setup(), pending = deferred()
    deps.prepareCommand.mockImplementation(async () => { await pending.promise; deps.hasCommand.value = true })
    deps.active.value = true; await nextTick()
    if (kind === 'binding') { deps.ready.value = false; deps.binding.value = 'B'; deps.binding.value = 'A' }
    if (kind === 'navigation') deps.active.value = false
    if (kind === 'dispose') scope.stop()
    pending.resolve(); await nextTick(); await nextTick()
    expect(deps.startCheck).not.toHaveBeenCalled()
  })
  it('事务忙时重试没有重复副作用，新配置可自动重新准备', async () => {
    const { deps, automation } = setup(); deps.active.value = true; deps.busy.value = true; await nextTick(); await automation.retryReview()
    expect(deps.prepareCommand).not.toHaveBeenCalled()
    deps.busy.value = false; await vi.waitFor(() => expect(deps.startCheck).toHaveBeenCalledTimes(1))
    deps.hasCheck.value = false; deps.hasCommand.value = false; deps.binding.value = 'B'
    await vi.waitFor(() => expect(deps.startCheck).toHaveBeenCalledTimes(2))
  })
})
