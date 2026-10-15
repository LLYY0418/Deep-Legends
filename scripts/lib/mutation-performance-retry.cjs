"use strict";
// Retry only a proven, preceding performance assertion; never an invalid kill.
const fs=require('node:fs'),path=require('node:path');
const root=path.resolve(__dirname,'../..');
function line(file,needle){const lines=fs.readFileSync(path.join(root,file),'utf8').split('\n');const hits=lines.flatMap((s,i)=>s.includes(needle)?[i+1]:[]);if(hits.length!==1)throw Error('performance assertion anchor changed: '+needle);return hits[0];}
function classifyAttempt({code,error='',log='',targetVerified=false}){
 const stack=String(log||error),match=stack.match(/(?:^|\n)\s*at [^\n]*?(scripts[\\/][^()\s]+\.cjs):(\d+):(\d+)/);
 const location=match?{file:match[1].replaceAll('\\','/'),line:Number(match[2]),column:Number(match[3])}:null;
 const result={kind:'invalid',location};
 if(code!==1 || /TypeError|ReferenceError|timeout/i.test(error) || /TypeError|ReferenceError/.test(log))return result;
 if(targetVerified)return {...result,kind:'target'};
 if(!location)return result;
 let known=false;
 if(location.file==='scripts/r266-browser.cjs'){
  const rules=[['assert(clickTasks<=100',/^click long tasks (\d+(?:\.\d+)?)$/,100],['assert(cycleTasks<=1000',/^five filter cycles long tasks (\d+(?:\.\d+)?)$/,1000],['assert(firstCount.ms<=300',/^collection first screen (\d+(?:\.\d+)?)$/,300],['assert(collectionMax<=200',/^collection long task (\d+(?:\.\d+)?)$/,200]];
  known=rules.some(([needle,re,limit])=>{const m=error.match(re);return m && Number(m[1])>limit && location.line===line(location.file,needle);});
  if(!known && location.line===line(location.file,'assert(total<=300 && max<=200')){
   try{const m=JSON.parse(error);known=Number.isFinite(m.nodes)&&m.nodes<=15000&&Number.isFinite(m.images)&&m.images<=1500&&Array.isArray(m.tasks)&&m.tasks.every(t=>Number.isFinite(t.duration)&&t.duration>=0)&&(m.tasks.reduce((s,t)=>s+t.duration,0)>300||m.tasks.some(t=>t.duration>200));}catch{}
  }

 }
 return {...result,kind:known?'known-performance':'invalid'};
}
function isMissingApiTarget({route,failure,log=''}){
 if(!route || !Array.isArray(failure?.errors) || typeof failure.error!=='string' || !failure.errors.some(row=>{try{return new URL(row?.url||'http://localhost/').pathname===route;}catch{return false;}}))return false;
 if(!failure.error.startsWith(JSON.stringify(failure.errors)) || !failure.error.includes(route) || /TypeError|ReferenceError|timeout/.test(failure.error))return false;
 if(!/(?:^|\n)AssertionError(?: \[ERR_ASSERTION\])?:/.test(log))return false;
 const file='scripts/r266-browser.cjs',needle='assert.equal(errors.length,0,JSON.stringify(errors))',lines=fs.readFileSync(path.join(root,file),'utf8').split('\n'),hits=lines.flatMap((text,index)=>text.includes(needle)?[{line:index+1,start:text.indexOf(needle)+1,end:text.indexOf(needle)+needle.length+1}]:[]);
 if(hits.length!==1)throw Error('missing-API target assertion anchor changed');
 const found=log.match(/(?:^|\n)\s*at [^\n]*?scripts[\\/]r266-browser\.cjs:(\d+):(\d+)/),target=hits[0];
 return !!found && Number(found[1])===target.line && Number(found[2])>=target.start && Number(found[2])<=target.end;
}
async function runWithPerformanceRetry(run,record=()=>{}){
 const attempts=[];
 for(let number=1;number<=3;number++){
  const at=Date.now();let attempt;try{attempt=await run(number);}catch(error){attempt={code:-1,error:String(error),log:error.stack||String(error)};}const decision=classifyAttempt(attempt);
  attempts.push({...attempt,number,elapsedMs:attempt.elapsedMs??Date.now()-at,...decision});await record(attempts);
  if(decision.kind==='target')return {verified:true,attempts};
  if(decision.kind!=='known-performance')return {verified:false,attempts};
 }
 return {verified:false,attempts};
}
module.exports={classifyAttempt,runWithPerformanceRetry,isMissingApiTarget};
if(require.main===module){let input='';process.stdin.setEncoding('utf8');process.stdin.on('data',s=>input+=s);process.stdin.on('end',()=>process.stdout.write(JSON.stringify(classifyAttempt(JSON.parse(input)))));}
