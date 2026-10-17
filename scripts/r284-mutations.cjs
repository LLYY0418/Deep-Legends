'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),cp=require('node:child_process'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..'),out=path.resolve(process.argv[2] || path.join(os.tmpdir(),'r284-mutations'));
const tmp=fs.mkdtempSync(path.join(os.tmpdir(),'r284-mutants-')),results=[];fs.mkdirSync(out,{recursive:true});
function check(name,result){
 const log=(result.stdout||'')+(result.stderr||'');fs.writeFileSync(path.join(out,name+'.log'),log);
 const killed=result.status!==0 && /ERR_ASSERTION|AssertionError|--- FAIL:/.test(log) && !/build failed|SyntaxError|Cannot find module|no tests to run|operation not permitted/.test(log);
 results.push({name,exitCode:result.status,killed});fs.writeFileSync(path.join(out,'results.json'),JSON.stringify(results,null,2)+'\n');assert(killed,name+' survived or failed without an assertion');console.log(name+' KILLED');
}
function mutate(file,name,from,to){const original=fs.readFileSync(path.join(root,file),'utf8');assert(original.includes(from),name+' injection absent');const target=path.join(tmp,name+path.extname(file));fs.writeFileSync(target,original.replace(from,to));return target;}
try{
 const js=[
  ['p0-friends-in-head','backend/web/index.html','</head>','<script src="/friends.js" defer></script></head>','R115_INDEX_SOURCE','backend/web/r115.test.cjs','R115 initial resources'],
  ['p0-no-friend-replay','backend/web/friends.js','window.deepLegendsSections.listen("deep-legends:status", onStatus)','window.addEventListener("deep-legends:status", onStatus)','R284_FRIENDS_SOURCE','backend/web/r284-baseline.test.cjs','R284 P0 delayed friends'],
  ['p0-pending-link-lost','backend/web/app.js','task.then(openPendingFriendsLink)','task.then(()=>{})','R284_APP_SOURCE','backend/web/r284-baseline.test.cjs','R284 P0 delayed friends'],
  ['p0-no-refresh-deadline','backend/web/app.js','performance.now()<syncDeadline','true','R284_APP_SOURCE','backend/web/r284-baseline.test.cjs','R284 P0 manual refresh']
 ];
 for(const [name,file,from,to,key,test,pattern] of js){const target=mutate(file,name,from,to);check(name,cp.spawnSync(process.execPath,['--test','--test-name-pattern='+pattern,test],{cwd:root,env:{...process.env,[key]:target},encoding:'utf8',timeout:20000}));}
 if(process.env.R284_JS_ONLY!=='1'){
  const go=[
   ['p0-snapshot-prepended','backend/history_filter_fallback.go','rows = mergeFallbackHistory("all", rows, cachedRows)','cachedRows = mergeFallbackHistory("all", cachedRows, nil)'],
   ['p0-filtered-cursor','backend/gameplay.go','page.Count = max(page.Count, localConsumed)','_ = localConsumed; page.Count = len(known)']
  ];
  for(const [name,file,from,to] of go){let target=mutate(file,name,from,to);
   if(name==='p0-snapshot-prepended'){let s=fs.readFileSync(target,'utf8');s=s.replace('len(rows))','len(cachedRows))').replace('rows[start:end], nil','rows, cachedRows[start:end]').replace('end < len(rows)','end < len(cachedRows)');fs.writeFileSync(target,s);}
   const overlay=path.join(tmp,name+'.json');fs.writeFileSync(overlay,JSON.stringify({Replace:{[path.join(root,file)]:target}}));
   check(name,cp.spawnSync('go',['test','-overlay='+overlay,'./backend','-run','^TestR284RetainedHistoryPagesConsumeUnionOnce$','-count=1'],{cwd:root,env:process.env,encoding:'utf8',timeout:180000,maxBuffer:4e6}));
  }
 }
}finally{fs.rmSync(tmp,{recursive:true,force:true});}
