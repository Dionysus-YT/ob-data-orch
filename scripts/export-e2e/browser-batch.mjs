// 在已登录的应用源页面运行普通 API；会话和 CSRF 永远不离开当前浏览器。
export async function browserBatch(input) {
  const state = { records: [], active: '', done: false, startedAt: new Date().toISOString(), aborted: false }
  window.__odoMysqlE2E = state
  const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))
  const fact = (condition, code) => { if (!condition) throw new Error(code) }
  const safe = error => /^[A-Z_0-9]+$/.test(error.message) ? error.message : 'BROWSER_TEST_UNAVAILABLE'
  const errorCode = response => response.body.code ?? response.body.error?.code ?? 'API_REQUEST_REJECTED'
  async function request(route, method = 'GET', body, revision, key = crypto.randomUUID()) {
    fact(route.startsWith('/api/v1/'), 'REQUEST_SCOPE_INVALID')
    const headers = { Accept: 'application/json' }
    if (method !== 'GET') {
      headers['Content-Type'] = 'application/json'
      headers['X-CSRF-Token'] = document.querySelector('meta[name="ob-data-orch-csrf-token"]')?.content ?? ''
      headers['Idempotency-Key'] = key
      if (revision !== undefined) headers['If-Match'] = `"rev-${revision}"`
    }
    let text = body === undefined ? undefined : JSON.stringify(body)
    // SCN 是 int64，测试传输保留十进制精度，避免经过 JavaScript Number 舍入。
    if (typeof body?.config?.filterConfig?.flashbackScn === 'string') text = text.replace(/"flashbackScn":"([0-9]+)"/, '"flashbackScn":$1')
    const response = await fetch(route, { method, headers, credentials: 'same-origin', signal: AbortSignal.timeout(20_000), ...(text !== undefined ? { body: text } : {}) })
    return { status: response.status, body: await response.json().catch(() => ({})) }
  }
  async function get(route) { const value = await request(route); fact(value.status === 200, errorCode(value)); return value.body.item ?? value.body }
  async function write(route, body = {}, revision, key) { const value = await request(route, 'POST', body, revision, key); fact(value.status >= 200 && value.status < 300, errorCode(value)); return value.body.item ?? value.body }
  async function poll(route, terminal, maxMs, code) {
    const end = Date.now() + maxMs
    while (Date.now() < end) { const value = await get(route); if (terminal(value)) return value; await sleep(600) }
    throw new Error(code)
  }
  const digest = async value => [...new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(value)))].map(byte => byte.toString(16).padStart(2, '0')).join('')
  for (const test of input.cases) {
    if (state.aborted) break
    state.active = test.id
    const started = Date.now()
    let record = { id: test.id, area: test.area, title: test.title, expected: test.expected }
    state.current = record
    try {
      if (test.blocked) { record.outcome = 'BLOCKED'; record.code = test.blocked }
      else {
        const created = await request('/api/v1/export-drafts', 'POST', test.input)
        if (test.expected === 'API_REJECTED') {
          // 危险负例只到创建接口，接受时也不触发预检查或真实执行。
          fact([400, 422].includes(created.status), 'INVALID_INPUT_ACCEPTED')
          record.outcome = 'PASS'; record.httpStatus = created.status; record.code = errorCode(created)
        } else {
          fact(created.status === 201, errorCode(created))
          record.draftId = created.body.id
          const draft = await get(`/api/v1/export-drafts/${record.draftId}`)
          const preview = await write(`/api/v1/export-drafts/${draft.id}:preview-command`, {}, draft.revision)
          fact(preview.configFingerprint === draft.configFingerprint && /-p\s+\*{6}/.test(preview.command), 'COMMAND_PREVIEW_INVALID')
          fact((test.expectedFlags ?? []).every(flag => preview.command.includes(flag)), 'PARAMETER_NOT_EMITTED')
          record.parameterFlags = test.expectedFlags ?? []
          record.commandHash = await digest(preview.command)
          const precheck = await write(`/api/v1/export-drafts/${draft.id}:precheck`, {}, draft.revision)
          record.precheckId = precheck.id
          const checked = await poll(`/api/v1/prechecks/${precheck.id}`, value => !['PENDING', 'LEASED'].includes(value.status), 100_000, 'PRECHECK_TIMEOUT')
          record.checks = checked.results
          const required = ['DATABASE_CONNECTIVITY', 'OBJECT_ACCESS', 'TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'OUTPUT_PATH', 'OUTPUT_EMPTY', 'AVAILABLE_SPACE']
          fact(checked.integrityStatus === 'COMPLETE' && checked.results.length === 7 && required.every(name => checked.results.filter(result => result.check === name).length === 1), 'PRECHECK_LIST_INCOMPLETE')
          fact(checked.draftId === draft.id && checked.draftRevision === draft.revision && checked.configFingerprint === draft.configFingerprint && checked.nodeId === input.nodeId, 'PRECHECK_BINDING_MISMATCH')
          if (test.expected === 'PRECHECK_FAILED') {
            fact(checked.status === 'FAILED' && checked.results.some(result => result.check === test.failedCheck && result.status === 'FAILED'), 'EXPECTED_PRECHECK_FAILURE_MISSING')
            if (['MX-097', 'MX-098', 'MX-099', 'MX-100'].includes(test.id)) fact(checked.results.filter(result => ['DATABASE_CONNECTIVITY', 'OBJECT_ACCESS'].includes(result.check)).every(result => result.status === 'PASSED'), 'INDEPENDENT_CHECK_SUPPRESSED')
            const rejected = await request(`/api/v1/export-drafts/${draft.id}:submit`, 'POST', { precheckId: precheck.id }, draft.revision)
            fact([409, 422].includes(rejected.status), 'FAILED_PRECHECK_SUBMITTED')
            record.outcome = 'PASS'; record.submitHttpStatus = rejected.status
          } else {
            fact(checked.status === 'SUCCEEDED' && checked.results.every(result => result.status === 'PASSED'), 'PRECHECK_NOT_PASSED')
            const submitted = await write(`/api/v1/export-drafts/${draft.id}:submit`, { precheckId: precheck.id }, draft.revision)
            record.taskId = submitted.id
            let execution = await poll(`/api/v1/tasks/${submitted.id}/execution`, value => ['SUCCEEDED', 'FAILED', 'CANCELLED'].includes(value.state), test.taskTimeoutMs??180_000, 'TASK_TIMEOUT')
            if (execution.state === 'SUCCEEDED') execution = await poll(`/api/v1/tasks/${submitted.id}/execution`, value => !!value.resultSummary, 20_000, 'RESULT_SUMMARY_TIMEOUT')
            const snapshot = await get(`/api/v1/tasks/${submitted.id}/snapshot`)
            const command = await get(`/api/v1/tasks/${submitted.id}/command-evidence`)
            fact(snapshot.configFingerprint === preview.configFingerprint && snapshot.toolVersion === '4.3.5-RELEASE', 'FROZEN_SNAPSHOT_MISMATCH')
            fact(command.command === preview.command && command.redaction === 'PASSWORD_ONLY', 'FROZEN_COMMAND_MISMATCH')
            record.state = execution.state; record.exitCode = execution.exitCode; record.errorCode = execution.errorCode
            record.executionId = execution.executionId; record.plannedActualMatch = execution.plannedActualMatch; record.summary = execution.resultSummary
            if (test.expected === 'TASK_FAILED') {
              fact(execution.state === 'FAILED' && Number.isInteger(execution.exitCode) && execution.exitCode !== 0, 'EXPECTED_TOOL_FAILURE_MISSING')
              const logs = await get(`/api/v1/tasks/${submitted.id}/logs`)
              fact(logs.items.some(log => /unknown column/i.test(log.message) && log.message.includes('missing_column')), 'EXPECTED_QUERY_FAILURE_MISSING')
              record.outcome = 'PASS'
            } else if (test.expected === 'OBSERVED') record.outcome = 'OBSERVED'
            else {
              fact(execution.state === 'SUCCEEDED' && execution.exitCode === 0 && execution.plannedActualMatch === 'MATCHED' && !execution.reconciliationRequired, 'EXPECTED_TASK_SUCCESS_MISSING')
              // 文件内容由本机独立校验器核对；工具成功不会直接变成内容 PASS。
              record.outcome = 'CONTENT_PENDING'
            }
          }
        }
      }
    } catch (error) {
      record.outcome = 'FAIL'; record.code = safe(error)
      if (record.taskId && record.code === 'TASK_TIMEOUT') {
        try {
          await write(`/api/v1/tasks/${record.taskId}:cancel`)
          const terminal = await poll(`/api/v1/tasks/${record.taskId}/execution`, value => ['SUCCEEDED', 'FAILED', 'CANCELLED'].includes(value.state) && !value.reconciliationRequired, 60_000, 'TIMED_OUT_TASK_NOT_RECOVERED')
          record.timeoutCleanupState = terminal.state
        } catch { state.aborted = true; record.cleanupCode = 'TIMED_OUT_TASK_NOT_RECOVERED' }
      }
    }
    record.elapsedMs = Date.now() - started
    state.records.push(record)
  }
  state.active = ''; state.done = true; state.finishedAt = new Date().toISOString()
}
