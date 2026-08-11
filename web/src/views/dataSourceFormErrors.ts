import type { ApiError, DataSourceWrite } from '@/api/browser'

const dataSourceFormFields = [
  'displayName',
  'environment',
  'compatibilityMode',
  'host',
  'port',
  'clusterName',
  'tenantName',
  'username',
  'defaultDatabase',
  'password',
  'sysUser',
  'sysPassword',
] as const

export type DataSourceFormField = typeof dataSourceFormFields[number]
export type DataSourceFormErrors = Partial<Record<DataSourceFormField, string>>
export type DataSourceFormInput = Pick<DataSourceWrite, DataSourceFormField>

export function validateDataSourceForm(input: DataSourceFormInput, isNew: boolean): DataSourceFormErrors {
  const errors: DataSourceFormErrors = {}
  if (!input.displayName.trim()) errors.displayName = '请输入数据源名称。'
  if (!input.host.trim()) errors.host = '请输入 ODP 地址。'
  if (!Number.isInteger(input.port) || input.port < 1 || input.port > 65535) errors.port = 'SQL 端口必须是 1 到 65535 之间的整数。'
  if (!input.clusterName.trim()) errors.clusterName = '请输入集群名称。'
  if (!input.tenantName.trim()) errors.tenantName = '请输入租户名称。'
  if (isNew && !input.username.trim()) errors.username = '新增数据源时必须填写用户名。'
  if (isNew && !input.password) errors.password = '新增数据源时必须填写密码。'
  // 可选的 sys 凭据必须成对提供（参考 ODC 数据源高级设置）：账号与密码要么同时填写，要么同时留空。
  if ((input.sysUser?.trim() ? true : false) !== Boolean(input.sysPassword)) {
    errors.sysPassword = 'sys 账号与密码必须同时填写或同时留空。'
  }
  return errors
}

export function dataSourceFieldErrorsFromApi(error: unknown): DataSourceFormErrors {
  const apiError = error as Partial<ApiError>
  const errors: DataSourceFormErrors = {}
  for (const item of apiError.fieldErrors ?? []) {
    if (!isDataSourceFormField(item.field)) continue
    errors[item.field] = item.message?.trim() || '请检查此字段。'
  }
  if (Object.keys(errors).length > 0) return errors
  if (apiError.code === 'DATA_SOURCE_NAME_UNAVAILABLE') {
    return { displayName: '数据源名称不可用，请更换后重试。' }
  }
  if (apiError.code === 'PASSWORD_REQUIRED') {
    return { password: '密码不能为空。' }
  }
  return errors
}

function isDataSourceFormField(value: string): value is DataSourceFormField {
  return (dataSourceFormFields as readonly string[]).includes(value)
}
