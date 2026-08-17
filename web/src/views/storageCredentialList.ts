import type { StorageCredentialListItem, StorageCredentialProvider } from '@/api/browser'

export interface StorageCredentialFieldErrors {
  displayName?: string
  provider?: string
  accessKey?: string
  secretKey?: string
}

// storageCredentialProviders 是创建/轮换表单的固定 provider 选项。
export const storageCredentialProviders: readonly StorageCredentialProvider[] = ['OSS', 'S3', 'COS', 'OBS']

export function isStorageCredentialProvider(value: string): value is StorageCredentialProvider {
  return storageCredentialProviders.includes(value as StorageCredentialProvider)
}

// containsCredentialControlBreak 检测密钥中的 NUL、回车与换行；
// 这些字符会破坏 core-site.xml 值区或参数边界，服务端同样失败关闭。
function containsCredentialControlBreak(value: string): boolean {
  for (const character of value) {
    if (character === '\u0000' || character === '\r' || character === '\n') return true
  }
  return false
}

// validateStorageCredentialWrite 校验创建/轮换输入，与服务端失败关闭边界同口径：
// 名称非空且不超过 256 字符；provider 白名单；两个密钥非空且不含回车换行与 NUL。
export function validateStorageCredentialWrite(input: {
  readonly displayName: string
  readonly provider: string
  readonly accessKey: string
  readonly secretKey: string
}): StorageCredentialFieldErrors {
  const errors: StorageCredentialFieldErrors = {}
  const displayName = input.displayName.trim()
  if (!displayName) {
    errors.displayName = '请输入凭据名称。'
  } else if (displayName.length > 256) {
    errors.displayName = '凭据名称不能超过 256 个字符。'
  }
  if (!isStorageCredentialProvider(input.provider)) {
    errors.provider = '请选择支持的存储提供方。'
  }
  if (!input.accessKey || input.accessKey.length > 4096 || containsCredentialControlBreak(input.accessKey)) {
    errors.accessKey = 'AccessKey 不能为空、超过 4096 字符或包含控制换行字符。'
  }
  if (!input.secretKey || input.secretKey.length > 4096 || containsCredentialControlBreak(input.secretKey)) {
    errors.secretKey = 'SecretKey 不能为空、超过 4096 字符或包含控制换行字符。'
  }
  return errors
}

// storageCredentialProviderMatches 判断凭据提供方与导出输出类型一致；
// 不一致的凭据不能绑定到该输出类型（服务端提交前也会复验）。
export function storageCredentialProviderMatches(credential: StorageCredentialListItem, outputKind: string): boolean {
  return credential.provider === outputKind
}

// filterStorageCredentials 按关键字与提供方筛选凭据；纯展示逻辑，不涉及秘密。
export function filterStorageCredentials(
  credentials: readonly StorageCredentialListItem[],
  filters: { readonly keyword: string; readonly provider: string },
): readonly StorageCredentialListItem[] {
  const keyword = filters.keyword.trim().toLowerCase()
  return credentials.filter((credential) => {
    if (filters.provider && credential.provider !== filters.provider) return false
    if (!keyword) return true
    return credential.displayName.toLowerCase().includes(keyword) || credential.id.toLowerCase().includes(keyword)
  })
}
