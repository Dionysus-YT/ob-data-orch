import { describe, expect, it } from 'vitest'

import type { DataSourceConnectionTest } from '../api/browser'
import { dataSourceConnectionTestDiagnostic } from './dataSourceConnectionTestDiagnostic'

function result(resultCode: string, status: DataSourceConnectionTest['status'] = 'FAILED'): DataSourceConnectionTest {
  return { id: 'test-1', status, nodeId: 'node-1', resultCode, verificationSource: 'AGENT_JDBC', realConnectionVerified: false, sysCredentialConfigured: false }
}

describe('dataSourceConnectionTestDiagnostic', () => {
  it('JDBC 失败定位到连接身份模块但不伪造具体凭据根因', () => {
    const diagnostic = dataSourceConnectionTestDiagnostic(result('DATABASE_CONNECTION_FAILED'))

    expect(diagnostic?.stage).toBe('JDBC 会话建立')
    expect(diagnostic?.target).toBe('connection')
    expect(diagnostic?.summary).toContain('无法安全区分')
    expect(diagnostic?.summary).not.toContain('密码错误')
  })

  it('DNS 失败明确尚未进入身份认证', () => {
    const diagnostic = dataSourceConnectionTestDiagnostic(result('DATABASE_HOST_UNRESOLVABLE'))

    expect(diagnostic?.moduleLabel).toContain('ODP 地址')
    expect(diagnostic?.summary).toContain('尚未进入')
  })

  it('未知结果定位到执行节点探针', () => {
    const diagnostic = dataSourceConnectionTestDiagnostic(result('DATABASE_CONNECTION_UNAVAILABLE', 'UNKNOWN'))

    expect(diagnostic?.target).toBe('execution-node')
  })

  it('成功结果不生成故障诊断', () => {
    expect(dataSourceConnectionTestDiagnostic(result('DATABASE_CONNECTED', 'SUCCEEDED'))).toBeUndefined()
  })
})
