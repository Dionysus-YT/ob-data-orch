// AccountError 仅保留受控状态和错误码，不持有秘密、正文或底层异常。
export class AccountError extends Error {
  constructor(readonly status: number, readonly code: string) { super(code) }
}

// createAccountApi 将认证请求集中在 API 边界；秘密只进入同源 POST 正文。
export function createAccountApi(options: { fetcher: typeof fetch; csrfToken: () => string | undefined }) {
  const errorCodes = new Set(['UNAUTHENTICATED', 'CSRF_INVALID', 'PASSWORD_FIELDS_INVALID', 'CURRENT_PASSWORD_INVALID', 'PASSWORD_RATE_LIMITED', 'PASSWORD_CHANGE_UNAVAILABLE'])
  async function request(path: string, init: RequestInit = {}) {
    let response: Response
    try { response = await options.fetcher(path, { ...init, credentials: 'same-origin', cache: 'no-store', redirect: 'error' }) }
    catch { throw new AccountError(0, 'NETWORK_UNAVAILABLE') }
    if (!response.ok) {
      let code = 'REQUEST_FAILED'
      try {
        const body = await response.json()
        if (typeof body?.error?.code === 'string' && errorCodes.has(body.error.code)) code = body.error.code
      } catch { /* 无法解析错误时仍保留 HTTP 状态，不展示响应正文。 */ }
      throw new AccountError(response.status, code)
    }
    return response
  }
  function csrfHeaders(): Record<string, string> {
    const token = options.csrfToken()?.trim()
    if (!token) throw new AccountError(0, 'CSRF_TOKEN_UNAVAILABLE')
    return { 'X-CSRF-Token': token }
  }
  return {
    async session() {
      const response = await request('/api/v1/session')
      let body: unknown
      try { body = await response.json() } catch { throw new AccountError(0, 'RESPONSE_INVALID') }
      const username = (body as { username?: unknown } | null)?.username
      if (typeof username !== 'string' || !username.trim() || username.length > 256 || Array.from(username).some(value => value.charCodeAt(0) < 32 || value.charCodeAt(0) === 127)) throw new AccountError(0, 'RESPONSE_INVALID')
      return username
    },
    async logout() {
      try { await request('/logout', { method: 'POST', headers: csrfHeaders() }) }
      catch (error) { if (!(error instanceof AccountError && error.status === 401)) throw error }
    },
    async changePassword(currentPassword: string, newPassword: string) {
      const headers = { ...csrfHeaders(), 'Content-Type': 'application/x-www-form-urlencoded' }
      await request('/api/v1/session/password', { method: 'POST', headers, body: new URLSearchParams({ currentPassword, newPassword }) })
    },
  }
}

// accountApi 绑定当前组件的取消信号，关闭后不再接纳旧响应。
export function accountApi(signal?: AbortSignal) {
  return createAccountApi({
    fetcher: (input, init) => fetch(input, { ...init, signal }),
    csrfToken: () => document.querySelector('meta[name="ob-data-orch-csrf-token"]')?.getAttribute('content') || undefined,
  })
}
