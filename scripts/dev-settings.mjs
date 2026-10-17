// 未指定开启时保留原值，不改变地址、身份或其他设置。
export function developmentSettings(settings, mode, enableRealExecution, enableNormalImport = false) {
  const address = mode === 'agent' ? settings.controlPlaneUrl : settings.publicUrl
  if (address !== 'https://127.0.0.1:18443' || typeof settings.realExecutionEnabled !== 'boolean') {
    throw new Error('开发配置须使用固定回环地址及有效的真实执行开关')
  }
  if (mode === 'agent' && settings.stateDirectory !== 'data/agent-security') {
    throw new Error('开发 Agent 身份目录不匹配')
  }
  if (settings.normalImportExecutionEnabled !== undefined && typeof settings.normalImportExecutionEnabled !== 'boolean') throw new Error('普通导入执行开关无效')
  if (enableNormalImport && !(enableRealExecution || settings.realExecutionEnabled)) throw new Error('普通导入要求同时开启真实执行')
  return { ...settings, realExecutionEnabled: enableRealExecution || settings.realExecutionEnabled, ...(enableNormalImport ? { normalImportExecutionEnabled: true } : {}) }
}

// 真实任务可能正在使用后端进程，源码保存只能提示，不自动取消其运行。
export function shouldReload(realExecutionEnabled, sourceChanged, requested) {
  return realExecutionEnabled ? requested : sourceChanged || requested
}
