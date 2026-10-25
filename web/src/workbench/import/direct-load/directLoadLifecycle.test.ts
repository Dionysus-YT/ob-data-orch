import { afterEach, describe, expect, it, vi } from 'vitest'
import { computed, effectScope, ref } from 'vue'
import type { DataSourceSummary, ExecutionNodeSummary } from '@/api/browser'
import type { DirectLoadApi, DirectLoadConfig, DirectLoadDraft, DirectLoadCheck } from '@/api/directLoad'
import { applyDirectLoadScenario, createDirectLoadForm, directLoadInput, hydrateDirectLoad } from './directLoadForm'
import { directLoadDefaultRpcPort, directLoadFormats, directLoadInputProgress, directLoadIssues, directLoadLocalParameters, directLoadScenarioForSource, directLoadScenarioLabels, isDirectLoadSource } from './directLoadRules'
import { useDirectLoadDraft } from './useDirectLoadDraft'
import { useDirectLoadReview } from './useDirectLoadReview'
const scopes: ReturnType<typeof effectScope>[] = []
afterEach(() => { scopes.splice(0).forEach(scope => scope.stop()); vi.unstubAllGlobals() })
const record = (config: DirectLoadConfig, revision = 1): DirectLoadDraft => ({ id:'draft', revision, configVersion:'direct-load-v1', config:JSON.parse(JSON.stringify(config)), configFingerprint:'a'.repeat(64), executionEnabled:false })
function deferred<T>() { let resolve!: (value:T) => void; const promise = new Promise<T>(yes => {resolve=yes}); return {promise,resolve} }
function setup() {
  const scope=effectScope(); scopes.push(scope)
  return scope.run(() => {
    const form=createDirectLoadForm(); Object.assign(form,{dataSourceId:'source',nodeId:'node',database:'synthetic_db',scenario:'PRIVATE_ODP',rpcPort:2885,table:'synthetic_table',filePath:'E:\\synthetic',format:'CSV'}); Object.keys(form.applicability).forEach(key => {form.applicability[key as keyof typeof form.applicability]=true})
    const api:DirectLoadApi={ createDirectLoadDraft:vi.fn(async config=>record(config)),updateDirectLoadDraft:vi.fn(async(draft,config)=>record(config,draft.revision+1)),getDirectLoadDraft:vi.fn(),deleteDirectLoadDraft:vi.fn(),previewDirectLoadCommand:vi.fn(async()=>({redactedCommand:'obloader --direct -p ******',configFingerprint:'a'.repeat(64),executionEnabled:false})),startDirectLoadCheck:vi.fn(async()=> 'check'),getDirectLoadCheck:vi.fn(),submitDirectLoad:vi.fn(async()=> 'task'),getDirectLoadParameters:vi.fn() }
    const input=computed(()=>directLoadInput(form)), draft=useDirectLoadDraft({api,form,input,bindRoute:vi.fn(async()=>undefined)}), binding=computed(()=>JSON.stringify(form)), navigateTask=vi.fn(async()=>undefined)
    const review=useDirectLoadReview({api,draft,binding,ready:ref(true),navigateTask})
    return {scope,form,api,draft,review,navigateTask}
  })!
}
function report(draft: DirectLoadDraft):DirectLoadCheck {return {id:'check',draftId:draft.id,draftRevision:draft.revision,configFingerprint:draft.configFingerprint,checkKind:'DIRECT_LOAD_PREFLIGHT',status:'SUCCEEDED',integrityStatus:'COMPLETE',validUntil:new Date(Date.now()+60000).toISOString(),executionEnabled:true,directLoadConfigFingerprint:'d'.repeat(64),results:['TOOL_ENVIRONMENT','NODE_RESOURCE','INPUT_FILES','DATABASE_CONNECTIVITY','RPC_CONNECTIVITY','DIRECT_LOAD_VERSION','WRITE_ACCESS','DIRECT_LOAD_STRUCTURE','DIRECT_LOAD_CONFIG'].map(check=>({check,status:'PASSED',evidenceCode:'DIRECT_LOAD_VERIFIED'})),inputManifest:{fileCount:1,totalBytes:1,digest:'b'.repeat(64),files:[{path:'synthetic.csv',object:'synthetic_table',size:1,modified:'1',sha256:'c'.repeat(64)}]}}}
describe('旁路表单、草稿与检查会话',()=>{
  it('使用服务端直连枚举并从数据源带出场景，未知别名不取得资格', () => {
    const source = { connectionKind: 'OBSERVER_DIRECT', state: 'ENABLED', lastTestStatus: 'SUCCEEDED', compatibilityMode: 'MYSQL' } as DataSourceSummary
    expect(isDirectLoadSource(source)).toBe(true)
    expect(directLoadScenarioForSource(source)).toBe('OBSERVER_DIRECT')
    expect(directLoadScenarioForSource({ connectionKind: 'ODP' })).toBe('PRIVATE_ODP')
    expect(isDirectLoadSource({ ...source, connectionKind: 'OBSERVER' })).toBe(false)
    expect(directLoadScenarioForSource({ connectionKind: 'OBSERVER' })).toBe('')
    expect(isDirectLoadSource({ ...source, lastTestStatus: 'INVALIDATED' })).toBe(false)
  })
  it('五步进度按字段与入口确认计算，无效可选值不能显示完成', () => {
    const form = createDirectLoadForm()
    expect(directLoadInputProgress(1, form, ['dataSourceId', 'scenario', 'rpcPort', 'applicability'].map(field => ({ step: 1, field, message: '' })))).toBe(0)
    expect(directLoadInputProgress(1, form, [{ step: 1, field: 'applicability', message: '' }])).toBe(75)
    expect(directLoadInputProgress(2, form, [{ step: 2, field: 'table', message: '' }])).toBe(50)
    expect(directLoadInputProgress(3, form, [{ step: 3, field: 'filePath', message: '' }])).toBe(67)
    expect(directLoadInputProgress(3, form, [{ step: 3, field: 'columnSeparator', message: '' }])).toBe(75)
    expect(directLoadInputProgress(4, form, [])).toBe(100)
    expect(directLoadInputProgress(4, form, [{ step: 4, field: 'parallel', message: '' }])).toBe(0)
  })
  it('RPC按场景预填且可修改，不复制SQL端口，不在恢复时改写已保存值', () => {
    const form = createDirectLoadForm()
    expect(directLoadDefaultRpcPort('')).toBe(0)
    applyDirectLoadScenario(form, 'PRIVATE_ODP'); expect(form.rpcPort).toBe(2885)
    applyDirectLoadScenario(form, 'OBSERVER_DIRECT'); expect(form.rpcPort).toBe(2882)
    form.rpcPort = 12885
    applyDirectLoadScenario(form, 'PRIVATE_ODP'); expect(form.rpcPort).toBe(12885)
    form.rpcPort = 0
    applyDirectLoadScenario(form, 'OBSERVER_DIRECT'); expect(form.rpcPort).toBe(2882)
    hydrateDirectLoad(form, { ...form, rpcPort: 12882 }); expect(form.rpcPort).toBe(12882)
    hydrateDirectLoad(form, { ...form, rpcPort: 0 }); expect(form.rpcPort).toBe(0)
  })
  it('本地场景和目录不包含云入口，历史配置保留原值且阻断活动资格', () => {
    expect(Object.keys(directLoadScenarioLabels)).toEqual(['PRIVATE_ODP', 'OBSERVER_DIRECT'])
    const s = setup(), source = { id: 'source', connectionKind: 'ODP', state: 'ENABLED', lastTestStatus: 'SUCCEEDED', compatibilityMode: 'MYSQL' } as DataSourceSummary
    s.form.scenario = 'CLOUD_ODP'
    expect(directLoadDefaultRpcPort(s.form.scenario)).toBe(0)
    expect(directLoadFormats(s.form.scenario)).toEqual([])
    expect(directLoadIssues(s.form, source).map(issue => issue.field)).toEqual(expect.arrayContaining(['scenario', 'format']))
    expect(s.form.scenario).toBe('CLOUD_ODP')
    const parameters = ['public-cloud', 'sql', 'par', 'orc', 'rpc-port', 'tenant'].map(longName => ({ longName, supportState: 'ENABLED', category: '', sourcePolicy: '', description: '历史 Cloud 云说明', disposition: '历史 Cloud 云说明' }))
    const local = directLoadLocalParameters(parameters)
    expect(local.some(item => item.longName === 'public-cloud')).toBe(false)
    expect(local.filter(item => ['sql', 'par', 'orc'].includes(item.longName)).every(item => item.supportState === 'VALIDATION_GATED')).toBe(true)
    expect(local.some(item => /云|Cloud/.test(item.description + item.disposition))).toBe(false)
    expect(parameters[0]?.description).toBe('历史 Cloud 云说明')
  })
  it('入口默认未确认、格式按场景、唯一路径与多表规则独立校验',()=>{
    const s=setup(), source={id:'source',connectionKind:'ODP',state:'ENABLED',lastTestStatus:'SUCCEEDED',compatibilityMode:'MYSQL'} as DataSourceSummary,node={id:'node',platform:'WINDOWS_AMD64',acceptsNewTasks:true} as ExecutionNodeSummary
    expect(Object.values(createDirectLoadForm().applicability).every(value=>!value)).toBe(true)
    expect(directLoadFormats('PRIVATE_ODP')).toEqual(['CSV']); expect(directLoadIssues(s.form,source,node)).toEqual([])
    Object.assign(s.form,{table:'a,b',scenario:'PRIVATE_ODP',format:'SQL',filePath:'/linux-on-windows'})
    expect(directLoadIssues(s.form,source,node).map(issue=>issue.field)).toEqual(expect.arrayContaining(['table','format','filePath']))
    Object.assign(s.form,{database:'synthetic.database',table:'synthetic.table',columnSeparator:'\t',format:'CSV'});expect(directLoadIssues(s.form,source,node).map(issue=>issue.field)).toEqual(expect.arrayContaining(['database','table','columnSeparator']))
  })
  it('非活动CSV字段仅在本页暂存，既不写入其他格式，也不拼接命令',()=>{const s=setup();Object.assign(s.form,{columnSeparator:'|',externalData:true,format:'ORC'});expect(directLoadInput(s.form)).toMatchObject({columnSeparator:'',externalData:false});s.form.format='CSV';expect(directLoadInput(s.form)).toMatchObject({columnSeparator:'|',externalData:true})})
  it('保存中编辑保留输入与dirty，重复保存只发送一次',async()=>{const s=setup(), pending=deferred<DirectLoadDraft>();vi.mocked(s.api.createDirectLoadDraft).mockReturnValue(pending.promise);const snapshot=record(s.form), first=s.draft.saveDraft();expect(await s.draft.saveDraft()).toBeUndefined();s.form.table='changed';pending.resolve(snapshot);await first;expect(s.form.table).toBe('changed');expect(s.draft.draftDirty.value).toBe(true);expect(s.draft.operationNotice.value).toBe('');expect(s.api.createDirectLoadDraft).toHaveBeenCalledTimes(1)})
  it('从头执行拒绝改参，仅允许原配置重绑，保存响应不能漂移来源与锁定',async()=>{
    const s=setup(),rerun:DirectLoadDraft={...record(s.form),sourceTaskId:'synthetic-parent',sourceDerivation:'RERUN_FROM_SCRATCH',parametersLocked:true}
    vi.mocked(s.api.getDirectLoadDraft).mockResolvedValue(rerun);await s.draft.loadDraft('draft');expect(s.draft.rerunLocked.value).toBe(true)
    s.form.table='changed';expect(await s.draft.saveDraft()).toBeUndefined();expect(s.api.updateDirectLoadDraft).not.toHaveBeenCalled()
    s.form.table='synthetic_table';vi.mocked(s.api.updateDirectLoadDraft).mockResolvedValue({...rerun,revision:2});await s.draft.saveDraft();expect(s.api.updateDirectLoadDraft).toHaveBeenCalledTimes(1)
    vi.mocked(s.api.updateDirectLoadDraft).mockResolvedValue({...rerun,revision:3,sourceDerivation:'REBUILD_FROM_CONFIG',parametersLocked:false});await s.draft.saveDraft();expect(s.draft.currentDraft.value?.revision).toBe(2);expect(s.draft.draftFailure.value).not.toBe('')
  })
  it('A→B→A与卸载拒绝迟到命令、错误与finally，不恢复提交资格',async()=>{const s=setup(),pending=deferred<{redactedCommand:string;configFingerprint:string;executionEnabled:boolean}>();vi.mocked(s.api.previewDirectLoadCommand).mockReturnValue(pending.promise);const work=s.review.prepareReview();await Promise.resolve();await Promise.resolve();s.form.table='b';s.form.table='synthetic_table';pending.resolve({redactedCommand:'obloader --direct -p ******',configFingerprint:'a'.repeat(64),executionEnabled:true});await work;expect(s.review.commandPreview.value).toBeUndefined();expect(s.api.startDirectLoadCheck).not.toHaveBeenCalled();s.scope.stop();expect(s.review.canSubmit.value).toBe(false)})
  it('即使所有合成检查通过，独立执行开关关闭也不能提交',async()=>{const s=setup();vi.mocked(s.api.getDirectLoadCheck).mockImplementation(async()=>report(s.draft.currentDraft.value!));await s.review.prepareReview();expect(s.review.canSubmit.value).toBe(false);s.review.openSubmit();await s.review.submitTask();expect(s.api.submitDirectLoad).not.toHaveBeenCalled()})
  it('合成开放情况下名称必填、所有任务确认风险且方法入口防重',async()=>{const s=setup();vi.mocked(s.api.createDirectLoadDraft).mockImplementation(async config=>({...record(config),executionEnabled:true}));vi.mocked(s.api.previewDirectLoadCommand).mockResolvedValue({redactedCommand:'obloader --direct -p ******',configFingerprint:'a'.repeat(64),executionEnabled:true});vi.mocked(s.api.getDirectLoadCheck).mockImplementation(async()=>report(s.draft.currentDraft.value!));await s.review.prepareReview();expect(s.review.canSubmit.value).toBe(true);s.review.openSubmit();await s.review.submitTask();expect(s.api.submitDirectLoad).not.toHaveBeenCalled();s.review.riskAccepted.value=true;await s.review.submitTask();expect(s.review.nameError.value).not.toBe('');s.review.taskName.value='合成旁路任务';const pending=deferred<string>();vi.mocked(s.api.submitDirectLoad).mockReturnValue(pending.promise);const work=s.review.submitTask();await s.review.submitTask();expect(s.api.submitDirectLoad).toHaveBeenCalledTimes(1);pending.resolve('task');await work;expect(s.navigateTask).toHaveBeenCalledWith('task')})
})

async function readySubmission() {
  const s = setup()
  vi.mocked(s.api.createDirectLoadDraft).mockImplementation(async config => ({ ...record(config), executionEnabled: true }))
  vi.mocked(s.api.previewDirectLoadCommand).mockResolvedValue({ redactedCommand: 'obloader --direct -p ******', configFingerprint: 'a'.repeat(64), executionEnabled: true })
  vi.mocked(s.api.getDirectLoadCheck).mockImplementation(async () => report(s.draft.currentDraft.value!))
  await s.review.prepareReview()
  s.review.openSubmit(); s.review.taskName.value = '合成防重任务'; s.review.riskAccepted.value = true
  return s
}

it('提交响应丢失后复用原键与原名称，关闭再打开仍不创建新意图', async () => {
  const s = await readySubmission()
  vi.mocked(s.api.submitDirectLoad).mockRejectedValueOnce(new Error('NETWORK_UNAVAILABLE')).mockResolvedValueOnce('original-task')
  await s.review.submitTask()
  expect(s.review.submissionUncertain.value).toBe(true)
  expect(s.review.canReview.value).toBe(false)
  s.review.taskName.value = '另一个名称'; await s.review.submitTask()
  expect(s.api.submitDirectLoad).toHaveBeenCalledTimes(1)
  s.review.closeSubmit(); s.review.openSubmit(); s.review.riskAccepted.value = true
  expect(s.review.taskName.value).toBe('合成防重任务')
  await s.review.submitTask()
  const calls = vi.mocked(s.api.submitDirectLoad).mock.calls
  expect(calls).toHaveLength(2); expect(calls[0]).toEqual(calls[1]); expect(calls[0]![4]).toMatch(/^[a-f0-9-]{36}$/)
  expect(s.navigateTask).toHaveBeenCalledWith('original-task'); expect(s.review.canSubmit.value).toBe(false)
})

it('响应丢失后从同一预检找回原任务，无第二次POST', async () => {
  const s = await readySubmission()
  vi.mocked(s.api.submitDirectLoad).mockRejectedValue(new Error('NETWORK_UNAVAILABLE'))
  vi.mocked(s.api.getDirectLoadCheck).mockResolvedValue({ ...report(s.draft.currentDraft.value!), submittedTaskId: 'original-task' })
  await s.review.submitTask(); await s.review.submitTask()
  expect(s.api.submitDirectLoad).toHaveBeenCalledTimes(1)
  expect(s.navigateTask).toHaveBeenCalledWith('original-task')
  expect(s.review.submissionUncertain.value).toBe(false)
})

it('原提交未确认时预检过期只允许原请求重放，明确拒绝则恢复检查入口', async () => {
  const s = await readySubmission()
  vi.mocked(s.api.submitDirectLoad).mockRejectedValueOnce(new Error('NETWORK_UNAVAILABLE')).mockRejectedValueOnce({ status: 422, code: 'PRECHECK_REQUIRED' })
  await s.review.submitTask()
  s.review.activePrecheck.value!.validUntil = new Date(0).toISOString()
  expect(s.review.canSubmit.value).toBe(true)
  await s.review.submitTask()
  expect(s.review.submissionUncertain.value).toBe(false); expect(s.review.activePrecheck.value).toBeUndefined()
  expect(s.review.canReview.value).toBe(true); expect(s.review.canSubmit.value).toBe(false)
})

it('A→B→A和卸载隔离迟到的提交核对，不跳到旧任务', async () => {
  for (const dispose of [false, true]) {
    const s = await readySubmission(), pending = deferred<DirectLoadCheck>()
    vi.mocked(s.api.submitDirectLoad).mockRejectedValue(new Error('NETWORK_UNAVAILABLE'))
    vi.mocked(s.api.getDirectLoadCheck).mockReturnValue(pending.promise)
    const work = s.review.submitTask(); await Promise.resolve(); await Promise.resolve()
    if (dispose) s.scope.stop(); else { s.form.table = 'changed'; s.form.table = 'synthetic_table' }
    pending.resolve({ ...report(s.draft.currentDraft.value ?? record(s.form)), submittedTaskId: 'late-task' }); await work
    expect(s.navigateTask).not.toHaveBeenCalled(); expect(s.review.submittedTaskId.value).toBe('')
  }
})

it('任务已接受但跳转失败不重新提交', async () => {
  const s = await readySubmission()
  s.navigateTask.mockRejectedValue(new Error('NAVIGATION_FAILED'))
  await s.review.submitTask(); await s.review.submitTask()
  expect(s.api.submitDirectLoad).toHaveBeenCalledTimes(1)
  expect(s.review.submittedTaskId.value).toBe('task'); expect(s.review.reviewFailure.value).toContain('任务已提交')
})
