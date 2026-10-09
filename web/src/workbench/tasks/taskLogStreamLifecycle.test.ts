import { describe, expect, it } from 'vitest'

import { createTaskLogStreamLifecycle } from './taskLogStreamLifecycle'

describe('任务日志流生命周期', () => {
  it('打开过程同步断开时关闭返回句柄，旧事件资格立即失效', () => {
    const lifecycle = createTaskLogStreamLifecycle()
    const close = () => { closed++ }
    let closed = 0
    let current!: () => boolean
    let interrupted = 0
    expect(lifecycle.open((disconnect, accepts) => {
      current = accepts
      expect(current()).toBe(true)
      disconnect()
      return { close }
    }, () => { interrupted++ })).toBe(false)
    expect(current()).toBe(false)
    expect(lifecycle.active()).toBe(false)
    expect(closed).toBe(1)
    expect(interrupted).toBe(1)
  })

  it('关闭使当前事件资格失效，不因同任务重连重新有效', () => {
    const lifecycle = createTaskLogStreamLifecycle()
    let current!: () => boolean
    lifecycle.open((_disconnect, accepts) => { current = accepts; return { close() {} } }, () => {})
    lifecycle.close()
    lifecycle.open(() => ({ close() {} }), () => {})
    expect(current()).toBe(false)
    expect(lifecycle.active()).toBe(true)
  })
  it('当前连接断开后释放并允许按最后可靠游标重建', () => {
    const lifecycle = createTaskLogStreamLifecycle()
    const callbacks: Array<() => void> = []
    const first = { closed: false, close() { this.closed = true } }
    const second = { closed: false, close() { this.closed = true } }
    let interrupted = 0

    expect(lifecycle.open((onDisconnected) => {
      callbacks.push(onDisconnected)
      return first
    }, () => { interrupted++ })).toBe(true)
    callbacks[0]?.()

    expect(first.closed).toBe(true)
    expect(lifecycle.active()).toBe(false)
    expect(interrupted).toBe(1)
    expect(lifecycle.open((onDisconnected) => {
      callbacks.push(onDisconnected)
      return second
    }, () => { interrupted++ })).toBe(true)
    expect(lifecycle.active()).toBe(true)
  })

  it('陈旧断开回调不能关闭续传后的连接', () => {
    const lifecycle = createTaskLogStreamLifecycle()
    const callbacks: Array<() => void> = []
    const first = { closed: false, close() { this.closed = true } }
    const second = { closed: false, close() { this.closed = true } }

    lifecycle.open((onDisconnected) => {
      callbacks.push(onDisconnected)
      return first
    }, () => {})
    callbacks[0]?.()
    lifecycle.open((onDisconnected) => {
      callbacks.push(onDisconnected)
      return second
    }, () => {})
    callbacks[0]?.()

    expect(lifecycle.active()).toBe(true)
    expect(second.closed).toBe(false)
    callbacks[1]?.()
    expect(second.closed).toBe(true)
    expect(lifecycle.active()).toBe(false)
  })
})
