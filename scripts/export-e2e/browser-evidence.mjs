// 只读取本次已授权任务的公共快照、脱敏命令和持久日志；不读取身份、凭据或 Cookie。
export async function browserEvidence(input){
  const surfaces=[]
  for(const id of input.taskIds){
    for(const suffix of ['snapshot','command-evidence','logs']){
      let cursor
      for(let page=0;page<20;page++){
        const response=await fetch('/api/v1/tasks/'+id+'/'+suffix+(cursor?'?cursor='+encodeURIComponent(cursor):''))
        if(response.status!==200)throw new Error('AUDIT_SURFACE_UNAVAILABLE')
        const body=await response.json();surfaces.push({suffix,body})
        cursor=suffix==='logs'?body.nextCursor:null
        if(!cursor)break
        if(page===19)throw new Error('AUDIT_PAGINATION_LIMIT')
      }
    }
  }
  return surfaces
}
