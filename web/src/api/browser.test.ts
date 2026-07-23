import { describe, expect, it } from 'vitest'

import { createBrowserApi, dataSourceErrorMessage, type FetchLike } from './browser'

function apiWith(response: Response, csrfToken = 'synthetic-csrf-token') {
  const calls: Array<{ path: string; init: RequestInit }> = []
  const fetcher = (async (path: string | URL | Request, init?: RequestInit) => {
    calls.push({ path: String(path), init: init ?? {} })
    return response
  }) as FetchLike
  return {
    api: createBrowserApi({ fetcher, csrfToken: () => csrfToken, idempotencyKey: () => 'synthetic-idempotency-key' }),
    calls,
  }
}

describe('浏览器 API 客户端', () => {
  it('只保留数据源脱敏摘要字段', async () => {
    const { api } = apiWith(Response.json({
      items: [{
        id: 'source-1',
        displayName: '合成数据源',
        environment: 'TEST',
        connectionKind: 'ODP',
        compatibilityMode: 'MYSQL',
        host: '127.0.0.1',
        port: 2881,
        state: 'ENABLED',
        revision: 1,
        credentialRevision: 1,
      }],
    }))

    const [source] = await api.listDataSources()

    expect(source).toEqual({
      id: 'source-1',
      displayName: '合成数据源',
      environment: 'TEST',
      connectionKind: 'ODP',
      compatibilityMode: 'MYSQL',
      host: '127.0.0.1',
      port: 2881,
      state: 'ENABLED',
      revision: 1,
      credentialRevision: 1,
    })
    expect('username' in source).toBe(false)
  })

  it('为创建草稿添加 CSRF 与幂等键', async () => {
    const { api, calls } = apiWith(Response.json({ id: 'draft-1' }))

    await api.createExportDraft({
      dataSourceId: 'source-1',
      nodeId: 'node-1',
      database: 'synthetic_db',
      table: 'synthetic_table',
      format: 'CSV',
      filePath: 'E:\\tmp\\output',
    })

    expect(calls[0]?.path).toBe('/api/v1/export-drafts')
    expect(calls[0]?.init.headers).toMatchObject({
      'X-CSRF-Token': 'synthetic-csrf-token',
      'Idempotency-Key': 'synthetic-idempotency-key',
    })
  })

  it('连接测试只向控制面发起受 CSRF 保护的动作', async () => {
    const { api, calls } = apiWith(Response.json({ status: 'PENDING', code: 'AGENT_CONNECTION_TEST_QUEUED' }))

    await expect(api.testDataSourceConnection('source-1')).resolves.toEqual({ status: 'PENDING', code: 'AGENT_CONNECTION_TEST_QUEUED', testedAt: undefined })

    expect(calls[0]?.path).toBe('/api/v1/data-sources/source-1:test-connection')
    expect(calls[0]?.init.headers).toMatchObject({ 'X-CSRF-Token': 'synthetic-csrf-token' })
    expect(calls[0]?.init.headers).not.toHaveProperty('Idempotency-Key')
  })

  it('数据源编辑使用版本条件，且空密码不会被发送', async () => {
    const { api, calls } = apiWith(Response.json({
      item: { id: 'source-1', displayName: '合成数据源', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '127.0.0.1', port: 2881, state: 'ENABLED', revision: 2, credentialRevision: 1 },
    }))

    await api.updateDataSource('source-1', 1, { displayName: '合成数据源', environment: 'TEST', connectionKind: 'ODP', compatibilityMode: 'MYSQL', host: '127.0.0.1', port: 2881, username: 'synthetic-user' })

    expect(calls[0]?.path).toBe('/api/v1/data-sources/source-1')
    expect(calls[0]?.init.headers).toMatchObject({ 'X-CSRF-Token': 'synthetic-csrf-token', 'If-Match': '"rev-1"' })
    expect(calls[0]?.init.body).not.toContain('password')
  })

  it('数据源启停使用版本条件且不生成幂等键', async () => {
    const { api, calls } = apiWith(Response.json({ state: 'DISABLED', revision: 3 }))

    await expect(api.changeDataSourceState('source-1', 2, 'DISABLED')).resolves.toEqual({
      state: 'DISABLED',
      revision: 3,
    })

    expect(calls[0]?.path).toBe('/api/v1/data-sources/source-1:disable')
    expect(calls[0]?.init).toMatchObject({ method: 'POST' })
    expect(calls[0]?.init.headers).toMatchObject({
      'X-CSRF-Token': 'synthetic-csrf-token',
      'If-Match': '"rev-2"',
    })
    expect(calls[0]?.init.headers).not.toHaveProperty('Idempotency-Key')
  })

  it('数据源归档使用版本条件且不生成幂等键', async () => {
    const { api, calls } = apiWith(new Response(null, { status: 204 }))

    await expect(api.archiveDataSource('source-1', 2)).resolves.toBeUndefined()

    expect(calls[0]?.path).toBe('/api/v1/data-sources/source-1')
    expect(calls[0]?.init).toMatchObject({ method: 'DELETE' })
    expect(calls[0]?.init.headers).toMatchObject({
      'X-CSRF-Token': 'synthetic-csrf-token',
      'If-Match': '"rev-2"',
    })
    expect(calls[0]?.init.headers).not.toHaveProperty('Idempotency-Key')
  })

  it('数据源启停与归档缺少 CSRF 时失败关闭', async () => {
    const { api, calls } = apiWith(Response.json({}), '')

    await expect(api.changeDataSourceState('source-1', 2, 'DISABLED')).rejects.toMatchObject({
      code: 'CSRF_TOKEN_UNAVAILABLE',
    })
    await expect(api.archiveDataSource('source-1', 2)).rejects.toMatchObject({
      code: 'CSRF_TOKEN_UNAVAILABLE',
    })

    expect(calls).toHaveLength(0)
  })

  it('缺少 CSRF 时失败关闭且不发送写请求', async () => {
    const { api, calls } = apiWith(Response.json({ id: 'draft-1' }), '')

    await expect(api.createExportDraft({
      dataSourceId: 'source-1', nodeId: 'node-1', database: 'synthetic_db', table: 'synthetic_table', format: 'CSV', filePath: 'E:\\tmp\\output',
    })).rejects.toMatchObject({ code: 'CSRF_TOKEN_UNAVAILABLE' })

    expect(calls).toHaveLength(0)
  })

  it('为数据源页面提供安全且可操作的错误反馈', () => {
    expect(dataSourceErrorMessage({ status: 401, message: 'unsafe' }, '请求失败。')).toContain('安全校验')
    expect(dataSourceErrorMessage({ status: 404, message: 'unsafe' }, '请求失败。')).toContain('无权访问')
    expect(dataSourceErrorMessage({ status: 409, conflict: true }, '请求失败。')).toContain('刷新')
    expect(dataSourceErrorMessage({ status: 412, conflict: true }, '请求失败。')).toContain('刷新')
    expect(dataSourceErrorMessage({ status: 0, code: 'NETWORK_UNAVAILABLE' }, '请求失败。')).toContain('控制面')
  })
})
