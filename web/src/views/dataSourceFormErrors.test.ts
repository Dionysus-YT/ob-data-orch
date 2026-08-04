import { describe, expect, it } from 'vitest'

import { dataSourceFieldErrorsFromApi, validateDataSourceForm } from './dataSourceFormErrors'

const validInput = {
  displayName: '合成数据源',
  environment: 'TEST' as const,
  connectionKind: 'ODP' as const,
  compatibilityMode: 'MYSQL' as const,
  host: '127.0.0.1',
  port: 2883,
  clusterName: 'synthetic-cluster',
  tenantName: 'synthetic-tenant',
  username: 'synthetic-user',
  defaultDatabase: '',
  password: 'synthetic-password',
}

describe('数据源表单字段错误', () => {
  it('把新增表单的本地校验绑定到对应字段', () => {
    expect(validateDataSourceForm({
      ...validInput,
      displayName: ' ',
      host: '',
      port: 65536,
      clusterName: '',
      tenantName: '',
      username: '',
      password: '',
    }, true)).toEqual({
      displayName: '请输入数据源名称。',
      host: '请输入 ODP 地址。',
      port: 'SQL 端口必须是 1 到 65535 之间的整数。',
      clusterName: '请输入集群名称。',
      tenantName: '请输入租户名称。',
      username: '新增数据源时必须填写用户名。',
      password: '新增数据源时必须填写密码。',
    })
  })

  it('允许编辑时不修改用户名和密码', () => {
    expect(validateDataSourceForm({ ...validInput, username: '', password: '' }, false)).toEqual({})
  })

  it('采用服务端字段错误，并为已知安全错误码回退到对应字段', () => {
    expect(dataSourceFieldErrorsFromApi({
      fieldErrors: [{ field: 'host', code: 'HOST_INVALID', message: '地址格式不正确。' }],
    })).toEqual({ host: '地址格式不正确。' })
    expect(dataSourceFieldErrorsFromApi({ code: 'DATA_SOURCE_NAME_UNAVAILABLE' })).toEqual({
      displayName: '数据源名称不可用，请更换后重试。',
    })
    expect(dataSourceFieldErrorsFromApi({ code: 'PASSWORD_REQUIRED' })).toEqual({ password: '密码不能为空。' })
  })

  it('不把无法识别的服务端字段错误放到错误位置', () => {
    expect(dataSourceFieldErrorsFromApi({
      fieldErrors: [{ field: 'unknownField', code: 'INVALID', message: '不应展示。' }],
    })).toEqual({})
  })
})
