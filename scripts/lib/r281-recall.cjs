"use strict";
const fs=require('node:fs'),path=require('node:path'),cp=require('node:child_process'),crypto=require('node:crypto');
const history=require('../fixtures/r281-history.json'),{query}=require('../contract-impact.cjs'),{analyze}=require('../fixture-alignment-check.cjs'),{scanSource,compare}=require('./test-hygiene-scan.cjs'),{classify}=require('../ci-failure-triage.cjs');
function text(row){let lines=[];for(const segment of row.segments){while(lines.length<segment.line-1)lines.push('');lines.push(...segment.text.replace(/\n$/,'').split('\n'));}return lines.join('\n');}
const pick=(prefix,file)=>history.sources.find(row=>row.ref.startsWith(prefix)&&row.file===file);
const source=rows=>({files:rows.map(r=>r.file),read:file=>text(rows.find(r=>r.file===file))});
function recall({go=false,root=path.resolve(__dirname,'../..')}={}){
 const rows=[],add=(id,failure,check,detail)=>rows.push({id,failure,check,detail,ok:Boolean(check)});
 const initial=history.sources.filter(row=>row.ref.startsWith('62a20bfe'));
 const contracts=query(source(initial),['search-server-id','search-region-manual','#player-search-region','retryText','秒']);
 const controls=['r265','r266','r269','r272'].map(n=>'scripts/'+n+'-persisted-ui.cjs');
 add('r281-follow-client-contracts','R278 P1 old persisted controls',controls.every(f=>contracts.files.some(row=>row.file===f&&row.assertions.some(a=>/\.controls\b/.test(a.text)))),contracts.files.map(row=>({file:row.file,references:row.references.map(r=>r.line),assertions:row.assertions.map(r=>r.line)})));
 add('r281-outage-countdown','R278 P2 countdown contract',contracts.files.some(row=>row.file==='scripts/r264-browser.cjs'&&row.assertions.length),contracts.files.find(row=>row.file==='scripts/r264-browser.cjs'));
 const alignment=analyze(source(initial),[['r267','scripts/r267-browser.cjs'],['r275-offline','scripts/r275-offline-browser.cjs']]);
 add('r281-announcement-fixtures','62a20bfe startup announcement bypass',['r267','r275-offline'].every(name=>alignment.issues.some(issue=>issue.guard===name)),alignment.issues);
 const layouts=['current-game','overview-banner','pro-players'].map(n=>'desktop/'+n+'-layout.cjs');
 // 03aa is the FIX, so its parent is the reproducible silent-timeout state.
 const silent=layouts.map(file=>{const candidates=history.sources.filter(r=>r.file===file);return {file,states:candidates.map(row=>({ref:row.ref,...scanSource(text(row))}))};});
 add('r281-silent-timeouts','three layout guards / six silent escapes before 03aa993f',silent.every(row=>row.states.some(s=>s['silent-timeout']>0)&&row.states.filter(s=>s.ref.startsWith('03aa993f')).every(s=>s['silent-timeout']===0)),silent);
 const old=scanSource(text(pick('782fbbf8','desktop/pro-players-layout.cjs')));
 const fixedRow=history.sources.find(r=>r.file==='desktop/pro-players-layout.cjs'&&r.requestedRef==='v0.12.85');
 add('r281-navigation-readiness','782fbbf8 pro-player navigation before app listener',old['early-ready']>0&&Boolean(fixedRow)&&scanSource(text(fixedRow))['early-ready']===0,{old,fixedRef:fixedRow?.ref});
 const silentMutant="await new Promise(resolve=>{if(ready())resolve();setTimeout(()=>resolve(),5000);});";
 const readyMutant="await until(\"!!document.querySelector('#app-frame')&&!app.hasAttribute('inert')\");dispatch('deep-legends:navigate');";
 add('r281-hygiene-negative-controls','new silent escape / initially true ready must fail',compare({}, {'silent.cjs':scanSource(silentMutant),'early.cjs':scanSource(readyMutant)}).increases.filter(r=>['silent-timeout','early-ready'].includes(r.category)).length===2);
 const perf=[112,126].map(ms=>({ms,category:classify('AssertionError: click long tasks '+ms)}));
 add('r281-performance-signature','historical 112/126ms click failures',perf.every(row=>row.category==='performance'),{failures:perf,policy:'Only unchanged test AND fixtures may retry one failed job once. Changed or repeated failure requires repair.'});
 if(go){
  const result=cp.spawnSync('go',['test','-race','-count=1','-run','^(TestR281BackgroundConvergenceWait|TestR278AgeCaptureProductTimestampContract|TestR278AgeCaptureRejectsWrongValues|TestR214FreshSeasonHeadBypassesRecentGate)$','./backend'],{cwd:root,encoding:'utf8',timeout:240000});
  add('r281-background-and-clock-controls','R180 exact age interval / R214 / 5s cleanup: never converges fails, 6s old fails/new passes',result.status===0,{command:'go test -race -count=1 -run <R281/R278 age/R214 cases> ./backend',status:result.status,stdout:result.stdout,stderr:result.stderr,error:result.error?.message});
 }
 return {rows,canonicalMapping:{available:false,expectedCount:30,reason:'Referenced R279 revision-3 numbered A–E/F/G24–28 list is absent from supplied docs and git history. Current R279 §2 contains A–I nine summaries. Known cases are replayed; no fabricated numbering or claim of 30/30.'},deviations:['03aa993f fixes silent timeouts; bad-state replay uses its parent.','v0.12.85 fixes pro-player readiness; current-game/banner still contain early-ready debt, fixed by R281.']};
}
function verifyGit(root){const errors=[];for(const row of history.sources){let actual;try{actual=cp.execFileSync('git',['show',row.ref+':'+row.file],{cwd:root,encoding:'utf8',maxBuffer:8*1024*1024});}catch(error){errors.push(row.ref+':'+row.file+': unavailable history');continue;}if(crypto.createHash('sha256').update(actual).digest('hex')!==row.source_sha256)errors.push(row.file+': sha mismatch');const lines=actual.split('\n');for(const segment of row.segments)if(lines.slice(segment.line-1,segment.line-1+segment.text.replace(/\n$/,'').split('\n').length).join('\n')!==segment.text.replace(/\n$/,''))errors.push(row.file+': segment mismatch');}return errors;}
module.exports={recall,verifyGit,text};
