import type { ExecutionNodePlatform } from '@/api/browser'

// nodeOperatingSystemOptions 只列出已确认平台范围中的操作系统分类。
export const nodeOperatingSystemOptions = [
  { value: 'WINDOWS', label: 'Windows' },
  { value: 'LINUX', label: 'Linux' },
] as const

// NodeOperatingSystem 表示页面操作系统分类，不直接作为机器协议值提交。
export type NodeOperatingSystem = typeof nodeOperatingSystemOptions[number]['value']

// nodeCPUArchitectureOptions 采用 lscpu 的架构命名，协议平台值仍由组合映射生成。
export const nodeCPUArchitectureOptions = [
  { value: 'x86_64', label: 'x86_64' },
  { value: 'aarch64', label: 'aarch64' },
] as const

// NodeCPUArchitecture 表示已确认平台中的 CPU 架构分类。
export type NodeCPUArchitecture = typeof nodeCPUArchitectureOptions[number]['value']

const platforms = {
  WINDOWS_AMD64: { operatingSystem: 'WINDOWS', cpuArchitecture: 'x86_64' },
  LINUX_AMD64: { operatingSystem: 'LINUX', cpuArchitecture: 'x86_64' },
  LINUX_ARM64: { operatingSystem: 'LINUX', cpuArchitecture: 'aarch64' },
} as const satisfies Record<ExecutionNodePlatform, { operatingSystem: NodeOperatingSystem; cpuArchitecture: NodeCPUArchitecture }>

// nodePlatformClassification 从唯一平台值派生操作系统和架构，避免两份可编辑状态。
export function nodePlatformClassification(platform: ExecutionNodePlatform) {
  return platforms[platform]
}

// nodePlatformHelp 从唯一平台值派生路径示例、权限和首次安装帮助，不持有第二份配置。
export function nodePlatformHelp(platform: ExecutionNodePlatform) {
  const { cpuArchitecture } = nodePlatformClassification(platform)
  if (platform === 'WINDOWS_AMD64') return {
    architectureHint: '使用 x86_64 架构；Agent、Java 与工具本地组件均需匹配目标机器。',
    toolHomePlaceholder: 'E:\\tools\\ob-loader-dumper-4.3.5-RELEASE',
    toolHomeHint: '填写包含 bin、lib、conf 的完整安装根目录。启动文件位于 bin\\windows\\obdumper.bat 和 bin\\windows\\obloader.bat。',
    javaPathPlaceholder: 'C:\\Program Files\\Java\\jdk8\\bin\\java.exe',
    javaPathHint: '填写目标机器上 Java 8 的 bin\\java.exe 绝对路径；包含空格也直接填写，无需加引号。',
    rootPlaceholder: '/E:/ob-data/exports',
    rootHint: '每行一个绝对目录，使用 /E:/exports 形式和正斜杠；导出输出与普通导入输入均须位于允许目录内；Agent 账户须分别具备所需的写入或读取权限。',
    agentInstallHint: '解压与 x86_64 架构匹配的 Agent 包，右键「启动.cmd」选择「以管理员身份运行」，菜单选择「1 启动」。',
  }
  return {
    architectureHint: '按目标机器实际架构选择；可用 lscpu 核对 x86_64 或 aarch64，Agent 与 Java 须匹配。',
    toolHomePlaceholder: '/opt/ob-loader-dumper-4.3.5-RELEASE',
    toolHomeHint: '填写包含 bin、lib、conf 的完整安装根目录。启动文件位于 bin/obdumper 和 bin/obloader，须有执行权限。',
    javaPathPlaceholder: '/usr/lib/jvm/java-8/bin/java',
    javaPathHint: `选择与 ${cpuArchitecture} 架构一致的 Java 8，填写以 / 开头的 bin/java 完整路径；Agent 运行账号须有执行权限。`,
    rootPlaceholder: '/var/lib/ob-data-orch/exports',
    rootHint: '每行一个以 / 开头的绝对目录，路径区分大小写；导出输出与普通导入输入均须位于允许目录内；Agent 账户须分别具备所需的写入或读取权限。',
    agentInstallHint: `解压与 ${cpuArchitecture} 架构匹配的 Agent 包，在解压目录用 root 或 sudo 权限运行 start.sh，菜单选择「1 启动」。`,
  }
}

// nodeAgentPlatformLabel 将实际上报值格式化为与登记平台相同的展示语言。
// 未知值原样显示供核对，不推断任务资格或扩展支持矩阵。
export function nodeAgentPlatformLabel(operatingSystem: string, architecture: string): string {
  const os = nodeOperatingSystemOptions.find(option => option.value === operatingSystem.toUpperCase())?.label ?? operatingSystem
  const architectures: Record<string, string> = { AMD64: 'x86_64', ARM64: 'aarch64' }
  return `${os} · ${architectures[architecture.toUpperCase()] ?? architecture}`
}

// nodeAgentPackageURL 与注册分类共用映射，下载名称不暴露 Go 的构建架构别名。
export function nodeAgentPackageURL(platform: ExecutionNodePlatform): string {
  const classification = nodePlatformClassification(platform)
  return `/ob-data-orch-agent-${classification.operatingSystem.toLowerCase()}-${classification.cpuArchitecture}.zip`
}

// nodePlatformFromSelection 仅接受原有三个组合；未支持的组合不能生成注册目标。
export function nodePlatformFromSelection(operatingSystem: NodeOperatingSystem, cpuArchitecture: NodeCPUArchitecture): ExecutionNodePlatform | undefined {
  return (Object.keys(platforms) as ExecutionNodePlatform[]).find(platform => {
    const classification = platforms[platform]
    return classification.operatingSystem === operatingSystem && classification.cpuArchitecture === cpuArchitecture
  })
}
