'use strict';
// No raw CDP IDs, headers, query values, URL origins or filesystem paths.
function safePath(value){
 try{const url=new URL(value);if(!['http:','https:'].includes(url.protocol))return '[non-http]';
  const name=url.pathname;
  if(name==='/demo-data.js'||name==='/api/image'||name==='/api/champion-asset')return name;
  if(/^\/[\w-]+\.(?:js|css|svg|png|ico)$/.test(name))return '/[asset].'+name.split('.').pop();
  if(name.startsWith('/api/'))return '/api/[redacted]';
  return name==='/'?'/':'/[redacted]';
 }catch{return '[invalid-url]';}
}
class PausedRequests{
 constructor(now=()=>performance.now()){this.now=now;this.pending=new Map();this.sessions=new Map();this.targets=new Map();this.requests=0;this.sessionCount=0;}
 alias(map,value,prefix){if(!map.has(value))map.set(value,prefix+(map.size+1));return map.get(value);}
 attach(session,target){this.sessions.set(session,{name:'session-'+(++this.sessionCount),target:this.alias(this.targets,target,'target-')});}
 enter(params,session){const key=session+':'+params.requestId;this.pending.set(key,{id:'request-'+(++this.requests),resourceType:params.resourceType||'Other',path:safePath(params.request.url),session:this.sessions.get(session)?.name||'unattached',target:this.sessions.get(session)?.target||'unattached',entered:this.now(),release:'not-started',networkId:params.networkId});return key;}
 release(key,method){const row=this.pending.get(key);if(row)row.release=method;}
 complete(key){this.pending.delete(key);}
 cancel(session,networkId){for(const [key,row] of this.pending)if(key.startsWith(session+':')&&row.networkId===networkId)this.pending.delete(key);}
 close(session){for(const key of this.pending.keys())if(key.startsWith(session+':'))this.pending.delete(key);this.sessions.delete(session);}
 snapshot(){return [...this.pending.values()].map(({networkId,entered,...row})=>({...row,dwellMs:Math.max(0,this.now()-entered)}));}
}
function timeoutDiagnostic(message,tracker){return Error(message+' pending_fetch='+JSON.stringify(tracker.snapshot()));}
module.exports={PausedRequests,timeoutDiagnostic,safePath};
