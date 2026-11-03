// 模板写入只进行本地事务；界面等待有界，超时并不代表服务端未接受。
// 保留原请求身份的责任仍在当前业务会话，本函数不自动重试或保存业务状态。
export async function withTemplateWriteDeadline<T>(operation: () => Promise<T>, signal?: AbortSignal, timeoutMs = 30_000): Promise<T> {
  const unknown = { status: 0, code: 'TEMPLATE_WRITE_UNCONFIRMED' }
  if (signal?.aborted) throw unknown
  let timer: ReturnType<typeof setTimeout> | undefined
  let stop: (() => void) | undefined
  const deadline = new Promise<never>((resolve, reject) => {
    void resolve
    stop = () => reject(unknown)
    timer = setTimeout(stop, timeoutMs)
    signal?.addEventListener('abort', stop, { once: true })
  })
  try { return await Promise.race([operation(), deadline]) }
  finally {
    clearTimeout(timer)
    if (stop) signal?.removeEventListener('abort', stop)
  }
}
