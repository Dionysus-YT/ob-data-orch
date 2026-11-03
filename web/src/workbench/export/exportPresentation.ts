import { TableOutlined, EyeOutlined, InboxOutlined, ThunderboltOutlined, CopyOutlined, AppstoreOutlined } from '@ant-design/icons-vue'
import { type DataSourceSummary, type ExportObjectType } from '@/api/browser'

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

export const dateFormatChoices = [{ value: '', label: '继承工具默认（按租户模式）' }, ...['yyyy-MM-dd', 'yyyyMMdd', 'yyyy/MM/dd'].map((value) => ({ value, label: value }))]

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
  { type: 'TABLE', label: '表', icon: TableOutlined, glyph: '', oracleOnly: false },
  { type: 'VIEW', label: '视图', icon: EyeOutlined, glyph: '', oracleOnly: false },
  { type: 'FUNCTION', label: '函数', icon: null, glyph: 'fₓ', oracleOnly: false },
  { type: 'PROCEDURE', label: '存储过程', icon: null, glyph: 'Pₓ', oracleOnly: false },
  { type: 'PACKAGE', label: '程序包', icon: InboxOutlined, glyph: '', oracleOnly: true },
  { type: 'TRIGGER', label: '触发器', icon: ThunderboltOutlined, glyph: '', oracleOnly: true },
  { type: 'SYNONYM', label: '同义词', icon: CopyOutlined, glyph: '', oracleOnly: true },
  { type: 'PUBLIC_SYNONYM', label: '公共同义词', icon: CopyOutlined, glyph: '', oracleOnly: true },
  { type: 'TYPE', label: '类型', icon: AppstoreOutlined, glyph: '', oracleOnly: true },
  { type: 'SEQUENCE', label: '序列', icon: null, glyph: '¹²³', oracleOnly: false },
] as const

// createObjectTypeMap 为唯一类型目录初始化独立状态，避免新增分类遗漏缓存或恢复引用。
export function createObjectTypeMap<T>(create: (type: ExportObjectType) => T): Record<ExportObjectType, T> {
  return Object.fromEntries(objectCategories.map(({ type }) => [type, create(type)])) as Record<ExportObjectType, T>
}

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
