import { permissionCapabilities, permissionRoles, permissionScopeKey, permissionScopeTypes, type PermissionAssignment, type PermissionCapability, type PermissionProfile, type PermissionRole, type PermissionScope, type PermissionScopeType } from '@/api/accessControl'

export const roleLabels: Record<PermissionRole, string> = { ROLE_BASIC_USER: '普通用户', ROLE_OPERATOR: '运维人员', ROLE_DATA_SOURCE_ADMIN: '数据源管理员', ROLE_NODE_ADMIN: '节点管理员', ROLE_SYSTEM_ADMIN: '系统管理员' }
export const roleDescriptions: Record<PermissionRole, string> = { ROLE_BASIC_USER: '使用明确授权的数据源、节点与模板创建任务，查看本人任务。', ROLE_OPERATOR: '按明确范围运维任务和管理模板；创建任务另需兼任普通用户。', ROLE_DATA_SOURCE_ADMIN: '管理授权数据源及固定连接诊断，不因管理权取得任务创建资格。', ROLE_NODE_ADMIN: '管理授权节点和环境，不因此取得节点上业务任务内容。', ROLE_SYSTEM_ADMIN: '管理设置和权限；审计另需显式授权，不天然取得业务对象、生产或敏感命令权限。' }
export const capabilityLabels: Record<PermissionCapability, string> = { CAP_PRODUCTION_TASK: '生产任务', CAP_EXPERT_CONFIG: '专家配置', CAP_SENSITIVE_COMMAND: '敏感命令', CAP_AUDIT_LOG: '审计访问', CAP_GLOBAL_SCHEDULING_LOG: '调度日志' }
export const scopeLabels: Record<PermissionScopeType, string> = { DATA_SOURCE_USE: '数据源使用', DATA_SOURCE_MANAGE: '数据源管理', NODE_USE: '节点使用 / 连接诊断', NODE_OPERATE: '节点运行操作', NODE_MANAGE: '节点管理', TASK_OPERATE_BY_DATA_SOURCE: '任务运维来源', TEMPLATE_USE: '模板使用', TEMPLATE_MANAGE: '模板管理' }
export const accountLabels = { ACTIVE: '已启用', DISABLED: '已停用', UNKNOWN: '状态未知' } as const
/** 消费链路未收口的授权仅允许保留或撤销，不在首批生成潜伏的新权限。 */
export const scopeAdditionGated = (type: PermissionScopeType) => type === 'NODE_OPERATE'

export interface PermissionForm { roles: PermissionRole[]; capabilities: PermissionCapability[]; scopes: PermissionScope[]; reason: string }
function hasControl(value: string) { return Array.from(value).some(char => char.charCodeAt(0) < 32 || char.charCodeAt(0) >= 127 && char.charCodeAt(0) <= 159) }
export function canonicalAssignment(value: PermissionAssignment): { roles: PermissionRole[]; capabilities: PermissionCapability[]; scopes: PermissionScope[] } { return { roles: [...value.roles].sort(), capabilities: [...value.capabilities].sort(), scopes: value.scopes.map(scope => ({ type: scope.type, objectId: scope.objectId })).sort((a, b) => permissionScopeKey(a).localeCompare(permissionScopeKey(b))) } }
export function assignmentFingerprint(value: PermissionAssignment): string { return JSON.stringify(canonicalAssignment(value)) }
export function formFingerprint(value: PermissionForm): string { return JSON.stringify({ ...canonicalAssignment(value), reason: value.reason.trim() }) }
export function scopeRoleAllowed(roles: readonly PermissionRole[], type: PermissionScopeType): boolean {
  switch (type) {
    case 'DATA_SOURCE_USE': case 'TEMPLATE_USE': return roles.includes('ROLE_BASIC_USER')
    case 'NODE_USE': return roles.includes('ROLE_BASIC_USER') || roles.includes('ROLE_DATA_SOURCE_ADMIN')
    case 'DATA_SOURCE_MANAGE': return roles.includes('ROLE_DATA_SOURCE_ADMIN')
    case 'NODE_MANAGE': return roles.includes('ROLE_NODE_ADMIN')
    case 'NODE_OPERATE': case 'TASK_OPERATE_BY_DATA_SOURCE': case 'TEMPLATE_MANAGE': return roles.includes('ROLE_OPERATOR')
  }
}
export function capabilityRoleAllowed(roles: readonly PermissionRole[], capability: PermissionCapability): boolean { if (capability === 'CAP_EXPERT_CONFIG') return roles.includes('ROLE_BASIC_USER'); if (capability === 'CAP_PRODUCTION_TASK' || capability === 'CAP_SENSITIVE_COMMAND') return roles.includes('ROLE_BASIC_USER') || roles.includes('ROLE_OPERATOR'); return true }

/** 依赖清理只修改当前草稿，差异仍会在保存确认中明确展示。 */
export function cleanRoleDependencies(form: PermissionForm) { form.scopes = form.scopes.filter(scope => scopeRoleAllowed(form.roles, scope.type)); form.capabilities = form.capabilities.filter(capability => capabilityRoleAllowed(form.roles, capability)) }
export function permissionFormError(form: PermissionForm, original: PermissionProfile): string {
  if (form.roles.length > 5 || form.capabilities.length > 5 || form.scopes.length > 200 || form.roles.some(role => !permissionRoles.includes(role)) || form.capabilities.some(capability => !permissionCapabilities.includes(capability))) return '授权配置超出固定角色或范围边界。'
  if (new Set(form.roles).size !== form.roles.length || new Set(form.capabilities).size !== form.capabilities.length || new Set(form.scopes.map(permissionScopeKey)).size !== form.scopes.length) return '角色、能力或对象范围不能重复。'
  if (form.scopes.some(scope => !permissionScopeTypes.includes(scope.type) || !scopeRoleAllowed(form.roles, scope.type) || !scope.objectId || hasControl(scope.objectId) || Array.from(scope.objectId).some(char => /\s/.test(char) || '*?[]/\\'.includes(char))) || form.capabilities.some(capability => !capabilityRoleAllowed(form.roles, capability))) return '请先配置依赖角色，并使用明确的对象范围。'
  if (original.subject.accountStatus !== 'ACTIVE' && (form.roles.some(role => !original.assignment.roles.includes(role)) || form.capabilities.some(capability => !original.assignment.capabilities.includes(capability)) || form.scopes.some(scope => !original.assignment.scopes.some(item => permissionScopeKey(item) === permissionScopeKey(scope))))) return '当前账号只能撤销原有授权。'
  if (form.capabilities.some(value => value !== 'CAP_AUDIT_LOG' && !original.assignment.capabilities.includes(value)) || form.scopes.some(value => scopeAdditionGated(value.type) && !original.assignment.scopes.some(item => permissionScopeKey(item) === permissionScopeKey(value)))) return '该能力尚未开放新增授权，只能保留或撤销已有记录。'
  return ''
}
export function permissionReasonError(value: string): string { const count = Array.from(value.trim()).length; if (count < 1) return '请填写变更原因。'; if (count > 200) return '变更原因最多 200 个字符。'; if (hasControl(value) || /(?:password|secret|token|access[_-]?key)\s*[:=]|-----BEGIN/i.test(value)) return '原因只填写职责和范围调整说明，不填写秘密或认证材料。'; return '' }
export function permissionDiff(before: PermissionAssignment, after: PermissionAssignment) {
  const addedRoles = after.roles.filter(value => !before.roles.includes(value)); const removedRoles = before.roles.filter(value => !after.roles.includes(value))
  const addedCapabilities = after.capabilities.filter(value => !before.capabilities.includes(value)); const removedCapabilities = before.capabilities.filter(value => !after.capabilities.includes(value))
  const addedScopes = after.scopes.filter(value => !before.scopes.some(item => permissionScopeKey(item) === permissionScopeKey(value))); const removedScopes = before.scopes.filter(value => !after.scopes.some(item => permissionScopeKey(item) === permissionScopeKey(value)))
  return { addedRoles, removedRoles, addedCapabilities, removedCapabilities, addedScopes, removedScopes, retainedScopes: after.scopes.filter(value => before.scopes.some(item => permissionScopeKey(item) === permissionScopeKey(value))), risk: addedRoles.includes('ROLE_SYSTEM_ADMIN') || addedCapabilities.some(value => ['CAP_PRODUCTION_TASK', 'CAP_SENSITIVE_COMMAND', 'CAP_AUDIT_LOG'].includes(value)) }
}
export function scopeSummary(value: PermissionAssignment): string { const count = (types: readonly PermissionScopeType[]) => new Set(value.scopes.filter(item => types.includes(item.type)).map(item => item.objectId)).size; return `数据源 ${count(['DATA_SOURCE_USE', 'DATA_SOURCE_MANAGE'])} · 节点 ${count(['NODE_USE', 'NODE_OPERATE', 'NODE_MANAGE'])} · 任务来源 ${count(['TASK_OPERATE_BY_DATA_SOURCE'])} · 模板 ${count(['TEMPLATE_USE', 'TEMPLATE_MANAGE'])}` }
export function permissionTime(value?: string): string { if (!value || !Number.isFinite(Date.parse(value))) return '—'; return new Intl.DateTimeFormat('zh-CN', { timeZone: 'UTC', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' }).format(new Date(value)).replaceAll('/', '-') }
export function permissionDenied(error: unknown): boolean { const status = (error as { status?: number } | undefined)?.status; return status === 401 || status === 403 || status === 404 }
export function permissionError(error: unknown, fallback: string): string { const value = (error && typeof error === 'object' ? error : {}) as { status?: number; code?: string }; if (value.status === 401) return '登录或页面安全校验已失效，请刷新后重试。'; if (value.status === 403 || value.status === 404) return '权限接口不可用，或当前身份已无访问资格。请刷新后核对授权。'; if (value.status === 412) return '权限版本已变化，请刷新详情后重新比较；未覆盖其他变更。'; if (value.code === 'IDEMPOTENCY_CONFLICT') return '请求标识与本次变更不一致，请刷新详情后重新比较。'; if (value.code === 'PERMISSION_SUBJECT_INACTIVE') return '账号状态已变化，只能撤销原有授权，请刷新详情。'; if (value.code === 'PERMISSION_CONFIRMATION_REQUIRED') return '请重新核对主体、权限差异与风险并确认。'; if (value.code === 'PERMISSION_FIELDS_INVALID') return '角色依赖、对象状态或变更原因无效，请检查后重试。'; if (value.code === 'RESPONSE_INVALID') return '权限数据无法校验，请核对网页与控制面版本。'; return fallback }
