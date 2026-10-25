// 复用本轮真实成功与失败任务，只验证普通 API 生命周期，不启动派生任务。
export async function lifecycle(input, runtime) {
  const state = { records: [], drafts: [], templates: [], done: false }
  if (runtime) runtime.state = state
  else window.__normalImportLifecycle = state
  const require = (condition, code) => { if (!condition) throw new Error(code) }
  async function request(path, method = 'GET', body, revision, key = crypto.randomUUID(), csrf = true) {
    require(path.startsWith('/api/v1/'), 'API_SCOPE_INVALID')
    if (runtime?.request) return runtime.request(path, method, body, revision, key, csrf)
    const headers = { Accept: 'application/json' }
    if (method !== 'GET') {
      headers['Content-Type'] = 'application/json'
      headers['Idempotency-Key'] = key
      if (csrf) headers['X-CSRF-Token'] = document.querySelector('meta[name="ob-data-orch-csrf-token"]')?.content ?? ''
      if (revision !== undefined) headers['If-Match'] = `"rev-${revision}"`
    }
    const r = await fetch(path, { method, headers, credentials: 'same-origin', signal: AbortSignal.timeout(20000), ...(body === undefined ? {} : { body: JSON.stringify(body) }) })
    return { status: r.status, body: await r.json().catch(() => ({})) }
  }
  async function ok(path, method, body, revision, key) {
    const r = await request(path, method, body, revision, key)
    require(r.status >= 200 && r.status < 300, r.body.error?.code ?? r.body.code ?? 'API_REJECTED')
    return r.body.item ?? r.body
  }
  async function test(id, title, action) {
    if (input.ids && !input.ids.includes(id)) return
    const record = { id, title }
    try { Object.assign(record, { outcome: 'PASS' }, await action()) }
    catch (e) { Object.assign(record, { outcome: 'FAIL', code: /^[A-Z_0-9]+$/.test(e.message) ? e.message : 'TEST_UNAVAILABLE' }) }
    state.records.push(record)
    await runtime?.publish?.(state)
  }
  let derived, template
  await test('NI-E2E-15', '成功任务派生、幂等与同类型隔离', async () => {
    const route = `/api/v1/tasks/${input.successTaskId}:rebuild-draft`, key = crypto.randomUUID(), body = { derivation: 'REBUILD_FROM_CONFIG' }
    const a = await ok(route, 'POST', body, undefined, key), b = await ok(route, 'POST', body, undefined, key)
    state.drafts.push(a.draftId)
    derived = await ok(`/api/v1/normal-import-drafts/${a.draftId}`)
    require(a.draftId === b.draftId && a.taskType === 'OBLOADER_NORMAL_IMPORT' && derived.sourceTaskId === input.successTaskId && derived.sourceDerivation === 'REBUILD_FROM_CONFIG' && !derived.config.taskName, 'DERIVATION_INVALID')
    const foreign = await request(`/api/v1/export-drafts/${a.draftId}`)
    require(foreign.status === 404, 'CROSS_TYPE_VISIBLE')
    return { draftId: a.draftId, idempotent: true, crossTypeStatus: foreign.status }
  })
  await test('NI-E2E-16', '成功任务不能从头重跑', async () => {
    const r = await request(`/api/v1/tasks/${input.successTaskId}:rebuild-draft`, 'POST', { derivation: 'RERUN_FROM_SCRATCH' })
    require(r.status === 422, 'SUCCESS_RERUN_ACCEPTED')
    return { httpStatus: r.status, code: r.body.code ?? r.body.error?.code }
  })
  await test('NI-E2E-17', '模板保存与重新绑定普通导入草稿', async () => {
    const saved = await ok(`/api/v1/tasks/${input.successTaskId}:save-template`, 'POST', { displayName: `普通导入真实验证-${input.runId.slice(0,8)}` })
    state.templates.push(saved.id)
    const list = await ok('/api/v1/config-templates')
    template = list.items.find(t => t.id === saved.id)
    require(template?.taskType === 'OBLOADER_NORMAL_IMPORT', 'TEMPLATE_TYPE_INVALID')
    const created = await ok(`/api/v1/config-templates/${saved.id}:create-draft`, 'POST', { dataSourceId: input.sourceId, nodeId: input.nodeId })
    state.drafts.push(created.draftId)
    const draft = await ok(`/api/v1/normal-import-drafts/${created.draftId}`)
    require(created.taskType === 'OBLOADER_NORMAL_IMPORT' && draft.config.dataSourceId === input.sourceId && draft.config.nodeId === input.nodeId && !draft.config.taskName && !draft.sourceTaskId, 'TEMPLATE_BINDING_INVALID')
    const r = await request(`/api/v1/normal-import-drafts/${draft.id}:submit`, 'POST', { precheckId: input.oldPrecheckId }, draft.revision)
    require(r.status === 422, 'TEMPLATE_COPIED_EXECUTION_PERMISSION')
    return { templateId: saved.id, draftId: created.draftId, oldCheckSubmitStatus: r.status }
  })
  await test('NI-E2E-18', 'CSRF 缺失与旧修订写入拒绝', async () => {
    require(derived, 'DERIVED_DRAFT_UNAVAILABLE')
    const route = `/api/v1/normal-import-drafts/${derived.id}`, body = { configVersion: 'normal-import-v1', config: derived.config }
    const csrf = await request(route, 'PATCH', body, derived.revision, crypto.randomUUID(), false)
    require(csrf.status === 401 && (csrf.body.code ?? csrf.body.error?.code) === 'CSRF_VALIDATION_FAILED', 'MISSING_CSRF_ACCEPTED')
    const old = await request(route, 'PATCH', body, derived.revision + 100)
    require(old.status === 412, 'STALE_REVISION_ACCEPTED')
    return { csrfStatus: csrf.status, revisionStatus: old.status }
  })
  await test('NI-E2E-19', '失败任务从头重跑参数锁定', async () => {
    require(input.failedTaskId, 'FAILED_TASK_UNAVAILABLE')
    const created = await ok(`/api/v1/tasks/${input.failedTaskId}:rebuild-draft`, 'POST', { derivation: 'RERUN_FROM_SCRATCH' })
    state.drafts.push(created.draftId)
    const draft = await ok(`/api/v1/normal-import-drafts/${created.draftId}`)
    const changed = await request(`/api/v1/normal-import-drafts/${draft.id}`, 'PATCH', { configVersion: 'normal-import-v1', config: { ...draft.config, execution: { ...draft.config.execution, memory: '2G' } } }, draft.revision)
    const after = await ok(`/api/v1/normal-import-drafts/${draft.id}`)
    const record = { draftId: draft.id, changedConfigStatus: changed.status, rejectionCode: changed.body.code ?? changed.body.error?.code,
      configUnchanged: after.config.execution.memory === draft.config.execution.memory && after.revision === draft.revision }
    require(changed.status >= 400 && draft.sourceDerivation === 'RERUN_FROM_SCRATCH' && record.configUnchanged, 'RERUN_CONFIG_CHANGE_ACCEPTED')
    // 锁定行为与错误语义分别核对，服务异常不能冒充用户配置错误。
    if (changed.status !== 422) return { ...record, outcome: 'FAIL', code: 'RERUN_REJECTION_HTTP_MISMATCH' }
    return record
  })
  // 本轮派生草稿与模板只用于验证；按当前修订清理，历史任务与用户对象保持原位。
  state.cleanup = []
  for (const id of state.drafts) {
    try {
      const draft = await ok(`/api/v1/normal-import-drafts/${id}`)
      const r = await request(`/api/v1/normal-import-drafts/${id}`, 'DELETE', {}, draft.revision)
      state.cleanup.push({ kind: 'DRAFT', id, status: r.status })
    } catch { state.cleanup.push({ kind: 'DRAFT', id, status: 'UNCONFIRMED' }) }
  }
  for (const id of state.templates) {
    try {
      const list = await ok('/api/v1/config-templates'), item = list.items.find(t => t.id === id)
      const r = await request(`/api/v1/config-templates/${id}`, 'DELETE', {}, item.revision)
      state.cleanup.push({ kind: 'TEMPLATE', id, status: r.status })
    } catch { state.cleanup.push({ kind: 'TEMPLATE', id, status: 'UNCONFIRMED' }) }
  }
  state.done = true
  await runtime?.publish?.(state)
  return state
}
