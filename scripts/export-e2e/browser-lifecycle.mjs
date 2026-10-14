// 生命周期使用同一正常浏览器会话和固定产品 API，不复制身份或跳过权限检查。
export async function browserLifecycle(input) {
  const result = { records: [], active: '', done: false }
  window.__odoMysqlLifecycle = result
  const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))
  const check = (condition, code) => { if (!condition) throw new Error(code) }
  async function api(route, method = 'GET', body, revision, key = crypto.randomUUID()) {
    const headers = { Accept: 'application/json' }
    if (method !== 'GET') { headers['Content-Type'] = 'application/json'; headers['X-CSRF-Token'] = document.querySelector('meta[name="ob-data-orch-csrf-token"]').content; headers['Idempotency-Key'] = key; if (revision !== undefined) headers['If-Match'] = `"rev-${revision}"` }
    const response = await fetch(route, { method, headers, ...(body !== undefined ? { body: JSON.stringify(body) } : {}) })
    return { status: response.status, body: await response.json().catch(() => ({})) }
  }
  async function read(route) { const value = await api(route); check(value.status === 200, value.body.code ?? 'READ_REJECTED'); return value.body.item ?? value.body }
  async function write(route, body = {}, revision, key, method = 'POST') { const value = await api(route, method, body, revision, key); check(value.status >= 200 && value.status < 300, value.body.code ?? 'WRITE_REJECTED'); return value.body.item ?? value.body }
  async function poll(route, predicate, max = 180_000) { const end = Date.now() + max; while (Date.now() < end) { const value = await read(route); if (predicate(value)) return value; await sleep(600) } throw new Error('LIFECYCLE_TIMEOUT') }
  async function ready(config) {
    const created = await write('/api/v1/export-drafts', { configVersion: 'v6', dataSourceId: input.sourceId, nodeId: input.nodeId, config })
    const draft = await read(`/api/v1/export-drafts/${created.id}`)
    const started = await write(`/api/v1/export-drafts/${draft.id}:precheck`, {}, draft.revision)
    const checked = await poll(`/api/v1/prechecks/${started.id}`, value => !['PENDING', 'LEASED'].includes(value.status))
    check(checked.status === 'SUCCEEDED' && checked.integrityStatus === 'COMPLETE' && checked.results.every(item => item.status === 'PASSED'), 'PRECHECK_NOT_PASSED')
    return { draft, checked }
  }
  const submit = (prepared, key) => write(`/api/v1/export-drafts/${prepared.draft.id}:submit`, { precheckId: prepared.checked.id }, prepared.draft.revision, key)
  const terminal = id => poll(`/api/v1/tasks/${id}/execution`, value => ['SUCCEEDED', 'FAILED', 'CANCELLED'].includes(value.state))
  for (const test of input.cases) {
    result.active = test.id
    let record = { id: test.id, outcome: 'PASS' }
    result.current=record
    try {
      if (test.workflow === 'INVALIDATION') {
        const prepared = await ready(test.config)
        const config = structuredClone(test.config); config.performanceConfig.thread = 2
        const updated = await write(`/api/v1/export-drafts/${prepared.draft.id}`, { configVersion: 'v6', dataSourceId: input.sourceId, nodeId: input.nodeId, config }, prepared.draft.revision, undefined, 'PATCH')
        check(updated.revision === prepared.draft.revision + 1 && updated.configFingerprint !== prepared.draft.configFingerprint, 'CONFIG_NOT_INVALIDATED')
        const stale = await read(`/api/v1/prechecks/${prepared.checked.id}`)
        // 历史检查可以保留原结论；修订与指纹绑定必须失效，提交门禁才是可用性的依据。
        check(stale.draftRevision !== updated.revision && stale.configFingerprint !== updated.configFingerprint, 'OLD_PRECHECK_BINDING_REUSED')
        const rejected = await api(`/api/v1/export-drafts/${updated.id}:submit`, 'POST', { precheckId: prepared.checked.id }, updated.revision)
        check([409, 422].includes(rejected.status), 'STALE_PRECHECK_SUBMITTED')
        record.oldStatus = stale.status; record.submitStatus = rejected.status
        const started = await write(`/api/v1/export-drafts/${updated.id}:precheck`, {}, updated.revision)
        const fresh = await poll(`/api/v1/prechecks/${started.id}`, value => !['PENDING','LEASED'].includes(value.status))
        check(fresh.status === 'SUCCEEDED', 'UPDATED_PRECHECK_NOT_PASSED'); record.newStatus = fresh.status
      } else if (['IDEMPOTENCY', 'SUBMIT_REPLAY'].includes(test.workflow)) {
        const body = { configVersion: 'v6', dataSourceId: input.sourceId, nodeId: input.nodeId, config: test.config }, key = crypto.randomUUID()
        const a = await write('/api/v1/export-drafts', body, undefined, key), b = await write('/api/v1/export-drafts', body, undefined, key)
        check(a.id === b.id, 'DRAFT_REPLAY_DUPLICATE')
        const conflicting = structuredClone(body); conflicting.config.performanceConfig.thread = 2
        check((await api('/api/v1/export-drafts', 'POST', conflicting, undefined, key)).status === 409, 'CONFLICTING_IDEMPOTENCY_ACCEPTED')
        const initial = await read(`/api/v1/export-drafts/${a.id}`), precheckKey = crypto.randomUUID()
        const draft = await write(`/api/v1/export-drafts/${a.id}`, body, initial.revision, undefined, 'PATCH')
        const stale = await api(`/api/v1/export-drafts/${a.id}`, 'PATCH', body, initial.revision)
        check([409,412].includes(stale.status), 'STALE_REVISION_ACCEPTED')
        const ca = await write(`/api/v1/export-drafts/${a.id}:precheck`, {}, draft.revision, precheckKey), cb = await write(`/api/v1/export-drafts/${a.id}:precheck`, {}, draft.revision, precheckKey)
        check(ca.id === cb.id, 'PRECHECK_REPLAY_DUPLICATE')
        const checked = await poll(`/api/v1/prechecks/${ca.id}`, value => !['PENDING', 'LEASED'].includes(value.status))
        check(checked.status === 'SUCCEEDED', 'PRECHECK_NOT_PASSED')
        const submittedKey = crypto.randomUUID(), ta = await submit({ draft, checked }, submittedKey), tb = await submit({ draft, checked }, submittedKey)
        check(ta.id === tb.id, 'SUBMIT_REPLAY_DUPLICATE')
        record.taskId = ta.id; const execution = await terminal(ta.id); check(execution.state === 'SUCCEEDED', 'REPLAY_TASK_FAILED')
        record.summary = execution.resultSummary; record.outcome = 'CONTENT_PENDING'; record.replaysVerified = 3
      } else if (test.workflow === 'EXPIRY') {
        const prepared = await ready(test.config)
        await sleep(Math.max(0, Date.parse(prepared.checked.validUntil) - Date.now()) + 1000)
        const rejected = await api(`/api/v1/export-drafts/${prepared.draft.id}:submit`, 'POST', { precheckId: prepared.checked.id }, prepared.draft.revision)
        check([409, 422].includes(rejected.status), 'EXPIRED_PRECHECK_SUBMITTED'); record.submitStatus = rejected.status
      } else if (test.workflow === 'CANCEL_QUEUED' || test.workflow === 'CANCEL_RUNNING') {
        const prepared = await ready(test.config), task = await submit(prepared)
        record.taskId = task.id
        const running = test.workflow === 'CANCEL_RUNNING'
        const before = running ? await poll(`/api/v1/tasks/${task.id}/execution`, value => value.state === 'RUNNING' || ['SUCCEEDED','FAILED'].includes(value.state)) : await read(`/api/v1/tasks/${task.id}/execution`)
        if (running && before.state !== 'RUNNING') { record.outcome = 'BLOCKED'; record.code = 'RUNNING_WINDOW_NOT_OBSERVED' }
        else {
          if(running){record.executionId=before.executionId;result.liveAuditExecutionId=before.executionId;await sleep(8000)}
          const key = crypto.randomUUID(), a = await write(`/api/v1/tasks/${task.id}:cancel`, {}, undefined, key), b = await write(`/api/v1/tasks/${task.id}:cancel`, {}, undefined, key)
          check(a.taskId === b.taskId && b.replayed === true, 'CANCEL_REPLAY_NOT_STABLE')
          const end = await terminal(task.id); check(end.state === 'CANCELLED' && !end.reconciliationRequired, 'CANCEL_NOT_CONFIRMED')
          record.state = end.state; record.runningProof = running
          if (!running && (before.state !== 'WAITING_SCHEDULE' || end.startedAt)) { record.outcome = 'PARTIAL'; record.code = 'QUEUED_WINDOW_NOT_OBSERVED' }
        }
      } else if (test.workflow === 'CHECKPOINT_NEGATIVE') {
        if (!input.failedTaskId) { record.outcome = 'BLOCKED'; record.code = 'FAILED_TASK_WITHOUT_CHECKPOINT_REQUIRED' }
        else { const rejected = await api(`/api/v1/tasks/${input.failedTaskId}:resume-checkpoint`, 'POST'); check([409, 422].includes(rejected.status), 'NO_CHECKPOINT_RESUMED'); record.httpStatus = rejected.status }
      } else if (test.workflow === 'TEMPLATE') {
        check(input.successfulTaskId, 'SUCCESSFUL_TASK_REQUIRED')
        const saved = await write(`/api/v1/tasks/${input.successfulTaskId}:save-template`, { displayName: `odo_e2e_${input.runId.slice(0,12)}` })
        let revision = 1
        try {
          const catalog = await read('/api/v1/export-config-templates'), template = catalog.items.find(value => value.id === saved.id)
          check(template, 'SAVED_TEMPLATE_NOT_LISTED'); revision = template.revision
          const changed = await write(`/api/v1/export-config-templates/${saved.id}`, { displayName: `odo_e2e_${input.runId.slice(0,12)}_renamed` }, revision, undefined, 'PATCH'); revision = changed.revision
          const created = await write(`/api/v1/export-config-templates/${saved.id}:create-draft`, { dataSourceId: input.sourceId, nodeId: input.nodeId })
          const draft = await read(`/api/v1/export-drafts/${created.draftId}`)
          check(draft.dataSourceId === input.sourceId && draft.nodeId === input.nodeId, 'TEMPLATE_BINDING_INVALID')
          record.draftId = draft.id; record.templateId = saved.id
          const config = structuredClone(draft.config?.config??draft.config)
          config.outputConfig = test.config.outputConfig
          const updated = await write(`/api/v1/export-drafts/${draft.id}`, {configVersion:'v6',dataSourceId:input.sourceId,nodeId:input.nodeId,config}, draft.revision, undefined, 'PATCH')
          const started = await write(`/api/v1/export-drafts/${draft.id}:precheck`, {}, updated.revision)
          const checked = await poll(`/api/v1/prechecks/${started.id}`, value => !['PENDING','LEASED'].includes(value.status))
          check(checked.status === 'SUCCEEDED', 'TEMPLATE_PRECHECK_NOT_PASSED')
          const task = await submit({draft:updated,checked}), execution = await terminal(task.id)
          check(execution.state === 'SUCCEEDED', 'TEMPLATE_EXECUTION_FAILED')
          record.taskId = task.id; record.summary = execution.resultSummary; record.outcome = 'CONTENT_PENDING'
        } finally { const deleted = await api(`/api/v1/export-config-templates/${saved.id}`, 'DELETE', {}, revision); check(deleted.status === 204 || deleted.status === 200, 'OWNED_TEMPLATE_CLEANUP_FAILED') }
      } else if (test.workflow === 'DERIVE') {
        // 先在当前节点事实版本生成失败种子，避免历史任务绑定变化混入派生语义。
        const seedConfig=structuredClone(test.config)
        seedConfig.objectScope={database:test.config.objectScope.database,scopeKind:'QUERY_RESULT'}
        seedConfig.filterConfig={querySql:`SELECT missing_column FROM ${test.config.objectScope.database}.rich`,queryResultLimit:10000}
        seedConfig.outputConfig.filePath+='-seed';seedConfig.outputConfig.logPath+='-seed'
        const seedPrepared=await ready(seedConfig),seedTask=await submit(seedPrepared),seedExecution=await terminal(seedTask.id)
        check(seedExecution.state==='FAILED'&&Number.isInteger(seedExecution.exitCode)&&seedExecution.exitCode!==0,'FAILED_SEED_REQUIRED')
        record.seedTaskId=seedTask.id
        const rebuilt = await write(`/api/v1/tasks/${seedTask.id}:rebuild-draft`, {derivation:'REBUILD_FROM_CONFIG'})
        const original = await read(`/api/v1/export-drafts/${rebuilt.draftId}`)
        const updated = await write(`/api/v1/export-drafts/${original.id}`, {configVersion:'v6',dataSourceId:input.sourceId,nodeId:input.nodeId,config:test.config}, original.revision, undefined, 'PATCH')
        const started = await write(`/api/v1/export-drafts/${updated.id}:precheck`, {}, updated.revision)
        const checked = await poll(`/api/v1/prechecks/${started.id}`, value => !['PENDING','LEASED'].includes(value.status))
        check(checked.status === 'SUCCEEDED', 'DERIVED_PRECHECK_NOT_PASSED')
        const task = await submit({draft:updated,checked}), execution = await terminal(task.id)
        check(execution.state === 'SUCCEEDED', 'DERIVED_EXECUTION_FAILED')
        const overview = await read(`/api/v1/tasks/${task.id}`)
        check(overview.parentTaskId === seedTask.id && overview.derivationKind === 'REBUILD_FROM_CONFIG', 'DERIVED_PARENT_MISMATCH')
        const rerun = await write(`/api/v1/tasks/${seedTask.id}:rebuild-draft`, {derivation:'RERUN_FROM_SCRATCH'})
        const rerunDraft = await read(`/api/v1/export-drafts/${rerun.draftId}`), snapshot = await read(`/api/v1/tasks/${seedTask.id}/snapshot`)
        check(rerunDraft.configFingerprint === snapshot.configFingerprint, 'RERUN_CONFIG_MISMATCH')
        const changedConfig=structuredClone(test.config)
        // 输出目录也属于冻结配置；使用新空目录才能独立验证提交时的从头重跑约束。
        changedConfig.outputConfig.filePath+='-rerun';changedConfig.outputConfig.logPath+='-rerun'
        const changed = await api(`/api/v1/export-drafts/${rerunDraft.id}`, 'PATCH', {configVersion:'v6',dataSourceId:input.sourceId,nodeId:input.nodeId,config:changedConfig}, rerunDraft.revision)
        if(changed.status===200){
          // 从头重跑的冻结约束在提交时复验；草稿可编辑不能替代该断言。
          const updatedRerun=await read(`/api/v1/export-drafts/${rerunDraft.id}`)
          const startedRerun=await write(`/api/v1/export-drafts/${updatedRerun.id}:precheck`,{},updatedRerun.revision)
          const checkedRerun=await poll(`/api/v1/prechecks/${startedRerun.id}`,value=>!['PENDING','LEASED'].includes(value.status))
          check(checkedRerun.status==='SUCCEEDED','RERUN_RECHECK_NOT_PASSED')
          const rejected=await api(`/api/v1/export-drafts/${updatedRerun.id}:submit`,'POST',{precheckId:checkedRerun.id},updatedRerun.revision)
          check([409,422].includes(rejected.status),'FROZEN_RERUN_SUBMITTED')
        } else check([409,422].includes(changed.status), 'FROZEN_RERUN_MODIFIED')
        record.taskId=task.id; record.summary=execution.resultSummary; record.outcome='CONTENT_PENDING'; record.rerunFingerprintMatched=true; record.rerunChangeRejected=true
      } else if (test.workflow === 'AUTHORIZATION') {
        const anonymous = await fetch('/api/v1/export-drafts/'+crypto.randomUUID(),{credentials:'omit'})
        check(anonymous.status===401, 'UNAUTHENTICATED_NOT_REJECTED')
        const csrf = await fetch('/api/v1/export-drafts',{method:'POST',headers:{'Content-Type':'application/json','Idempotency-Key':crypto.randomUUID()},body:'{}'})
        check([401,403].includes(csrf.status), 'MISSING_CSRF_NOT_REJECTED')
        check((await api('/api/v1/export-drafts/'+crypto.randomUUID())).status===404, 'UNKNOWN_OBJECT_DISCLOSED')
        record.outcome='PARTIAL'; record.code='SECOND_AUTHORIZED_IDENTITY_REQUIRED'; record.unauthenticatedRejected=true; record.csrfRejected=true; record.unknownObjectHidden=true
      } else if(test.workflow==='LOGS'){
        const prepared=await ready(test.config),task=await submit(prepared)
        record.taskId=task.id
        await poll(`/api/v1/tasks/${task.id}/execution`,value=>['STARTING','RUNNING','SUCCEEDED','FAILED'].includes(value.state))
        async function stream(after){
          const stop=new AbortController(),timer=setTimeout(()=>stop.abort(),45_000)
          let last=after,buffer='',events=[]
          try{
            const response=await fetch(`/api/v1/tasks/${task.id}/logs/stream`+(after?'?after='+encodeURIComponent(after):''),{signal:stop.signal})
            check(response.status===200&&response.headers.get('Content-Type')?.includes('text/event-stream'),'REAL_SSE_NOT_AVAILABLE')
            const reader=response.body.getReader(),decode=new TextDecoder()
            while(events.length<5){
              const chunk=await reader.read();if(chunk.done)break
              buffer+=decode.decode(chunk.value,{stream:true});check(buffer.length<256*1024,'SSE_RECORD_LIMIT')
              let end
              while((end=buffer.indexOf('\n\n'))>=0){
                const message=buffer.slice(0,end);buffer=buffer.slice(end+2)
                const id=/^id: (.+)$/m.exec(message),data=/^data: (.+)$/m.exec(message)
                if(id&&data){last=id[1];events.push(JSON.parse(data[1]))}
              }
            }
          }finally{clearTimeout(timer);stop.abort()}
          return {last,events}
        }
        const first=await stream();check(first.events.length>0,'SSE_EVENTS_MISSING')
        const live=await read(`/api/v1/tasks/${task.id}/execution`)
        let resumed={events:[]}
        if(['STARTING','RUNNING'].includes(live.state))resumed=await stream(first.last)
        const keys=first.events.concat(resumed.events).map(item=>`${item.sourceKind}:${item.sourceEpoch}:${item.sourceSeq}:${item.receivedAt}`)
        check(new Set(keys).size===keys.length,'SSE_RECONNECT_DUPLICATED')
        const execution=await terminal(task.id);check(execution.state==='SUCCEEDED','LOG_TASK_FAILED')
        let cursor,count=0,pages=0,seen=new Set()
        do{
          const page=await read(`/api/v1/tasks/${task.id}/logs`+(cursor?'?cursor='+encodeURIComponent(cursor):''))
          check(page.integrity==='PERSISTED_DOUBLE_REDACTED','LOG_INTEGRITY_NOT_CONFIRMED')
          for(const item of page.items){const key=`${item.sourceKind}:${item.sourceEpoch}:${item.sourceSeq}:${item.receivedAt}`;check(!seen.has(key),'REST_LOG_DUPLICATED');seen.add(key);count++}
          cursor=page.nextCursor;pages++;check(pages<20,'REST_LOG_LIMIT')
        }while(cursor)
        check(count>0,'PERSISTED_LOGS_MISSING')
        record.state=execution.state;record.executionId=execution.executionId;record.sseEvents=first.events.length;record.reconnectedEvents=resumed.events.length;record.restRecords=count;record.restPages=pages
        record.outcome='PARTIAL';record.code='LOG_DOWNLOAD_NOT_IMPLEMENTED'
      } else if (test.workflow === 'CHECKPOINT_POSITIVE') {
        record.outcome='BLOCKED'; record.code='REAL_FAILED_CHECKPOINT_UNAVAILABLE'
      } else if (test.workflow === 'AGENT_RECOVERY') {
        record.outcome='BLOCKED'; record.code='ACTIVE_PROCESS_REATTACH_ENTRY_UNAVAILABLE'
      } else { record.outcome = 'BLOCKED'; record.code = 'MANUAL_PREREQUISITE_REQUIRED' }
    } catch (error) { record.outcome = 'FAIL'; record.code = /^[A-Z_0-9]+$/.test(error.message) ? error.message : 'LIFECYCLE_UNAVAILABLE' }
    result.records.push(record)
  }
  result.active = ''; result.done = true
}
