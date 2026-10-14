import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref, type EffectScope } from 'vue'
import type { ExecutionNodeDetail } from '@/api/browser'
import type { NodeApi } from './nodeSession'
import { executionQuotaSegments, nodeDiskUsage, resourceCapacity, resourceCapacityLabel, resourcePercentage, resourceUsageColor } from './nodeResourcePresentation'
import { useNodeResourceSnapshots } from './useNodeResourceSnapshots'

const scopes: EffectScope[] = []
afterEach(() => { for (const scope of scopes.splice(0)) scope.stop() })
function node(id = 'synthetic-a', observedAt = '2026-10-09T00:00:00Z'): ExecutionNodeDetail {
  return { id, displayName: id, platform: 'WINDOWS_AMD64', managementState: 'ENABLED', agentAssociationStatus: 'ASSOCIATED', heartbeatStatus: 'ONLINE', lastHeartbeatAt: observedAt, environmentStatus: 'NORMAL', capacityStatus: 'AVAILABLE', acceptsNewTasks: true, unavailableReasons: [], revision: 1, updatedAt: observedAt, createdAt: observedAt, allowedRoots: ['/E:/synthetic'], toolHome: 'E:\\synthetic-tools', javaPath: 'C:\\synthetic-java', agentFacts: { os: 'windows', arch: 'amd64', agentVersion: 'synthetic', bootId: 'synthetic-boot', observedAt, capacityTotal: 1, capacityUsed: 0, cpuUsagePercent: 0, memoryUsagePercent: 50 }, dataRootUsages: [{ root: '/E:/synthetic', totalBytes: 100, availableBytes: 25 }] }
}
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(yes => { resolve = yes }); return { promise, resolve } }
async function flush() { for (let i = 0; i < 8; i++) await Promise.resolve() }
function mount(getExecutionNode: NodeApi['getExecutionNode'], initial = [node()]) {
  const scope = effectScope(); scopes.push(scope)
  const nodes = ref(initial)
  const signals: AbortSignal[] = []
  // 此只读能力仅使用详情方法，其他端口在本夹具中不可调用。
  const value = scope.run(() => useNodeResourceSnapshots(nodes, signal => { signals.push(signal); return { getExecutionNode } as NodeApi }))!
  return { nodes, value, signals, stop: () => scope.stop() }
}

describe('节点资源与额度的可信展示', () => {
  it('缺失、负值和非有限百分比保持未知，真实零值保持零', () => {
    for (const value of [undefined, -1, 101, NaN, Infinity]) expect(resourcePercentage(value)).toBeUndefined()
    expect(resourcePercentage(0)).toBe(0); expect(resourcePercentage(100)).toBe(100)
  })
  it('50% 进入黄色，80% 进入红色，未知和非法采样保持中性', () => {
    for (const value of [0, 49.99]) expect(resourceUsageColor(value)).toBe('var(--ob-color-primary-hover)')
    for (const value of [50, 79.99]) expect(resourceUsageColor(value)).toBe('color-mix(in srgb, var(--ob-foundation-warning-border) 70%, var(--ob-color-warning))')
    for (const value of [80, 100]) expect(resourceUsageColor(value)).toBe('var(--ob-color-danger)')
    for (const value of [undefined, -1, 101, NaN, Infinity]) expect(resourceUsageColor(value)).toBe('var(--ob-color-border)')
  })
  it('磁盘展示目录所在磁盘的最高使用率，不累加重复容量或丢弃异常目录', () => {
    const root = { root: '/E:/synthetic', totalBytes: 100, availableBytes: 25 }
    expect(nodeDiskUsage([root, { ...root, availableBytes: 10 }])).toEqual({ percent: 90, totalBytes: 100, usedBytes: 90 })
    expect(nodeDiskUsage([root, { ...root, totalBytes: 1000, availableBytes: 50 }])).toEqual({ percent: 95, totalBytes: 1000, usedBytes: 950 })
    expect(nodeDiskUsage([{ ...root, availableBytes: 100 }])).toEqual({ percent: 0, totalBytes: 100, usedBytes: 0 })
    for (const roots of [undefined, [], [{ ...root, totalBytes: 0 }], [{ ...root, availableBytes: -1 }], [root, { ...root, availableBytes: 101 }]]) expect(nodeDiskUsage(roots)).toBeUndefined()
  })
  it('已用与总量来自同一容量事实，耗尽和空闲零值保留，未知不反推容量', () => {
    const gib = 2 ** 30
    expect(resourceCapacityLabel(resourceCapacity(16 * gib, 4 * gib))).toBe('12.0 GiB / 16.0 GiB')
    expect(resourceCapacityLabel(resourceCapacity(16 * gib, 0))).toBe('16.0 GiB / 16.0 GiB')
    expect(resourceCapacityLabel(resourceCapacity(16 * gib, 16 * gib))).toBe('0.0 GiB / 16.0 GiB')
    expect(resourceCapacityLabel(resourceCapacity(100, 25))).toBe('75 B / 100 B')
    for (const [total, available] of [[undefined, 0], [100, undefined], [0, 0], [100, -1], [100, 101], [100, 0.5], [Infinity, 0], [Number.MAX_SAFE_INTEGER + 1, 0]]) {
      expect(resourceCapacity(total, available)).toBeUndefined()
      expect(resourceCapacityLabel(resourceCapacity(total, available))).toBeUndefined()
    }
  })
  it('预留与已用分段占用相加，不从固定容量或主机内存百分比推算', () => {
    const segments = executionQuotaSegments({ totalGiB: 20, reservedGiB: 5, usedGiB: 10 })!
    expect(segments.map(item => item.percent)).toEqual([50, 25, 25])
    expect(executionQuotaSegments({ totalGiB: 20, reservedGiB: 0, usedGiB: 0 })!.map(item => item.percent)).toEqual([0, 0, 100])
    for (const quota of [undefined, { totalGiB: 0, reservedGiB: 0, usedGiB: 0 }, { totalGiB: 20, reservedGiB: -1, usedGiB: 0 }, { totalGiB: 20, reservedGiB: 10, usedGiB: 11 }, { totalGiB: Infinity, reservedGiB: 0, usedGiB: 0 }]) expect(executionQuotaSegments(quota)).toBeUndefined()
  })
})

describe('磁盘快照读取生命周期', () => {
  it('同一采样缓存复用，采样变更清除旧值并拒绝迟到响应', async () => {
    const old = deferred<ExecutionNodeDetail>(); const fresh = deferred<ExecutionNodeDetail>()
    const get = vi.fn<NodeApi['getExecutionNode']>().mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise)
    const { nodes, value, signals } = mount(get)
    const next = node('synthetic-a', '2026-10-09T00:01:00Z'); nodes.value = [next]
    expect(signals.at(-2)?.aborted).toBe(true)
    fresh.resolve({ ...next, dataRootUsages: [{ root: '/E:/synthetic', totalBytes: 100, availableBytes: 50 }] }); await flush()
    expect(value.disks.value['synthetic-a']?.usage).toEqual({ percent: 50, totalBytes: 100, usedBytes: 50 })
    old.resolve(node()); await flush(); expect(value.disks.value['synthetic-a']?.usage).toEqual({ percent: 50, totalBytes: 100, usedBytes: 50 })
    nodes.value = [{ ...next }]; await flush(); expect(get).toHaveBeenCalledTimes(2)
  })
  it('列表失去授权与卸载清空缓存，中止请求且迟到事实不再回写', async () => {
    const late = deferred<ExecutionNodeDetail>(); const get = vi.fn<NodeApi['getExecutionNode']>().mockReturnValue(late.promise)
    const { nodes, value, signals, stop } = mount(get)
    nodes.value = []; late.resolve(node()); await flush(); expect(value.disks.value).toEqual({})
    nodes.value = [node()]; await flush(); expect(value.disks.value['synthetic-a']?.usage).toEqual({ percent: 75, totalBytes: 100, usedBytes: 75 })
    stop(); expect(signals.at(-1)?.aborted).toBe(true); expect(value.disks.value).toEqual({})
  })
  it('不接收其他节点、修订或采样的磁盘事实；读取失败不填零且刷新可重试', async () => {
    const get = vi.fn<NodeApi['getExecutionNode']>().mockResolvedValueOnce(node('synthetic-b')).mockResolvedValueOnce({ ...node(), revision: 2 }).mockResolvedValueOnce(node('synthetic-a', '2026-10-09T00:01:00Z')).mockRejectedValueOnce({ status: 403 }).mockResolvedValueOnce(node())
    const { nodes, value } = mount(get)
    for (let attempt = 0; attempt < 4; attempt++) { await flush(); expect(value.disks.value).toEqual({}); nodes.value = [node()] }
    await flush(); expect(value.disks.value['synthetic-a']?.usage).toEqual({ percent: 75, totalBytes: 100, usedBytes: 75 })
  })
  it('请求最多三个并发，未关联节点不请求详情，卸载后不再取队列', async () => {
    const first = deferred<ExecutionNodeDetail>(); const second = deferred<ExecutionNodeDetail>(); const third = deferred<ExecutionNodeDetail>()
    const get = vi.fn<NodeApi['getExecutionNode']>().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise).mockReturnValueOnce(third.promise).mockResolvedValue(node('synthetic-d'))
    const { value, stop } = mount(get, [node(), node('synthetic-b'), node('synthetic-c'), node('synthetic-d'), { ...node('synthetic-e'), agentAssociationStatus: 'PENDING' }])
    expect(get).toHaveBeenCalledTimes(3)
    first.resolve(node()); await flush(); expect(get).toHaveBeenCalledTimes(4)
    stop(); second.resolve(node('synthetic-b')); third.resolve(node('synthetic-c')); await flush()
    expect(get).toHaveBeenCalledTimes(4); expect(value.disks.value).toEqual({})
  })
})
