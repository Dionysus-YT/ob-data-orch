import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { runGates } from './api-gates.mjs'

test('不确定检查只能验证拒绝，账号角色与NLS成功检查不启动工具且不冒充功能通过', async () => {
  const directory=await mkdtemp(path.join(tmpdir(),'normal-gates-'))
  const drafts=new Map()
  const checks=new Map()
  const submitted=[]
  let sequence=0
  const client={request:async(route,method='GET',body)=>{
    if(route==='/api/v1/data-sources/synthetic-mysql')return {status:200,body:{item:{id:'synthetic-mysql',environment:'TEST',connectionKind:'ODP',compatibilityMode:'MYSQL',state:'ENABLED'}}}
    if(route.startsWith('/api/v1/data-sources/'))return {status:200,body:{item:{id:'synthetic-oracle',environment:'TEST',connectionKind:'ODP',compatibilityMode:'ORACLE',state:'ENABLED',defaultDatabase:'SYNTHETIC_SCHEMA'}}}
    if(route==='/api/v1/normal-import-drafts'){
      const id='synthetic-'+(++sequence)
      const draft={id,revision:1,configFingerprint:'fp-'+id,config:body.config}
      drafts.set(id,draft)
      return {status:201,body:draft}
    }
    if(route.startsWith('/api/v1/normal-import-prechecks/')){
      const draft=checks.get(route.split('/').at(-1))
      const nls=Object.keys(draft.config.execution).some(key=>key.startsWith('nls'))
      return {status:200,body:{id:'check-'+draft.id,draftRevision:1,configFingerprint:draft.configFingerprint,integrityStatus:'COMPLETE',status:nls?'SUCCEEDED':'FAILED',results:[{check:'INPUT_MAPPING',status:nls?'PASSED':'UNKNOWN',evidenceCode:'SYNTHETIC_ONLY'}]}}
    }
    const id=route.split('/').at(-1).split(':')[0]
    const draft=drafts.get(id)
    assert.ok(draft)
    if(route.endsWith(':preview-command'))return {status:200,body:{configFingerprint:draft.configFingerprint,redactedCommand:'obloader --ctl-path --exclude-data-types --skip-footer --compress --compression-algo --compression-level --file-encoding --table-group --view --trigger --obj-user --role --sequence --synonym --type --type-body --package --package-body --function --procedure --nls-date-format --nls-timestamp-format --nls-timestamp-tz-format'}}
    if(route.endsWith(':precheck')){checks.set('check-'+id,draft);return {status:201,body:{id:'check-'+id}}}
    if(route.endsWith(':submit')){submitted.push(draft);return {status:422,body:{code:'SYNTHETIC_REJECTED'}}}
    if(method==='DELETE'){drafts.delete(id);return {status:204,body:{}}}
    throw new Error('UNEXPECTED_SYNTHETIC_REQUEST')
  }}
  try {
    const state=await runGates({inputRoot:directory,runId:'a'.repeat(32),sourceId:'synthetic-mysql',nodeId:'synthetic-node',database:'synthetic_db'},client,async()=>{},directory)
    assert.equal(state.done,true)
    assert.equal(state.records.length,23)
    assert.equal(submitted.length,18)
    assert.ok(submitted.every(row=>!row.config.objects['obj-user']&&!row.config.objects.role))
    assert.equal(state.records.filter(row=>row.outcome==='BLOCKED_PREFLIGHT_ONLY').length,2)
    assert.equal(state.records.filter(row=>row.outcome==='PREFLIGHT_PASSED_TOOL_NOT_VALIDATED').length,3)
    assert.ok(state.records.every(row=>row.toolExecutionValidated===false&&row.outcome!=='PASS'))
    assert.equal(drafts.size,0)
  } finally { await rm(directory,{recursive:true}) }
})
