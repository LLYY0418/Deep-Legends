'use strict';
// Real same-origin HTTP requests only. In-memory demo Response objects are
// business fixtures, not HTTP traffic. No bodies, headers or payloads are saved.
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const policy=require('./r276-api-fault-policy.cjs');
class Census {
 constructor(guard=process.env.R276_API_CENSUS_GUARD){this.guard=guard;this.rows=new Map();this.sessions=0;this.connections=0;}
 key(source,connection,session,id){return [source,connection,session||'',id].join('|');}
 cancel(connection,session,reason){for(const source of ['network'])for(const [key,row] of this.rows)if(key.startsWith(this.key(source,connection,session,''))&&!row.status&&!row.failed){row.canceled=true;row.transportReason='net::ERR_ABORTED';row.cancelReason=reason;}}
 request(source,connection,session,id,url,method){
  let u;try{u=new URL(url)}catch{return}
  if(!['http:','https:'].includes(u.protocol)||!['localhost','127.0.0.1','[::1]'].includes(u.hostname)||!u.pathname.startsWith('/api/'))return;
  this.rows.set(this.key(source,connection,session,id),{source,path:u.pathname,method:method||'GET',status:null});
 }
 event(connection,m){
  const p=m.params||{},session=m.sessionId;
  if(m.method==='Network.requestWillBeSent')this.request('network',connection,session,p.requestId,p.request.url,p.request.method);
  if(m.method==='Network.responseReceived'){const row=this.rows.get(this.key('network',connection,session,p.requestId));if(row)row.status=p.response.status;}
  if(m.method==='Network.loadingFailed'){const row=this.rows.get(this.key('network',connection,session,p.requestId));if(row){row.failed=true;row.canceled=!!p.canceled;row.transportReason=p.errorText==='net::ERR_ABORTED'?'net::ERR_ABORTED':'OTHER';}}

 }
 report(){const observations=[...this.rows.values()];return {guard:this.guard,connections:this.connections,sessions:this.sessions,canceledCount:observations.filter(isCanceled).length,observations};}
}
function isCanceled(row){return row.canceled===true&&row.transportReason==='net::ERR_ABORTED';}
function failures(report){
 return report.observations.filter(row=>!isCanceled(row)&&(row.status>=400||!row.status||row.failed));
}
function classify(report,guard){assert.equal(report.guard,guard,'API census guard identity mismatch');return {...policy.classify(guard,report,failures(report)),canceled:report.observations.filter(isCanceled),canceledCount:report.observations.filter(isCanceled).length};}
function requireClean(report,guard){const classified=classify(report,guard);assert.equal(classified.otherFailures.length,0,'API census contains HTTP >=400, failed or unanswered requests: '+JSON.stringify(classified.otherFailures));return classified;}
function install(target=globalThis,census=new Census()){
 const Original=target.WebSocket;assert(Original,'API census requires real CDP WebSocket');
 target.WebSocket=class extends Original {
  constructor(url,...args){
   super(url,...args);const connection=++census.connections,attached=new Map(),targets=new Map();let internal=-100000;
   const command=(method,params,sessionId)=>super.send(JSON.stringify({id:--internal,method,params,sessionId}));
   const observe=session=>{census.sessions++;command('Network.enable',{},session);};
   this.addEventListener('open',()=>{if(String(url).includes('/devtools/page/'))observe(undefined);});
   this.addEventListener('message',event=>{const m=JSON.parse(event.data);if(attached.has(m.id)){const target=attached.get(m.id);attached.delete(m.id);if(m.result?.sessionId){targets.set(target,m.result.sessionId);observe(m.result.sessionId);}}census.event(connection,m);});
   this.send=value=>{const m=JSON.parse(value);if(m.method==='Target.attachToTarget')attached.set(m.id,m.params.targetId);if(m.method==='Page.navigate')census.cancel(connection,m.sessionId,'navigation');if(m.method==='Target.closeTarget'&&targets.has(m.params.targetId))census.cancel(connection,targets.get(m.params.targetId),'target-close');return super.send(value);};
   this.close=(...closeArgs)=>{for(const session of [undefined,...targets.values()])census.cancel(connection,session,'target-close');return super.close(...closeArgs);};
  }
 };
 return census;
}
function summarize(directory){
 const files=[];const walk=p=>{for(const entry of fs.readdirSync(p,{withFileTypes:true})){const f=path.join(p,entry.name);if(entry.isDirectory())walk(f);else if(entry.isFile()&&f.endsWith('.json'))files.push(f);}};walk(directory);const report={connections:0,sessions:0,observations:[]};
 assert(files.length,'missing API census evidence');
 for(const file of files){const data=JSON.parse(fs.readFileSync(file));if(report.guard===undefined)report.guard=data.guard;assert.equal(data.guard,report.guard,'mixed API census guard identities');report.connections+=data.connections;report.sessions+=data.sessions;report.observations.push(...data.observations);}
 assert(report.connections>0,'API census did not observe a Chromium connection');return report;
}
if(process.env.R276_API_CENSUS_OUTPUT){
 policy.validatePolicy();
 const census=install(),directory=process.env.R276_API_CENSUS_OUTPUT;
 process.on('exit',()=>{fs.mkdirSync(directory,{recursive:true});fs.writeFileSync(path.join(directory,process.pid+'.json'),JSON.stringify(census.report(),null,2)+'\n');});
}
module.exports={Census,install,failures,classify,requireClean,summarize};
