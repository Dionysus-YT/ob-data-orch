export function stateLabel(state?: string, reconciliationRequired = false) {
  if (reconciliationRequired) return '状态核对中'
  return {
    WAITING_SCHEDULE: '等待 Agent 领取',
    STARTING: '正在启动',
    RUNNING: '正在运行',
    SUCCEEDED: '导出成功',
    FAILED: '导出失败',
  }[state ?? ''] ?? '状态核对中'
}

export function stateClass(state?: string) {
  return state === 'SUCCEEDED' ? 'success' : state === 'FAILED' ? 'danger' : 'neutral'
}

export function dateTime(value?: string) {
  if (!value) return '尚未发生'
  const parsed = new Date(value)
  return Number.isNaN(parsed.valueOf()) ? '时间未知' : parsed.toLocaleString()
}

// formatBytes 用受控单位展示字节数，不承诺精确换算。
export function formatBytes(value: number) {
  if (!Number.isSafeInteger(value) || value < 0) return '未知'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`
  if (value < 1024 * 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MiB`
  return `${(value / 1024 / 1024 / 1024).toFixed(1)} GiB`
}
