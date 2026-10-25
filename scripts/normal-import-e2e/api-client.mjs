// 测试客户端只连接现有回环 HTTPS；登录、Cookie 与 CSRF 均走原应用协议，不修改服务身份。
import https from 'node:https'
import { randomUUID } from 'node:crypto'

const origin = 'https://127.0.0.1:18443'

// encodeLogin 将短时密码字节编码为表单；调用方在请求完成后清零原文和编码体。
export function encodeLogin(password) {
  if (!Buffer.isBuffer(password) || password.length < 1 || password.length > 1024) throw new Error('LOGIN_INPUT_INVALID')
  const prefix = Buffer.from('username=admin&password=')
  const output = Buffer.alloc(prefix.length + password.length * 3)
  prefix.copy(output)
  let offset = prefix.length
  const hex = '0123456789ABCDEF'
  for (const value of password) {
    if ((value >= 65 && value <= 90) || (value >= 97 && value <= 122) || (value >= 48 && value <= 57) || [45, 46, 95, 126].includes(value)) output[offset++] = value
    else { output[offset++] = 37; output[offset++] = hex.charCodeAt(value >> 4); output[offset++] = hex.charCodeAt(value & 15) }
  }
  return output.subarray(0, offset)
}

export class ApiClient {
  #agent
  #cookie = ''
  #csrf = ''

  constructor(ca) {
    if (!Buffer.isBuffer(ca) || ca.length === 0) throw new Error('TLS_CA_REQUIRED')
    this.#agent = new https.Agent({ ca, keepAlive: true, rejectUnauthorized: true, minVersion: 'TLSv1.2' })
  }

  async #raw(path, method = 'GET', payload, headers = {}) {
    if (!path.startsWith('/') || path.startsWith('//') || /[\r\n]/.test(path)) throw new Error('API_SCOPE_INVALID')
    return new Promise((resolve, reject) => {
      const request = https.request(new URL(path, origin), { agent: this.#agent, method, headers: { Accept: 'application/json', Origin: origin, ...(this.#cookie ? { Cookie: this.#cookie } : {}), ...(payload ? { 'Content-Length': payload.length } : {}), ...headers }, timeout: 20000 }, response => {
        const chunks = []
        let length = 0
        response.on('data', chunk => {
          length += chunk.length
          if (length > 4 * 1024 * 1024) response.destroy(new Error('RESPONSE_LIMIT'))
          else chunks.push(chunk)
        })
        response.on('error', () => reject(new Error('API_RESPONSE_UNAVAILABLE')))
        response.on('end', () => {
          const bytes = Buffer.concat(chunks)
          chunks.forEach(chunk => chunk.fill(0))
          resolve({ status: response.statusCode, headers: response.headers, bytes })
        })
      })
      request.on('timeout', () => request.destroy(new Error('API_TIMEOUT')))
      // 底层错误不带 URL、请求体或 Header 返回测试输出。
      request.on('error', () => reject(new Error('API_TRANSPORT_UNAVAILABLE')))
      request.end(payload)
    })
  }

  async login(password) {
    let form
    try {
      form = encodeLogin(password)
      const response = await this.#raw('/login', 'POST', form, { 'Content-Type': 'application/x-www-form-urlencoded' })
      const cookies = response.headers['set-cookie'] ?? []
      if (response.status !== 303 || response.headers.location !== '/' || cookies.length !== 1) throw new Error('API_LOGIN_REJECTED')
      const cookie = cookies[0]
      if (!cookie.startsWith('__Host-ob_data_orch_session=') || !/;\s*Secure/i.test(cookie) || !/;\s*HttpOnly/i.test(cookie) || !/;\s*SameSite=Strict/i.test(cookie)) throw new Error('SESSION_COOKIE_INVALID')
      this.#cookie = cookie.split(';', 1)[0]
      response.bytes.fill(0)
      // 开发代理仅对 HTML 导航提供页面，不能沿用业务 API 的 JSON Accept。
      const page = await this.#raw('/', 'GET', undefined, { Accept: 'text/html' })
      try {
        const match = page.bytes.toString('utf8').match(/<meta\s+name="ob-data-orch-csrf-token"\s+content="([^"]+)"/)
        if (page.status !== 200 || !match || match[1] === 'local-mvp-csrf-v1') throw new Error('CSRF_TOKEN_UNAVAILABLE')
        this.#csrf = match[1]
      } finally { page.bytes.fill(0) }
      // 现役控制面未实现契约中的 session 查询；用已实现的受保护读取确认同一登录身份。
      const session = await this.request('/api/v1/execution-nodes')
      if (session.status !== 200) throw new Error('API_SESSION_REJECTED')
    } finally { password?.fill(0); form?.fill(0) }
  }

  async request(path, method = 'GET', body, revision, key = randomUUID(), csrf = true) {
    if (!path.startsWith('/api/v1/')) throw new Error('API_SCOPE_INVALID')
    const payload = body === undefined ? undefined : Buffer.from(JSON.stringify(body))
    const headers = method === 'GET' ? {} : { 'Content-Type': 'application/json', 'Idempotency-Key': key, ...(csrf && this.#csrf ? { 'X-CSRF-Token': this.#csrf } : {}), ...(revision === undefined ? {} : { 'If-Match': `"rev-${revision}"` }) }
    try {
      const response = await this.#raw(path, method, payload, headers)
      try { return { status: response.status, body: JSON.parse(response.bytes.toString('utf8')) } }
      finally { response.bytes.fill(0) }
    } finally { payload?.fill(0) }
  }

  async health() {
    const response = await this.#raw('/healthz')
    response.bytes.fill(0)
    return response.status
  }

  // 仅供已授权测试建立独立数据源：密码由固定辅助进程编码为短时 JSON 字节，避免在 JS 对象中保留明文。
  async createDataSourceWithJSONBytes(payload, key = randomUUID()) {
    if (!Buffer.isBuffer(payload) || payload.length > 16384 || !this.#csrf || !this.#cookie) throw new Error('SOURCE_INPUT_INVALID')
    try {
      const response = await this.#raw('/api/v1/data-sources', 'POST', payload, { 'Content-Type': 'application/json', 'Idempotency-Key': key, 'X-CSRF-Token': this.#csrf })
      try { return { status: response.status, body: JSON.parse(response.bytes.toString('utf8')) } }
      finally { response.bytes.fill(0) }
    } finally { payload.fill(0) }
  }

  async close() {
    try { if (this.#cookie && this.#csrf) (await this.#raw('/logout', 'POST', undefined, { 'X-CSRF-Token': this.#csrf })).bytes.fill(0) }
    finally { this.#cookie = ''; this.#csrf = ''; this.#agent.destroy() }
  }
}
