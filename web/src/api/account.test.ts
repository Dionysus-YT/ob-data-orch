import { describe, expect, it, vi } from 'vitest'
import { createAccountApi } from './account'

describe('账户认证 API', () => {
  it('用户名只能来自已解析的会话，不接受空白或无效响应', async () => {
    for (const body of [{}, { username: '' }, { username: '\nadmin' }, null]) {
      const api = createAccountApi({ fetcher: vi.fn().mockResolvedValue(Response.json(body)), csrfToken: () => undefined })
      await expect(api.session()).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
    }
    const api = createAccountApi({ fetcher: vi.fn().mockResolvedValue(Response.json({ username: 'synthetic-user' })), csrfToken: () => undefined })
    expect(await api.session()).toBe('synthetic-user')
  })
  it('修改只使用同源正文和 CSRF；缺少安全令牌不发送请求', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
    await createAccountApi({ fetcher, csrfToken: () => 'synthetic-csrf' }).changePassword('synthetic-current', 'synthetic-next')
    const [path, init] = fetcher.mock.calls[0]!
    expect(path).toBe('/api/v1/session/password')
    expect(init).toMatchObject({ method: 'POST', credentials: 'same-origin', redirect: 'error', cache: 'no-store', headers: { 'X-CSRF-Token': 'synthetic-csrf', 'Content-Type': 'application/x-www-form-urlencoded' } })
    expect((init.body as URLSearchParams).get('currentPassword')).toBe('synthetic-current')
    fetcher.mockClear()
    const api = createAccountApi({ fetcher, csrfToken: () => undefined })
    await expect(api.changePassword('synthetic-current', 'synthetic-next')).rejects.toMatchObject({ code: 'CSRF_TOKEN_UNAVAILABLE' })
    await expect(api.logout()).rejects.toMatchObject({ code: 'CSRF_TOKEN_UNAVAILABLE' })
    expect(fetcher).not.toHaveBeenCalled()
  })
  it('注销允许已失效会话，改密 401、错误正文和网络异常不得成为成功或泄露', async () => {
    const api = createAccountApi({ fetcher: vi.fn().mockImplementation(() => Promise.resolve(Response.json({ error: { code: 'UNAUTHENTICATED', message: 'synthetic-secret' } }, { status: 401 }))), csrfToken: () => 'synthetic-csrf' })
    await expect(api.logout()).resolves.toBeUndefined()
    await expect(api.changePassword('synthetic-current', 'synthetic-next')).rejects.toMatchObject({ status: 401, code: 'UNAUTHENTICATED', message: 'UNAUTHENTICATED' })
    const failing = createAccountApi({ fetcher: vi.fn().mockRejectedValue(new Error('synthetic-secret')), csrfToken: () => 'synthetic-csrf' })
    await expect(failing.changePassword('synthetic-current', 'synthetic-next')).rejects.toMatchObject({ message: 'NETWORK_UNAVAILABLE' })
  })
})
