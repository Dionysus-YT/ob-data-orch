// 使用正常浏览器身份验证 Oracle 直连；未知预检查只能验证拒绝，不能伪造证明或绕过提交。
import { readFile, writeFile, rename, mkdir, rm, lstat, realpath } from 'node:fs/promises'
import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { ApiClient } from '../normal-import-e2e/api-client.mjs'

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..'),relative=process.argv[2]
if(process.argv.length!==3||!/^artifacts[\\/]direct-load-e2e-\d{8}-[a-f0-9]{12}$/.test(relative??''))throw new Error('ORACLE_RUN_SCOPE_REQUIRED')
const work=path.join(root,relative),owned=JSON.parse(await readFile(path.join(work,'run.json'),'utf8'))
if(!/^[a-f0-9]{32}$/.test(owned.runId)||!new RegExp(`^odo_e2e_[0-9]{8}_${owned.runId.slice(0,12)}$`).test(owned.database)||!work.endsWith(owned.runId.slice(0,12)))throw new Error('ORACLE_RUN_OWNER_INVALID')
await writeFile(path.join(work,'oracle-started.json'),JSON.stringify({at:new Date().toISOString()}),{flag:'wx'})
const originalId='55208ffb-2171-4e69-bb97-5af53c2f0d81',nodeId='7ebf034b-7874-48e8-8222-9c030baa7446',base='/api/v1/direct-load-drafts'
const execute=promisify(execFile),helper=path.join(root,'artifacts/direct-load-oracle-tools/fixture-oracle.exe')
const helperArgs=action=>['-action',action,'-database',owned.database,'-run-id',owned.runId,'-python','C:/Users/Dionysus/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/python.exe']
const inputRoot=path.join(root,'ob-data',`dlvt-oracle-${owned.runId.slice(0,12)}`),table=`DLVO_${owned.runId.slice(0,12).toUpperCase()}_BASIC`
const client=new ApiClient(await readFile(path.join(root,'var/dev/control-plane/data/control-plane-ca.pem')))
const state={scope:'LIVE_ORACLE_DIRECT_API_VALIDATION',runId:owned.runId,records:[],drafts:[],checks:[],tasks:[]}
let activeCheck=false,fixtureCreated=false,inputCreated=false,originalBefore
const requireFact=(value,code)=>{if(!value)throw new Error(code)}
const safeCode=error=>/^[A-Z_0-9]+$/.test(error?.message??'')?error.message:'ORACLE_VALIDATION_UNAVAILABLE'
async function save(name,value){const target=path.join(work,name),temporary=`${target}.${process.pid}.tmp`;await writeFile(temporary,JSON.stringify(value,null,2),{flag:'wx'});await rename(temporary,target)}
const checkpoint=()=>save('oracle-api.json',state)
async function accepted(route,method='GET',body,revision){const response=await client.request(route,method,body,revision);requireFact(response.status>=200&&response.status<300,response.body.code??'API_REJECTED');return response.body.item??response.body}
async function poll(route,terminal){const until=Date.now()+180000;while(Date.now()<until){const result=await accepted(route);if(terminal(result))return result;await new Promise(resolve=>setTimeout(resolve,750))}throw new Error('ORACLE_POLL_TIMEOUT')}
async function fixture(action,name){let output;try{output=await execute(helper,helperArgs(action),{cwd:root,windowsHide:true,maxBuffer:1024*1024,timeout:120000})}catch{throw new Error('ORACLE_FIXED_FIXTURE_UNAVAILABLE')}const value=JSON.parse(output.stdout);await save(`${name}.json`,value);return value}
async function record(id,action){const row={id,startedAt:new Date().toISOString()};state.current=row;await checkpoint();try{Object.assign(row,{outcome:'PASS'},await action(row))}catch(error){Object.assign(row,{outcome:'FAIL',code:safeCode(error)})}row.finishedAt=new Date().toISOString();state.records.push(row);delete state.current;await checkpoint();console.log(`${id}: ${row.outcome}`);return row}
const sourceFacts=source=>Object.fromEntries(['id','revision','connectionKind','compatibilityMode','state','credentialRevision','port'].map(key=>[key,source[key]]))
async function draft(config){const value=await accepted(base,'POST',{configVersion:'direct-load-v1',config});state.drafts.push({id:value.id,revision:value.revision});await checkpoint();return value}
async function check(item,files,name){const started=await accepted(`${base}/${item.id}:${files?'discover-files':'precheck'}`,'POST',{},item.revision);state.checks.push(started.id);activeCheck=true;await checkpoint();const value=await poll(`/api/v1/direct-load-prechecks/${started.id}`,row=>!['PENDING','LEASED'].includes(row.status));activeCheck=false;await save(`${name}.json`,value);requireFact(value.results.length===(files?1:9),'ORACLE_CHECK_PROJECTION_INVALID');return value}
try{
  const chunks=[];let size=0
  try{for await(const chunk of process.stdin){chunks.push(chunk);size+=chunk.length;requireFact(size<=1024,'LOGIN_INPUT_INVALID')}const password=Buffer.concat(chunks);chunks.forEach(chunk=>chunk.fill(0));await client.login(password)}finally{chunks.forEach(chunk=>chunk.fill(0))}
  state.authenticated=true;await checkpoint()
  originalBefore=sourceFacts(await accepted(`/api/v1/data-sources/${originalId}`));await save('original-source-before.json',originalBefore)
  const probe=await fixture('direct-oracle-probe','source-probe');requireFact(probe.mode==='ORACLE'&&probe.currentUserMatched,'ORACLE_IDENTITY_NOT_VERIFIED');state.observerVersion=probe.observerVersion
  let bytes
  try{bytes=(await execute(helper,helperArgs('direct-oracle-source-request'),{cwd:root,windowsHide:true,encoding:'buffer',maxBuffer:16384})).stdout;const created=await client.createDataSourceWithJSONBytes(bytes);requireFact(created.status===201,'ORACLE_DIRECT_SOURCE_CREATE_REJECTED');state.sourceId=created.body.id;await checkpoint()}finally{bytes?.fill(0)}
  let source=await accepted(`/api/v1/data-sources/${state.sourceId}`)
  requireFact(source.connectionKind==='OBSERVER_DIRECT'&&source.compatibilityMode==='ORACLE'&&source.state==='DISABLED'&&source.displayName===`direct-oracle-api-${owned.runId.slice(0,12)}`,'ORACLE_SOURCE_SCOPE_INVALID')
  requireFact(/^[A-Z_][A-Z0-9_$#]*$/.test(source.username)&&!source.clusterName&&source.tenantName,'ORACLE_SOURCE_IDENTITY_INVALID')
  const connected=await record('ORACLE_SOURCE_CONNECTION',async row=>{const started=await accepted(`/api/v1/data-sources/${source.id}:test-connection`,'POST',{nodeId},source.revision);row.testId=started.id;const value=await poll(`/api/v1/data-source-connection-tests/${started.id}`,item=>!['PENDING','LEASED'].includes(item.status));await save('source-connection-test.json',value);requireFact(value.status==='SUCCEEDED'&&value.verificationSource==='AGENT_JDBC','ORACLE_CONNECTION_NOT_VERIFIED');return{status:value.status,verificationSource:value.verificationSource}})
  requireFact(connected.outcome==='PASS','ORACLE_CONNECTION_NOT_VERIFIED')
  source=await accepted(`/api/v1/data-sources/${source.id}`);await accepted(`/api/v1/data-sources/${source.id}:enable`,'POST',{},source.revision)
  state.unresolvedFixtureCreate=true;await checkpoint();const created=await fixture('direct-oracle-create','fixture-create');requireFact(created.ownerVerified&&created.table===table,'ORACLE_FIXTURE_OWNER_INVALID');fixtureCreated=true;state.unresolvedFixtureCreate=false;await fixture('direct-oracle-inspect','fixture-before')
  requireFact(await realpath(path.join(root,'ob-data'))===path.join(root,'ob-data'),'ORACLE_INPUT_ROOT_CHANGED')
  await mkdir(inputRoot);inputCreated=true;await writeFile(path.join(inputRoot,'.owner.json'),JSON.stringify({runId:owned.runId}),{flag:'wx'})
  for(const directory of ['basic','shards'])await mkdir(path.join(inputRoot,directory))
  await writeFile(path.join(inputRoot,'basic',`${table}.csv`),'1,alpha,12.3400\n2,中文,-0.0100\n',{flag:'wx'})
  await writeFile(path.join(inputRoot,'shards',`${table}.0.csv`),'1,alpha,12.3400\n',{flag:'wx'});await writeFile(path.join(inputRoot,'shards',`${table}.1.csv`),'2,中文,-0.0100\n',{flag:'wx'})
  const config={dataSourceId:source.id,nodeId,database:source.username,table,scenario:'OBSERVER_DIRECT',rpcPort:2882,filePath:path.join(inputRoot,'basic'),format:'CSV',externalData:true,columnSeparator:'',thread:'1',parallel:'1',replaceData:false,truncateTable:false,applicability:{singleTable:true,wholeTableCommit:true,noCheckpoint:true,structureLimits:true}}
  const basic=await draft(config)
  await record('ORACLE_COMMAND_MAPPING',async()=>{const value=await accepted(`${base}/${basic.id}:preview-command`,'POST',{},basic.revision);requireFact(value.redactedCommand.includes('--tenant')&&!value.redactedCommand.includes('--cluster')&&value.redactedCommand.includes(source.username+'@'+source.tenantName)&&value.configFingerprint===basic.configFingerprint,'ORACLE_COMMAND_IDENTITY_INVALID');await save('command-preview.json',value);return{uppercaseUser:true,tenantRetained:true,clusterAbsent:true}})
  for(const [id,item,count] of [['ORACLE_FILES_SINGLE',basic,1],['ORACLE_FILES_SHARDS',await draft({...config,filePath:path.join(inputRoot,'shards')}),2]])await record(id,async()=>{const value=await check(item,true,id);requireFact(value.status==='SUCCEEDED'&&value.results[0].status==='PASSED'&&value.inputManifest?.fileCount===count,'ORACLE_FILES_NOT_VERIFIED');return{status:value.status,inputFiles:count}})
  const full=await check(basic,false,'full-precheck');state.fullPrecheck={id:full.id,status:full.status,results:full.results};await checkpoint()
  await record('ORACLE_COMPLETE_PREFLIGHT',async()=>{requireFact(full.integrityStatus==='COMPLETE','ORACLE_PREFLIGHT_INCOMPLETE');const unresolved=full.results.filter(row=>row.status!=='PASSED');state.functionalE2EPassed=false;return{outcome:unresolved.length?'BLOCKED':'PASS',status:full.status,unresolved}})
  requireFact(full.status!=='SUCCEEDED','ORACLE_UNEXPECTED_EXECUTION_ELIGIBILITY')
  await record('ORACLE_UNPROVEN_SUBMISSION_REJECTED',async()=>{state.unresolvedSubmit=true;await checkpoint();const value=await client.request(`${base}/${basic.id}:submit`,'POST',{precheckId:full.id,taskName:`oracle-direct-${owned.runId.slice(0,12)}`,confirmedConfigFingerprint:basic.configFingerprint},basic.revision);if(value.status>=200&&value.status<300){state.tasks.push(value.body.id);throw new Error('ORACLE_UNPROVEN_SUBMISSION_ACCEPTED')}state.unresolvedSubmit=false;requireFact(value.status===422&&value.body.code==='PRECHECK_REQUIRED','ORACLE_SUBMISSION_GATE_INVALID');return{status:value.status,code:value.body.code}})
}catch(error){state.error=safeCode(error);process.exitCode=1}
finally{
  if(state.authenticated&&!activeCheck&&!state.unresolvedSubmit&&!state.unresolvedFixtureCreate&&state.tasks.length===0){
    if(fixtureCreated)try{const after=await fixture('direct-oracle-inspect','fixture-after'),before=JSON.parse(await readFile(path.join(work,'fixture-before.json'),'utf8'));state.targetUnchanged=JSON.stringify(before)===JSON.stringify(after);requireFact(state.targetUnchanged,'ORACLE_TARGET_CHANGED')}catch(error){state.targetReadError=safeCode(error);process.exitCode=1}
    for(const item of state.drafts)try{item.cleanupStatus=(await client.request(`${base}/${item.id}`,'DELETE',{},item.revision)).status;requireFact([200,204].includes(item.cleanupStatus),'ORACLE_DRAFT_CLEANUP_REJECTED')}catch{item.cleanupStatus='UNAVAILABLE';state.draftCleanupError='ORACLE_DRAFT_CLEANUP_UNAVAILABLE';process.exitCode=1}
    if(state.sourceId)try{let source=await accepted(`/api/v1/data-sources/${state.sourceId}`);requireFact(source.displayName===`direct-oracle-api-${owned.runId.slice(0,12)}`,'ORACLE_SOURCE_OWNER_INVALID');if(source.state==='ENABLED'){await accepted(`/api/v1/data-sources/${source.id}:disable`,'POST',{},source.revision);source=await accepted(`/api/v1/data-sources/${source.id}`)}state.sourceCleanup=(await accepted(`/api/v1/data-sources/${source.id}`,'DELETE',{},source.revision)).outcome}catch(error){state.sourceCleanupError=safeCode(error);process.exitCode=1}
    if(fixtureCreated)try{const result=await fixture('direct-oracle-cleanup','fixture-cleanup');requireFact(result.ownerVerified&&result.tablesAbsent,'ORACLE_CLEANUP_UNCONFIRMED');state.fixtureCleaned=true}catch{state.fixtureCleanupError='ORACLE_FIXTURE_CLEANUP_UNAVAILABLE';process.exitCode=1}
    if(inputCreated&&state.fixtureCleaned)try{const marker=JSON.parse(await readFile(path.join(inputRoot,'.owner.json'),'utf8'));requireFact(marker.runId===owned.runId&&path.dirname(inputRoot)===path.join(root,'ob-data')&&!(await lstat(inputRoot)).isSymbolicLink(),'ORACLE_INPUT_OWNER_INVALID');await rm(inputRoot,{recursive:true});state.inputCleaned=true}catch{state.inputCleanupError='ORACLE_INPUT_CLEANUP_UNAVAILABLE';process.exitCode=1}
    if(originalBefore)try{const after=sourceFacts(await accepted(`/api/v1/data-sources/${originalId}`));await save('original-source-after.json',after);state.originalSourceUnchanged=JSON.stringify(originalBefore)===JSON.stringify(after);requireFact(state.originalSourceUnchanged,'ORACLE_ORIGINAL_SOURCE_CHANGED')}catch(error){state.originalSourceReadError=safeCode(error);process.exitCode=1}
  }else state.cleanupDeferred='ORACLE_UNRESOLVED_CHECK_OR_SUBMISSION'
  await client.close();state.finishedAt=new Date().toISOString();const failed=state.error||state.records.some(row=>row.outcome==='FAIL')||['targetReadError','draftCleanupError','sourceCleanupError','fixtureCleanupError','inputCleanupError','originalSourceReadError','cleanupDeferred'].some(key=>state[key]);state.overall=failed?'FAILED':'ORACLE_API_E2E_BLOCKED';state.apiSemanticsPassed=!failed;await checkpoint();console.log(JSON.stringify({overall:state.overall,records:state.records.length,tasks:state.tasks.length,fixtureCleaned:state.fixtureCleaned,inputCleaned:state.inputCleaned,error:state.error}));if(failed)process.exitCode=1
}
