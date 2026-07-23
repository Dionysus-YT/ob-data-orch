import type { DataSourceWrite } from '@/api/browser'

export type ParsedDataSourceConnection = Pick<DataSourceWrite, 'compatibilityMode' | 'host' | 'port' | 'clusterName' | 'tenantName' | 'username' | 'defaultDatabase' | 'password'>

// parseDataSourceConnectionString 只解析 ODC 支持的固定客户端参数，绝不执行或保存输入的连接串。
export function parseDataSourceConnectionString(value: string): ParsedDataSourceConnection | undefined {
  const tokens = tokenize(value)
  if (!tokens || tokens.length < 2) return undefined
  const command = tokens[0]?.toLowerCase()
  const compatibilityMode = command === 'mysql' ? 'MYSQL' : command === 'obclient' ? 'ORACLE' : undefined
  if (!compatibilityMode) return undefined

  const options = new Map<string, string>()
  for (let index = 1; index < tokens.length; index += 1) {
    if (tokens[index] === '-A' || tokens[index] === '-c') continue
    const parsed = option(tokens, index)
    if (!parsed || options.has(parsed.name)) return undefined
    options.set(parsed.name, parsed.value)
    index += parsed.consumed
  }
  const host = options.get('host')
  const port = Number(options.get('port'))
  const password = options.get('password')
  const identity = splitODPIdentity(options.get('username'))
  if (!host || !Number.isInteger(port) || port < 1 || port > 65535 || !password || !identity) return undefined

  return {
    compatibilityMode,
    host,
    port,
    clusterName: identity.clusterName,
    tenantName: identity.tenantName,
    username: identity.username,
    defaultDatabase: compatibilityMode === 'MYSQL' ? options.get('database') : undefined,
    password,
  }
}

function option(tokens: string[], index: number): { name: 'host' | 'port' | 'username' | 'password' | 'database'; value: string; consumed: number } | undefined {
  const token = tokens[index]
  if (!token) return undefined
  const names: Record<string, 'host' | 'port' | 'username' | 'password' | 'database'> = { '-h': 'host', '-P': 'port', '-u': 'username', '-p': 'password', '-D': 'database' }
  const name = names[token]
  if (name) {
    const value = tokens[index + 1]
    return value ? { name, value, consumed: 1 } : undefined
  }
  for (const [prefix, compactName] of Object.entries(names)) {
    if (token.startsWith(prefix) && token.length > prefix.length) return { name: compactName, value: token.slice(prefix.length), consumed: 0 }
  }
  return undefined
}

function splitODPIdentity(value: string | undefined): { username: string; tenantName: string; clusterName: string } | undefined {
  if (!value) return undefined
  const matched = /^([^@#\s]+)@([^#\s]+)#([^\s]+)$/.exec(value)
  if (!matched) return undefined
  return { username: matched[1] ?? '', tenantName: matched[2] ?? '', clusterName: matched[3] ?? '' }
}

function tokenize(value: string): string[] | undefined {
  if (!value || value.length > 8192) return undefined
  const tokens: string[] = []
  let current = ''
  let quote = ''
  let escaping = false
  for (const character of value) {
    if (escaping) {
      current += character
      escaping = false
      continue
    }
    if (quote && character === '\\') {
      escaping = true
      continue
    }
    if (character === '\'' || character === '"') {
      if (!quote) quote = character
      else if (quote === character) quote = ''
      else current += character
      continue
    }
    if (!quote && /\s/.test(character)) {
      if (current) tokens.push(current)
      current = ''
      continue
    }
    current += character
  }
  if (quote || escaping) return undefined
  if (current) tokens.push(current)
  return tokens
}
