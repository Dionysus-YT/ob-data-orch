import assert from 'node:assert/strict'
import { readFile, readdir, stat, realpath } from 'node:fs/promises'
import path from 'node:path'
import { createHash } from 'node:crypto'
import * as zlib from 'node:zlib'

export function inside(root, candidate) {
  const relative = path.relative(path.resolve(root), path.resolve(candidate))
  return relative === '' || !relative.startsWith(`..${path.sep}`) && relative !== '..' && !path.isAbsolute(relative)
}

// 读取仅限本次受控输出；拒绝符号链接、路径逃逸和无界文件。
export async function inventory(root) {
  const canonicalRoot = await realpath(root)
  const files = []
  async function visit(directory) {
    for (const entry of await readdir(directory, { withFileTypes: true })) {
      const file = path.join(directory, entry.name)
      if (entry.isSymbolicLink() || !inside(canonicalRoot, await realpath(file))) throw new Error('OUTPUT_PATH_ESCAPE')
      if (entry.isDirectory()) await visit(file)
      else if (entry.isFile()) {
        const value = await stat(file)
        if (value.size > 64 * 1024 * 1024) throw new Error('OUTPUT_FILE_LIMIT')
        files.push({ path: path.relative(root, file).replaceAll('\\', '/'), size: value.size, fullPath: file })
      }
    }
  }
  await visit(root)
  return files.sort((a, b) => a.path.localeCompare(b.path))
}

// CSV 解析保留引用事实，确保未引用的 NULL 标记不会与字符串值混同。
export function parseCsv(text, { separator = ',', quote = "'", escape = null } = {}) {
  const rows = []
  let row = [], field = '', quoted = false, inQuote = false, started = false
  function pushField() { row.push({ value: field, quoted }); field = ''; quoted = false; started = false }
  function pushRow() { pushField(); rows.push(row); row = [] }
  for (let i = 0; i < text.length; i++) {
    const ch = text[i]
    if (inQuote) {
      if (ch === quote && text[i + 1] === quote) { field += quote; i++ }
      else if (ch === quote) inQuote = false
      else if (escape && ch === escape && i + 1 < text.length) field += text[++i]
      else field += ch
    } else if (!started && ch === quote) { inQuote = true; quoted = true; started = true }
    else if (ch === separator) pushField()
    else if (ch === '\n' || ch === '\r') { if (ch === '\r' && text[i + 1] === '\n') i++; pushRow() }
    else if (ch === escape && (text[i + 1] === separator || text[i + 1] === escape)) { field += text[++i]; started = true }
    else { field += ch; started = true }
  }
  if (inQuote) throw new Error('CSV_QUOTE_UNCLOSED')
  if (started || field || row.length) pushRow()
  return rows
}

// Snappy 解码只服务于固定测试输出；限制长度和回溯，拒绝损坏块。
export function decodeSnappyRaw(buffer) {
  let offset = 0, length = 0, shift = 0
  for (;;) {
    if (offset >= buffer.length || shift > 28) throw new Error('SNAPPY_INVALID')
    const byte = buffer[offset++]; length |= (byte & 127) << shift
    if (!(byte & 128)) break
    shift += 7
  }
  if (length < 0 || length > 64 * 1024 * 1024) throw new Error('SNAPPY_LIMIT')
  const result = Buffer.alloc(length)
  let written = 0
  while (offset < buffer.length && written < length) {
    const tag = buffer[offset++], type = tag & 3
    let count, back
    if (type === 0) {
      count = tag >> 2
      if (count < 60) count++
      else { const size = count - 59; count = 0; for (let i = 0; i < size; i++) count += buffer[offset++] * 2 ** (8 * i); count++ }
      if (offset + count > buffer.length || written + count > length) throw new Error('SNAPPY_INVALID')
      buffer.copy(result, written, offset, offset + count); written += count; offset += count
    } else {
      if (type === 1) { count = 4 + ((tag >> 2) & 7); back = ((tag & 224) << 3) | buffer[offset++] }
      else { count = 1 + (tag >> 2); const bytes = type === 2 ? 2 : 4; back = 0; for (let i = 0; i < bytes; i++) back += buffer[offset++] * 2 ** (8 * i) }
      if (!Number.isInteger(back) || back < 1 || back > written || written + count > length) throw new Error('SNAPPY_INVALID')
      for (let i = 0; i < count; i++) { result[written] = result[written - back]; written++ }
    }
  }
  if (offset !== buffer.length || written !== length) throw new Error('SNAPPY_INVALID')
  return result
}

function decodeSnappyHadoop(buffer) {
  let offset = 0
  const chunks = []
  while (offset < buffer.length) {
    if (offset + 4 > buffer.length) throw new Error('SNAPPY_FRAME_INVALID')
    const expected = buffer.readUInt32BE(offset); offset += 4
    let written = 0
    if (expected > 64 * 1024 * 1024) throw new Error('SNAPPY_LIMIT')
    while (written < expected) {
      if (offset + 4 > buffer.length) throw new Error('SNAPPY_FRAME_INVALID')
      const size = buffer.readUInt32BE(offset); offset += 4
      if (size < 1 || offset + size > buffer.length) throw new Error('SNAPPY_FRAME_INVALID')
      const block = decodeSnappyRaw(buffer.subarray(offset, offset + size)); offset += size; written += block.length; chunks.push(block)
    }
    if (written !== expected) throw new Error('SNAPPY_FRAME_INVALID')
  }
  return Buffer.concat(chunks)
}

// 官方 4.3.5 的 .sz 使用 Xerial 流，不能把其魔数当作 Hadoop 的解压长度。
// 只接受已核验的 v1 头；块长、截断及累计解压字节均有界，损坏文件不能降级为通过。
function decodeSnappyXerial(buffer) {
  const magic = Buffer.from([0x82, 0x53, 0x4e, 0x41, 0x50, 0x50, 0x59, 0])
  const limit = 64 * 1024 * 1024, chunks = []
  let offset = 0, total = 0
  while (offset < buffer.length) {
    if (buffer.subarray(offset, offset + magic.length).equals(magic)) {
      if (offset + 16 > buffer.length || buffer.readUInt32BE(offset + 8) !== 1 || buffer.readUInt32BE(offset + 12) !== 1) throw new Error('SNAPPY_FRAME_INVALID')
      offset += 16
      continue
    }
    if (offset + 4 > buffer.length) throw new Error('SNAPPY_FRAME_INVALID')
    const size = buffer.readUInt32BE(offset); offset += 4
    if (size < 1 || size > limit || offset + size > buffer.length) throw new Error('SNAPPY_FRAME_INVALID')
    const block = decodeSnappyRaw(buffer.subarray(offset, offset + size))
    offset += size; total += block.length
    if (total > limit) throw new Error('SNAPPY_LIMIT')
    chunks.push(block)
  }
  return Buffer.concat(chunks, total)
}

export function decompress(buffer, algorithm) {
  if (!algorithm) return buffer
  const options = { maxOutputLength: 64 * 1024 * 1024 }
  if (algorithm === 'gzip') return zlib.gunzipSync(buffer, options)
  if (algorithm === 'zlib') return zlib.inflateSync(buffer, options)
  if (algorithm === 'zstd') return zlib.zstdDecompressSync(buffer, options)
  if (algorithm === 'snappy') return buffer.subarray(0, 8).equals(Buffer.from([0x82, 0x53, 0x4e, 0x41, 0x50, 0x50, 0x59, 0])) ? decodeSnappyXerial(buffer) : decodeSnappyHadoop(buffer)
  throw new Error('COMPRESSION_UNSUPPORTED')
}

export function decode(buffer, encoding = 'UTF-8') {
  if (encoding === 'UTF-32') {
    const little = buffer.subarray(0, 4).equals(Buffer.from([255, 254, 0, 0]))
    let offset = buffer.subarray(0, 4).equals(Buffer.from(little ? [255, 254, 0, 0] : [0, 0, 254, 255])) ? 4 : 0
    let text = ''
    for (; offset < buffer.length; offset += 4) text += String.fromCodePoint(little ? buffer.readUInt32LE(offset) : buffer.readUInt32BE(offset))
    return text
  }
  if (encoding === 'UTF-16') {
    const little = buffer.subarray(0, 2).equals(Buffer.from([255, 254]))
    return new TextDecoder(little ? 'utf-16le' : 'utf-16be', { fatal: true }).decode(buffer)
  }
  return new TextDecoder(encoding, { fatal: true }).decode(buffer)
}

function normalizeRows(rows) { return rows.map(row => JSON.stringify(row)).sort() }
function unescapeCut(value, escape = '\\') {
  return value.replace(new RegExp(`${escape === '\\' ? '\\\\' : escape}([nrt\\\\|])`, 'g'), (_, ch) => ({ n: '\n', r: '\r', t: '\t' }[ch] ?? ch))
}

// SQL 校验只做词法解析，不把导出 SQL 发往数据库或执行入口。
export function parseSqlRows(text) {
  const rows = []
  const insert = /\bINSERT\s+INTO[\s\S]*?\bVALUES\s*/gi
  let match
  while ((match = insert.exec(text))) {
    let index = insert.lastIndex
    while (text[index] === '(') {
      index++; const row = []
      while (index < text.length) {
        while (/\s/.test(text[index] ?? '') && index < text.length) index++
        let value = ''
        if (text[index] === "'") {
          index++
          while (index < text.length) {
            const ch = text[index++]
            if (ch === "'" && text[index] === "'") { value += "'"; index++ }
            else if (ch === "'") break
            else if (ch === '\\') { const next = text[index++]; value += ({ n: '\n', r: '\r', t: '\t', '0': '\x00', Z: '\x1a' }[next] ?? next) }
            else value += ch
          }
        } else {
          const start = index
          while (index < text.length && ![',', ')'].includes(text[index])) index++
          value = text.slice(start, index).trim()
          if (value.toUpperCase() === 'NULL') value = null
        }
        row.push(value)
        while (/\s/.test(text[index] ?? '') && index < text.length) index++
        if (text[index] === ')') { index++; rows.push(row); break }
        if (text[index++] !== ',') throw new Error('SQL_VALUES_INVALID')
      }
      while (/\s/.test(text[index] ?? '') && index < text.length) index++
      if (text[index] === ',') { index++; while (/\s/.test(text[index] ?? '') && index < text.length) index++ }
      else break
    }
    insert.lastIndex = index
  }
  return rows
}

// 整库与多表必须逐表核对完整行集，不能仅以 rich 的内容代替整个范围。
async function validateMultipleCsv(test, context, files, summary) {
  const f=context.fixture,p=test.patch
  const fixtureTables={
    rich:{columns:f.richColumns,rows:f.richRows},
    empty:{columns:f.richColumns,rows:[]},
    nopk:{columns:['id','ascii_label'],rows:[[1,'a'],[2,'b'],[3,'c']]},
    bulk:{columns:['id','ascii_label','notes'],rows:Array.from({length:1024},(_,i)=>[i+1,`row${i+1}`,'x'.repeat(1024)])},
    slow:{columns:['id','ascii_label','notes'],rows:Array.from({length:f.slowRows},(_,i)=>[i+1,`row${i+1}`,'x'.repeat(512)])},
    temporal:{columns:['id','date_col','time_col','datetime_col','timestamp_col'],rows:[[1,'2024-02-29','12:34:56','2024-02-29 12:34:56','2024-02-29 12:34:56'],[2,null,null,null,null]]},
    types:{columns:['id','ascii_label','bin_col'],rows:[[1,'binary','00017fff']]},
    odo_owner:{columns:['run_id'],rows:[[context.runId]]},
  }
  if(f.capabilities?.GENERATED)fixtureTables.generated_values={columns:['id','value_col','doubled'],rows:[[1,7,14],[2,9,18]]}
  if(f.capabilities?.ZERO_DATETIME){
    const preserve=p.timestampFormats?.preserveZeroDatetime&&f.zeroColumnsNullable===false
    fixtureTables.zeros={columns:['id','date_col','datetime_col'],rows:[[1,preserve?'0000-00-00':null,preserve?'0000-00-00 00:00:00':null]]}
  }
  for(const [name,capability] of [['partitioned','PARTITION'],['subpartitioned','SUBPARTITION']]){
    if(f.capabilities?.[capability])fixtureTables[name]={columns:['id','ascii_label'],rows:[[1,'a'],[2,'b'],[3,'c'],[4,'d']]}
  }
  const names=(p.tables??Object.keys(fixtureTables)).filter(name=>!p.excludeTables?.includes(name))
  const data=files.filter(file=>/\.csv$/i.test(file.path)),seen=new Set(),digest=[]
  let count=0
  for(const name of names){
    const expected=fixtureTables[name]
    assert.ok(expected,'MULTI_TABLE_EXPECTATION_MISSING')
    const matching=data.filter(file=>new RegExp(`(?:^|/)${name}(?:[._/-]|$)`,'i').test(file.path))
    assert.ok(matching.length>0||expected.rows.length===0,'MULTI_TABLE_FILE_MISSING')
    const rows=[]
    for(const file of matching){
      seen.add(file.path)
      const parsed=parseCsv(decode(await readFile(file.fullPath)))
      if(parsed.length)assert.deepEqual(parsed.shift().map(cell=>cell.value),expected.columns,'MULTI_TABLE_COLUMNS')
      rows.push(...parsed.map(row=>row.map(cell=>!cell.quoted&&cell.value==='\\N'?null:cell.value)))
    }
    const expectedRows=expected.rows.map(row=>row.map(value=>value===null?null:String(value)))
    assert.deepEqual(normalizeRows(rows),normalizeRows(expectedRows),'MULTI_TABLE_ROWS_'+name.toUpperCase())
    digest.push(name+'\n'+normalizeRows(rows).join('\n'));count+=rows.length
  }
  assert.equal(seen.size,data.length,'MULTI_TABLE_UNEXPECTED_FILE')
  return {fileCount:files.length,dataFileCount:data.length,bytes:summary.totalBytes,rows:count,tablesVerified:names.length,contentVerified:true,contentHash:createHash('sha256').update(digest.sort().join('\n')).digest('hex')}
}

// hasQualifiedCreateTable 只核对固定夹具的 CREATE TABLE 目标，不把注释或字符串中的名称当作前缀证据。
export function hasQualifiedCreateTable(text, database, table) {
  const tokens = []
  const lexical = /--[^\r\n]*|#[^\r\n]*|\/\*[\s\S]*?\*\/|'(?:''|\\[\s\S]|[^'\\])*'|`(?:``|[^`])*`|"(?:""|[^"])*"|[A-Za-z0-9_$]+|[^\s]/g
  for (const match of text.matchAll(lexical)) {
    const value = match[0]
    if (value.startsWith('--') || value.startsWith('#') || value.startsWith('/*')) continue
    const quote = value[0]
    tokens.push({ kind: quote === "'" ? 'STRING' : quote === '`' || quote === '"' ? 'IDENTIFIER' : 'WORD', value: quote === '`' || quote === '"' ? value.slice(1, -1).replaceAll(quote + quote, quote) : value })
  }
  const keyword = (index, value) => tokens[index]?.kind === 'WORD' && tokens[index].value.toUpperCase() === value
  const name = (index, value) => tokens[index]?.kind !== 'STRING' && tokens[index]?.value === value
  for (let index = 0; index < tokens.length; index++) {
    if ((index > 0 && tokens[index - 1].value !== ';') || !keyword(index, 'CREATE') || !keyword(index + 1, 'TABLE')) continue
    let target = index + 2
    if (keyword(target, 'IF') && keyword(target + 1, 'NOT') && keyword(target + 2, 'EXISTS')) target += 3
    if (name(target, database) && tokens[target + 1]?.value === '.' && name(target + 2, table)) return true
  }
  return false
}

export async function validateOutput(test, context, summary) {
  const files = await inventory(context.outputPath)
  if (!summary || summary.result !== 'VERIFIED') throw new Error('RESULT_NOT_VERIFIED')
  // 非空目录用例有一个非结果 sentinel，摘要仍须与实际完整文件事实一致。
  assert.equal(summary.fileCount, files.length, 'RESULT_FILE_COUNT')
  assert.equal(summary.totalBytes, files.reduce((sum, file) => sum + file.size, 0), 'RESULT_BYTES')
  if (summary.files.length === files.length) assert.deepEqual(summary.files.map(f => [f.path.replaceAll('\\', '/'), f.size]).sort(), files.map(f => [f.path, f.size]).sort(), 'RESULT_MANIFEST')
  const p = test.patch
  const format = p.format ?? 'CSV'
  const extension = format === 'CSV' ? '.csv' : format === 'CUT' ? '.dat' : '.sql'
  const dataFiles = files.filter(f => f.path.toLowerCase().includes(extension) && !f.path.toLowerCase().includes('.ddl.'))
  const ddlFiles = files.filter(f => /\.sql(?:$|\.)/i.test(f.path) && !/\.data\.sql/i.test(f.path))
  if (p.retainSchema) {
    const definitions = (await Promise.all(ddlFiles.map(async file => decode(await readFile(file.fullPath))))).join('\n;\n')
    const tables = (p.objects ?? [[p.objectType ?? 'TABLE', p.object ?? p.table ?? 'rich']]).filter(([type]) => type === 'TABLE')
    for (const [, table] of tables) assert.ok(hasQualifiedCreateTable(definitions, context.database, table), 'DDL_SCHEMA_MISSING')
  }
  if (p.content === 'DDL_ONLY') {
    const text = (await Promise.all(ddlFiles.map(async file => decode(await readFile(file.fullPath))))).join('\n')
    assert.ok(/CREATE\s/i.test(text), 'DDL_CREATE_MISSING')
    const objects=p.objects??[[p.objectType??'TABLE',p.object??p.table??'rich']]
    for(const [type,name] of objects){
      assert.ok(text.includes(name),'DDL_OBJECT_MISSING')
      assert.ok(new RegExp(`CREATE\\s+(?:OR\\s+REPLACE\\s+)?${type}\\s`,'i').test(text),'DDL_OBJECT_TYPE_MISSING')
    }
    if (p.dropObject) assert.ok(/DROP\s+TABLE/i.test(text), 'DDL_DROP_MISSING')
    if (p.retainSchema) assert.ok(text.includes(context.database), 'DDL_SCHEMA_MISSING')
    if (p.sequencePolicy) {
      assert.ok(/CREATE\s+SEQUENCE/i.test(text), 'SEQUENCE_DDL_MISSING'); assert.ok(/INCREMENT\s+BY\s+3/i.test(text), 'SEQUENCE_INCREMENT')
      assert.ok(context.fixture.sequence?.status === 'SEQUENCE_EXERCISED', 'SEQUENCE_SOURCE_STATE_UNAVAILABLE')
      const start = /START\s+WITH\s+(\d+)/i.exec(text)
      assert.equal(Number(start?.[1]), p.sequencePolicy === 'RESTART' ? context.fixture.sequence.restart : context.fixture.sequence.expectedNext, 'SEQUENCE_POLICY_VALUE')
    }
    return { fileCount: files.length, bytes: summary.totalBytes, contentVerified: true, ddl: true }
  }
  if((p.scope==='ALL'||p.tables)&&format==='CSV')return validateMultipleCsv(test,context,files,summary)
  let selected = dataFiles
  const table = p.table ?? 'rich'
  if (p.scope === 'ALL' || p.tables) selected = dataFiles.filter(f => /(?:^|\/)rich(?:[./_-]|$)/i.test(f.path))
  if (p.query) selected = dataFiles
  const texts = await Promise.all(selected.map(async file => decode(decompress(await readFile(file.fullPath), p.compress ? p.compressionAlgo ?? 'zstd' : undefined), p.csv?.fileEncoding)))
  let columns, rows = [], rawCells = []
  if (format === 'CSV') {
    for (const text of texts) {
      const parsed = parseCsv(text, { separator: p.csv?.columnSeparator, quote: p.csv?.columnQuote, escape: p.csv?.escapeCharacter })
      if (!p.csv?.skipHeader && parsed.length) { const header = parsed.shift().map(field => field.value); if (columns) assert.deepEqual(header, columns, 'CSV_SPLIT_HEADER'); else columns = header }
      rawCells.push(...parsed)
    }
    rows = rawCells.map(row => row.map(field => (!field.quoted || p.csv?.columnQuoteMode === 'all') && field.value === (p.csv?.nullString ?? '\\N') ? null : field.value))
  } else if (format === 'SQL') rows = texts.flatMap(parseSqlRows)
  else {
    const splitter = p.cut?.columnSplitter ?? '|'
    rows = texts.flatMap(text => text.trimEnd().split(new RegExp(`\\r?\\n(?=[0-9]+${splitter.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`)).filter(Boolean).map(line => {
      const values = line.replace(/\r?\n$/, '').split(splitter)
      if (values.at(-1) === '' && line.endsWith(splitter)) values.pop()
      return values.map(value => value === (p.csv?.nullString ?? '\\N') ? null : unescapeCut(value))
    }))
  }
  let expectedColumns = context.fixture.richColumns
  let expected = context.fixture.richRows.map(row => row.map(value => value === null ? null : String(value)))
  if (p.query) {
    if (/id < 0/.test(p.query)) expected = []
    else { expected = expected.map(row => row.slice(0, 2)); expectedColumns = ['id', p.query.includes('AS label') ? 'label' : 'ascii_label']; if (p.queryLimit || /id <= 2/.test(p.query)) expected = expected.slice(0, 2) }
  } else if (table === 'empty') expected = []
  else if (table === 'nopk') { expectedColumns = ['id', 'ascii_label']; expected = [['1', 'a'], ['2', 'b'], ['3', 'c']] }
  else if (table === 'generated_values') { expectedColumns = ['id', 'value_col']; expected = [['1', '7'], ['2', '9']] }
  else if (table === 'partitioned' || table === 'subpartitioned') { expectedColumns = ['id', 'ascii_label']; expected = context.fixture.partitionRows[table].map(row => row.map(String)) }
  else if (table === 'types') { expectedColumns = ['id', 'ascii_label']; expected = [['1', 'binary']] }
  else if (table === 'bulk' || table === 'slow') {
    expectedColumns = ['id', 'ascii_label', 'notes']
    const count = table === 'bulk' ? 1024 : context.fixture.slowRows
    expected = Array.from({ length: count }, (_, index) => [String(index + 1), `row${index + 1}`, 'x'.repeat(table === 'bulk' ? 1024 : 512)])
  } else if (table === 'temporal') {
    const f = p.timestampFormats ?? {}
    expectedColumns = ['id', 'date_col', 'time_col', 'datetime_col', 'timestamp_col']
    const date = f.dateValueFormat ? '2024/02/29' : '2024-02-29'
    const time = f.timeValueFormat ? '12-34-56' : '12:34:56'
    expected = [['1', date, time, f.datetimeValueFormat ? '2024/02/29 12:34:56' : '2024-02-29 12:34:56', f.timestampValueFormat ? '2024/02/29 12:34:56' : '2024-02-29 12:34:56'], ['2', null, null, null, null]]
  } else if (table === 'zeros') {
    expectedColumns = ['id', 'date_col', 'datetime_col']
    if (!p.timestampFormats?.preserveZeroDatetime) return { fileCount: files.length, bytes: summary.totalBytes, observedZeroRows: rows, contentVerified: false }
    if (typeof context.fixture.zeroColumnsNullable !== 'boolean') throw new Error('ZERO_NULLABILITY_EVIDENCE_REQUIRED')
    expected = context.fixture.zeroColumnsNullable ? [['1', null, null]] : [['1', '0000-00-00', '0000-00-00 00:00:00']]
  }
  if (p.where) expected = expected.filter(row => Number(row[0]) <= 2)
  if (p.include || p.exclude) {
    const indexes = expectedColumns.map((name, index) => [name, index]).filter(([name]) => p.include ? p.include.includes(name) : !p.exclude.includes(name))
    expectedColumns = indexes.map(([name]) => name); expected = expected.map(row => indexes.map(([, index]) => row[index]))
  }
  if (p.csv?.withTrim) expected = expected.map(row => row.map(value => typeof value === 'string' ? value.trim() : value))
  if (p.cut?.removeNewline) expected = expected.map(row => row.map(value => typeof value === 'string' ? value.replace(/[\r\n]/g, '') : value))
  if (columns) assert.deepEqual(columns, expectedColumns, 'CSV_COLUMN_SET')
  if (p.maxFileSize) {
    // 总量上限是停止条件；检查已输出行完整且属于源夹具，不要求导出全部或创建多个分片。
    const expectedSet = new Set(normalizeRows(expected)), actual = normalizeRows(rows)
    assert.ok(rows.length>0 && rows.length<expected.length, 'OUTPUT_LIMIT_NOT_OBSERVED')
    assert.equal(new Set(actual).size, actual.length, 'OUTPUT_LIMIT_DUPLICATES')
    assert.ok(actual.every(row=>expectedSet.has(row)), 'OUTPUT_LIMIT_CONTENT')
    assert.ok(summary.totalBytes>=p.maxFileSize && summary.totalBytes<=p.maxFileSize+2048, 'OUTPUT_LIMIT_BYTES')
  } else assert.deepEqual(normalizeRows(rows), normalizeRows(expected), 'OUTPUT_ROW_SET')
  if (p.blockSize === '256ROW') assert.ok(selected.length > 1, 'OUTPUT_SPLIT_NOT_OBSERVED')
  if (p.noNestedDir) assert.ok(selected.every(file => !file.path.includes('/')), 'OUTPUT_NOT_FLAT')
  if (p.content === 'DDL_AND_DATA') assert.ok(files.some(file => /ddl|schema/i.test(file.path)), 'COMBINED_DDL_MISSING')
  return { fileCount: files.length, dataFileCount: selected.length, bytes: summary.totalBytes, rows: rows.length, contentVerified: true, ...(p.maxFileSize?{totalLimitObserved:true,sourceRows:expected.length}:{}), ...(p.cut?.trailDelimiter ? { trailDelimiterObserved: 'REMOVES_TRAILING_SEPARATOR' } : {}), contentHash: createHash('sha256').update(normalizeRows(rows).join('\n')).digest('hex') }
}
