import type { DirectLoadCheck } from '@/api/directLoad'

export const directLoadCheckItems = [
  { check: 'TOOL_ENVIRONMENT', name: '工具环境', step: 3 }, { check: 'NODE_RESOURCE', name: '节点资源', step: 3 },
  { check: 'INPUT_FILES', name: '单表文件集合', step: 3 }, { check: 'DATABASE_CONNECTIVITY', name: 'SQL 连接', step: 1 },
  { check: 'RPC_CONNECTIVITY', name: 'RPC 连接', step: 1 }, { check: 'DIRECT_LOAD_VERSION', name: '旁路版本矩阵', step: 1 },
  { check: 'WRITE_ACCESS', name: '目标表写入权限', step: 2 }, { check: 'DIRECT_LOAD_STRUCTURE', name: '结构与替换条件', step: 2 },
  { check: 'DIRECT_LOAD_CONFIG', name: 'Direct Load 节点配置', step: 4 },
] as const

/** 总览和步骤进度只消费旁路九项当前证据；过期、未返回和请求等待不能复用旧成功。 */
export function directLoadPrecheckProgress(check: DirectLoadCheck | undefined, expired: boolean) {
  if (!check || expired || ['INVALIDATED', 'EXPIRED', 'PENDING', 'LEASED'].includes(check.status)) return 0
  return Math.round(directLoadCheckItems.filter(item => check.results.find(result => result.check === item.check)?.status === 'PASSED').length * 100 / directLoadCheckItems.length)
}

/** 领域检查失败与证据不足均表达阻断，执行开关的说明由调用方单独保留。 */
export function directLoadPrecheckOverview(check: DirectLoadCheck | undefined, expired: boolean, checking: boolean) {
  if (check && (expired || ['INVALIDATED', 'EXPIRED'].includes(check.status))) return { type: 'warning' as const, message: '旁路预检查已失效，请重新检查' }
  if (checking || check && ['PENDING', 'LEASED'].includes(check.status)) return { type: 'info' as const, message: '正在核对旁路执行条件' }
  if (!check) return { type: 'info' as const, message: '旁路执行条件尚未全部确认' }
  if (check.results.some(result => result.status === 'FAILED')) return { type: 'error' as const, message: '旁路预检查未通过，提交已阻断' }
  if (check.status !== 'SUCCEEDED' || check.integrityStatus !== 'COMPLETE' || directLoadPrecheckProgress(check, false) !== 100) return { type: 'error' as const, message: '旁路预检查存在无法确认项，提交已阻断' }
  return { type: 'success' as const, message: '当前预检查通过' }
}

/** 安全原因只由固定证据码解释；未知不能映射为通过。 */
export function directLoadCheckPresentation(check: DirectLoadCheck | undefined, result: DirectLoadCheck['results'][number] | undefined, expired: boolean) {
  if (!check) return { status: 'default' as const, text: '未开始', detail: '等待检查结果。' }
  if (expired || ['INVALIDATED', 'EXPIRED'].includes(check.status)) return { status: 'warning' as const, text: '已失效', detail: '配置或证据已经变化，请重新预检查。' }
  if (['PENDING', 'LEASED'].includes(check.status)) return { status: 'processing' as const, text: '检查中', detail: '等待 Agent 返回受控事实。' }
  if (result?.status === 'PASSED') return { status: 'success' as const, text: '通过', detail: '当前检查证据已核验；不代表独立执行门禁已开放。' }
  const structureReasons: Record<string, string> = {
    DIRECT_LOAD_TABLE_NOT_FOUND: '目标表不存在，请核对 Schema 和表名；旁路不会创建表。',
    DIRECT_LOAD_BIT_BLOCKED: '目标表含 BIT 列，超出当前平台受控范围；请使用其他已验证导入方式。',
    DIRECT_LOAD_BINARY_BLOCKED: '目标表含受限二进制列（如 RAW、BINARY），超出当前平台受控范围；请使用其他已验证导入方式。',
    DIRECT_LOAD_VIRTUAL_BLOCKED: '目标表含虚拟生成列，当前旁路范围不允许该结构。',
    DIRECT_LOAD_REPLACE_INDEX_BLOCKED: '目标表非空且有索引，不能执行当前增量替换；请关闭“替换数据”并重新检查。',
    DIRECT_LOAD_REPLACE_LOB_BLOCKED: '目标表非空且有 LOB 列，不能执行当前增量替换；请关闭“替换数据”并重新检查。',
    DIRECT_LOAD_TABLE_FACT_UNAVAILABLE: '无法完整读取目标表空状态、列或索引事实；请核对 Schema、元数据权限和结构后重试。',
  }
  if (result?.check === 'DIRECT_LOAD_STRUCTURE' && structureReasons[result.evidenceCode]) return { status: result.status === 'FAILED' ? 'error' as const : 'warning' as const, text: result.status === 'FAILED' ? '未通过' : '无法确认', detail: structureReasons[result.evidenceCode]! }
  if (result?.evidenceCode.startsWith('PRECHECK_BLOCKED_BY_')) return { status: 'warning' as const, text: '前置阻断，未执行', detail: '依赖的工具或数据库检查未通过，本项未执行。' }
  if (result?.status === 'FAILED') return { status: 'error' as const, text: '未通过', detail: '当前检查事实未满足旁路要求，请修正后重试。' }
  return { status: 'warning' as const, text: '无法确认', detail: result?.evidenceCode === 'DIRECT_LOAD_PROBE_NOT_CONFIGURED' ? '旁路专用探针尚未装配，当前不能确认此项事实。' : '当前证据不足，不能提交。' }
}
