const sectionLabels = {
  list: '节点列表', summary: '节点基本状态', configuration: '节点环境配置', platform: '节点平台',
  taskSummary: '节点任务统计', quota: '执行内存额度', budgetPolicy: '节点内存预算配置', agentFacts: 'Agent 资源采样',
  environmentCheck: '环境检查记录', inspection: '环境检查明细', host: '主机信息', diagnostics: 'Agent 运行信息',
  runtimeVersions: '运行时版本', logQueue: '日志上传队列', activeExecutions: '活动任务', dataRootUsages: '导出目录空间采样',
  recentTasks: '节点近期任务', activity: '节点操作记录', enrollment: 'Agent 注册材料', creation: '节点创建结果', deletion: '节点删除结果',
} as const

export type NodeResponseSection = keyof typeof sectionLabels

// 错误只持有固定的区域和原因，不附带响应值、路径、注册材料或原始异常。
// 保留 RESPONSE_INVALID，让原有失败关闭、旧事实保留与会话隔离继续生效。
export class NodeResponseError extends Error {
  readonly status = 502
  readonly code = 'RESPONSE_INVALID'
  readonly retryable = false
  readonly conflict = false
  readonly fieldErrors: readonly [] = []

  constructor(readonly reason: string, readonly section?: NodeResponseSection, readonly writeMayHaveSucceeded = false) {
    let advice = ''
    if (section) {
      advice = section === 'recentTasks' || section === 'activity'
        ? '请刷新页面；若仍出现，请联系管理员核对网页与控制面版本。'
        : '请刷新页面；若仍出现，请联系管理员检查控制面与 Agent 的版本及上报记录。'
      if (writeMayHaveSucceeded) advice = '操作可能已经生效，请先刷新节点核对结果，再决定是否重试；若仍出现，请联系管理员核对服务版本与记录。'
      if (section === 'enrollment') advice = '未确认的注册材料不能使用。请关闭弹层并刷新节点；若仍出现，请联系管理员核对网页与控制面版本。'
    }
    super(`${section ? `${sectionLabels[section]}无法校验：` : ''}${reason}${advice}`)
    this.name = 'NodeResponseError'
  }
}

// 校验原因由代码中的固定文案提供，调用方不得拼入控制面或 Agent 返回的原值。
export function invalidNodeResponse(reason: string): never {
  throw new NodeResponseError(reason)
}

// 仅包装已知的响应校验错误；保留内层更具体的区域，不把程序异常伪装成协议问题。
// 写入后的响应不可信时提示先核对结果，不能声称服务端事务已经撤销。
export function readNodeResponse<T>(section: NodeResponseSection, read: () => T, writeMayHaveSucceeded = false): T {
  try {
    return read()
  } catch (error) {
    if (error instanceof NodeResponseError) {
      if (error.section && !writeMayHaveSucceeded) throw error
      throw new NodeResponseError(error.reason, error.section ?? section, writeMayHaveSucceeded)
    }
    if (error && typeof error === 'object' && 'code' in error && error.code === 'RESPONSE_INVALID') {
      throw new NodeResponseError('返回的数据缺少必需字段，或字段格式不正确。', section, writeMayHaveSucceeded)
    }
    throw error
  }
}
