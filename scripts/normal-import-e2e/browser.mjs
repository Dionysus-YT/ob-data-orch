// 仅在已登录应用源运行普通 API；不导出会话、CSRF 或凭据。
export async function run(input, runtime) {
  const state = { records: [], active: '', done: false, startedAt: new Date().toISOString() }
  if (runtime) runtime.state = state
  else window.__normalImportE2E = state
  const require = (value, code) => { if (!value) throw new Error(code) }
  const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))
  async function request(path, method = 'GET', body, revision, key = crypto.randomUUID()) {
    require(path.startsWith('/api/v1/'), 'API_SCOPE_INVALID')
    if (runtime?.request) return runtime.request(path, method, body, revision, key)
    const headers = { Accept: 'application/json' }
    if (method !== 'GET') {
      Object.assign(headers, { 'Content-Type': 'application/json', 'X-CSRF-Token': document.querySelector('meta[name="ob-data-orch-csrf-token"]')?.content ?? '', 'Idempotency-Key': key })
      if (revision !== undefined) headers['If-Match'] = `"rev-${revision}"`
    }
    const response = await fetch(path, { method, headers, credentials: 'same-origin', signal: AbortSignal.timeout(20000), ...(body === undefined ? {} : { body: JSON.stringify(body) }) })
    return { status: response.status, body: await response.json().catch(() => ({})) }
  }
  async function accepted(path, method = 'GET', body, revision, key) {
    const response = await request(path, method, body, revision, key)
    require(response.status >= 200 && response.status < 300, response.body.error?.code ?? response.body.code ?? 'API_REJECTED')
    return response.body.item ?? response.body
  }
  async function poll(path, terminal, timeout = 90000) {
    const until = Date.now() + timeout
    while (Date.now() < until) {
      const item = await accepted(path)
      if (terminal(item)) return item
      await sleep(700)
    }
    throw new Error('POLL_TIMEOUT')
  }
  const base = (name, directory, format = 'CSV', content = 'DATA_ONLY') => ({
    configVersion: 'normal-import-v1', config: { dataSourceId: input.sourceId, nodeId: input.nodeId, database: input.database,
      filePath: `${input.inputRoot}\\${directory}`, logPath: `${input.inputRoot}\\logs-${name}`, content, format,
      scope: 'SPECIFIC', objects: { table: `nivt_${name}` }, parser: format === 'CSV' ? { columnSeparator: ',', columnQuote: '"', skipHeader: true } : format === 'CUT' ? { columnSplitter: '|', lineSeparator: '\\n' } : {},
      mapping: { kind: 'POSITION' }, execution: { memory: '1G', thread: '1' }, cleanup: 'NONE' }
  })
  let cases = [
    { id: 'NI-E2E-02', title: 'CUT 数据导入', input: base('cut', 'cut', 'CUT') },
    { id: 'NI-E2E-03', title: 'SQL 数据导入', input: base('sql', 'sql', 'SQL') },
    { id: 'NI-E2E-04', title: '仅结构导入', input: base('ddl', 'ddl', '', 'DDL_ONLY') },
    { id: 'NI-E2E-05', title: 'CSV 结构和数据导入', input: base('both', 'both', 'CSV', 'DDL_AND_DATA') },
    { id: 'NI-E2E-06', title: '列数不匹配阻断', input: base('mismatch', 'mismatch'), failure: 'IMPORT_COLUMN_COUNT_MISMATCH' },
    { id: 'NI-E2E-07', title: '低堆阻断', input: base('csv', 'csv'), failure: 'IMPORT_JVM_HEAP_TOO_SMALL' },
    { id: 'NI-E2E-08', title: '空输入阻断', input: base('csv', 'empty'), failureCheck: 'INPUT_FILES' },
    { id: 'NI-E2E-09', title: '旁路参数拒绝', input: base('csv', 'csv'), apiRejected: true },
    { id: 'NI-E2E-10', title: '任务级密码字段拒绝', input: base('csv', 'csv'), apiRejected: true },
    { id: 'NI-E2E-11', title: '文件发现不能解锁提交', input: base('csv', 'csv'), filesOnly: true },
    { id: 'NI-E2E-12', title: '配置变化使旧检查失效', input: base('csv', 'csv'), stale: true },
    { id: 'NI-E2E-13', title: '全字段加引号 CSV 特殊值导入', input: base('quoted', 'quoted') },
    { id: 'NI-E2E-14', title: '重复主键写入失败', input: base('duplicate', 'duplicate', 'SQL'), taskFailed: true },
    { id: 'NI-E2E-20', title: '继承 strict 默认值时重复主键写入失败', input: base('duplicate', 'duplicate', 'SQL'), taskFailed: true },
  ]
  if (input.retest) {
    cases.unshift({ id: 'NI-E2E-01', title: '未引用空白 CSV 在完整预检查阻断', input: base('csv', 'csv'), failure: 'IMPORT_CSV_WHITESPACE_UNQUOTED' })
    cases.find(c => c.id === 'NI-E2E-05').failure = 'IMPORT_CSV_WHITESPACE_UNQUOTED'
    cases.find(c => c.id === 'NI-E2E-12').input = base('quoted', 'quoted')
    cases.push(
      { id: 'NI-E2E-21', title: '显式 with-trim 按预期删除空白', input: base('trim', 'trim') },
      { id: 'NI-E2E-22', title: '引用空白的 CSV 结构和数据保真导入', input: base('both_quoted', 'both_quoted', 'CSV', 'DDL_AND_DATA') },
      { id: 'NI-E2E-23', title: '显式 strict=false 正常数据导入', input: base('strict_false', 'strict_false', 'SQL') },
    )
    cases.find(c => c.id === 'NI-E2E-21').input.config.parser.withTrim = true
    cases.find(c => c.id === 'NI-E2E-23').input.config.execution.strict = 'false'
  }
  cases.find(c => c.id === 'NI-E2E-06').input.config.parser = { columnSeparator: ',' }
  cases.find(c => c.id === 'NI-E2E-07').input.config.execution.memory = '512M'
  cases.find(c => c.id === 'NI-E2E-09').input.config.direct = true
  cases.find(c => c.id === 'NI-E2E-10').input.config.password = 'synthetic-rejected-only'
  Object.assign(cases.find(c => c.id === 'NI-E2E-14').input.config.execution, { strict: 'true', maxDiscards: '0', maxErrors: '0' })
  Object.assign(cases.find(c => c.id === 'NI-E2E-20').input.config.execution, { maxDiscards: '0', maxErrors: '0' })
  cases.find(c => c.id === 'NI-E2E-20').input.config.logPath += '-default-strict'
  if (input.parameterCases) {
    // 扩展批次仍使用同一普通业务 API 与执行证据；只接受本轮所有权目录内的固定夹具。
    require(Array.isArray(input.parameterCases) && input.parameterCases.length > 0 && input.parameterCases.length <= 72, 'PARAMETER_CASES_INVALID')
    cases = input.parameterCases.map(test => {
      require(/^NI-PARAM-[a-z_]+$/.test(test.id) && test.input?.configVersion === 'normal-import-v1', 'PARAMETER_CASE_INVALID')
      const config = test.input.config
      require(config.filePath.startsWith(`${input.inputRoot}\\parameters\\`) && config.logPath.startsWith(`${input.inputRoot}\\logs-parameter-`), 'PARAMETER_FIXTURE_SCOPE_INVALID')
      return { ...test, input: { configVersion: 'normal-import-v1', config: { ...config, dataSourceId: input.sourceId, nodeId: input.nodeId, database: input.database } } }
    })
  }
  if (input.uiTaskId) {
    state.active = 'NI-E2E-01'
    const task = await poll(`/api/v1/tasks/${input.uiTaskId}/execution`, value => ['SUCCEEDED', 'FAILED', 'CANCELLED'].includes(value.state), 180000)
    const command = await accepted(`/api/v1/tasks/${input.uiTaskId}/command-evidence`)
    const snapshot = await accepted(`/api/v1/tasks/${input.uiTaskId}/snapshot`)
    const check = await accepted(`/api/v1/normal-import-prechecks/${snapshot.precheckId}`)
    const logs = await accepted(`/api/v1/tasks/${input.uiTaskId}/logs`)
    state.records.push({ id: 'NI-E2E-01', title: '真实五步页面 CSV 特殊值导入', taskId: input.uiTaskId,
      state: task.state, exitCode: task.exitCode, plannedActualMatch: task.plannedActualMatch, reconciliationRequired: task.reconciliationRequired,
      summary: task.resultSummary, snapshotMatched: check.configFingerprint === snapshot.configFingerprint,
      checks: check.results, precheckId: check.id, commandMasked: command.redaction === 'PASSWORD_ONLY' && command.command.includes('******'),
      logCount: logs.items?.length, successLog: logs.items?.some(l => /System exit 0/.test(l.message)),
      outcome: task.state === 'SUCCEEDED' && task.exitCode === 0 && task.plannedActualMatch === 'MATCHED' && !task.reconciliationRequired ? 'CONTENT_PENDING' : 'FAIL' })
  }
  for (const test of cases.filter(c => !input.ids || input.ids.includes(c.id))) {
    const record = { id: test.id, title: test.title, startedAt: new Date().toISOString() }
    state.active = test.id
    state.current = record
    try {
      const created = await request('/api/v1/normal-import-drafts', 'POST', test.input)
      if (test.apiRejected) {
        require([400, 422].includes(created.status), 'UNKNOWN_FIELD_ACCEPTED')
        Object.assign(record, { outcome: 'PASS', httpStatus: created.status, code: created.body.error?.code ?? created.body.code })
      } else {
        require(created.status === 201, created.body.error?.code ?? created.body.code ?? 'DRAFT_CREATE_FAILED')
        let draft = created.body
        record.draftId = draft.id
        await runtime?.publish?.(state)
        const route = `/api/v1/normal-import-drafts/${draft.id}`
        const preview = await accepted(`${route}:preview-command`, 'POST', {}, draft.revision)
        require(preview.configFingerprint === draft.configFingerprint && preview.redactedCommand.startsWith('obloader ') && preview.redactedCommand.includes('******') && !/--direct|--rpc-port|--parallel/.test(preview.redactedCommand), 'COMMAND_INVALID')
        record.fingerprint = draft.configFingerprint
        if (test.names) {
          require(test.names.every(name => preview.redactedCommand.includes(name)), 'EXPECTED_PARAMETER_MISSING')
          record.parameterNames = test.names
        }
        const check = await accepted(`${route}:${test.filesOnly ? 'discover-files' : 'precheck'}`, 'POST', {}, draft.revision)
        record.precheckId = check.id
        await runtime?.publish?.(state)
        const checked = await poll(`/api/v1/normal-import-prechecks/${check.id}`, value => !['PENDING', 'LEASED'].includes(value.status))
        Object.assign(record, { checks: checked.results, checkStatus: checked.status, manifestFiles: checked.inputManifest?.fileCount })
        require(checked.draftRevision === draft.revision && checked.configFingerprint === draft.configFingerprint && checked.integrityStatus === 'COMPLETE', 'CHECK_BINDING_INVALID')
        if (test.failure || test.failureCheck || test.filesOnly || test.stale) {
          if (test.failure) require(checked.status === 'FAILED' && checked.results.some(r => r.status === 'FAILED' && r.evidenceCode === test.failure), 'EXPECTED_FAILURE_MISSING')
          if (test.failureCheck) require(checked.status === 'FAILED' && checked.results.some(r => r.check === test.failureCheck && r.status !== 'PASSED'), 'EXPECTED_FAILURE_MISSING')
          if (test.filesOnly || test.stale) require(checked.status === 'SUCCEEDED', 'CHECK_NOT_PASSED')
          if (test.stale) {
            draft = await accepted(route, 'PATCH', { configVersion: 'normal-import-v1', config: { ...test.input.config, execution: { memory: '1G', thread: '2' } } }, draft.revision)
          }
          const submitted = await request(`${route}:submit`, 'POST', { precheckId: check.id }, draft.revision)
          require([409, 422].includes(submitted.status), 'INVALID_CHECK_SUBMITTED')
          Object.assign(record, { outcome: 'PASS', submitStatus: submitted.status, submitCode: submitted.body.error?.code ?? submitted.body.code })
        } else {
          require(checked.status === 'SUCCEEDED' && checked.results.length === 6 && checked.results.every(r => r.status === 'PASSED'), 'PRECHECK_NOT_PASSED')
          const body = { precheckId: check.id, confirmedConfigFingerprint: draft.configFingerprint, taskName: `普通导入真实验证-${test.id}-${input.runId.slice(0,8)}` }
          if (test.input.config.replaceData || test.input.config.replaceObject || ['TRUNCATE', 'DELETE'].includes(test.input.config.cleanup) || test.input.config.objects?.['obj-user'] || test.input.config.objects?.role) {
            const unconfirmed = await request(`${route}:submit`, 'POST', { precheckId: check.id }, draft.revision)
            require(unconfirmed.status === 422, 'UNCONFIRMED_RISK_SUBMITTED')
            record.unconfirmedRiskRejected = true
          }
          const key = crypto.randomUUID()
          const submitted = await accepted(`${route}:submit`, 'POST', body, draft.revision, key)
          record.taskId = submitted.id
          await runtime?.publish?.(state)
          const replay = await accepted(`${route}:submit`, 'POST', body, draft.revision, key)
          record.submitIdempotent = replay.id === submitted.id
          require(record.submitIdempotent, 'SUBMIT_REPLAY_MISMATCH')
          const execution = await poll(`/api/v1/tasks/${submitted.id}/execution`, value => ['SUCCEEDED', 'FAILED', 'CANCELLED'].includes(value.state), 180000)
          Object.assign(record, { state: execution.state, exitCode: execution.exitCode, plannedActualMatch: execution.plannedActualMatch, reconciliationRequired: execution.reconciliationRequired, summary: execution.resultSummary, errorCode: execution.errorCode })
          const command = await accepted(`/api/v1/tasks/${submitted.id}/command-evidence`)
          record.commandMatched = command.command === preview.redactedCommand
          const snapshot = await accepted(`/api/v1/tasks/${submitted.id}/snapshot`)
          record.snapshotMatched = snapshot.configFingerprint === preview.configFingerprint
          const logs = await accepted(`/api/v1/tasks/${submitted.id}/logs`)
          record.logCount = logs.items?.length
          record.successLog = logs.items?.some(l => /System exit 0/.test(l.message))
          require(execution.plannedActualMatch === 'MATCHED' && !execution.reconciliationRequired && record.commandMatched && record.snapshotMatched, 'EXECUTION_NOT_VERIFIED')
          if (test.taskFailed) {
            require(execution.state === 'FAILED' && execution.exitCode !== 0, 'EXPECTED_TOOL_FAILURE_MISSING')
            record.duplicateLog = logs.items?.some(l => /duplicate entry|duplicate key|Too many discard records/.test(l.message))
            require(record.duplicateLog, 'DUPLICATE_EVIDENCE_MISSING')
            record.outcome = 'FAILURE_CONTENT_PENDING'
          } else {
            require(execution.state === 'SUCCEEDED' && execution.exitCode === 0, 'EXECUTION_NOT_VERIFIED')
            record.outcome = 'CONTENT_PENDING'
          }
        }
      }
    } catch (error) {
      record.outcome = 'FAIL'
      record.code = /^[A-Z_0-9]+$/.test(error.message) ? error.message : 'TEST_UNAVAILABLE'
      if (record.code === 'POLL_TIMEOUT' && record.taskId) {
        // 超时后仅取消本轮唯一任务，未确认终态前不再启动其他任务。
        await request(`/api/v1/tasks/${record.taskId}:cancel`, 'POST', {})
        state.stopped = true
      }
    }
    record.finishedAt = new Date().toISOString()
    state.records.push(record)
    await runtime?.publish?.(state)
    if (state.stopped) break
  }
  state.done = true
  state.active = ''
  state.finishedAt = new Date().toISOString()
  await runtime?.publish?.(state)
  return state
}
