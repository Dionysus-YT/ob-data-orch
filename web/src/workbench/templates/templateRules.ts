import type { ApiError, ConfigTemplateItem, TemplateAction } from '@/api/browser'

// 名称限制与服务端的 UTF-8 字节边界一致，不把汉字数量当作字节数。
export function templateNameError(value: string): string {
  const name = value.trim()
  if (!name) return '请输入模板名称。'
  return new TextEncoder().encode(name).length > 256 ? '模板名称过长，请缩短后重试。' : ''
}

// 网络、服务端或响应校验失败都可能发生在已提交之后，不能自动改用新请求身份。
export function templateWriteUncertain(error: unknown): boolean {
  const failure = error as Partial<ApiError> | undefined
  return !failure?.status || failure.status >= 500 || failure.code === 'RESPONSE_INVALID'
}

export function templateAllows(item: ConfigTemplateItem, action: TemplateAction): boolean {
  return item.availableActions?.includes(action) === true
}
