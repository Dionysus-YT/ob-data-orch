import {readFile,writeFile} from 'node:fs/promises'
import path from 'node:path'
import {mysqlExportCases,buildConfig} from './cases.mjs'
import {expectedFlags} from './parameter-coverage.mjs'

// 仅将公开参数名和本次任务引用交给已登录浏览器；完整命令不写入报告。
const root=path.resolve('artifacts/mysql-export-e2e')
const json=async file=>JSON.parse((await readFile(path.join(root,file),'utf8')).replace(/^\uFEFF/,''))
const verified=await json('verified-results.json'),catalog=await json('catalog.json')
const pairs=process.argv.slice(2),contexts={}
for(const file of pairs)Object.assign(contexts,await json(file))
const tasks=verified.cases.filter(row=>row.taskId&&contexts[row.id]).map(row=>{
  const config=buildConfig(mysqlExportCases.find(test=>test.id===row.id),contexts[row.id])
  return {id:row.id,taskId:row.taskId,expectedFlags:expectedFlags(config),outputPath:config.outputConfig.filePath,logPath:config.outputConfig.logPath}
})
await writeFile(path.join(root,'command-coverage-input.json'),JSON.stringify({tasks,definitions:catalog.definitions.map(({longName,shortName})=>({longName,shortName}))}))
process.stdout.write(JSON.stringify({tasks:tasks.length,definitions:catalog.definitions.length})+'\n')
