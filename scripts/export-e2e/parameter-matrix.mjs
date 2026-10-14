import {readFile,writeFile} from 'node:fs/promises'
import path from 'node:path'
import {mysqlExportCases,buildConfig} from './cases.mjs'
import {expectedFlags} from './parameter-coverage.mjs'

const root=path.resolve('artifacts/mysql-export-e2e')
const json=async file=>JSON.parse((await readFile(path.join(root,file),'utf8')).replace(/^\uFEFF/,''))
const catalog=await json('catalog.json'),actual=await json('command-coverage.json').catch(()=>({records:[]}))
const synthetic={database:'synthetic_db',outputPath:'E:\\synthetic\\output',logPath:'E:\\synthetic\\logs',disallowedPath:'C:\\synthetic\\outside',scn:'100'}
const cases=mysqlExportCases.map(test=>({...test,flags:expectedFlags(buildConfig(test,synthetic))}))
const oracle=new Set(['--flashback-timestamp','--fetch-size','--timestamp-tz-value-format','--timestamp-ltz-value-format','--nls-date-format','--nls-timestamp-format','--nls-timestamp-tz-format'])
const historical=new Map([['--pos','MX-119-POS'],['--par','MX-119-PARQUET'],['--orc','MX-119-ORC'],['--avro','MX-119-AVRO'],['--ctl-path','MX-145']])
const lines=['# MySQL 本地导出参数追踪','','参数目录：`'+catalog.version+'`。共 '+catalog.definitions.length+' 个定义，按真实授权范围区分活动参数、身份派生、秘密槽位、Oracle/云端排除及历史门控。命令取证只保存参数名，不保存原始命令；“发射观察”不等于该功能实测通过。','','| 参数 | 本轮适用与条件 | 正例 / 负例 | 命令发射观察 |','| --- | --- | --- | --- |']
for(const def of catalog.definitions){
  const related=cases.filter(test=>test.flags.includes(def.longName)),positive=related.filter(test=>test.expected==='SUCCEEDED'||test.expected==='OBSERVED').map(test=>test.id),negative=related.filter(test=>['API_REJECTED','PRECHECK_FAILED'].includes(test.expected)).map(test=>test.id)
  const emitted=actual.records.filter(row=>row.emitted?.includes(def.longName)).map(row=>row.id)
  let scope='MySQL / 本地；逐项结果见实测表'
  if(oracle.has(def.longName))scope='Oracle 专属，正例不在本轮；MySQL 越界负例'
  else if(historical.has(def.longName)){scope='当前普通向导门控；不启动历史能力';negative.push(historical.get(def.longName))}
  else if(def.emissionTarget==='SECURITY_FILE')scope='短时安全文件；真实秘密核对与清理见 MX-132；sys 正例依赖已配置 sys'
  else if(def.sourcePolicy==='DATA_SOURCE')scope='由授权数据源派生；主链路冻结快照、argv 一致性和脱敏核对'
  else if(def.supportState!=='ENABLED')scope='目录状态 '+def.supportState+'；保持原门禁'
  else if(def.longName==='--retry'){scope='检查点继续；真实检查点缺失';negative.push('MX-127');positive.push('MX-128（受阻）')}
  else if(def.longName==='--sys-user'){scope='附加表定义；sys 凭据未配置';positive.push('MX-073（受阻）');negative.push('MX-073-NO-SYS')}
  else if(!related.length&&['--bucket','--storage-type','--endpoint','--region','--oss','--s3','--cos','--obs'].some(name=>def.longName.includes(name)))scope='对象存储不在本轮授权范围'
  else if(!related.length&&def.sourcePolicy!=='DATA_SOURCE'&&def.emissionTarget!=='SECURITY_FILE')scope+='；无直接正例，不能声明全覆盖'
  lines.push('| `'+def.longName+'` | '+scope+' | '+(positive.join('、')||'—')+' / '+(negative.join('、')||'—')+' | '+(emitted.length?emitted.length+' 个任务':'未观察')+' |')
}
lines.push('','连接参数在表中按目录长名归一化；真实生成器可能使用官方短名。Oracle 格式、对象存储、真实检查点与 sys 正例不得由其他通过项外推为可用。','')
await writeFile('docs/03-technical/evidence/export-mysql-parameter-coverage.md',lines.join('\n'))
process.stdout.write(JSON.stringify({definitions:catalog.definitions.length,observedTasks:actual.records.length})+'\n')
