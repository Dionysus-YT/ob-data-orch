// 十九项补齐的真实执行仅通过普通 API；固定夹具不提供任意 SQL 或凭据输出。
import { readFile, writeFile } from 'node:fs/promises'
import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
import path from 'node:path'
import { run } from './browser.mjs'
const execute = promisify(execFile)
const python='C:/Users/Dionysus/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/python.exe'
const require=(v,c)=>{if(!v)throw new Error(c)}

export async function runCapabilities({client,root,save,accepted,readJSON}) {
  const work=path.join(root,'artifacts/normal-import-capabilities-20261020'),input=await readJSON(path.join(work,'run.json'))
  require(/^[a-f0-9]{32}$/.test(input.runId)&&input.inputRoot===path.join(root,'ob-data',`nivt-normal-20261019-${input.runId.slice(0,12)}`),'CAPABILITY_OWNER_INVALID')
  for(const name of ['mysql-api.json','oracle-api.json'])try {await readFile(path.join(work,name));throw new Error('CAPABILITY_RECONCILIATION_REQUIRED')}catch(e){if(e.code!=='ENOENT')throw e}
  const sources=[]
  for(const [id,mode] of [[input.sourceId,'MYSQL'],[input.oracleSourceId,'ORACLE']]) {
    const source=await accepted(`/api/v1/data-sources/${id}`)
    require(source.environment==='TEST'&&source.connectionKind==='ODP'&&source.compatibilityMode===mode&&source.state==='ENABLED','CAPABILITY_TEST_SOURCE_REQUIRED')
    sources.push(source)
  }
  let node=await accepted(`/api/v1/execution-nodes/${input.nodeId}`)
  require(node.taskSummary?.running===0&&node.taskSummary?.waiting===0&&node.taskSummary?.reconciling===0,'CAPABILITY_NODE_NOT_IDLE')
  const requested=await accepted(`/api/v1/execution-nodes/${input.nodeId}:environment-check`,'POST',undefined,node.revision)
  for(let i=0;i<80;i++) {
    node=await accepted(`/api/v1/execution-nodes/${input.nodeId}`)
    require(node.environmentCheck?.checkId===requested.environmentCheck?.checkId,'CAPABILITY_NODE_CHECK_CHANGED')
    if(node.environmentCheck?.status==='PASSED')break
    require(node.environmentCheck?.status!=='FAILED','CAPABILITY_ENVIRONMENT_FAILED')
    await new Promise(r=>setTimeout(r,1500))
  }
  require(node.environmentCheck?.status==='PASSED','CAPABILITY_ENVIRONMENT_TIMEOUT')
  await save(path.join(work,'node-before.json'),{status:node.environmentCheck.status,revision:node.revision,factsRevision:node.environmentCheck.factsRevision})
  const fixture=async action=>{
    let result
    try{result=await execute(path.join(work,'fixture.exe'),['-action',action,'-database',input.database,'-run-id',input.runId,'-python',python],{cwd:root,windowsHide:true,maxBuffer:2*1024*1024})}catch{throw new Error('CAPABILITY_FIXED_FIXTURE_UNAVAILABLE')}
    const value=JSON.parse(result.stdout);await save(path.join(work,action+'.json'),value);return value
  }
  require((await fixture('import-create')).status==='CREATED','CAPABILITY_CREATE_FAILED')
  const mysqlFixture=await fixture('import-capabilities-create')
  require(mysqlFixture.ownerVerified,'CAPABILITY_MYSQL_OWNER_FAILED')
  require((await fixture('import-oracle-capabilities-create')).ownerVerified,'CAPABILITY_ORACLE_OWNER_FAILED')
  const config=(key,kind,name,ddl=false)=>({configVersion:'normal-import-v1',config:{dataSourceId:input.sourceId,nodeId:input.nodeId,database:input.database,filePath:path.join(input.inputRoot,'parameters',key),logPath:path.join(input.inputRoot,'logs-parameter-'+key),content:ddl?'DDL_ONLY':'DATA_ONLY',...(ddl?{}:{format:'CSV'}),scope:'SPECIFIC',objects:{[kind]:name},parser:ddl?{}:{columnSeparator:',',columnQuote:'"'},mapping:{kind:'POSITION'},execution:{memory:'1G',thread:'1',strict:'true',maxErrors:'0',maxDiscards:'0'},cleanup:'NONE'}})
  const mysqlCases=mysqlFixture.dataKeys.map(key=>({id:'NI-PARAM-'+key,title:'文件参数独立结果核对',names:['--'+key.replaceAll('_','-')],input:config(key,'table','nivc_'+key)}))
  for(const test of mysqlCases) {
    const c=test.input.config,key=test.id.slice(9)
    if(key==='ctl_path')c.mapping={kind:'CONTROL',ctlPath:path.join(input.inputRoot,'control')}
    if(key==='exclude_data_types')c.mapping.excludeDataTypes='BLOB'
    if(key==='skip_footer'){c.format='CUT';c.parser={columnSplitter:'|',lineSeparator:'\\n',skipFooter:true}}
    if(['compress','compression_algo'].includes(key))c.parser={...c.parser,compress:true,compressionAlgo:'gzip'}
    if(key==='compression_level')c.parser={...c.parser,compress:true,compressionAlgo:'zlib',compressionLevel:'1'}
  }
  for(const o of mysqlFixture.objects)mysqlCases.push({id:'NI-PARAM-'+o.key,title:'MySQL 对象独立核对',names:['--'+o.kind],input:config(o.key,o.kind,o.name,true)})
  const prefix='NIVC_'+input.runId.slice(0,12).toUpperCase()+'_'
  const oracleDefs=[
    ['role','ROLE','CREATE ROLE '+prefix+'ROLE;'],
    ['sequence','SEQ','CREATE SEQUENCE '+prefix+'SEQ START WITH 1 INCREMENT BY 1 NOCACHE;'],
    ['synonym','SYN','CREATE SYNONYM '+prefix+'SYN FOR '+prefix+'BASE;'],
    ['type','TYP','CREATE TYPE '+prefix+'TYP AS OBJECT (ID NUMBER);\n/'],
    ['type_body','TB','CREATE TYPE BODY '+prefix+'TB AS MEMBER FUNCTION VALUE1 RETURN NUMBER AS BEGIN RETURN 1; END; END;\n/'],
    ['package','PKG','CREATE PACKAGE '+prefix+'PKG AS PROCEDURE P; END;\n/'],
    ['package_body','PB','CREATE PACKAGE BODY '+prefix+'PB AS PROCEDURE P AS BEGIN NULL; END; END;\n/'],
  ]
  const {mkdir}=await import('node:fs/promises'),oracleCases=[]
  for(const [key,suffix,sql] of oracleDefs) {
    const kind=key.replaceAll('_','-'),name=prefix+suffix,directory=path.join(input.inputRoot,'parameters',key)
    const definition=['type_body','package','package_body'].includes(key)?'DELIMITER /\n'+sql.trim().replace(/\s*\/\s*$/,'')+'\n/\nDELIMITER ;':sql
    await mkdir(directory);await writeFile(path.join(directory,name+'-schema.sql'),definition+'\n',{flag:'wx'})
    oracleCases.push({id:'NI-PARAM-'+key,title:'Oracle 对象独立核对',names:['--'+kind],input:config(key,kind,name,true)})
  }
  const schema=sources[1].defaultDatabase||sources[1].username?.toUpperCase()
  require(schema&&/^[A-Za-z_][A-Za-z0-9_$#]{0,127}$/.test(schema),'CAPABILITY_ORACLE_SCHEMA_INVALID')
  for(const [mode,cases,sourceId,database] of [['mysql',mysqlCases,input.sourceId,input.database],['oracle',oracleCases,input.oracleSourceId,schema]]) {
    await save(path.join(work,mode+'-cases.json'),cases)
    await save(path.join(work,mode+'-before.json'),await fixture(mode==='mysql'?'import-capabilities-inspect':'import-oracle-capabilities-inspect'))
    let reported=0
    const state=await run({...input,sourceId,database,parameterCases:cases},{request:client.request.bind(client),publish:async state=>{
      await save(path.join(work,mode+'-api.json'),state)
      while(reported<state.records.length){const r=state.records[reported++];console.log(`${r.id}: ${r.outcome} ${r.code??''}`)}
    }})
    require(state.done&&!state.stopped,'CAPABILITY_BATCH_INCOMPLETE')
    await save(path.join(work,mode+'-after.json'),await fixture(mode==='mysql'?'import-capabilities-inspect':'import-oracle-capabilities-inspect'))
  }
  console.log('十九项 API 尝试已完成，结论等待数据库事实与持久事件核对。')
}

export async function runCapabilityNegatives({client,root,save,readJSON}) {
  const work=path.join(root,'artifacts/normal-import-capabilities-20261020'),input=await readJSON(path.join(work,'run.json'))
  for(const mode of ['mysql','oracle'])require((await readJSON(path.join(work,mode+'-api.json'))).done,'CAPABILITY_POSITIVE_BATCH_PENDING')
  let previous
  try {previous=await readJSON(path.join(work,'negative-api.json'))}catch(e){if(e.code!=='ENOENT')throw e}
  if(previous) {
    const failed=previous.records.filter(r=>r.outcome==='FAIL')
    require(previous.done&&failed.length===2&&failed.every(r=>['NI-PARAM-negative_ddl_tail','NI-PARAM-negative_user_host'].includes(r.id)&&r.code==='EXPECTED_FAILURE_MISSING'&&!r.taskId),'CAPABILITY_NEGATIVE_REPLAY_REJECTED')
    try{await readFile(path.join(work,'negative-retry-api.json'));throw new Error('CAPABILITY_NEGATIVE_RETRY_EXISTS')}catch(e){if(e.code!=='ENOENT')throw e}
    const cases=[]
    for(const row of failed) {
      const response=await client.request(`/api/v1/normal-import-drafts/${row.draftId}`)
      require(response.status===200&&response.body.config.dataSourceId===input.sourceId&&response.body.config.filePath.startsWith(path.join(input.inputRoot,'parameters','negative_')),'CAPABILITY_NEGATIVE_RETRY_SCOPE_CHANGED')
      cases.push({id:row.id,title:'定义权限未知阻断的检查分类复验',failureCheck:'WRITE_ACCESS',input:{configVersion:'normal-import-v1',config:response.body.config}})
    }
    const state=await run({...input,parameterCases:cases},{request:client.request.bind(client),publish:state=>save(path.join(work,'negative-retry-api.json'),state)})
    require(state.done&&state.records.length===2&&state.records.every(r=>r.outcome==='PASS'&&!r.taskId),'CAPABILITY_NEGATIVE_RETRY_FAILED')
    console.log('两项定义反例按 WRITE_ACCESS/UNKNOWN 验证拒绝提交通过。');return
  }
  const originals=await readJSON(path.join(work,'mysql-cases.json')),{mkdir}=await import('node:fs/promises')
  const cases=[]
  async function add(key,base,filename,data,edit,check='INPUT_MAPPING') {
    const test=structuredClone(originals.find(c=>c.id==='NI-PARAM-'+base)),dir=path.join(input.inputRoot,'parameters','negative_'+key)
    await mkdir(dir);await writeFile(path.join(dir,filename),data,{flag:'wx'})
    test.id='NI-PARAM-negative_'+key;test.title='补齐能力失败关闭反例';delete test.names
    Object.assign(test.input.config,{filePath:dir,logPath:path.join(input.inputRoot,'logs-parameter-negative_'+key)})
    await edit(test.input.config,dir);test.failureCheck=check;cases.push(test)
  }
  await add('control','ctl_path','nivc_ctl_path.csv','"1","alpha","plain"\n',async(c,dir)=>{
    await writeFile(path.join(dir,'nivc_ctl_path.ctrl'),'lang=java (id "CONSTANT(1)",label,note);\n',{flag:'wx'});c.mapping.ctlPath=dir
  })
  const compressed=await readFile(path.join(input.inputRoot,'parameters','compress','nivc_compress.csv.gz'))
  const broken=Buffer.from(compressed);broken[broken.length-1]^=1
  await add('crc','compress','nivc_compress.csv.gz',broken,async()=>{})
  await add('codec','compress','nivc_compress.csv.gz',compressed,async c=>{c.parser.compressionAlgo='zstd'},'INPUT_FILES')
  await add('footer','skip_footer','nivc_skip_footer.dat','footer\n',async()=>{})
  await add('required','exclude_data_types','nivc_exclude_data_types.csv','"alpha","plain"\n',async c=>{c.mapping.excludeDataTypes='INT'})
  await add('ddl_tail','view','nivc_view-schema.sql','CREATE VIEW nivc_view AS SELECT 1 AS id; DROP TABLE nivc_base;\n',async()=>{},'WRITE_ACCESS')
  const user=originals.find(c=>c.id==='NI-PARAM-obj_user').input.config.objects['obj-user']
  await add('user_host','obj_user',user+'-schema.sql',"CREATE USER "+user+"@'%';\n",async()=>{},'WRITE_ACCESS')
  await add('dependency','trigger','nivc_trigger-schema.sql','CREATE TRIGGER nivc_trigger BEFORE INSERT ON nivc_missing_base FOR EACH ROW SET NEW.id=NEW.id;\n',async()=>{},'WRITE_ACCESS')
  let reported=0
  const state=await run({...input,parameterCases:cases},{request:client.request.bind(client),publish:async state=>{
    await save(path.join(work,'negative-api.json'),state)
    while(reported<state.records.length){const r=state.records[reported++];console.log(`${r.id}: ${r.outcome} ${r.code??''}`)}
  }})
  require(state.done&&!state.stopped&&state.records.length===8&&state.records.every(r=>r.outcome==='PASS'&&!r.taskId),'CAPABILITY_NEGATIVE_BATCH_FAILED')
}

export async function runCapabilityRetry({client,root,save,accepted,readJSON}) {
  const work=path.join(root,'artifacts/normal-import-capabilities-20261020'),input=await readJSON(path.join(work,'run.json'))
  const initial=await readJSON(path.join(work,'mysql-api.json')),oracle=await readJSON(path.join(work,'oracle-api.json'))
  require(initial.done&&oracle.done&&!initial.stopped&&!oracle.stopped,'CAPABILITY_INITIAL_BATCH_PENDING')
  try{await readFile(path.join(work,'retry-mysql-api.json'));throw new Error('CAPABILITY_RETRY_RECONCILIATION_REQUIRED')}catch(e){if(e.code!=='ENOENT')throw e}
  for(const row of [...initial.records,...oracle.records].filter(r=>r.taskId)) {
    const task=await accepted(`/api/v1/tasks/${row.taskId}/execution`)
    require(['SUCCEEDED','FAILED','CANCELLED'].includes(task.state)&&!task.reconciliationRequired,'CAPABILITY_RETRY_TASK_NOT_TERMINAL')
  }
  const fixture=async action=>{
    let result;try{result=await execute(path.join(work,'fixture.exe'),['-action',action,'-database',input.database,'-run-id',input.runId,'-python',python],{cwd:root,windowsHide:true,maxBuffer:2*1024*1024})}catch{throw new Error('CAPABILITY_FIXED_RETRY_FIXTURE_UNAVAILABLE')}
    const value=JSON.parse(result.stdout);await save(path.join(work,'retry-'+action+'.json'),value);return value
  }
  require((await fixture('import-capabilities-repair')).ownerVerified,'CAPABILITY_REPAIR_OWNER_FAILED')
  const cases=(await readJSON(path.join(work,'mysql-cases.json'))).filter(c=>['NI-PARAM-ctl_path','NI-PARAM-compression_level','NI-PARAM-procedure'].includes(c.id))
  for(const test of cases){const key=test.id.slice(9);test.input.config.filePath=path.join(input.inputRoot,'parameters','retry_'+key);test.input.config.logPath=path.join(input.inputRoot,'logs-parameter-retry_'+key);if(key==='ctl_path')test.input.config.mapping.ctlPath=path.join(input.inputRoot,'retry-control')}
  const {mkdir}=await import('node:fs/promises'),prefix='NIVC_'+input.runId.slice(0,12).toUpperCase()+'_',directory=path.join(input.inputRoot,'parameters','retry_trigger')
  await mkdir(directory);await writeFile(path.join(directory,prefix+'TRG-schema.sql'),'DELIMITER /\nCREATE TRIGGER '+prefix+'TRG BEFORE INSERT ON '+prefix+'BASE FOR EACH ROW BEGIN :NEW.ID := :NEW.ID; END;\n/\nDELIMITER ;\n',{flag:'wx'})
  const trigger=structuredClone((await readJSON(path.join(work,'oracle-cases.json')))[0]);trigger.id='NI-PARAM-trigger';trigger.names=['--trigger'];trigger.title='Oracle 触发器权限与对象补验'
  Object.assign(trigger.input.config,{objects:{trigger:prefix+'TRG'},filePath:directory,logPath:path.join(input.inputRoot,'logs-parameter-retry_trigger')})
  const oracleRetries=(await readJSON(path.join(work,'oracle-cases.json'))).filter(c=>['NI-PARAM-type_body','NI-PARAM-package','NI-PARAM-package_body'].includes(c.id))
  for(const test of oracleRetries) {
    const c=test.input.config,key=test.id.slice(9),name=Object.values(c.objects)[0],filename=name+'-schema.sql'
    const sql=(await readFile(path.join(c.filePath,filename),'utf8')).trim().replace(/\s*\/\s*$/,'')
    c.filePath=path.join(input.inputRoot,'parameters','retry_'+key);c.logPath=path.join(input.inputRoot,'logs-parameter-retry_'+key)
    await mkdir(c.filePath);await writeFile(path.join(c.filePath,filename),'DELIMITER /\n'+sql+'\n/\nDELIMITER ;\n',{flag:'wx'})
  }
  oracleRetries.push(trigger)
  const source=await accepted(`/api/v1/data-sources/${input.oracleSourceId}`),schema=source.defaultDatabase||source.username.toUpperCase()
  for(const [mode,tests,sourceId,database] of [['mysql',cases,input.sourceId,input.database],['oracle',oracleRetries,input.oracleSourceId,schema]]) {
    await save(path.join(work,'retry-'+mode+'-cases.json'),tests)
    const action=mode==='mysql'?'import-capabilities-inspect':'import-oracle-capabilities-inspect'
    await save(path.join(work,'retry-'+mode+'-before.json'),await fixture(action))
    let reported=0
    const state=await run({...input,sourceId,database,parameterCases:tests},{request:client.request.bind(client),publish:async state=>{await save(path.join(work,'retry-'+mode+'-api.json'),state);while(reported<state.records.length){const r=state.records[reported++];console.log(`${r.id}: ${r.outcome} ${r.code??''}`)}}})
    require(state.done&&!state.stopped,'CAPABILITY_RETRY_INCOMPLETE')
    await save(path.join(work,'retry-'+mode+'-after.json'),await fixture(action))
  }
}
