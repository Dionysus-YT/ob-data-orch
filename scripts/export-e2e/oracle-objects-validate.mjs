import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { inventory, parseCsv } from './validate.mjs'

// 固定夹具的 DDL 只认可实际 CREATE 目标；注释、字符串、文件名和工具退出码不能替代对象定义。
export function createTargets(text) {
  // 4.3.5 的 Oracle 例程、类型、触发器和视图使用 DELIMITER $$ 包装。
  text = text.replace(/^[ \t]*DELIMITER[ \t]+(?:\$\$|;)[ \t]*(?:\r?\n|$)/gm, '')
  const tokens = []
  const lexical = /--[^\r\n]*|\/\*[\s\S]*?\*\/|'(?:''|[^'])*'|"(?:""|[^"])*"|[A-Za-z0-9_$]+|[^\s]/g
  for (const match of text.matchAll(lexical)) {
    const value = match[0]
    if (value.startsWith('--') || value.startsWith('/*') || value.startsWith("'")) continue
    tokens.push({ value: value.startsWith('"') ? value.slice(1, -1).replaceAll('""', '"') : value, quoted: value.startsWith('"') })
  }
  const word = (index, value) => !tokens[index]?.quoted && tokens[index]?.value.toUpperCase() === value
  const qualified = index => tokens[index + 1]?.value === '.' ? { name: tokens[index + 2]?.value, end: index + 3 } : { name: tokens[index]?.value, end: index + 1 }
  const targets = []
  for (let index = 0; index < tokens.length; index++) {
    if (!word(index, 'CREATE') || index > 0 && ![';', '/'].includes(tokens[index - 1].value)) continue
    let cursor = index + 1
    if (word(cursor, 'OR') && word(cursor + 1, 'REPLACE')) cursor += 2
    let isPublic = false
    if (word(cursor, 'PUBLIC')) { isPublic = true; cursor++ }
    if (word(cursor, 'EDITIONABLE') || word(cursor, 'NONEDITIONABLE')) cursor++
    let type = tokens[cursor++]?.value.toUpperCase()
    if (!['TABLE', 'VIEW', 'FUNCTION', 'PROCEDURE', 'PACKAGE', 'TRIGGER', 'SYNONYM', 'TYPE', 'SEQUENCE'].includes(type)) continue
    if (word(cursor, 'BODY')) { type += '_BODY'; cursor++ }
    if (isPublic) { assert.equal(type, 'SYNONYM', 'UNEXPECTED_PUBLIC_DDL'); type = 'PUBLIC_SYNONYM' }
    const target = qualified(cursor)
    const fact = { objectType: type, name: target.name }
    if (type.endsWith('SYNONYM')) {
      assert.ok(word(target.end, 'FOR'), 'SYNONYM_TARGET_MISSING')
      fact.forName = qualified(target.end + 1).name
    }
    targets.push(fact)
  }
  return targets
}

async function main() {
  const runDirectory = (await readFile('artifacts/oracle-export-current-run.txt', 'utf8')).trim()
  const run = JSON.parse(await readFile(path.join(runDirectory, 'run.json'), 'utf8'))
  assert.match(run.runId, /^[a-f0-9]{32}$/)
  const prefix = 'ODOO_' + run.runId.slice(0, 12).toUpperCase() + '_'
  const base = path.resolve('ob-data', 'odo-oracle-' + run.runId.slice(0, 12))
  const cases = ['package', 'trigger', 'synonym', 'public-synonym', 'type', 'mixed-ddl', 'mixed-csv']
  const results = []
  const auditInput = []
  for (const id of cases) {
    const receipt = JSON.parse(await readFile(path.join(runDirectory, 'fixture-component-' + id + '.json'), 'utf8'))
    assert.equal(receipt.status, 'COMPONENT_OBSERVED', 'TOOL_NOT_STARTED')
    for (const field of ['ownerVerified', 'processExited', 'outputComplete', 'terminalObserved']) assert.equal(receipt[field], true, field)
    assert.equal(receipt.exitCode, 0, 'NONZERO_EXIT')
    assert.equal(receipt.errorsObserved, false, 'TOOL_ERROR_OBSERVED')
    const expected = Object.entries(receipt.objectTypes).flatMap(([type, names]) => names.map(name => type + ':' + name)).sort()
    const files = await inventory(path.join(base, 'component-' + id))
    const definitions = files.filter(file => file.path.endsWith('-schema.sql'))
    assert.equal(definitions.length, expected.length, 'DDL_FILE_COUNT_MISMATCH')
    const targets = []
    const facts = []
    let csvRows = null
    for (const file of files) {
      const bytes = await readFile(file.fullPath)
      auditInput.push(bytes)
      const metadata = path.basename(file.path) === '.MANIFEST.bin'
      facts.push({ path: file.path, size: bytes.length, sha256: createHash('sha256').update(bytes).digest('hex'), kind: metadata ? 'METADATA' : 'RESULT' })
      if (file.path.endsWith('-schema.sql')) {
        assert.ok(bytes.length > 0, 'EMPTY_DDL')
        targets.push(...createTargets(bytes.toString('utf8')))
      } else if (file.path.endsWith('.csv')) {
        assert.equal(id, 'mixed-csv', 'UNEXPECTED_DATA_OUTPUT')
        assert.equal(csvRows, null, 'UNEXPECTED_CSV_FILE_COUNT')
        csvRows = parseCsv(bytes.toString('utf8').replace(/^\uFEFF/, '')).map(row => row.map(field => field.value))
      } else if (metadata) assert.equal(id, 'mixed-csv', 'UNEXPECTED_MANIFEST')
      else assert.fail('UNEXPECTED_RESULT_FILE')
    }
    assert.deepEqual(targets.map(target => target.objectType + ':' + target.name).sort(), expected, 'DDL_CREATE_TARGETS_MISMATCH')
    for (const target of targets.filter(target => target.objectType.endsWith('SYNONYM'))) assert.equal(target.forName, prefix + 'DATA', 'SYNONYM_TARGET_MISMATCH')
    if (id === 'mixed-csv') {
      assert.ok(csvRows, 'CSV_MISSING')
      assert.deepEqual(csvRows[0], ['ID', 'LABEL'], 'CSV_HEADER_MISMATCH')
      assert.deepEqual(csvRows.slice(1).sort((a, b) => a[0].localeCompare(b[0])), [['1', 'alpha'], ['2', 'beta']], 'CSV_FULL_ROWSET_MISMATCH')
    } else assert.equal(csvRows, null, 'DDL_ONLY_CONTAINS_DATA')
    for (const log of await inventory(path.join(base, 'component-' + id + '-logs'))) auditInput.push(await readFile(log.fullPath))
    results.push({ id, outcome: 'PASS', scope: 'FIXED_TOOL_COMPONENT', ddlObjectCount: targets.length, dataRowCount: csvRows ? csvRows.length - 1 : null, files: facts, configFingerprint: receipt.configFingerprint })
  }
  const audit = spawnSync(path.resolve('artifacts/oracle-export-fixture-tools/fixture.exe'), ['-action', 'import-oracle-audit', '-database', 'odo_e2e_20261102_' + run.runId.slice(0, 12), '-run-id', run.runId], { input: Buffer.concat(auditInput), encoding: 'utf8', timeout: 20000 })
  assert.equal(audit.status, 0, 'SECRET_AUDIT_UNAVAILABLE')
  assert.equal(JSON.parse(audit.stdout).plaintextOrRecoverableEncodingPresent, false, 'SECRET_IN_EVIDENCE')
  const proof = { recordedAt: new Date().toISOString(), scope: 'FIXED_TOOL_COMPONENT', platformTasksVerified: false, results, secretsAbsent: true }
  await writeFile(path.join(runDirectory, 'verified-results.json'), JSON.stringify(proof, null, 2))
  console.log(JSON.stringify({ cases: results.length, passed: results.length, ddlCounts: results.map(result => result.ddlObjectCount), csvRows: 2, secretsAbsent: true, platformTasksVerified: false }))
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch(error => { const code = error.message.split('\n')[0]; console.error(/^[A-Z_]+$/.test(code) ? code : 'ORACLE_CONTENT_VERIFICATION_FAILED'); process.exitCode = 1 })
}
