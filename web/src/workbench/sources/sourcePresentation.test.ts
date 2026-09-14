import { describe, expect, it } from 'vitest'
import { DATA_SOURCE_UI_FIXTURES } from '@/views/dataSourceUiFixture'
import { blankSourceForm, connectionFact, connectionListFact, formatVerifiedTime, invalidatesConnection, sourceUpdate } from './sourcePresentation'

describe('新工作区的数据源呈现边界', () => {
  const source = DATA_SOURCE_UI_FIXTURES[0]!
  const form = () => ({ ...blankSourceForm(), displayName: source.displayName, environment: 'PRODUCTION' as const, host: source.host, port: source.port, clusterName: source.clusterName, tenantName: source.tenantName, username: source.username, defaultDatabase: source.defaultDatabase })
  it('没有改动时不轮换或回传密码', () => { expect(sourceUpdate(form(), source, false)).toEqual({}) })
  it('名称和环境调整不导致连接结论失效', () => {
    const update = sourceUpdate({ ...form(), displayName: 'visual-validation-renamed', environment: 'TEST' }, source, false)
    expect(update).toEqual({ displayName: 'visual-validation-renamed', environment: 'TEST' })
    expect(invalidatesConnection(update)).toBe(false)
  })
  it('主机与凭据调整导致失效，保留 API 参数语义', () => {
    const update = sourceUpdate({ ...form(), host: '198.51.100.12', password: 'synthetic-test-only' }, source, false)
    expect(update.host).toBe('198.51.100.12')
    expect(invalidatesConnection(update)).toBe(true)
  })
  it('Oracle 不携带旧的 MySQL 默认数据库', () => { expect(sourceUpdate({ ...form(), compatibilityMode: 'ORACLE' }, source, false)).toMatchObject({ compatibilityMode: 'ORACLE', defaultDatabase: '' }) })
  it('清除 sys 凭据与留空保留不同', () => {
    expect(sourceUpdate(form(), source, false)).not.toHaveProperty('sysPassword')
    expect(sourceUpdate(form(), source, true)).toEqual({ sysUser: '', sysPassword: '' })
  })
  it('列表成功不能被包装为真实资格通过', () => { expect(connectionFact(source)).toMatchObject({ label: '最近测试成功', tone: 'neutral' }) })
  it('精简列表成功使用成功色，未知和失效保持独立', () => {
    expect(connectionListFact(source)).toEqual({ label: '测试成功', tone: 'success' })
    expect(connectionListFact({ lastTestStatus: 'UNRECOGNIZED' })).toEqual({ label: '待确认', tone: 'neutral' })
    const statuses = [undefined, 'UNKNOWN', 'INVALIDATED', 'EXPIRED', 'PENDING', 'FAILED', 'SUCCEEDED']
    expect(new Set(statuses.map((lastTestStatus) => connectionListFact({ lastTestStatus }).label)).size).toBe(7)
    expect(connectionListFact({ lastTestStatus: 'LEASED' })).toEqual(connectionListFact({ lastTestStatus: 'PENDING' }))
  })
  it('未知、未测、失效、进行中与失败保持独立文本', () => {
    const statuses = [undefined, 'UNKNOWN', 'INVALIDATED', 'PENDING', 'FAILED']
    expect(new Set(statuses.map((lastTestStatus) => connectionFact({ lastTestStatus }).label)).size).toBe(5)
  })
  it('无效测试时间不伪装为当前时间', () => { expect(formatVerifiedTime('invalid')).toBe('验证时间未提供'); expect(formatVerifiedTime()).toBe('验证时间未提供') })
})
