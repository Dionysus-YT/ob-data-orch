import assert from 'node:assert/strict'
import test from 'node:test'
import { mysqlExportCases, buildConfig } from './cases.mjs'
import { parseCsv, parseSqlRows, inside, decodeSnappyRaw, decompress, decode, inventory, validateOutput } from './validate.mjs'
import * as zlib from 'node:zlib'
import {mkdtemp,writeFile,rm} from 'node:fs/promises'
import path from 'node:path'
import os from 'node:os'

const context = { database: 'odo_e2e_20261014_0123456789ab', outputPath: 'E:\\synthetic\\output', logPath: 'E:\\synthetic\\logs', disallowedPath: 'C:\\synthetic\\outside', scn: '123' }
test('矩阵标识唯一且覆盖活动的格式、对象和专家参数', () => {
  assert.equal(new Set(mysqlExportCases.map(row => row.id)).size, mysqlExportCases.length)
  assert.ok(mysqlExportCases.length >= 150)
  for (const format of ['CSV', 'CUT', 'SQL']) assert.ok(mysqlExportCases.some(row => (row.patch.format ?? 'CSV') === format && row.expected === 'SUCCEEDED'))
  for (const option of ['weakRead', 'enableHiddenPk', 'addExtraMessage', 'sequencePolicy', 'timestampFormats']) assert.ok(mysqlExportCases.some(row => row.patch[option] !== undefined))
  for (const capability of ['VIEW', 'FUNCTION', 'PROCEDURE', 'SEQUENCE']) assert.ok(mysqlExportCases.some(row => row.requires?.includes(capability)))
})
test('DTO 遵循 CUT 分隔符所有者、MySQL 枚举和现役节点路径格式', () => {
  const cut = buildConfig(mysqlExportCases.find(row => row.id === 'MX-030'), context)
  assert.equal(cut.dataFormat.csvOptions.columnSplitter, '||')
  assert.equal(cut.dataFormat.cutOptions.columnSplitter, undefined)
  assert.equal(cut.outputConfig.filePath, '/E:/synthetic/output')
  assert.equal(cut.outputConfig.logPath, '/E:/synthetic/logs')
  assert.equal(buildConfig(mysqlExportCases.find(row => row.id === 'MX-023'), context).dataFormat.csvOptions.columnQuoteMode, 'all')
  assert.equal(buildConfig(mysqlExportCases.find(row => row.id === 'MX-001'), context).outputConfig.logPath, undefined)
  assert.deepEqual(buildConfig(mysqlExportCases.find(row => row.id === 'MX-010'), context).performanceConfig, {})
  assert.equal(buildConfig(mysqlExportCases.find(row => row.id === 'MX-027'), context).dataFormat.csvOptions.lineSeparator, String.raw`\r\n`)
  assert.equal(buildConfig(mysqlExportCases.find(row => row.id === 'MX-025-NONE'), context).dataFormat.csvOptions.escapeCharacter, '\\')
})
test('SQL 负例保持 API_REJECTED，正常查询提供限行且不混入对象', () => {
  for (const id of ['MX-017', 'MX-018', 'MX-141']) assert.equal(mysqlExportCases.find(row => row.id === id).expected, 'API_REJECTED')
  const query = buildConfig(mysqlExportCases.find(row => row.id === 'MX-006'), context)
  assert.equal(query.objectScope.scopeKind, 'QUERY_RESULT')
  assert.equal(query.objectScope.expressions, undefined)
  assert.equal(query.filterConfig.queryResultLimit, 10000)
  assert.ok(query.filterConfig.querySql.includes(context.database))
})
test('CSV 解析保留引用、NULL、空串、嵌入换行和尾部反斜杠', () => {
  const rows = parseCsv("'id','value'\n1,\\N\n2,'\\N'\n3,''\n4,'line\nnext'\n5,'slash\\'\n")
  assert.equal(rows.length, 6)
  assert.deepEqual(rows[1][1], { value: '\\N', quoted: false })
  assert.deepEqual(rows[2][1], { value: '\\N', quoted: true })
  assert.equal(rows[3][1].value, '')
  assert.equal(rows[4][1].value, 'line\nnext')
  assert.equal(rows[5][1].value, 'slash\\')
  assert.throws(() => parseCsv("'unterminated"), /CSV_QUOTE_UNCLOSED/)
})
test('SQL 解析只读地保留逗号、转义、数值和 NULL', () => {
  assert.deepEqual(parseSqlRows("INSERT INTO synthetic_table VALUES (1,'comma,quote\\\'newline\\n',NULL),(2,'x','');"), [['1', "comma,quote'newline\n", null], ['2', 'x', '']])
})
test('内容解压与损坏 Snappy 块有界失败', () => {
  const data = Buffer.from('synthetic export content')
  for (const [algorithm, compressed] of [['gzip', zlib.gzipSync(data)], ['zlib', zlib.deflateSync(data)], ['zstd', zlib.zstdCompressSync(data)]]) assert.deepEqual(decompress(compressed, algorithm), data)
  assert.equal(decodeSnappyRaw(Buffer.from([3, 8, 97, 98, 99])).toString(), 'abc')
  assert.throws(() => decodeSnappyRaw(Buffer.from([10, 1, 255])), /SNAPPY/)
  assert.throws(() => decompress(data, 'unrecognized'), /COMPRESSION_UNSUPPORTED/)
})
test('输出目录不允许同前缀逃逸或跨根路径', () => {
  assert.ok(inside('E:\\synthetic\\owned', 'E:\\synthetic\\owned\\case'))
  assert.ok(!inside('E:\\synthetic\\owned', 'E:\\synthetic\\owned-other\\case'))
  assert.ok(!inside('E:\\synthetic\\owned', 'E:\\synthetic\\owned\\..\\outside'))
  assert.ok(!inside('E:\\synthetic\\owned', 'C:\\synthetic\\case'))
})
test('UTF-16 与 UTF-32 按实际 BOM 解码', () => {
  assert.equal(decode(Buffer.from([255, 254, 97, 0]), 'UTF-16'), 'a')
  assert.equal(decode(Buffer.from([0, 0, 254, 255, 0, 0, 0, 97]), 'UTF-32'), 'a')
})

test('多表不能只核对 rich，缺表、少行或混入其他对象均失败',async()=>{
  const root=await mkdtemp(path.join(os.tmpdir(),'odo-e2e-validator-'))
  assert.ok(inside(os.tmpdir(),root))
  try{
    const fixture={richColumns:['id','ascii_label'],richRows:[[1,'alpha']],slowRows:0,capabilities:{}}
    const ctx={outputPath:root,fixture,runId:'synthetic'}
    const testCase={patch:{tables:['rich','nopk']}}
    const summary=async()=>{const files=await inventory(root);return {result:'VERIFIED',fileCount:files.length,totalBytes:files.reduce((n,f)=>n+f.size,0),files}}
    await writeFile(path.join(root,'rich.csv'),"'id','ascii_label'\n1,'alpha'\n")
    await assert.rejects(validateOutput(testCase,ctx,await summary()),/MULTI_TABLE_FILE_MISSING/)
    await writeFile(path.join(root,'nopk.csv'),"'id','ascii_label'\n1,'a'\n2,'b'\n")
    await assert.rejects(validateOutput(testCase,ctx,await summary()),/MULTI_TABLE_ROWS/)
    await writeFile(path.join(root,'nopk.csv'),"'id','ascii_label'\n1,'a'\n2,'b'\n3,'c'\n")
    assert.equal((await validateOutput(testCase,ctx,await summary())).tablesVerified,2)
    await writeFile(path.join(root,'unexpected.csv'),"'id'\n1\n")
    await assert.rejects(validateOutput(testCase,ctx,await summary()),/MULTI_TABLE_UNEXPECTED_FILE/)
  }finally{await rm(root,{recursive:true})}
})

test('多对象 DDL 不以单个对象或名称注释冒充完整导出',async()=>{
  const root=await mkdtemp(path.join(os.tmpdir(),'odo-e2e-validator-'))
  assert.ok(inside(os.tmpdir(),root))
  try{
    const file=path.join(root,'schema.sql'),ctx={outputPath:root},testCase={patch:{content:'DDL_ONLY',objects:[['TABLE','rich'],['VIEW','v_rich']]}}
    const summary=async()=>{const files=await inventory(root);return {result:'VERIFIED',fileCount:files.length,totalBytes:files.reduce((n,f)=>n+f.size,0),files}}
    await writeFile(file,'CREATE TABLE rich(id INT);\n-- v_rich\n')
    await assert.rejects(validateOutput(testCase,ctx,await summary()),/DDL_OBJECT_TYPE_MISSING/)
    await writeFile(file,'CREATE TABLE rich(id INT);\nCREATE OR REPLACE VIEW v_rich AS SELECT id FROM rich;\n')
    assert.equal((await validateOutput(testCase,ctx,await summary())).contentVerified,true)
  }finally{await rm(root,{recursive:true})}
})
