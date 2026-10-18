'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),cp=require('node:child_process'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..'),out=path.resolve(process.argv[2]),tmp=fs.mkdtempSync(path.join(os.tmpdir(),'r285-mutants-')),results=[];fs.mkdirSync(out,{recursive:true});
function check(name,r){const log=(r.stdout||'')+(r.stderr||'');fs.writeFileSync(path.join(out,name+'.log'),log);const killed=r.status!==0&&/AssertionError|ERR_ASSERTION|--- FAIL:/.test(log)&&!/build failed|SyntaxError|Cannot find module|operation not permitted/.test(log);results.push({name,killed,exit:r.status});assert(killed,name+' survived or failed outside assertion');console.log(name+' KILLED')}
try{
 for(const [name,file,from,to,pattern] of [
  ['P2-cancel-inflight','backend/watch_reconnect.go','postCtx, postCancel := context.WithTimeout(parent, 8*time.Second)','postCtx, postCancel := context.WithTimeout(ctx, 8*time.Second)','TestR285ReconnectHTTPAndPhaseConfirmation/sse-before-http'],
  ['P3-zero-minimum','backend/watch_reconnect.go','reconnectSettlementDelayMS = 5000','reconnectSettlementDelayMS = 0','TestR285ReconnectDelayChoicesAndSettlement'],
  ['P7-unsplit','backend/arena_fame_discovery.go','for _, part := range arenaFameDiscoveryParts(event) {\n\t\ta.recordDiagnostic(part)\n\t}','a.recordDiagnostic(event)','TestR285ArenaDiscoveryPartsRoundTrip']]){
   const original=fs.readFileSync(path.join(root,file),'utf8');assert(original.includes(from));const target=path.join(tmp,name+'.go');fs.writeFileSync(target,original.replace(from,to));const overlay=path.join(tmp,name+'.json');fs.writeFileSync(overlay,JSON.stringify({Replace:{[path.join(root,file)]:target}}));check(name,cp.spawnSync('go',['test','-overlay='+overlay,'./backend','-run','^'+pattern+'$','-count=1'],{cwd:root,encoding:'utf8',timeout:180000,maxBuffer:4e6}));
 }
 for(const [name,from,to,testFile] of [
  ['P4-no-description','const description = plainText(spell?.description || "");','const description = "";','backend/web/r285-ui.test.cjs'],
  ['P5-disconnect-subtree','if (!container?.querySelector("[data-match-sentinel]") && overviewContainer(tab) !== container) return;','// restored unconditional disconnect','backend/web/r285-autoload.test.cjs']]){
  const original=fs.readFileSync(path.join(root,'backend/web/gameplay.js'),'utf8');assert(original.includes(from));const target=path.join(tmp,name+'.js');fs.writeFileSync(target,original.replace(from,to));const env={...process.env,R285_GAMEPLAY_SOURCE:target};check(name,cp.spawnSync(process.execPath,['--test',testFile],{cwd:root,env,encoding:'utf8',timeout:60000,maxBuffer:4e6}));
  if(name.startsWith('P5'))check('P5-native',cp.spawnSync(process.execPath,['scripts/r285-autoload-browser.cjs'],{cwd:root,env:{...env,R280_BROWSER_OUT:path.join(out,'p5-native')},encoding:'utf8',timeout:120000,maxBuffer:4e6}));
 }
 const original=fs.readFileSync(path.join(root,'backend/web/gameplay.css'),'utf8'),target=path.join(tmp,'P6.css');fs.writeFileSync(target,original+'\n.live-teams.is-insight:not(.is-arena):has(.live-player.is-hidden) { grid-template-rows: none; }\n.live-teams.is-insight:not(.is-arena):has(.live-player.is-hidden) > .live-team { display: block; grid-row: auto; }\n.live-teams.is-insight:not(.is-arena):has(.live-player.is-hidden) .live-player-list.is-insight { display: grid; }\n');
 check('P6-native-hidden-override',cp.spawnSync(process.execPath,['scripts/r285-ui-browser.cjs'],{cwd:root,env:{...process.env,R284_GAMEPLAY_CSS_SOURCE:target,R280_BROWSER_OUT:path.join(out,'p6-native')},encoding:'utf8',timeout:120000,maxBuffer:4e6}));
}finally{fs.writeFileSync(path.join(out,'results.json'),JSON.stringify(results,null,2)+'\n');fs.rmSync(tmp,{recursive:true,force:true})}
