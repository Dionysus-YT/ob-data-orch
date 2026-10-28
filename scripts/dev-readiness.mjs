import { performance } from 'node:perf_hooks'
import { setTimeout as delay } from 'node:timers/promises'

// 等待实际就绪，给 SQLite 校验和迁移留出时间；使用单调时钟限制总等待。
export async function waitForDevelopmentHealth({ probe, exited, canceled = () => false, now = () => performance.now(), sleep = delay, timeoutMs = 120000 }) {
  const deadline = now() + timeoutMs
  let lastError = '尚未收到健康响应'
  while (now() < deadline) {
    if (canceled()) throw new Error('开发控制面启动已取消')
    const exit = exited()
    if (exit !== null) throw new Error(`开发控制面在就绪前退出（${exit}），请检查上方控制面日志`)
    try {
      await probe(Math.min(2000, deadline - now()))
      return
    } catch (error) {
      lastError = error.message
    }
    await sleep(Math.min(250, Math.max(0, deadline - now())))
  }
  throw new Error(`开发控制面健康检查失败：等待 ${timeoutMs / 1000} 秒仍未就绪；最后检查：${lastError}。即将正常停止进程，随后出现的 context canceled 可能由停止触发；请检查此前日志`)
}
