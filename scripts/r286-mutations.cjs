'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),cp=require('node:child_process'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..'),out=path.resolve(process.argv[2]),phase=process.argv[3]||'all',tmp=fs.mkdtempSync(path.join(os.tmpdir(),'r286-mutants-')),results=[];fs.mkdirSync(out,{recursive:true});
function check(name,r){const log=(r.stdout||'')+(r.stderr||'');fs.writeFileSync(path.join(out,name+'.log'),log);const killed=r.status!==0&&/AssertionError|ERR_ASSERTION|--- FAIL:/.test(log)&&!/build failed|SyntaxError|Cannot find module|operation not permitted/.test(log);results.push({name,killed,exit:r.status});assert(killed,name+' survived or failed outside assertion');console.log(name+' KILLED')}
try{
 if(['p1','all'].includes(phase)){
 for(const [name,file,from,to,pattern] of [
 ['P1-global-complete','backend/gameplay.go','if !progress.RankedComplete {','if !progress.Complete {','TestR286RankedCompletesWithoutMayhem'],
 ['P1-first-empty-skip','backend/season_decode_retry.go','prior.Scans = min(2, prior.Scans+1)','prior.Scans = 2','TestR286EmptyNeedsTwoDistinctScans'],
 ['P1-rank-diagnostic-whitelist','backend/features.go','"rank_record_state":              {"known": true, "collecting": true, "paused": true, "unavailable": true},','','TestR286RankRecordDiagnosticHTTP']
 ]){
 const original=fs.readFileSync(path.join(root,file),'utf8');assert(original.includes(from),name+' anchor');const target=path.join(tmp,name+'.go');fs.writeFileSync(target,original.replace(from,to));const overlay=path.join(tmp,name+'.json');fs.writeFileSync(overlay,JSON.stringify({Replace:{[path.join(root,file)]:target}}));check(name,cp.spawnSync('go',['test','-overlay='+overlay,'./backend','-run','^'+pattern+'$','-count=1'],{cwd:root,encoding:'utf8',timeout:180000,maxBuffer:4e6}));
 }
 const original=fs.readFileSync(path.join(root,'backend/web/gameplay.js'),'utf8'),target=path.join(tmp,'paused.js');assert(original.includes('const paused = !recordKnown && seasonProgress?.paused === true;'));fs.writeFileSync(target,original.replace('const paused = !recordKnown && seasonProgress?.paused === true;','const paused = false;'));check('P1-ignore-paused',cp.spawnSync(process.execPath,['--test','backend/web/r286-season.test.cjs'],{cwd:root,env:{...process.env,R286_GAME_SOURCE:target},encoding:'utf8',timeout:60000,maxBuffer:4e6}));
 }
 if(['ui','all'].includes(phase)){
 for(const [name,file,from,to,env,only] of [
 ['P2-unfixed-header','backend/web/suite.css','min-height: 38px; align-items: center; margin-bottom: 10px;','margin-bottom: 10px;','R284_SUITE_CSS_SOURCE','p2'],
 ['P3-muted','backend/web/gameplay.css','.service-outage-compact>span{color:var(--primary);font-weight:700}', '.service-outage-compact>span{color:var(--muted);font-weight:700}','R284_GAMEPLAY_CSS_SOURCE','p3'],
 ['P4-forced-wrap','backend/web/gameplay.css','.live-toolbar {display:grid;grid-template-columns:minmax(280px,1fr) minmax(0,1fr) auto;align-items:center;}', '.live-toolbar {display:flex;flex-wrap:wrap;align-items:center;}\n.live-toolbar [data-live-status]:has(.is-service-outage) {order:1;flex:1 0 100%;}\n.live-toolbar .live-refresh-status.is-service-outage {align-items:flex-start;justify-content:flex-start;white-space:normal;}','R284_GAMEPLAY_CSS_SOURCE','p4']
 ]){
  const original=fs.readFileSync(path.join(root,file),'utf8'),target=path.join(tmp,name+'.css');assert(original.includes(from),name+' anchor');fs.writeFileSync(target,original.replace(from,to));check(name,cp.spawnSync(process.execPath,['scripts/r286-ui-browser.cjs'],{cwd:root,env:{...process.env,[env]:target,R286_ONLY:only,R280_BROWSER_OUT:path.join(out,name)},encoding:'utf8',timeout:120000,maxBuffer:4e6}));
 }
 const original=JSON.parse(fs.readFileSync(path.join(root,'backend/testdata/r285-game-log-signatures.json'),'utf8')),target=path.join(tmp,'fixtures.json');fs.writeFileSync(target,JSON.stringify(original.filter(x=>x.directory!=='2026-10-18T22-58-32')));check('P6-missing-taskkill',cp.spawnSync('go',['test','./backend','-run','^TestR286RealExitAndTaskkillFixtures$','-count=1'],{cwd:root,env:{...process.env,R286_RECONNECT_FIXTURES:target},encoding:'utf8',timeout:180000,maxBuffer:4e6}));
 }
 if(['p5','all'].includes(phase)){
 for(const [name,file,from,to,pattern] of [
 ['P5-unchanged-included','backend/arena_fame_progression.go','ok && prior != value','ok','TestR286ProgressionBeforeAfterReadOnlyResolvedFromFullHelp'],
 ['P5-unsplit','backend/arena_fame_discovery.go','func arenaFameDiscoveryParts(event map[string]any) []map[string]any {','func arenaFameDiscoveryParts(event map[string]any) []map[string]any { return []map[string]any{event};','TestR286ProgressionPartsReconstructWithin4KB'],
 ['P5-guessed-get','backend/arena_fame_progression.go','if op.Method != http.MethodGet || op.Path == "" {','if op.Method != http.MethodGet && op.Method != "" || op.Path == "" {','TestR286ProgressionDoesNotGuessMissingHTTPMethodsOrQueryArguments']
 ]){
 const original=fs.readFileSync(path.join(root,file),'utf8');assert(original.includes(from),name+' anchor');const target=path.join(tmp,name+'.go');fs.writeFileSync(target,original.replace(from,to));const overlay=path.join(tmp,name+'.json');fs.writeFileSync(overlay,JSON.stringify({Replace:{[path.join(root,file)]:target}}));check(name,cp.spawnSync('go',['test','-overlay='+overlay,'./backend','-run','^'+pattern+'$','-count=1'],{cwd:root,encoding:'utf8',timeout:180000,maxBuffer:4e6}));
 }
 }
}finally{fs.writeFileSync(path.join(out,'results.json'),JSON.stringify(results,null,2)+'\n');fs.rmSync(tmp,{recursive:true,force:true})}
