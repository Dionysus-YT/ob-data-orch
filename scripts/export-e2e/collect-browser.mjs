import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { mysqlExportCases } from './cases.mjs'
import { validateOutput } from './validate.mjs'

const root = path.resolve('artifacts/mysql-export-e2e')
const json = async name => JSON.parse((await readFile(path.join(root, name), 'utf8')).replace(/^\uFEFF/, ''))
const pairs = process.argv.slice(2)
if (!pairs.length || pairs.length % 2) throw new Error('RESULT_CONTEXT_PAIRS_REQUIRED')
const records = new Map()
for (let index = 0; index < pairs.length; index += 2) {
  const results = await json(pairs[index]), contexts = await json(pairs[index + 1])
  for (const item of results.records) {
    const testCase = mysqlExportCases.find(test => test.id === item.id)
    const record = { ...item }
    if (item.outcome === 'CONTENT_PENDING' || item.outcome === 'OBSERVED' && item.state === 'SUCCEEDED') {
      try {
        const validated = await validateOutput(testCase, contexts[item.id], item.summary)
        Object.assign(record, validated)
        record.outcome = item.outcome === 'OBSERVED' ? 'OBSERVED' : 'PASS'
      } catch (error) {
        record.outcome = 'FAIL'
        const first = error.message.split('\n')[0]
        record.code = /^[A-Z_0-9]+$/.test(first) ? first : 'CONTENT_ASSERTION_FAILED'
        if (Array.isArray(error.actual) && Array.isArray(error.expected)) record.contentCounts={actual:error.actual.length,expected:error.expected.length}
      }
    }
    records.set(item.id, record)
  }
}
const cases = [...records.values()]
const coverage=await json('command-coverage.json').catch(()=>({records:[]}))
for(const record of cases){
  const test=mysqlExportCases.find(test=>test.id===record.id)
  if(test?.baseline&&record.outcome==='PASS'){
    const baseline=records.get(test.baseline),a=coverage.records.find(row=>row.id===record.id),b=coverage.records.find(row=>row.id===test.baseline)
    if(baseline?.outcome!=='PASS'||record.contentHash!==baseline.contentHash){record.outcome='FAIL';record.code='REPEATED_CONTENT_MISMATCH'}
    else if(!a?.businessCommandHash||a.businessCommandHash!==b?.businessCommandHash){record.outcome='PARTIAL';record.code='REPEATED_BUSINESS_COMMAND_CHECK_REQUIRED'}
    else record.independentRunsCompared=2
  }
}
const counts = cases.reduce((all, record) => ({ ...all, [record.outcome]: (all[record.outcome] ?? 0) + 1 }), {})
await writeFile(path.join(root, 'verified-results.json'), JSON.stringify({ counts, cases }, null, 2))
process.stdout.write(JSON.stringify({ verified: cases.length, counts, failures: cases.filter(row => row.outcome === 'FAIL').map(({id,code}) => ({id,code})) }) + '\n')
