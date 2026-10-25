// 真实预检查矩阵只读取数据库元信息；未知/失败绑定尝试提交必须拒绝，不启动未支持工具路径。
import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { gzipSync, deflateSync } from 'node:zlib'

export async function runGates(input, client, save, work) {
  const require = (condition, code) => { if (!condition) throw new Error(code) }
  const accepted = async (route, method, body, revision) => {
    const result = await client.request(route, method, body, revision)
    require(result.status >= 200 && result.status < 300, result.body.error?.code ?? result.body.code ?? 'API_REJECTED')
    return result.body.item ?? result.body
  }
  const mysql = await accepted('/api/v1/data-sources/'+input.sourceId)
  require(mysql.environment==='TEST'&&mysql.connectionKind==='ODP'&&mysql.compatibilityMode==='MYSQL'&&mysql.state==='ENABLED','MYSQL_SOURCE_OUTSIDE_TEST_SCOPE')
  const oracle = await accepted('/api/v1/data-sources/55208ffb-2171-4e69-bb97-5af53c2f0d81')
  require(oracle.environment === 'TEST' && oracle.connectionKind === 'ODP' && oracle.compatibilityMode === 'ORACLE' && oracle.state === 'ENABLED', 'ORACLE_SOURCE_OUTSIDE_TEST_SCOPE')
  const schema = oracle.defaultDatabase || oracle.username?.toUpperCase()
  require(schema && /^[A-Za-z_][A-Za-z0-9_$#]{0,127}$/.test(schema), 'ORACLE_SCHEMA_FACT_UNAVAILABLE')
  const directory = path.join(input.inputRoot,'gate-files')
  // 独占本轮子目录；已有证据不覆盖，重试前先核对批次回执。
  await mkdir(directory)
  const state = { scope:'REAL_READONLY_PREFLIGHT', done:false, records:[], startedAt:new Date().toISOString() }
  const output = path.join(work,'api-capability-prechecks.json')
  const recipes = [
    { id:'control', names:['--ctl-path'], mapping:{ kind:'CONTROL', ctlPath:path.join(directory,'synthetic.ctl') } },
    { id:'type_exclusion', names:['--exclude-data-types'], mapping:{ kind:'POSITION', excludeDataTypes:'BLOB' } },
    { id:'footer', names:['--skip-footer'], format:'CUT', parser:{columnSplitter:'|',skipFooter:true}, data:'1|alpha|plain\nfooter\n', suffix:'.dat' },
    { id:'compress', names:['--compress'], parser:{columnSeparator:',',columnQuote:'"',compress:true}, codec:'gzip' },
    { id:'compression_algo', names:['--compression-algo'], parser:{columnSeparator:',',columnQuote:'"',compress:true,compressionAlgo:'gzip'}, codec:'gzip' },
    { id:'compression_level', names:['--compression-level'], parser:{columnSeparator:',',columnQuote:'"',compress:true,compressionAlgo:'zlib',compressionLevel:'1'}, codec:'zlib' },
    { id:'encoding_gbk', names:['--file-encoding'], fileEncoding:'GBK' },
  ]
  const oracleKinds = ['role','sequence','synonym','type','type-body','package','package-body']
  const definitions = {
    'table-group':'CREATE TABLEGROUP NIVT_GATE;',
    view:'CREATE VIEW NIVT_GATE AS SELECT 1 AS ID;',
    trigger:'CREATE TRIGGER NIVT_GATE BEFORE INSERT ON NIVT_GATE_TABLE FOR EACH ROW SET NEW.ID=NEW.ID;',
    'obj-user':'CREATE USER NIVT_GATE;',
    role:'CREATE ROLE NIVT_GATE;', sequence:'CREATE SEQUENCE NIVT_GATE START WITH 1;',
    synonym:'CREATE SYNONYM NIVT_GATE FOR NIVT_GATE_TABLE;', type:'CREATE TYPE NIVT_GATE AS OBJECT (ID NUMBER);',
    'type-body':'CREATE TYPE BODY NIVT_GATE AS MEMBER FUNCTION VALUE RETURN NUMBER IS BEGIN RETURN 1; END; END;',
    package:'CREATE PACKAGE NIVT_GATE AS PROCEDURE VALUE; END;',
    'package-body':'CREATE PACKAGE BODY NIVT_GATE AS PROCEDURE VALUE IS BEGIN NULL; END; END;',
    function:'CREATE FUNCTION NIVT_GATE() RETURNS INT RETURN 1;', procedure:'CREATE PROCEDURE NIVT_GATE() SELECT 1;',
  }
  for (const [kind, definition] of Object.entries(definitions)) recipes.push({id:'object_'+kind.replaceAll('-','_'),names:['--'+kind],kind,definition,oracle:oracleKinds.includes(kind)})
  for (const [id, field, name, type, value, format] of [
    ['nls_date','nlsDateFormat','--nls-date-format','DATE','20/10/2026 12:34:56','DD/MM/YYYY HH24:MI:SS'],
    ['nls_timestamp','nlsTimestampFormat','--nls-timestamp-format','TIMESTAMP(6)','20/10/2026 12:34:56.123456','DD/MM/YYYY HH24:MI:SS.FF6'],
    ['nls_timezone','nlsTimestampTzFormat','--nls-timestamp-tz-format','TIMESTAMP(6) WITH TIME ZONE','20/10/2026 12:34:56.123456 +08:00','DD/MM/YYYY HH24:MI:SS.FF6 TZH:TZM'],
  ]) recipes.push({id,names:[name],oracle:true,nls:true,type,value,execution:{memory:'1G',thread:'1',[field]:format}})
  await writeFile(path.join(directory,'synthetic.ctl'),'-- 合成控制文件占位；只验证当前 CONTROL 预检查的阻断，不声称其工具语法有效。\n',{flag:'wx'})
  for (const recipe of recipes.filter(row => input.gateFilter !== 'remaining' || !row.nls && row.id !== 'encoding_gbk')) {
    const record = {id:'NI-GATE-'+recipe.id,names:recipe.names,outcome:'FAIL',toolExecutionValidated:false}
    let draft
    try {
      const current = path.join(directory,recipe.id)
      await mkdir(current)
      const object = recipe.definition || recipe.nls ? (recipe.oracle ? 'NIVT_' : 'nivt_')+'GATE_'+recipe.id.toUpperCase()+'_'+input.runId.slice(0,6).toUpperCase() : 'nivt_p_encoding'
      let suffix = recipe.suffix || '.csv'
      let bytes = Buffer.from(recipe.data || '1,alpha,plain\n')
      if (recipe.codec === 'gzip') { bytes=gzipSync(bytes); suffix+='.gz' }
      if (recipe.codec === 'zlib') { bytes=deflateSync(bytes,{level:1}); suffix+='.zlib' }
      if (recipe.definition) {
        // 定义文本只供受控分类/权限检查；下文从不提交 SUCCEEDED 的定义检查。
        await writeFile(path.join(current,object+'-schema.sql'),recipe.definition.replaceAll('NIVT_GATE',object)+'\n',{flag:'wx'})
      } else if (recipe.nls) {
        await writeFile(path.join(current,object+'-schema.sql'),`CREATE TABLE ${object} (ID NUMBER PRIMARY KEY, VALUE1 ${recipe.type});\n`,{flag:'wx'})
        await writeFile(path.join(current,object+'.csv'),`"1","${recipe.value}"\n`,{flag:'wx'})
      } else await writeFile(path.join(current,object+suffix),bytes,{flag:'wx'})
      bytes.fill(0)
      const config = {dataSourceId:recipe.oracle?oracle.id:input.sourceId,nodeId:input.nodeId,database:recipe.oracle?schema:input.database,filePath:current,
        logPath:path.join(input.inputRoot,'logs-gate-'+recipe.id),content:recipe.definition?'DDL_ONLY':recipe.nls?'DDL_AND_DATA':'DATA_ONLY',format:recipe.format||'CSV',scope:'SPECIFIC',
        objects:{[recipe.kind||'table']:object},parser:recipe.parser||{columnSeparator:',',columnQuote:'"'},mapping:recipe.mapping||{kind:'POSITION'},execution:recipe.execution||{memory:'1G',thread:'1'},cleanup:'NONE',...(recipe.fileEncoding?{fileEncoding:recipe.fileEncoding}:{})}
      const created=await client.request('/api/v1/normal-import-drafts','POST',{configVersion:'normal-import-v1',config})
      record.createStatus=created.status
      require(created.status===201,'GATE_DRAFT_REJECTED')
      draft=created.body; record.draftId=draft.id
      const route=`/api/v1/normal-import-drafts/${draft.id}`
      const preview=await accepted(route+':preview-command','POST',{},draft.revision)
      require(recipe.names.every(name=>preview.redactedCommand.includes(name)),'GATE_PARAMETER_MISSING')
      const started=await accepted(route+':precheck','POST',{},draft.revision)
      record.precheckId=started.id
      state.current=record; await save(output,state)
      const until=Date.now()+90000
      let checked
      while(Date.now()<until){checked=await accepted('/api/v1/normal-import-prechecks/'+started.id);if(!['PENDING','LEASED'].includes(checked.status))break;await new Promise(resolve=>setTimeout(resolve,1000))}
      require(checked?.integrityStatus==='COMPLETE'&&!['PENDING','LEASED'].includes(checked.status),'GATE_CHECK_INCOMPLETE')
      record.checkStatus=checked.status;record.checks=checked.results
      record.bindingMatched=checked.draftRevision===draft.revision&&checked.configFingerprint===preview.configFingerprint
      require(record.bindingMatched,'GATE_CHECK_BINDING_INVALID')
      if(checked.status==='SUCCEEDED') {
        // NLS 的预检成功不能证明工具转换结果；没有独立 Oracle 行集证据时不冒充通过。
        record.outcome='PREFLIGHT_PASSED_TOOL_NOT_VALIDATED'
        record.code=recipe.nls?'ORACLE_INDEPENDENT_RESULT_AND_CLEANUP_NOT_CONFIGURED':'CAPABILITY_GATE_EXPECTATION_CHANGED'
      } else if (['obj-user','role'].includes(recipe.kind)) {
        // 账号/角色定义超出独立表夹具的写入边界，只读取检查，不拿真实安全对象试探提交漏洞。
        record.outcome='BLOCKED_PREFLIGHT_ONLY'
        record.code='GLOBAL_OBJECT_SUBMISSION_OUTSIDE_FIXTURE_SCOPE'
      } else {
        const submitted=await client.request(route+':submit','POST',{precheckId:started.id,confirmedConfigFingerprint:draft.configFingerprint},draft.revision)
        record.submitStatus=submitted.status
        if(submitted.status===201) {
          record.unexpectedTaskId=submitted.body.id
          if(record.unexpectedTaskId) await client.request(`/api/v1/tasks/${record.unexpectedTaskId}:cancel`,'POST',{})
          state.stopped=true
        }
        require([409,422].includes(submitted.status),'GATE_INVALID_CHECK_SUBMITTED')
        record.outcome='BLOCKED_WITH_SUBMIT_REJECTION_VERIFIED'
      }
    } catch(error){record.code=/^[A-Z_0-9]+$/.test(error.message)?error.message:'GATE_TEST_UNAVAILABLE'}
    finally {
      if(draft){const deleted=await client.request(`/api/v1/normal-import-drafts/${draft.id}`,'DELETE',{},draft.revision);record.cleanupStatus=deleted.status;if(![200,204].includes(deleted.status)){record.outcome='FAIL';record.code='GATE_DRAFT_CLEANUP_UNCONFIRMED'}}
    }
    state.records.push(record);await save(output,state)
    console.log(`${record.id}: ${record.outcome}`)
    if(state.stopped) break
  }
  state.done=true;state.finishedAt=new Date().toISOString();await save(output,state)
  return state
}
