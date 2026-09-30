// 未指定开启时保留原值，不改变地址、身份或其他设置。
export function developmentSettings(settings, mode, enableRealExecution) {
  const address = mode === 'agent' ? settings.controlPlaneUrl : settings.publicUrl
  if (address !== 'https://127.0.0.1:18443' || typeof settings.realExecutionEnabled !== 'boolean') {
    throw new Error('开发配置须使用固定回环地址及有效的真实执行开关')
  }
  if (mode === 'agent' && settings.stateDirectory !== 'data/agent-security') {
    throw new Error('开发 Agent 身份目录不匹配')
  }
  return { ...settings, realExecutionEnabled: enableRealExecution || settings.realExecutionEnabled }
}

// 真实任务可能正在使用后端进程，源码保存只能提示，不自动取消其运行。
export function shouldReload(realExecutionEnabled, sourceChanged, requested) {
  return realExecutionEnabled ? requested : sourceChanged || requested
}
