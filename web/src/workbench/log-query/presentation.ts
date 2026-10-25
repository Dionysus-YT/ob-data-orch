export const logSourceLabel: Record<string, string> = { OBDUMPER_STDOUT: 'OBDUMPER · stdout', OBDUMPER_STDERR: 'OBDUMPER · stderr', OBDUMPER_FILE: 'OBDUMPER · 工具文件', OBLOADER_STDOUT: 'OBLOADER · stdout', OBLOADER_STDERR: 'OBLOADER · stderr' }
export function logTime(value: string): string { return `${new Date(value).toISOString().replace('T', ' ').replace('Z', '')} UTC` }
/** 表格只渲染有界摘要；完整脱敏消息在同源抽屉和下载中保留。 */
export function logMessagePreview(value: string): string { return value.length > 1000 ? `${value.slice(0, 1000)}…` : value }
/** 只分割字面包含片段，模板使用文本插值，不生成 HTML 或正则表达式。 */
export function logHighlights(text: string, keyword: string): readonly { text: string; hit: boolean }[] {
  if (!keyword) return [{ text, hit: false }]
  const parts: { text: string; hit: boolean }[] = []; let at = 0
  while (at < text.length) { const next = text.indexOf(keyword, at); if (next < 0) { parts.push({ text: text.slice(at), hit: false }); break }; if (next > at) parts.push({ text: text.slice(at, next), hit: false }); parts.push({ text: keyword, hit: true }); at = next + keyword.length }
  return parts
}
