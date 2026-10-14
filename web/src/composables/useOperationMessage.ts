import { message } from 'ant-design-vue'
import { cloneVNode, h, isRef, nextTick, onScopeDispose, ref, toValue, watch, type MaybeRef, type Ref } from 'vue'

/** 操作提示仅确认当前动作结果，不表达持续的业务资格或检查结论。 */
export type OperationMessageType = 'success' | 'info'

/** 业务模块持有提示内容与语义，共享逻辑只管理 Ant Message 的展示生命周期。 */
export interface OperationNotice {
  readonly content: Readonly<Ref<string>>
  readonly type?: MaybeRef<OperationMessageType>
}

/** 将当前会话的操作结果交给原生 Message，重置和卸载时清除该会话的提示。 */
export function useOperationMessage(notices: Readonly<Ref<string>> | readonly OperationNotice[] = []) {
  const [api, nativeContext] = message.useMessage({ duration: 3, maxCount: 1 })
  const bindings: readonly OperationNotice[] = isRef(notices) ? [{ content: notices }] : notices
  const contextVersion = ref(0)
  const MessageContext = () => cloneVNode(nativeContext(), { key: contextVersion.value })
  let disposed = false
  let sequence = 0
  let activeSource: OperationNotice | undefined

  async function show(content: string, type: OperationMessageType, source?: OperationNotice) {
    if (disposed || !content) return
    const requested = ++sequence
    activeSource = source
    api.destroy()
    // Ant 4.2.6 的旧关闭动画会清理同位置的新提示；更换原生上下文使旧回调随实例结束。
    contextVersion.value++
    await nextTick()
    // 等待原生上下文挂载期间也可能重置会话或发布新结果，不能把旧结果补放到新上下文。
    if (disposed || requested !== sequence) return
    void api.open({ key: `operation-${requested}`, type, duration: 3, content: h('span', { role: 'status', 'aria-live': 'polite' }, content) })
  }

  function clear(source?: OperationNotice) {
    if (source && source !== activeSource) return
    sequence++
    activeSource = undefined
    api.destroy()
    contextVersion.value++
  }

  bindings.forEach(binding => {
    // 同步消费清空再写入，保证重复操作的相同文案仍有反馈；初始事实不重放成功提示。
    watch(binding.content, content => {
      if (!content) clear(binding)
      else void show(content, toValue(binding.type) ?? 'success', binding)
    }, { flush: 'sync' })
  })
  onScopeDispose(() => { disposed = true; sequence++; activeSource = undefined; api.destroy() })

  return {
    MessageContext,
    success: (content: string) => show(content, 'success'),
    info: (content: string) => show(content, 'info'),
    clear: () => clear(),
  }
}
