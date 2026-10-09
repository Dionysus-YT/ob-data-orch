import type { ApiError, ExecutionNodeWrite } from '@/api/browser'

export type NodeFormField = 'displayName' | 'platform' | 'allowedRoots' | 'toolHome' | 'javaPath'
export type NodeFormErrors = Partial<Record<NodeFormField, string>>

export const fieldLabels: Record<NodeFormField, string> = {
  displayName: '节点名称', platform: '目标平台', allowedRoots: '导出数据目录白名单',
  toolHome: 'OB Loader/Dumper 安装目录', javaPath: '工具专用 Java 8 路径',
}
export const fieldIds: Record<NodeFormField, string> = {
  displayName: 'node-display-name', platform: 'node-platform', allowedRoots: 'node-allowed-roots',
  toolHome: 'node-tool-home', javaPath: 'node-java-path',
}
export function validateNodeForm(input: ExecutionNodeWrite): NodeFormErrors {
  const errors: NodeFormErrors = {}
  if (!input.displayName || input.displayName.length > 200) errors.displayName = '请输入不超过 200 个字符的节点名称。'
  if (!input.toolHome) errors.toolHome = '请填写 OB Loader/Dumper 安装目录。'
  if (!input.javaPath) errors.javaPath = '请填写工具专用 Java 8 可执行文件路径。'
  if (!input.allowedRoots.length) errors.allowedRoots = '请按行填写至少一个节点侧导出数据目录。'
  if (input.platform === 'WINDOWS_AMD64' && input.allowedRoots.some((root) => !/^\/[A-Za-z]:\//.test(root) || root.includes('\\') || root.split('/').some((part) => part === '.' || part === '..'))) {
    errors.allowedRoots = 'Windows 节点的每个导出数据目录必须使用 /E:/exports 形式。'
  }
  if (input.platform !== 'WINDOWS_AMD64' && input.allowedRoots.some((root) => !root.startsWith('/'))) {
    errors.allowedRoots = 'Linux 节点的每个导出数据目录必须是以 / 开头的绝对路径。'
  }
  return errors
}

export function nodeApiFieldErrors(error: unknown): NodeFormErrors {
  const errors: NodeFormErrors = {}
  for (const item of (error as Partial<ApiError>).fieldErrors ?? []) {
    if (item.field === 'displayName' || item.field === 'platform' || item.field === 'allowedRoots' || item.field === 'toolHome' || item.field === 'javaPath') errors[item.field] = item.message || '该字段不符合要求。'
  }
  return errors
}
