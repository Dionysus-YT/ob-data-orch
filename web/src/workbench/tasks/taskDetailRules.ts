import type { TaskListAction, TaskListItem, TaskLog, TaskSnapshot, TaskSnapshotGroup } from '@/api/browser'

/** 主操作仍须在服务端资格中；状态只决定已有资格的显示顺序。 */
export function taskDetailPrimary(task?: Pick<TaskListItem, 'state' | 'availableActions'>): TaskListAction | undefined {
  if (!task) return
  const order: readonly TaskListAction[] = task.state === 'SUCCEEDED' ? ['SAVE_TEMPLATE'] : task.state === 'FAILED' ? ['CHECKPOINT_RESUME', 'REBUILD_FROM_CONFIG'] : task.state === 'CANCELLED' ? ['REBUILD_FROM_CONFIG'] : ['CANCEL']
  return order.find(action => task.availableActions.includes(action))
}

/** 只识别工具明确标注的 ERROR，不把全部 stderr 当作错误。 */
export function taskLogIsError(record: TaskLog): boolean { return /\[ERROR\]/.test(record.message) && !/\bSystem exit \d+\b/i.test(record.message) }
export function taskLogSource(kind: string): string { return ({ STDOUT: '标准输出', STDERR: '标准错误', TOOL_FILE: '工具日志', AGENT_EVENT: 'Agent 事件' })[kind] ?? kind }
export function taskOperationLabel(action: string): string { return ({ TASK_SUBMITTED: '提交任务', TASK_CLAIMED: 'Agent 领取', TASK_CANCELLATION_REQUESTED: '请求取消', TASK_CANCELLATION_NOOP: '取消未改变终态', TASK_CANCELLATION_TIMEOUT: '取消超时核对', TASK_RESUMED_FROM_CHECKPOINT: '从检查点继续', TASK_EXECUTION_LEASE_EXPIRED: '执行租约过期', TASK_EXECUTION_RECONCILIATION_REQUIRED: '请求状态核对', TASK_TEMPLATE_SAVED: '保存模板', TASK_DRAFT_DERIVED: '派生草稿', STARTING: '开始启动', RUNNING: '工具运行', PROCESS_STARTED: '进程启动', SUCCEEDED: '执行成功', FAILED: '执行失败', CANCELLED: '确认取消' } as Record<string, string>)[action] ?? action }
export function taskOperationResult(result: string): string { return ({ SUCCEEDED: '成功', FAILED: '失败', DENIED: '拒绝', RECORDED: '已记录' } as Record<string, string>)[result] ?? '未知' }

/** 结果失败优先于远端取证说明；未上报结果时不提前给出结论。 */
export function taskResultText(result: { readonly result: 'VERIFIED' | 'FAILED' } | undefined, remote: boolean): string {
  if (!result) return '尚无结果证据'
  if (result.result === 'FAILED') return '结果未通过核验'
  return remote ? '远端对象未逐项核验' : '结果已核验'
}

/** 旧服务的最小快照仍按五步显示已有事实，缺失内容和来源不补造。 */
export function taskSnapshotGroups(snapshot: TaskSnapshot): readonly TaskSnapshotGroup[] {
  if (snapshot.groups) return snapshot.groups
  const field = (key: string, label: string, value?: string) => ({ key, label, value: value || '—', source: 'UNKNOWN' as const })
  return [
    { key: 'source', label: '1 · 数据源与执行节点', fields: [field('source', '数据源', snapshot.dataSourceId), field('node', '执行节点', snapshot.nodeId)] },
    { key: 'objects', label: '2 · 导出对象与内容', fields: [field('object', '导出对象', snapshot.objectSummary)] },
    { key: 'format', label: '3 · 数据格式', fields: [field('format', '数据格式', snapshot.format)] },
    { key: 'output', label: '4 · 输出与性能', fields: [field('missing', '配置明细', '历史快照未提供此项展示证据')] },
    { key: 'review', label: '5 · 预检查与提交', fields: [field('precheck', '预检查', snapshot.precheckId)] },
  ]
}
