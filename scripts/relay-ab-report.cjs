'use strict';
const fs=require('node:fs'),zlib=require('node:zlib');
const business=new Set(['account','summoner','match','league','spectator','mastery']);
function parseLog(text){
 try{const data=JSON.parse(text);if(Array.isArray(data))return data;if(Array.isArray(data.events))return data.events;}catch{}
 return text.split(/\r?\n/).flatMap(line=>{try{const at=line.indexOf('{');return at>=0?[JSON.parse(line.slice(at))]:[];}catch{return [];}});
}
function stamp(row){return Date.parse(row.at || row.timestamp || row.time || row.window_end_utc);}
function evening(time){if(!Number.isFinite(time))return null;const date=new Date(time+8*3600000),hour=date.getUTCHours();return hour>=20 && hour<22?date.toISOString().slice(0,10):null;}
function percentile(values,p){const sorted=values.slice().sort((a,b)=>a-b);if(!sorted.length)return null;if(p===.5){const n=sorted.length;return n%2?sorted[Math.floor(n/2)]:(sorted[n/2-1]+sorted[n/2])/2;}return sorted[Math.ceil(sorted.length*p)-1];}
function report(rows){
 const groups=new Map(),seen=new Set();
 function group(date,label){const key=date+'|'+label;if(!groups.has(key))groups.set(key,{date,entry:label,requests:null,failures:0,failuresKnown:true,ttfb:[],missingTTFB:false,probeTimeouts:0,fallbacks:0});return groups.get(key);}
 for(const row of rows){const serial=JSON.stringify(row);if(seen.has(serial))continue;seen.add(serial);
  if(row.event==='riot_relay_request_summary'){
   if(Array.isArray(row.business_samples)){
    for(const sample of row.business_samples){const date=evening(Date.parse(sample.at));if(!date)continue;const g=group(date,row.relay_entry || 'A');g.requests=(g.requests || 0)+1;if(sample.failure && !['canceled','not_found'].includes(sample.failure) || sample.status===429)g.failures++;if(Number(sample.ttfb_ms)>0)g.ttfb.push(Number(sample.ttfb_ms));}
    if(row.business_samples_truncated){for(const g of groups.values())if(g.entry===(row.relay_entry || 'A')){g.requestsKnown=false;g.failuresKnown=false;g.missingTTFB=true;}}
    continue;
   }
   const end=Date.parse(row.window_end_utc) || stamp(row),start=Date.parse(row.window_start_utc) || end-Number(row.window_ms || 600000),date=evening(start);
   // A summary spanning the 20/22 boundary cannot be split accurately.
   if(!date || evening(end-1)!==date)continue;
   const g=group(date,row.relay_entry || 'A'),categories=row.categories;
   const count=categories?Object.entries(categories).filter(([k])=>business.has(k)).reduce((n,[,v])=>n+Number(v),0):Number(row.requests);
   g.requests=(g.requests || 0)+count;
   if(row.failure_categories){for(const [failure,byCategory] of Object.entries(row.failure_categories)){if(['canceled','not_found'].includes(failure))continue;g.failures+=Object.entries(byCategory).filter(([k])=>business.has(k)).reduce((n,[,v])=>n+Number(v),0);}g.failures+=Number(row.rate_limited || 0);}
   else{const other=Number(categories?.other || 0);for(const [failure,value] of Object.entries(row.failures || {})){if(['canceled','not_found'].includes(failure))continue;if(typeof value==='object')g.failures+=Object.entries(value).filter(([k])=>business.has(k)).reduce((n,[,v])=>n+Number(v),0);else if(other)g.failuresKnown=false;else g.failures+=Number(value);}g.failures+=Number(row.rate_limited || 0);}
   if(Array.isArray(row.ttfb_values_ms) && row.ttfb_values_ms.length===Number(row.ttfb_samples))g.ttfb.push(...row.ttfb_values_ms.filter(n=>Number.isFinite(n) && n>=0));else if(Number(row.ttfb_samples)>0)g.missingTTFB=true;
  }else if(row.event==='riot_relay_probe'){
   const date=evening(stamp(row));if(date && row.result==='failed' && (row.failure_stage==='timeout' || row.timeout===true))group(date,row.relay_entry || 'A').probeTimeouts++;
  }else if(row.event==='relay_entry_switch' && row.to==='A' && ['timeout_or_certificate','network_failures'].includes(row.reason)){
   const date=evening(stamp(row));if(date)group(date,row.from).fallbacks++;
  }
 }
 return [...groups.values()].sort((a,b)=>a.date.localeCompare(b.date) || a.entry.localeCompare(b.entry)).map(g=>({...g,requests:g.requestsKnown===false?null:g.requests,window:g.date+' 20:00–22:00',failureRate:g.requestsKnown!==false && g.requests && g.failuresKnown?100*g.failures/g.requests:null,ttfbSamples:g.ttfb.length,median:!g.missingTTFB && g.ttfb.length>=5?percentile(g.ttfb,.5):null,p90:!g.missingTTFB && g.ttfb.length>=5?percentile(g.ttfb,.9):null}));
}
function table(rows){
 const value=(n,digits=0)=>n==null?'样本不足':Number(n).toFixed(digits);
 return ['| 北京时间窗口 | 入口 | 请求数 | 失败数 | 失败率 | TTFB 中位(ms) | P90(ms) | 探测超时 | 回退 |','|---|---|---:|---:|---:|---:|---:|---:|---:|',...rows.map(r=>`| ${r.window} | ${r.entry} | ${value(r.requests)} | ${value(r.failuresKnown && r.requests!=null?r.failures:null)} | ${r.failureRate==null?'样本不足':value(r.failureRate,2)+'%'} | ${value(r.median,1)} | ${value(r.p90,1)} | ${r.probeTimeouts} | ${r.fallbacks} |`)].join('\n');
}
module.exports={parseLog,report,table,evening};
if(require.main===module){const files=process.argv.slice(2);if(!files.length){console.error('用法：node scripts/relay-ab-report.cjs <诊断日志或.gz> [...]');process.exitCode=1;}else{const rows=files.flatMap(file=>{const data=fs.readFileSync(file);return parseLog((file.endsWith('.gz')?zlib.gunzipSync(data):data).toString('utf8'));});console.log(table(report(rows)));}}
