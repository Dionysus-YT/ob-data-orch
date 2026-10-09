import { afterEach, describe, expect, it, vi } from 'vitest'
import { DATA_SOURCE_UI_FIXTURES } from '@/workbench/sources/dataSourceUiFixture'
import { createVisualSourceGateway } from './sourceGateway'
import { blankSourceForm } from './sourcePresentation'

describe('隔离的视觉验证数据源适配器', () => {
  afterEach(() => vi.useRealTimers())
  it('返回副本，不改变固定 P3 形状样本', async () => {
    const api = createVisualSourceGateway(DATA_SOURCE_UI_FIXTURES)
    const rows = await api.listDataSources()
    rows.pop()
    expect(await api.listDataSources()).toHaveLength(DATA_SOURCE_UI_FIXTURES.length)
    await api.updateDataSource(DATA_SOURCE_UI_FIXTURES[0]!.id, 101, { displayName: 'visual-validation-rename' })
    expect(DATA_SOURCE_UI_FIXTURES[0]!.displayName).toBe('Production finance reporting')
  })
  it('使用现有修订拒绝覆盖新配置', async () => {
    const api = createVisualSourceGateway(DATA_SOURCE_UI_FIXTURES)
    await expect(api.updateDataSource(DATA_SOURCE_UI_FIXTURES[0]!.id, 1, { host: '198.51.100.1' })).rejects.toMatchObject({ status: 412 })
  })
  it('创建只保留公开投影，初始不可用且未经测试', async () => {
    const api = createVisualSourceGateway([])
    const id = await api.createDataSource({ ...blankSourceForm(), environment: 'TEST', displayName: 'visual-validation-created', username: 'visual-user', password: 'synthetic-primary-only', sysUser: 'visual-sys', sysPassword: 'synthetic-sys-only' })
    const result = await api.getDataSource(id)
    expect(result).toMatchObject({ state: 'DISABLED', sysCredentialState: 'AVAILABLE' })
    expect(result).not.toHaveProperty('lastTestStatus')
    for (const key of ['password', 'sysPassword', 'sysUser']) expect(result).not.toHaveProperty(key)
    expect(JSON.stringify(await api.listDataSources())).not.toContain('synthetic-')
  })
  it('名称保存保留测试事实，连接字段保存使结论失效', async () => {
    const api = createVisualSourceGateway(DATA_SOURCE_UI_FIXTURES)
    const id = DATA_SOURCE_UI_FIXTURES[0]!.id
    const renamed = await api.updateDataSource(id, 101, { displayName: 'visual-validation-name' })
    expect(renamed.lastTestStatus).toBe('SUCCEEDED')
    const changed = await api.updateDataSource(id, 102, { password: 'synthetic-rotation-only' })
    expect(changed.lastTestStatus).toBe('INVALIDATED')
    expect(changed).not.toHaveProperty('password')
  })
  it('不能根据 ENABLED 或无引用自行生成生命周期资格', async () => {
    const api = createVisualSourceGateway(DATA_SOURCE_UI_FIXTURES)
    const withoutEligibility = DATA_SOURCE_UI_FIXTURES[1]!
    await expect(api.deleteDataSource(withoutEligibility.id, withoutEligibility.revision)).rejects.toMatchObject({ status: 409 })
    await expect(api.changeDataSourceState(withoutEligibility.id, withoutEligibility.revision, 'DISABLED')).rejects.toMatchObject({ status: 409 })
  })
  it('测试只产生 schema-bound 合成诊断，不形成真实资格或自动启用', async () => {
    vi.useFakeTimers()
    const row = DATA_SOURCE_UI_FIXTURES[4]!
    const api = createVisualSourceGateway([row])
    const request = await api.startDataSourceConnectionTest(row.id, row.revision, 'visual-validation-runtime')
    expect((await api.getDataSourceConnectionTest(request.id)).status).toBe('PENDING')
    vi.advanceTimersByTime(2000)
    const result = await api.getDataSourceConnectionTest(request.id)
    expect(result).toMatchObject({ status: 'FAILED', resultCode: 'DATABASE_TCP_TIMEOUT', verificationSource: 'G2_SYNTHETIC', realConnectionVerified: false })
    expect((await api.getDataSource(row.id)).state).toBe('DISABLED')
  })
  it('拒绝未知执行节点和已变更的测试配置', async () => {
    const row = DATA_SOURCE_UI_FIXTURES[0]!
    const api = createVisualSourceGateway([row])
    await expect(api.startDataSourceConnectionTest(row.id, row.revision, 'unrecognized')).rejects.toMatchObject({ status: 409 })
    const request = await api.startDataSourceConnectionTest(row.id, row.revision, 'visual-validation-runtime')
    await api.updateDataSource(row.id, row.revision, { host: '198.51.100.22' })
    expect((await api.getDataSourceConnectionTest(request.id)).status).toBe('INVALIDATED')
  })
})
