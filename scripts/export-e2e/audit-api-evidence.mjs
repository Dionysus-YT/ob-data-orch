import {spawn} from 'node:child_process'
import path from 'node:path'

// 响应只进入固定辅助进程的匿名 stdin；核对完成后清零字节，不保存未知脱敏状态的响应。
export async function auditApiEvidence(value,run,repository=path.resolve('.')){
  const bytes=Buffer.from(JSON.stringify(value))
  if(bytes.length>=16*1024*1024){bytes.fill(0);throw new Error('AUDIT_EVIDENCE_LIMIT')}
  try{
    const child=spawn(path.resolve(repository,'artifacts/mysql-export-e2e/fixture.exe'),['-action','audit','-database',run.database,'-run-id',run.runId],{cwd:repository,windowsHide:true,stdio:['pipe','pipe','ignore']})
    let output=''
    const result=new Promise((resolve,reject)=>{
      child.stdout.on('data',chunk=>{output+=chunk; if(output.length>1024){child.kill();reject(new Error('AUDIT_OUTPUT_INVALID'))}})
      child.once('error',()=>reject(new Error('AUDIT_HELPER_UNAVAILABLE')))
      child.once('close',code=>{
        if(code!==0){reject(new Error('AUDIT_HELPER_FAILED'));return}
        try{resolve(JSON.parse(output))}catch{reject(new Error('AUDIT_OUTPUT_INVALID'))}
      })
    })
    child.stdin.on('error',()=>{})
    child.stdin.end(bytes)
    return await result
  }finally{bytes.fill(0)}
}
