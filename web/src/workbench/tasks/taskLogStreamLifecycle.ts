// TaskLogStreamHandle 是页面持有的最小日志流关闭能力。
// 页面只依赖关闭语义，不能借此读取、写入或重放任何日志正文。
export interface TaskLogStreamHandle {
  close(): void
}

// TaskLogStreamLifecycle 管理单任务页面的当前 SSE 连接。
// 断开时只释放当前代连接；陈旧回调不能关闭已经按最后可靠游标重新建立的新连接。
export interface TaskLogStreamLifecycle {
  readonly active: () => boolean
  open(factory: (onDisconnected: () => void, isCurrent: () => boolean) => TaskLogStreamHandle, onDisconnected: () => void): boolean
  close(): void
}

// createTaskLogStreamLifecycle 创建页面私有的单流监督器，不在组件间共享连接状态。
export function createTaskLogStreamLifecycle(): TaskLogStreamLifecycle {
  let stream: TaskLogStreamHandle | undefined
  let generation = 0

  return {
    active: () => stream !== undefined,
    open(factory, onDisconnected) {
      if (stream !== undefined) return true
      const currentGeneration = ++generation
      let disconnectedDuringOpen = false
      const created = factory(() => {
        if (generation !== currentGeneration) return
        disconnectedDuringOpen = true
        generation++
        stream?.close()
        stream = undefined
        onDisconnected()
      }, () => generation === currentGeneration && !disconnectedDuringOpen)
      if (disconnectedDuringOpen) {
        created.close()
        return false
      }
      stream = created
      return true
    },
    close() {
      generation++
      stream?.close()
      stream = undefined
    },
  }
}
