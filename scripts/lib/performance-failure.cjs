"use strict";
const os=require('node:os');
const SIGNATURE='R281_PERFORMANCE_ASSERTION';
function evidence({name,value,limit,tasks,operations=[],iteration,currentCase,cpu=os.cpus()[0]?.model||'unknown'}) {
  if(value<=limit)return null;
  return {signature:SIGNATURE,name,value,limit,cpu,iteration,currentCase,tasks:tasks.map(task=>{
    const active=operations.filter(op=>op.start<=task.start&&(!Number.isFinite(op.end)||task.start<op.end)).at(-1);
    return {...task,operation:active?.operation||name,iteration:active?.iteration??iteration};
  }),operations};
}
module.exports={SIGNATURE,evidence};
