import { ref, watch, type Ref } from 'vue'
import type { ExecutionNodeSummary } from '@/api/browser'
import { useNodeSession, type NodeApiFactory } from './nodeSession'
import { nodeDiskUsage, nodeSnapshotKey, type NodeDiskUsage } from './nodeResourcePresentation'

interface DiskSnapshot { key: string; usage: NodeDiskUsage | undefined }

// 只补读已授权节点的磁盘事实，不复制节点详情、路径或表单；刷新复用同一采样的缓存。
export function useNodeResourceSnapshots(nodes: Readonly<Ref<ExecutionNodeSummary[]>>, createApi?: NodeApiFactory) {
  const generation = ref('0')
  const session = useNodeSession(generation, createApi)
  const disks = ref<Record<string, DiskSnapshot>>({})
  session.onReset(() => { if (!session.active()) disks.value = {} })
  watch(nodes, current => {
    generation.value = String(Number(generation.value) + 1)
    const active = session.capture()
    const cache: Record<string, DiskSnapshot> = {}
    for (const node of current) {
      const cached = disks.value[node.id]
      if (cached?.key === nodeSnapshotKey(node)) cache[node.id] = cached
    }
    disks.value = cache
    const pending = current.filter(node => node.agentAssociationStatus === 'ASSOCIATED' && node.agentFacts && !cache[node.id])
    // 每代最多三个详情请求；新列表中止旧代等待，旧响应即使抵达也不能发布。
    async function readNext() {
      while (active() && pending.length) {
        const node = pending.shift()!
        try {
          const detail = await session.api.getExecutionNode(node.id)
          if (!active()) return
          if (nodeSnapshotKey(detail) !== nodeSnapshotKey(node)) continue
          disks.value = { ...disks.value, [node.id]: { key: nodeSnapshotKey(node), usage: nodeDiskUsage(detail.dataRootUsages) } }
        } catch {
          // 缺失或拒绝的详情保持未知；不能把读取失败转为 0%，下一次列表刷新可重试。
          if (!active()) return
        }
      }
    }
    const workers = Math.min(3, pending.length)
    for (let worker = 0; worker < workers; worker++) void readNext()
  }, { immediate: true, flush: 'sync' })
  return { disks }
}
