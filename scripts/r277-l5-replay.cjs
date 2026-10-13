'use strict';
const fs=require('node:fs'),crypto=require('node:crypto'),path=require('node:path');
const EVENTS=['client_view_published','client_view_client','client_view_stuck','ui_zoom_state','riot_route_switch'];
const LIMIT=50000;
function replay(input,addedReport){
 const raw=fs.readFileSync(input),lines=raw.toString('utf8').split(/\r?\n/).filter(Boolean);
 if(!lines.length)throw Error('Empty diagnostic input');
 let lo=Infinity,hi=-Infinity,total=0,added=0;const counts=Object.fromEntries(EVENTS.map(e=>[e,0])),bytesByEvent={};
 for(const line of lines){
  const row=JSON.parse(line),at=Date.parse(row.time);
  if(!Number.isFinite(at))throw Error('Missing/invalid diagnostic time');
  lo=Math.min(lo,at);hi=Math.max(hi,at);const bytes=Buffer.byteLength(line)+1;total+=bytes;
  if(EVENTS.includes(row.event)&& (row.event!=='riot_route_switch'||row.reason==='quota-overflow')){
   counts[row.event]++;added+=bytes;bytesByEvent[row.event]=(bytesByEvent[row.event]||0)+bytes;
  }
 }
 const hours=(hi-lo)/3600000;if(hours<=0)throw Error('Diagnostic window must span positive time');
 let rate=added/hours,kind='real-stored-added-events',allPresent=EVENTS.every(e=>counts[e]>0),real=true,addedEvidence;
 if(addedReport){
  const model=JSON.parse(fs.readFileSync(addedReport,'utf8'));
  if(model.kind!=='synthetic-pressure-production-storage'||model.real_device_replay!==false||model.duration_hours!==1||!EVENTS.every(e=>model.counts[e]>0)||!Number.isFinite(model.added_bytes_per_hour))throw Error('Invalid all-event synthetic evidence');
  rate=model.added_bytes_per_hour;kind='real-baseline-plus-synthetic-increment-estimate';real=false;
  addedEvidence={file:path.basename(addedReport),sha256:crypto.createHash('sha256').update(fs.readFileSync(addedReport)).digest('hex'),counts:model.counts};
 }else if(!allPresent){throw Error('Not a complete real added-event replay: missing '+EVENTS.filter(e=>!counts[e]).join(', '));}
 const report={kind,real_device_added_replay:real,input:path.basename(input),input_sha256:crypto.createHash('sha256').update(raw).digest('hex'),window_start:new Date(lo).toISOString(),window_end:new Date(hi).toISOString(),duration_hours:hours,source_rows:lines.length,source_bytes:total,source_bytes_per_hour:total/hours,added_counts:counts,added_bytes_by_event:bytesByEvent,added_bytes_per_hour:rate,added_limit_bytes_per_hour:LIMIT,added_budget_pass:rate<=LIMIT};
 if(addedEvidence){report.synthetic_evidence=addedEvidence;report.combined_estimate_bytes_per_hour=total/hours+rate;}
 return report;
}
function main(argv){
 const opts={};for(let i=0;i<argv.length;i+=2){if(!['--input','--added-report','--out'].includes(argv[i])||!argv[i+1])throw Error('Usage: --input JSONL [--added-report synthetic-result.json] --out RESULT.json');opts[argv[i]]=argv[i+1];}
 if(!opts['--input']||!opts['--out'])throw Error('--input and --out required');
 const report=replay(opts['--input'],opts['--added-report']);fs.mkdirSync(path.dirname(opts['--out']),{recursive:true});fs.writeFileSync(opts['--out'],JSON.stringify(report,null,2)+'\n');
 if(!report.added_budget_pass)throw Error('Added diagnostic rate exceeds 50KB/hour');console.log(JSON.stringify(report,null,2));
}
module.exports={replay,EVENTS,LIMIT};if(require.main===module){try{main(process.argv.slice(2));}catch(e){console.error(e.message);process.exitCode=1;}}
