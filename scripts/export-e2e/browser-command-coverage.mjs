// 固定命令取证 API 只在浏览器内读取；返回公开参数名和断言，不输出原始命令或会话材料。
export async function browserCommandCoverage(input){
  const has=(command,flag)=>flag&&new RegExp('(?:^|\\s)'+flag.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')+'(?:\\s|=|$)').test(command)
  const short=(command,flag)=>flag&&new RegExp('(?:^|\\s)'+flag.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')+'(?:\\s|=|[A-Za-z0-9])').test(command)
  const digest=async value=>[...new Uint8Array(await crypto.subtle.digest('SHA-256',new TextEncoder().encode(value)))].map(byte=>byte.toString(16).padStart(2,'0')).join('')
  const records=[]
  for(const test of input.tasks){
    const response=await fetch('/api/v1/tasks/'+test.taskId+'/command-evidence')
    const body=await response.json(),value=body.item??body
    if(response.status!==200){records.push({id:test.id,code:'COMMAND_EVIDENCE_UNAVAILABLE'});continue}
    const emitted=input.definitions.filter(def=>has(value.command,def.longName)||short(value.command,def.shortName)).map(def=>def.longName)
    const missing=test.expectedFlags.filter(flag=>!emitted.includes(flag))
    const business=value.command.replaceAll(test.outputPath,'{OUTPUT}').replaceAll(test.logPath??'\u0000','{LOG}')
    records.push({id:test.id,emitted,missing,businessCommandHash:await digest(business),redaction:value.redaction,passwordPlaceholder:has(value.command,'-p')&&/-p\s+\*{6}/.test(value.command)})
  }
  return {records}
}
