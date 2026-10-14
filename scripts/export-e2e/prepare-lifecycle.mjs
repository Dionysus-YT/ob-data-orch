import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { mysqlExportCases, buildConfig } from './cases.mjs'
const root = path.resolve('artifacts/mysql-export-e2e')
const json = async file => JSON.parse((await readFile(path.join(root,file),'utf8')).replace(/^\uFEFF/,''))
const run = await json('run.json'), binding = await json('inspection.json')
const core = await json('verified-results.json')
const workflowNames = ['INVALIDATION','IDEMPOTENCY','EXPIRY','SUBMIT_REPLAY','CANCEL_QUEUED','CANCEL_RUNNING','CHECKPOINT_NEGATIVE','CHECKPOINT_POSITIVE','DERIVE','TEMPLATE','AUTHORIZATION','AGENT_RECOVERY','LOGS']
const cases = [], contexts = {}
for (const test of mysqlExportCases.filter(row => workflowNames.includes(row.workflow))) {
  const context = { ...run, outputPath:path.resolve('ob-data',`odo_e2e_${run.runId}`,`${test.id}-lifecycle`,'output'),logPath:path.resolve('ob-data',`odo_e2e_${run.runId}`,`${test.id}-lifecycle`,'logs') }
  const selected = test.workflow === 'CANCEL_RUNNING' ? {...test,patch:{table:'slow',thread:1,pageSize:100,blockSize:'1000ROW'}} : test.workflow==='LOGS'?{...test,patch:{table:'bulk',thread:1,pageSize:100}}:test
  contexts[test.id] = {...context,fixture:await json('fixture.json')}
  cases.push({id:test.id,workflow:test.workflow,config:buildConfig(selected,context)})
}
const successfulTaskId = core.cases.find(row => row.state === 'SUCCEEDED' && row.outcome === 'PASS')?.taskId
const failedTaskId = core.cases.find(row => row.id === 'MX-009' && row.state === 'FAILED' && !row.summary?.checkpointPresent)?.taskId
await writeFile(path.join(root,'lifecycle-input.json'),JSON.stringify({sourceId:binding.sourceId,nodeId:binding.nodeId,runId:run.runId,successfulTaskId,failedTaskId,cases}))
await writeFile(path.join(root,'lifecycle-contexts.json'),JSON.stringify(contexts))
process.stdout.write(JSON.stringify({cases:cases.length,successfulSeed:Boolean(successfulTaskId),failedSeed:Boolean(failedTaskId)})+'\n')
