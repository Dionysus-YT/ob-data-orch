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
      tenantName: '请输入租户名称。',
      username: '请输入用户名。',
      password: '新增数据源时必须填写密码。',
    })
  })

  it('集群名留空也可通过本地校验', () => {
    expect(validateDataSourceForm({ ...validInput, clusterName: '' }, true)).toEqual({})
  })

  it('未选择环境时不能保存，默认提示不是环境值', () => {
    expect(validateDataSourceForm({ ...validInput, environment: undefined }, true)).toEqual({ environment: '请选择环境。' })
  })

  it('编辑时仍要求保留结构化用户名，但允许不修改密码', () => {
    expect(validateDataSourceForm({ ...validInput, username: '', password: '' }, false)).toEqual({
      username: '请输入用户名。',
    })
  })

  it('sys 凭据必须成对填写（参考 ODC 数据源高级设置）', () => {
    // 成对填写通过。
    expect(validateDataSourceForm({ ...validInput, sysUser: 'root', sysPassword: 'synthetic-sys-password' }, true)).toEqual({})
    // 只填账号或只填密码均拒绝。
    expect(validateDataSourceForm({ ...validInput, sysUser: 'root', sysPassword: '' }, true)).toEqual({
      sysPassword: 'sys 账号与密码必须同时填写或同时留空。',
    })
    expect(validateDataSourceForm({ ...validInput, sysUser: '', sysPassword: 'synthetic-sys-password' }, true)).toEqual({
      sysPassword: 'sys 账号与密码必须同时填写或同时留空。',
    })
    // 同时留空通过（可选项）。
    expect(validateDataSourceForm({ ...validInput, sysUser: '', sysPassword: '' }, true)).toEqual({})
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
