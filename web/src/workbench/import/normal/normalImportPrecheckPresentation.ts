import type { NormalImportCheck } from '@/api/normalImport'
import { importPrecheckItems } from './normalImportOptions'

type CheckResult = NormalImportCheck['results'][number]

const reasons: Record<string, string> = {
  TOOL_RUNTIME_INVALID: 'Java、OBLOADER 或本机依赖布局不符合要求，请在执行节点中检查环境。',
  TOOL_RUNTIME_UNAVAILABLE: '未能取得执行工具环境证据，请检查节点状态后重试。',
  DATABASE_CONNECTION_FAILED: '目标数据库连接失败，请核对数据源配置和节点连通性后重试。',
  DATABASE_CONNECTION_UNAVAILABLE: '未能取得目标数据库连接证据，请检查节点状态与检查能力。',
  NODE_RESOURCE_INSUFFICIENT: '当前节点可用内存或任务资源额度不足，请调整执行参数或更换节点。',
  IMPORT_JVM_HEAP_TOO_SMALL: '包含数据的普通导入要求 JVM 堆至少 700 MiB，请在执行参数中调高 JVM 内存后重新检查；节点额度仍须另行满足。',
  NODE_RESOURCE_UNAVAILABLE: '未能取得当前节点资源事实，请刷新节点状态后重试。',
  IMPORT_REJECTED: '当前输入或配置未通过检查，请核对对应配置后重新检查。',
  IMPORT_UNAVAILABLE: '当前未能取得这项普通导入检查的有效证据，无法确认是否满足要求。',
  IMPORT_WRITE_PERMISSION_REJECTED: '当前账号的授权未覆盖本次导入所需的写入或对象定义权限，请核对数据源账号授权。',
  IMPORT_WRITE_PERMISSION_UNAVAILABLE: '未能可靠核实本次导入的有效权限，请核对账号与角色授权；预检查不会尝试写入。',
  IMPORT_INPUT_MAPPING_MISMATCH: '输入文件的列数或列名与目标结构不匹配，或遗漏了没有默认值的必填列。',
  IMPORT_INPUT_MAPPING_UNAVAILABLE: '未能取得当前输入与目标表结构的完整证据，请核对文件、DDL 与目标库表后重试。',
  IMPORT_INPUT_MAPPING_UNSUPPORTED: '当前文件编码、压缩或映射语法尚不能可靠检查，请核对解析与映射配置后重新检查。',
  IMPORT_WRITE_INSERT_REQUIRED: '当前可核实的授权未覆盖本次涉及表的 INSERT 权限。',
  IMPORT_WRITE_UPDATE_REQUIRED: '当前可核实的授权未覆盖替换数据所需的 UPDATE 权限。',
  IMPORT_WRITE_DELETE_REQUIRED: '当前可核实的授权未覆盖替换数据或删除清理所需的 DELETE 权限。',
  IMPORT_WRITE_CREATE_REQUIRED: '当前可核实的授权未覆盖输入建表定义所需的 CREATE 权限。',
  IMPORT_WRITE_ALTER_REQUIRED: '当前可核实的授权未覆盖本次涉及对象的 ALTER 权限。',
  IMPORT_WRITE_DROP_REQUIRED: '当前可核实的授权未覆盖删表定义、替换对象或截断清理所需的 DROP 权限。',
  IMPORT_WRITE_GRANTS_UNAVAILABLE: '读取当前账号的授权信息失败或未返回有效结果，尚不能判定是否具备写入权限。',
  IMPORT_WRITE_GRANTS_UNSUPPORTED: '当前授权包含尚不能可靠展开的角色、列级授权或通配范围，无法确认所需权限是否齐全。',
  IMPORT_WRITE_DDL_UNSUPPORTED: '输入包含尚不能完整核算权限的 DDL、外部对象引用或复杂表达式，请核对定义文件。',
  IMPORT_WRITE_INPUT_UNAVAILABLE: '输入文件结构证据不完整，尚不能确定本次各表所需的写入与建表权限。',
  IMPORT_WRITE_CLEANUP_SCOPE_UNAVAILABLE: '当前清理范围可能包含输入清单之外的目标表，尚未核对完整范围的权限；精确指定表且输入证据完整后再检查。',
  IMPORT_WRITE_SCHEMA_UNAVAILABLE: '无法从当前用户与系统授权中确认目标 Schema 的有效权限，跨 Schema 对象授权仍需核实。',
  IMPORT_TARGET_METADATA_UNAVAILABLE: '读取目标表列元信息失败或返回不完整，请核对元信息读取权限与节点连接状态。',
  IMPORT_TARGET_STRUCTURE_MISSING: '目标表没有可用列结构，输入中也没有可识别的同名建表定义；新表导入需要对应 DDL。',
  IMPORT_INPUT_DEFINITION_UNSUPPORTED: '同名建表定义未被完整识别，可能包含额外语句、限定库名不符或内容截断，无法确定目标列结构。',
  IMPORT_AUTO_COLUMNS_UNSUPPORTED: '自动列映射涉及目标结构之外的源列，当前检查器尚不能可靠确认此组合。',
  IMPORT_INPUT_FILES_CHANGED: '文件在发现后发生变化，或无法重新读取并复核摘要，请核对输入后重新预检查。',
  IMPORT_COLUMN_COUNT_MISMATCH: '首条数据记录的列数与当前映射后的目标列数不一致。',
  IMPORT_CSV_WHITESPACE_UNQUOTED: 'CSV 存在未用字符串包围符引用的首尾空白，当前工具会移除这些空白。请先将需要保留空白的字段正确加引号；只有明确需要删除空白时才启用“去除左右空格”并重新检查。',
  IMPORT_COLUMN_NAME_MISMATCH: '输入列名或显式列名单包含目标表中不存在的列。',
  IMPORT_REQUIRED_COLUMN_MISSING: '当前映射遗漏了没有默认值、非自增的必填目标列。',
}
const unconfiguredChecks: Record<string, string> = {
  DATABASE_CONNECTIVITY: '当前节点尚未启用普通导入数据库连接检查，请更新 Agent 并核对本机只读 JDBC 检查配置。',
  WRITE_ACCESS: '当前节点尚未启用普通导入写权限检查，请更新 Agent 并核对本机只读 JDBC 检查配置。',
  INPUT_MAPPING: '当前节点尚未启用普通导入映射检查，请更新 Agent 并核对本机只读 JDBC 检查配置。',
}

// 只呈现固定证据码的安全说明；失效、等待和前置阻断优先于旧的单项通过结果。
export function normalImportPrecheckPresentation(check: NormalImportCheck | null, result: CheckResult | undefined, expired: boolean) {
  if (!check) return { status: 'default', text: '未开始', detail: '' }
  if (expired || ['INVALIDATED', 'EXPIRED'].includes(check.status)) return { status: 'warning', text: '已失效', detail: '检查已过期或绑定事实已变化，请重新检查。' }
  if (['PENDING', 'LEASED'].includes(check.status)) return { status: 'processing', text: '等待检查结果', detail: '等待 Agent 返回当前配置的检查结果。' }
  if (!result) return { status: 'warning', text: '未返回结果', detail: '未取得本项结果，请重新检查。' }
  if (result.status === 'PASSED') return { status: 'success', text: '通过', detail: '' }
  if (result.status === 'FAILED') return { status: 'error', text: '未通过', detail: reasons[result.evidenceCode] ?? '检查未通过，请核对对应配置后重新检查。' }
  if (result.evidenceCode === 'PRECHECK_BLOCKED_BY_TOOL') return { status: 'warning', text: '前置阻断，未执行', detail: '执行工具检查未通过，当前检查尚未执行；请先检查执行节点的工具环境。' }
  if (result.evidenceCode === 'PRECHECK_BLOCKED_BY_DATABASE') return { status: 'warning', text: '前置阻断，未执行', detail: '目标数据库连接检查未通过，当前检查尚未执行。' }
  if (result.evidenceCode === 'NORMAL_IMPORT_PROBE_NOT_CONFIGURED') return { status: 'warning', text: '未启用', detail: unconfiguredChecks[result.check] ?? '当前节点尚未启用普通导入的这项检查，请核对 Agent 版本与配置。' }
  return { status: 'warning', text: '无法确认', detail: reasons[result.evidenceCode] ?? '未能取得有效检查证据，请重试或核对节点状态。' }
}

// 只按普通导入的固定六项汇总，不使用导出清单或客户端推测检查依赖。
export function normalImportBlockingSummary(check: NormalImportCheck) {
  const results = importPrecheckItems.map(item => check.results.find(result => result.check === item.check))
  const blocked = (result: CheckResult | undefined) => result?.status === 'UNKNOWN' && ['PRECHECK_BLOCKED_BY_TOOL', 'PRECHECK_BLOCKED_BY_DATABASE'].includes(result.evidenceCode)
  const failed = results.filter(result => result?.status === 'FAILED').length
  const blockedCount = results.filter(blocked).length
  const unknown = results.filter(result => !result || result.status === 'UNKNOWN' && !blocked(result)).length
  return [failed && `${failed} 项未通过`, blockedCount && `${blockedCount} 项因前置条件未执行`, unknown && `${unknown} 项无法确认`].filter(Boolean).join('，')
}

export function normalImportPrecheckFixAction(check: string, result: CheckResult | undefined) {
  if (result?.evidenceCode === 'PRECHECK_BLOCKED_BY_TOOL' || check === 'TOOL_ENVIRONMENT') return { step: 2, label: '检查执行节点' }
  if (result?.evidenceCode === 'PRECHECK_BLOCKED_BY_DATABASE' || ['DATABASE_CONNECTIVITY', 'WRITE_ACCESS'].includes(check)) return { step: 1, label: '检查数据源' }
  if (check === 'NODE_RESOURCE') return { step: 4, label: '修改执行参数' }
  if (check === 'INPUT_MAPPING') return { step: 3, label: '修改对象与映射' }
  return { step: 2, label: '修改输入文件' }
}
