import { describe, expect, it, vi } from 'vitest'
import { createBrowserApi, type FetchLike } from './browser'
import { createDirectLoadForm } from '@/workbench/import/direct-load/directLoadForm'
import type { DirectLoadDraft } from './directLoad'
import metadataText from '../../../internal/parammeta/direct/obloader-4.3.5-direct-v1.json?raw'

const config = JSON.parse(JSON.stringify(createDirectLoadForm()))
const draft = { id: 'direct-draft', revision: 1, configVersion: 'direct-load-v1', config, configFingerprint: 'a'.repeat(64), metadataVersion: 'obloader-4.3.5-direct-v1', capabilityVersion: 'direct-load-local-v1', executionEnabled: false, rpcPortSource: 'MANUAL' }
function setup(response: unknown) { const fetcher = vi.fn<FetchLike>(async () => Response.json(response)); return { fetcher, api: createBrowserApi({ fetcher, csrfToken: () => 'synthetic-csrf', idempotencyKey: () => 'synthetic-key' }) } }
const checks = ['TOOL_ENVIRONMENT', 'NODE_RESOURCE', 'INPUT_FILES', 'DATABASE_CONNECTIVITY', 'RPC_CONNECTIVITY', 'DIRECT_LOAD_VERSION', 'WRITE_ACCESS', 'DIRECT_LOAD_STRUCTURE', 'DIRECT_LOAD_CONFIG']
function report() { return { id: 'check', draftId: draft.id, draftRevision: 1, configFingerprint: draft.configFingerprint, checkKind: 'DIRECT_LOAD_PREFLIGHT', status: 'FAILED', integrityStatus: 'COMPLETE', executionEnabled: false, validUntil: new Date(Date.now() + 60000).toISOString(), results: checks.map(check => ({ check, status: 'UNKNOWN', evidenceCode: 'DIRECT_LOAD_PROBE_NOT_CONFIGURED' })) } }
describe('旁路 API 独立传输边界', () => {
  it('真实目录保留canonical --前缀契约，草稿省略空值归一化而不放宽已提供类型', async () => {
    const metadata = JSON.parse(metadataText)
    const rows = await setup(metadata).api.getDirectLoadParameters(); expect(rows).toHaveLength(103); expect(rows.some(item=>item.longName==='rpc-port')).toBe(true)
    const omitted = {...config}; for(const key of ['externalData','replaceData','truncateTable','columnSeparator','thread','parallel']) delete omitted[key]
    await expect(setup({...draft,config:omitted}).api.getDirectLoadDraft(draft.id)).resolves.toMatchObject({config:{externalData:false,thread:''}})
    await expect(setup({...draft,config:{...omitted,thread:false}}).api.getDirectLoadDraft(draft.id)).rejects.toMatchObject({code:'RESPONSE_INVALID'})
  })
  it('写操作携带安全头和当前修订，配置不接收普通导入字段', async () => {
    const s = setup(draft); await s.api.updateDirectLoadDraft(draft as DirectLoadDraft, config)
    const [url, init] = s.fetcher.mock.calls[0]!, headers = new Headers(init!.headers)
    expect(String(url)).toBe('/api/v1/direct-load-drafts/direct-draft'); expect(init?.method).toBe('PATCH')
    expect(headers.get('if-match')).toBe('"rev-1"'); expect(headers.get('x-csrf-token')).toBeTruthy(); expect(headers.get('idempotency-key')).toBeTruthy()
    expect(JSON.parse(String(init?.body))).toEqual({ configVersion: 'direct-load-v1', config })
    for (const polluted of [{...draft,configVersion:'normal-import-v1'}, {...draft,config:{...config,cleanup:'NONE'}}, {...draft,config:{...config,format:'CUT'}}, {...draft,rpcPortSource:'INFERRED'}]) await expect(setup(polluted).api.getDirectLoadDraft(draft.id)).rejects.toMatchObject({code:'RESPONSE_INVALID'})
  })
  it('派生来源与锁定必须配对，旧RERUN响应仍按来源锁定', async () => {
    const rerun = { ...draft, sourceTaskId:'synthetic-parent',sourceDerivation:'RERUN_FROM_SCRATCH' }
    await expect(setup(rerun).api.getDirectLoadDraft(draft.id)).resolves.toMatchObject({parametersLocked:true})
    await expect(setup({...rerun,parametersLocked:true}).api.getDirectLoadDraft(draft.id)).resolves.toMatchObject({sourceTaskId:'synthetic-parent',parametersLocked:true})
    for(const invalid of [{...draft,parametersLocked:true},{...rerun,parametersLocked:false},{...rerun,parametersLocked:'true'},{...rerun,sourceTaskId:''}])await expect(setup(invalid).api.getDirectLoadDraft(draft.id)).rejects.toMatchObject({code:'RESPONSE_INVALID'})
  })
  it('严格验证九项顺序、状态与固定原因，未知事实不能变成通过', async () => {
    await expect(setup(report()).api.getDirectLoadCheck('check')).resolves.toMatchObject({ status: 'FAILED', executionEnabled: false })
    for (const results of [report().results.slice(1), [...report().results].reverse(), report().results.map((item,index) => index ? item : {...item,status:'PASSED'}), report().results.map((item,index) => index ? item : {...item,evidenceCode:'UNTRUSTED_REASON'})]) await expect(setup({...report(),results}).api.getDirectLoadCheck('check')).rejects.toMatchObject({code:'RESPONSE_INVALID'})
  })
  it('节点配置通过须携带独立安全指纹，缺失或无关指纹失败关闭', async () => {
    const success={...report(),status:'SUCCEEDED',executionEnabled:true,directLoadConfigFingerprint:'d'.repeat(64),results:checks.map(check=>({check,status:'PASSED',evidenceCode:'DIRECT_LOAD_VERIFIED'})),inputManifest:{fileCount:1,totalBytes:1,digest:'b'.repeat(64),files:[{path:'synthetic.csv',kind:'DATA',object:'synthetic_table',size:1,modified:'1',sha256:'c'.repeat(64)}]}}
    await expect(setup(success).api.getDirectLoadCheck('check')).resolves.toMatchObject({directLoadConfigFingerprint:'d'.repeat(64)})
    for(const invalid of [{...success,directLoadConfigFingerprint:undefined},{...success,directLoadConfigFingerprint:'invalid'},{...report(),directLoadConfigFingerprint:'d'.repeat(64)},{...success,results:success.results.map((result,index)=>index?result:{...result,evidenceCode:'TOOL_RUNTIME_READY'})}])await expect(setup(invalid).api.getDirectLoadCheck('check')).rejects.toMatchObject({code:'RESPONSE_INVALID'})
  })
  it('旁路命令必须有direct且秘密已脱敏，不能接收普通命令或未脱敏片段', async () => {
    for (const command of ['obloader --csv -p ******', 'obloader --direct --password synthetic-secret']) await expect(setup({ redactedCommand: command, configFingerprint: draft.configFingerprint, executionEnabled: false }).api.previewDirectLoadCommand(draft as DirectLoadDraft)).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
    await expect(setup({ redactedCommand: 'obloader --direct --csv -p ******', configFingerprint: draft.configFingerprint, executionEnabled: false }).api.previewDirectLoadCommand(draft as DirectLoadDraft)).resolves.toMatchObject({ executionEnabled: false })
  })
  it('提交名称在最终事务中发送，不进入配置；旁路模板继续拒绝', async () => {
    const s = setup({ id: 'synthetic-task' }); await s.api.submitDirectLoad(draft as DirectLoadDraft, 'check', draft.configFingerprint, ' 合成旁路任务 ')
    expect(JSON.parse(String(s.fetcher.mock.calls[0]![1]?.body))).toEqual({ precheckId: 'check', confirmedConfigFingerprint: draft.configFingerprint, taskName: '合成旁路任务' })
    await expect(setup({ items: [{ taskType:'OBLOADER_DIRECT_LOAD' }] }).api.listConfigTemplates()).rejects.toMatchObject({code:'RESPONSE_INVALID'})
  })
  it('同一提交意图显式复用原幂等键，非法键不发请求', async () => {
    const fetcher = vi.fn<FetchLike>(async () => Response.json({ id: 'original-task' }))
    const api = createBrowserApi({ fetcher, csrfToken: () => 'synthetic-csrf', idempotencyKey: () => 'new-key-must-not-be-used' })
    for (let i = 0; i < 2; i++) await api.submitDirectLoad(draft as DirectLoadDraft, 'check', draft.configFingerprint, '合成任务', 'original-submit-key-0001')
    expect(fetcher.mock.calls.map(call => new Headers(call[1]?.headers).get('idempotency-key'))).toEqual(['original-submit-key-0001', 'original-submit-key-0001'])
    await expect(api.submitDirectLoad(draft as DirectLoadDraft, 'check', draft.configFingerprint, '合成任务', 'invalid\nkey')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
    expect(fetcher).toHaveBeenCalledTimes(2)
  })
  it('结构详细原因只能用于正确检查项和状态，原任务核对限定完整预检', async () => {
    const code = 'DIRECT_LOAD_REPLACE_INDEX_BLOCKED'
    const value = { ...report(), results: report().results.map(item => item.check === 'DIRECT_LOAD_STRUCTURE' ? { ...item, status: 'FAILED', evidenceCode: code } : item), submittedTaskId: 'original-task' }
    await expect(setup(value).api.getDirectLoadCheck('check')).resolves.toMatchObject({ submittedTaskId: 'original-task' })
    for (const invalid of [{ ...value, results: value.results.map(item => item.check === 'DIRECT_LOAD_STRUCTURE' ? { ...item, status: 'PASSED' } : item) }, { ...value, results: value.results.map(item => item.check === 'WRITE_ACCESS' ? { ...item, status: 'FAILED', evidenceCode: code } : item) }, { ...value, submittedTaskId: 'bad/id' }, { ...value, checkKind: 'DIRECT_LOAD_FILES', results: [value.results[2]] }]) await expect(setup(invalid).api.getDirectLoadCheck('check')).rejects.toMatchObject({ code: 'RESPONSE_INVALID' })
  })
  it('统一任务保留旁路六组冻结证据，不借用普通导入格式或按页面步数删组', async () => {
    const item={type:'OBLOADER_DIRECT_LOAD',snapshotVersion:'direct-load-v1',dataSourceId:'source',nodeId:'node',precheckId:'check',format:'CSV',configFingerprint:draft.configFingerprint,toolVersion:'4.3.5-RELEASE',metadataVersion:draft.metadataVersion,capabilityVersion:draft.capabilityVersion,groups:['applicability','connection','input','target','execution','review'].map(key=>({key,label:key,fields:[]}))}
    await expect(setup({item}).api.getTaskSnapshot('synthetic-task')).resolves.toMatchObject({type:'OBLOADER_DIRECT_LOAD',groups:expect.arrayContaining([{key:'target',label:'target',fields:[]}])})
    for(const invalid of [{...item,format:'CUT'},{...item,snapshotVersion:'normal-import-v1'},{...item,groups:item.groups.map(group=>({...group,key:group.key==='target'?'objects':group.key}))}])await expect(setup({item:invalid}).api.getTaskSnapshot('synthetic-task')).rejects.toMatchObject({code:'RESPONSE_INVALID'})
  })
})
