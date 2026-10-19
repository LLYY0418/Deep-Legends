'use strict';
// Retry a measurement, never a test or an arbitrary exception. Limits belong to callers.
const fs=require('node:fs'),path=require('node:path'),os=require('node:os');
const records=[];
function recordTiming(row){
 records.push(row);console.log('TIMING '+JSON.stringify(row));
 const folder=process.env.R266_BROWSER_OUT||process.env.R277_REPORT_DIR||process.env.RUNNER_TEMP||os.tmpdir();
 const file=process.env.R287_TIMING_OUTPUT||path.join(folder,`r287-timing-${process.pid}.jsonl`);
 fs.mkdirSync(path.dirname(file),{recursive:true});fs.appendFileSync(file,JSON.stringify(row)+'\n');
}
async function timingRetry({measure,within,label='timing measurement',record=recordTiming,env=process.env}){
 const attempts=[],maximum=env.R287_MUTATION_PATH==='1'?1:2;
 for(let number=1;number<=maximum;number++){
  // Exceptions (including content/geometry assertions) immediately propagate.
  const value=await measure(number);attempts.push(value);
  const ok=Boolean(within(value));await record({label,path:maximum===1?'mutation':'baseline',attempts:structuredClone(attempts),ok});
  if(ok)return {value,attempts};
 }
 const error=Error(`${label}: ${maximum} timing measurement(s) exceeded the unchanged limit; attempts=${JSON.stringify(attempts)}`);
 error.code='TIMING_LIMIT';error.attempts=attempts;error.measurement=measure;throw error;
}
module.exports={timingRetry,recordTiming,timingRecords:()=>structuredClone(records)};
