import { readFile, writeFile, mkdir } from 'node:fs/promises'
import path from 'node:path'
import { mysqlExportCases, buildConfig } from './cases.mjs'
import { expectedFlags } from './parameter-coverage.mjs'
const root = path.resolve('artifacts/mysql-export-e2e')
const json = async name => JSON.parse((await readFile(path.join(root, name), 'utf8')).replace(/^\uFEFF/, ''))
const run = await json('run.json'), binding = await json('inspection.json'), fixture = await json('fixture.json'), augmented = await json('fixture-augment.json')
const current = await json('fixture-current.json').catch(() => ({}))
fixture.capabilities.GENERATED = augmented.generated; fixture.capabilities.SYS = false; fixture.capabilities.NO_SYS = true
fixture.partitionRows = augmented.partitionRows
fixture.capabilities.HIDDEN_PK_READABLE = current.hiddenPkZeroRowReadable === true
fixture.sequence = await json('sequence-state.json').catch(() => ({}))
const outputRoot = path.resolve('ob-data', `odo_e2e_${run.runId}`)
const options = process.argv[2] ? JSON.parse((await readFile(process.argv[2], 'utf8')).replace(/^\uFEFF/, '')) : {}
const suffix = options.suffix ?? '-browser'
const contexts = {}, cases = []
for (const test of mysqlExportCases.filter(test => !test.workflow && (!options.cases || options.cases.includes(test.id)))) {
  const outputPath = path.join(outputRoot, `${test.id}${suffix}`, 'output'), logPath = path.join(outputRoot, `${test.id}${suffix}`, 'logs')
  const context = { ...run, fixture, outputPath, logPath, disallowedPath: 'C:\\odo_e2e_outside', scn: current.scn ?? fixture.scn }
  if (test.patch.nonempty) { await mkdir(outputPath, { recursive: true }); await writeFile(path.join(outputPath, 'fixture-sentinel.txt'), 'owned fixture marker', { flag: 'wx' }) }
  const config = buildConfig(test, context)
  if (test.patch.flashbackScn === '{scn}') config.filterConfig.flashbackScn = context.scn
  const blocked = test.requires?.find(capability => !fixture.capabilities[capability])
  contexts[test.id] = context
  cases.push({ id: test.id, area: test.area, title: test.title, expected: test.expected, failedCheck: test.failedCheck, taskTimeoutMs:test.id==='MX-005'?600_000:180_000, expectedFlags: expectedFlags(config), ...(blocked ? { blocked: `FIXTURE_${blocked}_UNAVAILABLE` } : {}), input: { configVersion: 'v6', dataSourceId: binding.sourceId, nodeId: binding.nodeId, config } })
}
await writeFile(path.join(root, 'browser-input.json'), JSON.stringify({ nodeId: binding.nodeId, cases }))
await writeFile(path.join(root, 'browser-contexts.json'), JSON.stringify(contexts))
process.stdout.write(JSON.stringify({ prepared: cases.length }) + '\n')
