import { describe, expect, it } from 'vitest'

import type { DataSourceConnectionTest } from '../api/browser'
import { dataSourceConnectionTestNotice } from './dataSourceConnectionTestNotice'

function testResult(overrides: Partial<DataSourceConnectionTest> = {}): DataSourceConnectionTest {
  return {
    id: 'connection-test-1', status: 'FAILED', nodeId: 'node-1', verificationSource: 'AGENT_JDBC',
    realConnectionVerified: false, ...overrides,
  }
}

describe('dataSourceConnectionTestNotice', () => {
  it('TCP 不可达时说明网络监听阻断而不归因账号', () => {
    const notice = dataSourceConnectionTestNotice(testResult({ resultCode: 'DATABASE_TCP_UNREACHABLE' }))

    expect(notice).toContain('无法建立到目标数据库监听端口的 TCP 连接')
    expect(notice).toContain('尚未进入 JDBC 认证')
    expect(notice).not.toContain('密码错误')
  })

  it('TCP 已建立后的 JDBC 失败提示连接身份范围', () => {
    const notice = dataSourceConnectionTestNotice(testResult({ resultCode: 'DATABASE_CONNECTION_FAILED' }))

    expect(notice).toContain('已建立 TCP 连接')
    expect(notice).toContain('不会展示地址、账号、密码或异常原文')
  })

  it('连接被拒绝时说明目标监听范围', () => {
    const notice = dataSourceConnectionTestNotice(testResult({ resultCode: 'DATABASE_TCP_REFUSED' }))

    expect(notice).toContain('明确拒绝')
    expect(notice).toContain('ODP 监听服务')
    expect(notice).toContain('未进入 JDBC 认证')
  })
})
