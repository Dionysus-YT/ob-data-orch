import { TableOutlined, EyeOutlined } from '@ant-design/icons-vue'
import { type DataSourceSummary } from '@/api/browser'

export const fileEncodingChoices = [
  { value: '', label: 'UTF8（默认）' },
  { value: 'UTF-8', label: 'UTF8' },
  { value: 'UTF-16', label: 'UTF16' },
  { value: 'UTF-32', label: 'UTF32' },
  { value: 'ISO-8859-1', label: 'ISO-8859-1' },
  { value: 'US-ASCII', label: 'ASCII' },
  { value: 'GB2312', label: 'GB2312' },
  { value: 'GBK', label: 'GBK' },
  { value: 'GB18030', label: 'GB18030' },
  { value: 'Big5', label: 'BIG5' },
]

export const blockSizeChoicesMB = [
  { value: '', label: '工具默认（不指定）' },
  { value: '64', label: '64 MB' },
  { value: '512', label: '512 MB' },
  { value: '1024', label: '1024 MB' },
  { value: '2048', label: '2048 MB' },
]

export const dateFormatChoices = [{ value: '', label: '继承工具默认（yyyy-MM-dd）' }, ...['yyyy-MM-dd', 'yyyyMMdd', 'yyyy/MM/dd'].map((value) => ({ value, label: value }))]

export const datetimeFormatChoices = [{ value: '', label: '继承工具默认（保留源精度）' }, ...['yyyy-MM-dd HH:mm:ss', 'yyyyMMddHHmmss', "yyyy-MM-dd'T'HH:mm:ss.SSS"].map((value) => ({ value, label: value }))]

export const fieldSeparatorChoices = [
  { value: '', label: '英文逗号（默认）' },
  { value: ',', label: '英文逗号 ,' },
  { value: ';', label: '分号 ;' },
  { value: '|', label: '竖线 |' },
]

export const cutSeparatorChoices = [
  { value: '', label: '继承官方默认' },
  ...fieldSeparatorChoices.slice(1),
]

export const quoteChoices = [
  { value: '', label: '单引号（默认）' },
  { value: "'", label: "单引号 '" },
  { value: '"', label: '双引号 "' },
]

export const lineSeparatorChoices = [
  { value: '', label: '继承官方默认' },
  { value: '\\r\\n', label: '\\r\\n' },
  { value: '\\n', label: '\\n' },
  { value: '\\r', label: '\\r' },
]

export const objectCategories = [
  { type: 'TABLE', label: '表', icon: TableOutlined, glyph: '' },
  { type: 'VIEW', label: '视图', icon: EyeOutlined, glyph: '' },
  { type: 'FUNCTION', label: '函数', icon: null, glyph: 'fₓ' },
  { type: 'PROCEDURE', label: '存储过程', icon: null, glyph: 'Pₓ' },
  { type: 'SEQUENCE', label: '序列', icon: null, glyph: '¹²³' },
] as const

export function precheckStatusLabel(status?: string) {
  return {
    PENDING: '等待 Agent 领取',
    LEASED: 'Agent 正在检查',
    SUCCEEDED: '已通过',
    FAILED: '未通过',
    EXPIRED: '已过期',
    INVALIDATED: '已失效',
  }[status ?? ''] ?? '尚未执行'
}

export function environmentLabel(value: string) {
  return { DEVELOPMENT: '开发', TEST: '测试', STAGING: '预生产', PRODUCTION: '生产' }[value] ?? value
}

export function lastTestLabel(source: DataSourceSummary) {
  return source.lastTestedAt ? new Date(source.lastTestedAt).toLocaleString() : '已成功测试'
}
