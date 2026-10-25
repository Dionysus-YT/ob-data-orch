// 只清理本轮回执中未提交的草稿，保留已提交任务和证据。
import path from 'node:path'

export async function closeout(input, { client, accepted, save, work, readJSON, require, journals: suppliedJournals, refreshEnvironment = false }) {
  const journals = suppliedJournals ?? [await readJSON(path.join(work,'api-repair-batch.json')), await readJSON(path.join(work,'../normal-import-retest-20261019/browser-initial.json'))]
  const records = []
  for (const row of journals.flatMap(journal => journal.records).filter(row => row.draftId && !row.taskId)) {
    const response = await client.request(`/api/v1/normal-import-drafts/${row.draftId}`)
    if (response.status === 404) { records.push({ id: row.id, draftId: row.draftId, status: 'ALREADY_REMOVED' }); continue }
    require(response.status === 200, 'CLOSEOUT_DRAFT_READ_FAILED')
    const draft = response.body
    const databaseOwned = draft.config.database === input.database && draft.config.dataSourceId === input.sourceId || input.oracleDatabase && draft.config.database === input.oracleDatabase && draft.config.dataSourceId === input.oracleSourceId
    require(databaseOwned && draft.config.filePath.startsWith(input.inputRoot), 'CLOSEOUT_DRAFT_SCOPE_CHANGED')
    const deleted = await client.request(`/api/v1/normal-import-drafts/${draft.id}`, 'DELETE', {}, draft.revision)
    require(deleted.status === 200, 'CLOSEOUT_DRAFT_DELETE_FAILED')
    records.push({ id: row.id, draftId: row.draftId, status: 'REMOVED' })
  }
  let node = await accepted(`/api/v1/execution-nodes/${input.nodeId}`)
  if (refreshEnvironment) {
    const requested = await accepted(`/api/v1/execution-nodes/${input.nodeId}:environment-check`, 'POST', undefined, node.revision)
    for (let attempt = 0; attempt < 80; attempt++) {
      node = await accepted(`/api/v1/execution-nodes/${input.nodeId}`)
      require(node.environmentCheck?.checkId === requested.environmentCheck?.checkId, 'CLOSEOUT_ENVIRONMENT_BINDING_CHANGED')
      if (node.environmentCheck?.status === 'PASSED') break
      require(node.environmentCheck?.status !== 'FAILED', 'CLOSEOUT_ENVIRONMENT_FAILED')
      await new Promise(resolve => setTimeout(resolve, 1500))
    }
  }
  const health = await client.health()
  const heartbeatAgeMs = Date.now() - Date.parse(node.lastHeartbeatAt)
  const idle = node.taskSummary?.running === 0 && node.taskSummary?.waiting === 0 && node.taskSummary?.reconciling === 0
  require(health === 200 && heartbeatAgeMs >= 0 && heartbeatAgeMs < 60000 && idle && node.environmentCheck?.status === 'PASSED', 'CLOSEOUT_SERVICE_NOT_READY')
  await save(path.join(work,'api-closeout.json'), { at: new Date().toISOString(), records, health, heartbeatStatus: node.heartbeatStatus, lastHeartbeatAt: node.lastHeartbeatAt, heartbeatAgeMs, environmentCheckStatus: node.environmentCheck.status, taskSummary: node.taskSummary, capacityStatus: node.capacityStatus, executionQuota: node.executionQuota, agentLogQueue: node.agentDiagnostics?.logQueue, acceptsNewTasks: node.acceptsNewTasks })
  console.log('本轮未提交草稿已清理；HTTPS 健康、近期心跳、环境检查与空闲执行状态已确认。')
}
