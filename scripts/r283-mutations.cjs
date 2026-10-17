'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),cp=require('node:child_process'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..'),out=process.env.R283_MUTATION_OUT||path.join(os.tmpdir(),'r283-mutations'),tmp=fs.mkdtempSync(path.join(os.tmpdir(),'r283-mutants-'));fs.mkdirSync(out,{recursive:true});
const js=[
 ['P1-LP','gameplay.js','GAMEPLAY','P1','previous?.lpDelta!=null','false'],
 ['P1-images','gameplay.js','GAMEPLAY','P1 unchanged SSE','const changed=old!==match','const changed=true || old!==match'],
 ['P2','app.js','APP','P2','// Manual reads preserve the live channel; genuine reconnects still resync.','setupLiveUpdates();'],
 ['P5','app.js','APP','P5','tasks.length<=48?48:24','tasks.length<=48?1:1'],
 ['P6','gameplay.css','GAMEPLAY_CSS','P6 P7','flex:0 0 auto; width:auto;','flex:0 0 80px; width:80px;'],
 ['P7','app.css','APP_CSS','P6 P7','z-index: 55; zoom: var(--ui-zoom, 1);','z-index: 55; zoom: 1;'],
 ['P8','gameplay.css','GAMEPLAY_CSS','P6 P7','{background:#1d2533;}','{background:white;}'],
 ['P9','app.css','APP_CSS','P6 P7','aspect-ratio: auto; margin: 0;','aspect-ratio: auto; margin: 12px auto 0;'],
 ['P10','gameplay.css','GAMEPLAY_CSS','P6 P7','grid-template-rows:repeat(2,var(--match-loadout-size));grid-auto-flow:column;','grid-template-rows:repeat(2,var(--match-loadout-size));grid-auto-flow:row;'],
 ['P11','live-arena.js','ARENA','P11',"rankHTML(player.flexRank,'组排')","rankHTML(player.flexRank,'')"],
 ['P12','bravery.svg','BRAVERY','P12','id="bv2-ring"','id="wrong-ring"'],
 ['P13','suite.css','SUITE_CSS','P13','.facade-film button.is-selected { border:2px','.facade-film button.is-selected { border:1px'],
 ['P14','app.js','APP','P14','el.appScroll.scrollTo({top:0,behavior:"instant"})','el.appScroll.scrollTo({top:500,behavior:"instant"})'],
 ['P15','gameplay.js','GAMEPLAY','P15','if (!tab) {','if (!tab && group === "pro") {'],
 ['P17-row','gameplay.js','GAMEPLAY','P17','<small class="insight-none">读取失败</small></div>','<small class="insight-none">读取失败</small><button data-live-history-retry>重试</button></div>'],
 ['P17-duplicate','gameplay.js','GAMEPLAY','P17',': state.liveLoadingVisible ? "正在刷新…"\n      : "";',': state.liveLoadingVisible ? "正在刷新…"\n      : liveAutoRefreshStopped(data);'],
];
const go=[
 ['P1-early','backend/lp_tracker.go','TestR283LPFirstEligibleAndCorrection','snapshot.games() == baseline.games()+1 && afterOK','false && snapshot.games() == baseline.games()+1 && afterOK'],
 ['P1-unified','backend/gameplay.go','TestR283LPUnifiedPublicOutput','a.annotateOverviewLP(response, "public")','// annotation removed'],
 ['P3-help','backend/arena_fame_discovery.go','TestR283ArenaHelpFormats','case string:','case bool:'],
 ['P3-window','backend/arena_fame_discovery.go','TestR283ArenaPostGameWindow','schedule(120*time.Second','schedule(1*time.Second'],
 ['P4','backend/history_filter_fallback.go','TestR283FallbackUnion','[][]gameplayMatch{known, incoming}','[][]gameplayMatch{nil, incoming}'],
 ['P16','backend/data/augment_descriptions_20261017.json','TestR283Augment391Chinese','"id":391,','"id":999391,'],
 ['P17-classification','backend/live_history_freshness.go','TestR283LiveOfficialOutage','return errors.Is(err, errSGPHistoryCooling) ||','return err!=nil || errors.Is(err, errSGPHistoryCooling) ||'],
];
const results=[];
function check(id,result){fs.writeFileSync(path.join(out,id+'.log'),(result.stdout||'')+(result.stderr||''));const output=(result.stdout||'')+(result.stderr||'');const killed=result.status!==0&&/FAIL|not ok|✖/.test(output)&&!/failed to listen on a port|operation not permitted|no tests to run|build failed|SyntaxError|Cannot find module/.test(output);results.push({id,killed,status:result.status});assert(killed,id+' survived or failed without assertion');console.log(id+' KILLED');}
try {
 for(const [id,file,env,pattern,from,to] of js){const original=fs.readFileSync(path.join(root,'backend/web',file),'utf8');assert(original.includes(from),id+' injection absent');const target=path.join(tmp,id+path.extname(file));fs.writeFileSync(target,original.replace(from,to));const r=cp.spawnSync(process.execPath,['--test','--test-name-pattern=R283 '+pattern,path.join(root,'backend/web/r283.test.cjs')],{cwd:root,env:{...process.env,['R283_'+env+'_SOURCE']:target},encoding:'utf8',timeout:60000,maxBuffer:4e6});check(id,r);}
 for(const [id,file,pattern,from,to] of go){const original=fs.readFileSync(path.join(root,file),'utf8');assert(original.includes(from),id+' injection absent');const target=path.join(tmp,id+path.extname(file));fs.writeFileSync(target,original.replace(from,to));const overlay=path.join(tmp,id+'.overlay.json');fs.writeFileSync(overlay,JSON.stringify({Replace:{[path.join(root,file)]:target}}));const r=cp.spawnSync('go',['test','-overlay='+overlay,'./backend','-run','^'+pattern+'$','-count=1'],{cwd:root,env:process.env,encoding:'utf8',timeout:90000,maxBuffer:4e6});check(id,r);}
}finally{fs.writeFileSync(path.join(out,'results.json'),JSON.stringify(results,null,2)+'\n');fs.rmSync(tmp,{recursive:true,force:true});}
