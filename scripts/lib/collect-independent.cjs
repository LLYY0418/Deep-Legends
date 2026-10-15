'use strict';
// Sequential tasks may share setup, but only explicit prerequisites can suppress a task.
async function collectIndependent(tasks,{onResult=()=>{}}={}){
 const rows=[];
 for(const task of tasks){
  const blocked=(task.dependsOn||[]).filter(name=>rows.find(row=>row.name===name)?.status!=='passed');
  const start=performance.now();let row={name:task.name};
  if(blocked.length)row={...row,status:'skipped-dependency-failed',exitCode:null,dependencies:blocked};
  else try{const detail=await task.run();row={...row,...detail,status:'passed',exitCode:0};}
  catch(error){row={...row,status:'failed',exitCode:error.exitCode||1,error:error.message,stack:error.stack};}
  row.duration_ms=performance.now()-start;rows.push(row);await onResult(row,rows);
 }
 return rows;
}
function requirePassed(rows){
 const failed=rows.filter(row=>row.status!=='passed');
 if(failed.length){const error=new Error('失败清单: '+failed.map(row=>`${row.name}: ${row.status} ${row.error||row.dependencies?.join(',')}`).join('\n'));error.failures=failed;throw error;}
}
module.exports={collectIndependent,requirePassed};
