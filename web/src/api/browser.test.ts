import { describe, expect, it } from 'vitest'

import { createBrowserApi, type FetchLike } from './browser'

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
        connectionKind: 'OBSERVER_DIRECT',
        compatibilityMode: 'MYSQL',
        host: '127.0.0.1',
        port: 2881,
        state: 'ENABLED',
        revision: 1,
      }],
    }))

    const [source] = await api.listDataSources()

    expect(source).toEqual({
      id: 'source-1',
      displayName: '合成数据源',
      environment: 'TEST',
      connectionKind: 'OBSERVER_DIRECT',
      compatibilityMode: 'MYSQL',
      host: '127.0.0.1',
      port: 2881,
      state: 'ENABLED',
      revision: 1,
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

  it('缺少 CSRF 时失败关闭且不发送写请求', async () => {
    const { api, calls } = apiWith(Response.json({ id: 'draft-1' }), '')

    await expect(api.createExportDraft({
      dataSourceId: 'source-1', nodeId: 'node-1', database: 'synthetic_db', table: 'synthetic_table', format: 'CSV', filePath: 'E:\\tmp\\output',
    })).rejects.toMatchObject({ code: 'CSRF_TOKEN_UNAVAILABLE' })

    expect(calls).toHaveLength(0)
  })
})
