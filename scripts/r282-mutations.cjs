'use strict';
const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),cp=require('node:child_process'),assert=require('node:assert/strict');
const root=path.resolve(__dirname,'..'),out=process.env.R282_MUTATION_OUT||path.join(os.tmpdir(),'r282-mutations'),temp=fs.mkdtempSync(path.join(os.tmpdir(),'r282-mutants-'));fs.mkdirSync(out,{recursive:true});const results=[];
const mutations=[
 ['P19','backend/web/overview-casual.js','R282_CASUAL_SOURCE','P19','.some(m => ranked(m.queueId))','.some(() => false)'],
 ['P20','backend/web/overview-art.js','R282_ART_SOURCE','P20','const animation = player.backgroundVideoPath ?','const animation = player.backgroundVideoPath && !reduced?.matches ?'],
 ['P1','backend/web/gameplay.js','R282_GAMEPLAY_SOURCE','P1','<linearGradient','<g'],
 ['P1-buttons','backend/web/index.html','R282_HTML_SOURCE','P16','id="skin-dialog-close" class="dialog-action control-icon-button"','id="skin-dialog-close" class="dialog-action dialog-close-button control-icon-button"'],
 ['P2','backend/web/app.js','R282_APP_SOURCE','P2','scale >= 1.5','scale >= 1.6'],
 ['P5','backend/web/gameplay.js','R282_GAMEPLAY_SOURCE','P5 P6','class="summoner-tag','class="broken-tag'],
 ['P6','backend/web/gameplay.css','R282_GAMEPLAY_CSS_SOURCE','P5 P6',null,'\n[data-career-key="ranks"]{height:444px}'],
 ['P7','backend/web/gameplay.js','R282_GAMEPLAY_SOURCE','P7','heights[0]<=heights[1]','heights[0]>=heights[1]'],
 ['P8','backend/web/gameplay.js','R282_GAMEPLAY_SOURCE','P8','    tab.data = null;','    /* stale data retained */'],
 ['P9','backend/web/app.js','R282_APP_SOURCE','P9','state.poolRenderInterrupted || state.poolRenderPending) renderPoolCatalog()','false) renderPoolCatalog()'],
 ['P10','backend/web/gameplay.js','R282_GAMEPLAY_SOURCE','P10','signature,next:0,hasMore:true','signature,next:0,hasMore:false'],
 ['P11','backend/web/app.css','R282_APP_CSS_SOURCE','P11 P12 P13','[data-theme="light"] .facade-grid .skin-copy strong','[data-theme="light"] .broken-grid .skin-copy strong'],
 ['P13','backend/web/app.js','R282_APP_SOURCE','P11 P12 P13','busy ? "正在获取" : "重新读取"','busy ? "重新读取" : "重新读取"'],
 ['P14','desktop/desktop-log.cjs','R282_DESKTOP_LOG_SOURCE','P14',"['name','serviceName']","['name']"],
 ['P17','backend/web/app.css','R282_APP_CSS_SOURCE','P17','repeat(auto-fill,minmax(min(100%,340px),1fr))','repeat(auto-fit,minmax(min(100%,340px),1fr))'],
 ['P16-mode','backend/web/gameplay.js','R282_GAMEPLAY_SOURCE','P16','Boolean(arenaMode &&','Boolean(true &&'],
 ['P16-transition','backend/web/gameplay.js','R282_GAMEPLAY_SOURCE','P16','player.bravery=true','player.bravery=false'],
];
const go=[
 ['P3','backend/riot_auth_confirmation.go','TestR282Request403DoesNotRejectHealthyEmbeddedKey','"/lol/status/v4/platform-data"','"/lol/status/v4/platform-data-broken"'],
 ['P12','backend/pro_runes.go','TestR282WorldsOutsideRosterAndCrossLaneRows','return proNumericID(e.Match.Teams[0].ID) && proNumericID(e.Match.Teams[1].ID)','return (proRuneTeams[e.Match.Teams[0].ID] != "" || proRuneTeams[e.Match.Teams[1].ID] != "") && proNumericID(e.Match.Teams[0].ID) && proNumericID(e.Match.Teams[1].ID)'],
 ['P18-cache','backend/rank_insights.go','TestR282LiveRankOneLookupPerGameBeyondNormalCacheTTL','strings.ToUpper(serverID) + "|" + playerRef','strings.ToUpper(serverID) + ":" + playerRef'],
 ['P18','backend/gameplay.go','TestR282LiveRankSoloFirstAndFlexFallback','[]*gameplayRank{solo, flex}','[]*gameplayRank{flex, solo}'],
 ['P15-privacy','backend/arena_fame_discovery.go','TestR282ArenaDiscoveryReadOnlyOnceAndPrivate','arenaDiscoveryPrivate.MatchString(k)','false'],
 ['P15-once','backend/arena_fame_discovery.go','TestR282ArenaDiscoveryReadOnlyOnceAndPrivate','if s.started {','if false {'],
];
function source(file,from,to){const original=fs.readFileSync(path.join(root,file),'utf8');if(from===null)return original+to;assert(original.includes(from),'mutation target missing '+file+' '+from);return original.replace(from,to)}
function record(id,args,options){const run=cp.spawnSync(args[0],args.slice(1),{cwd:root,encoding:'utf8',maxBuffer:2*1024*1024,...options}),log=(run.stdout||'')+(run.stderr||'');fs.writeFileSync(path.join(out,id+'.log'),log);const killed=run.status!==0 && /AssertionError|ERR_ASSERTION|--- FAIL:|✖/.test(log) && !/\[build failed\]|ReferenceError:|SyntaxError:|is not defined/.test(log);results.push({id,killed,status:run.status,command:args[0]+' '+args.slice(1).join(' '),log:id+'.log'});assert(killed,'mutant not killed by assertion: '+id+'\n'+log.slice(-1500));}
try{
 for(const [id,file,env,pattern,from,to] of mutations){const target=path.join(temp,id+path.extname(file));fs.writeFileSync(target,source(file,from,to));record(id,[process.execPath,'--test','--test-name-pattern=R282 '+pattern,file.startsWith('desktop/')?'desktop/r282-network.test.cjs':'backend/web/r282.test.cjs'],{env:{...process.env,[env]:target}})}
 for(const [id,file,pattern,from,to] of go){const target=path.join(temp,id+'.go');fs.writeFileSync(target,source(file,from,to));const overlay=path.join(temp,id+'.json');fs.writeFileSync(overlay,JSON.stringify({Replace:{[path.join(root,file)]:target}}));record(id,['go','test','./backend','-run','^'+pattern+'$','-count=1','-overlay='+overlay],{env:{...process.env,GOCACHE:process.env.GOCACHE||path.join(os.tmpdir(),'r282-gocache')}})}
}finally{fs.writeFileSync(path.join(out,'results.json'),JSON.stringify(results,null,2));fs.rmSync(temp,{recursive:true,force:true})}
console.log(JSON.stringify({killed:results.filter(r=>r.killed).length,total:results.length}));
