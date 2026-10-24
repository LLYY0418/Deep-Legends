'use strict';
const fs=require('node:fs'),readline=require('node:readline'),crypto=require('node:crypto');
const MAX_SAMPLES=100000;
const object=value=>value!==null&&typeof value==='object'&&!Array.isArray(value);
const integer=(value,min=0,max=Number.MAX_SAFE_INTEGER)=>Number.isSafeInteger(value)&&value>=min&&value<=max;
const hash=text=>crypto.createHash('sha256').update(text).digest('hex');
// Reassemble JSON only. Completeness describes retained evidence, never casts.
function collector(){
 const groups=new Map(),captures=new Map(),issues=[],observations=[];
 let lines=0;
 const issue=(line,reason,extra={})=>issues.push({line,reason,...extra});
 function add(line){
  lines++;
  let row;try{row=JSON.parse(line)}catch{issue(lines,'invalid-json');return}
  if(!object(row)||!String(row.event||'').startsWith('live_spell_'))return;
  if(typeof row.capture!=='string'||!row.capture||!integer(row.sample,0,MAX_SAMPLES)||!integer(row.epoch||0)){
   issue(lines,'invalid-capture-envelope');return;
  }
  const captureKey=JSON.stringify([row.run_id||'',row.capture]);
  if(!captures.has(captureKey))captures.set(captureKey,{capture:row.capture,run_id:row.run_id,schemas:[],samples:0,failed_samples:0,rejected_event_samples:0,stopped:false,start:null,stop:null,sample_ids:new Map()});
  const c=captures.get(captureKey),schema=row.diagnostic_schema||1;
  if(!c.schemas.includes(schema))c.schemas.push(schema);
  if(row.event==='live_spell_start'){
   if(row.sample!==0){issue(lines,'invalid-start-sample');return}
   c.start={line:lines,sample:row.sample,game_time:row.game_time};
  }
  if(row.event==='live_spell_stop'){
   if(schema===2&&!integer(row.observations,0,1000000)){issue(lines,'invalid-stop-observation-count');return}
   c.stopped=true;c.stop={line:lines,sample:row.sample,game_time:row.game_time,observations:row.observations,successes:row.successes,failures:row.failures};
  }
  if(row.event==='live_spell_sample'){
   if(schema!==2||!integer(row.sample,1,MAX_SAMPLES)||typeof row.completed!=='boolean'||typeof row.all_ok!=='boolean'||!object(row.requests)){
    issue(lines,'invalid-sample-envelope');return;
   }
   const requested=['gamestats','playerlist','eventdata'];
   if(row.completed&&!requested.every(k=>object(row.requests[k]))||row.all_ok&&(!row.completed||!requested.every(k=>row.requests[k]?.result==='ok'&&row.requests[k]?.parse_valid===true))){
    issue(lines,'invalid-sample-results');return;
   }
   const signature=JSON.stringify({requests:row.requests,completed:row.completed,all_ok:row.all_ok});
   if(c.sample_ids.has(row.sample)){
    if(c.sample_ids.get(row.sample)!==signature)issue(lines,'conflicting-sample');
   }else{c.sample_ids.set(row.sample,signature);c.samples++;if(!row.completed||!row.all_ok)c.failed_samples++;if(row.requests.eventdata?.rejected_events>0)c.rejected_event_samples++}
  }
  if(row.event==='live_spell_limit')issue(lines,'capture-limit',{limit:row.limit,reason_detail:row.reason});
  if(!['live_spell_snapshot','live_spell_change'].includes(row.event))return;
  if(schema!==2||!integer(row.observation,1)||!integer(row.part_count,1,16384)||!integer(row.part_index,1,row.part_count)||typeof row.payload!=='string'||row.payload.length>8192||!integer(row.payload_bytes,1,16*1024*1024)||!/^([a-f0-9]{64})$/.test(row.payload_sha256)||typeof row.source!=='string'||typeof row.subject!=='string'){
   issue(lines,'invalid-detail-envelope');return;
  }
  const key=JSON.stringify([captureKey,row.epoch,row.observation]);
  if(!groups.has(key))groups.set(key,{line:lines,row,parts:new Map()});
  const g=groups.get(key);
  for(const k of ['part_count','event','source','subject','sample','previous_observation','payload_bytes','payload_sha256','player_slot','identity_scope','identity_matched']){
   if(g.row[k]!==row[k]){g.invalid=true;issue(lines,'conflicting-detail-envelope');return}
  }
  if(g.parts.has(row.part_index)&&g.parts.get(row.part_index)!==row.payload){g.invalid=true;issue(lines,'conflicting-detail-part');return}
  g.parts.set(row.part_index,row.payload);
 }
 function finish(){
  for(const g of groups.values()){
   if(g.invalid)continue;
   if(g.parts.size!==g.row.part_count){issue(g.line,'missing-detail-parts',{capture:g.row.capture,observation:g.row.observation,received:g.parts.size,expected:g.row.part_count});continue}
   const text=Array.from({length:g.row.part_count},(_,i)=>g.parts.get(i+1)).join('');
   if(Buffer.byteLength(text)!==g.row.payload_bytes||hash(text)!==g.row.payload_sha256){issue(g.line,'detail-payload-digest-mismatch');continue}
   let payload;try{payload=JSON.parse(text)}catch{issue(g.line,'invalid-detail-payload');continue}
   const change=g.row.event==='live_spell_change';
   if(!object(payload)||payload.source!==g.row.source||payload.subject!==g.row.subject||!Array.isArray(payload.limits||[])||(change?!Array.isArray(payload.changes):!object(payload.fields))){issue(g.line,'detail-payload-mismatch');continue}
   if(change&&(!integer(payload.previous_observation,1,g.row.observation-1)||!integer(payload.previous_sample,0,g.row.sample)||payload.previous_observation!==g.row.previous_observation)){
    issue(g.line,'invalid-previous-observation');continue;
   }
   observations.push({...payload,line:g.line,capture:g.row.capture,run_id:g.row.run_id,epoch:g.row.epoch,sample:g.row.sample,game_time:g.row.game_time,clock_fresh:g.row.clock_fresh,identity_scope:g.row.identity_scope,identity_matched:g.row.identity_matched,event:g.row.event,observation:g.row.observation});
  }
  const ids=new Map(observations.map(o=>[JSON.stringify([o.run_id||'',o.capture,o.epoch,o.observation]),o]));
  for(const o of observations){
   if(o.previous_observation){
    const previous=ids.get(JSON.stringify([o.run_id||'',o.capture,o.epoch,o.previous_observation]));
    if(!previous)issue(o.line,'missing-previous-observation',{previous_observation:o.previous_observation});
    else if(previous.source!==o.source||previous.subject!==o.subject||previous.sample!==o.previous_sample)issue(o.line,'previous-observation-mismatch');
   }
   if(o.limits?.length)issue(o.line,'capture-limit',{limits:o.limits});
  }
  const captureRows=[];
  for(const c of captures.values()){
   const missing_samples=[],missing_observations=[];
   if(c.stop?.observations!==undefined){const seen=new Set(observations.filter(o=>o.capture===c.capture&&(o.run_id||'')===(c.run_id||'')).map(o=>o.observation));for(let id=1;id<=c.stop.observations;id++)if(!seen.has(id))missing_observations.push(id);if([...seen].some(id=>id>c.stop.observations))issue(c.stop.line,'observation-after-stop',{capture:c.capture});}
   if(missing_observations.length)issue(c.stop?.line||0,'missing-observation-records',{capture:c.capture,count:missing_observations.length});
   if(!c.start)issue(c.stop?.line||0,'missing-capture-start',{capture:c.capture});
   if(!c.stop)issue(c.start?.line||0,'missing-capture-stop',{capture:c.capture});
   if(c.stop){
    for(let sample=1;sample<=c.stop.sample;sample++)if(!c.sample_ids.has(sample))missing_samples.push(sample);
    if([...c.sample_ids.keys()].some(sample=>sample>c.stop.sample))issue(c.stop.line,'sample-after-stop',{capture:c.capture});
   }
   if(missing_samples.length)issue(c.stop?.line||0,'missing-sample-records',{capture:c.capture,count:missing_samples.length});
   const {sample_ids,...summary}=c;captureRows.push({...summary,missing_samples,missing_observations});
  }
  const legacy_only=captures.size>0&&captureRows.every(c=>c.schemas.every(s=>s===1));
  const details_complete=captures.size>0&&issues.length===0&&captureRows.every(c=>c.schemas.every(s=>s===2));
  return {schema:1,lines,captures:captureRows,observations,issues,legacy_only,details_complete,capture_coverage_complete:details_complete&&observations.length>0&&captureRows.every(c=>c.failed_samples===0&&c.rejected_event_samples===0)};
 }
 return {add,finish};
}
async function analyzeFile(file){
 const c=collector();for await(const line of readline.createInterface({input:fs.createReadStream(file),crlfDelay:Infinity}))c.add(line);
 return c.finish();
}
module.exports={collector,analyzeFile};
if(require.main===module)(async()=>{
 const args=process.argv.slice(2);
 if(!args[0]||![1,3].includes(args.length)||args.length===3&&args[1]!=='--output')throw Error('Usage: node scripts/analyze-live-spell-diagnostics.cjs <log.jsonl> [--output <report.json>]');
 const report=await analyzeFile(args[0]),output=JSON.stringify(report,null,2)+'\n';
 if(args[2])fs.writeFileSync(args[2],output);else process.stdout.write(output);
 process.exitCode=report.details_complete?0:2;
})().catch(error=>{console.error(error.message);process.exitCode=1});
