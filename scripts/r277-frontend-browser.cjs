'use strict';
// Reuse the existing isolated Chromium/CSP/cleanup harness without changing it.
const fs=require('node:fs'),path=require('node:path'),Module=require('node:module');
process.env.R272_CHECKS='1';process.env.R266_CASE_ONE='1';process.env.R272_SCENE_ONLY='1';
let source=fs.readFileSync(path.join(__dirname,'r266-browser.cjs'),'utf8');
function replace(a,b){if(source.split(a).length!==2)throw Error('Unexpected browser harness anchor: '+a);source=source.replace(a,b);}
replace("require('./r272-browser-checks.cjs')({evaluate,until,click,call,shot,ready,results,errors})", "require('./r277-frontend-browser-checks.cjs')({evaluate,until,click,call,shot,ready,results,errors})");
replace("['/app.js','/gameplay.js','/history-filters.js','/gameplay.css'].includes(pathname)","['/app.js','/gameplay.js','/history-filters.js','/gameplay.css','/app.css','/history-filters.css','/history-recovery.js','/collection-new-items.js','/runtime.js','/favorites-facade.js'].includes(pathname)");
replace("'*/gameplay.css*','*/demo-data.js*'", "'*/gameplay.css*','*/app.css*','*/history-filters.css*','*/history-recovery.js*','*/collection-new-items.js*','*/runtime.js*','*/favorites-facade.js*','*/demo-data.js*'");
replace('reconcileFilteredMatchList,renderLiveInsights,fitMatchTags}', 'reconcileFilteredMatchList,renderLiveInsights,fitMatchTags,renderOverviewPendingPage,renderOverview,renderOverviewBody,updateClientView,renderHistoricalRanks,bindRankHistoryControls,appendOverviewMatches}');
replace('window.__r266App={state,renderItems};', 'window.__r266App={state,renderItems,acceptClientView,applySearchRegion,refreshStatus};');
replace("if(args[0]==='overview_card_ready')__r266CardEvents.push(args[2]);", "if(args[0]==='overview_card_ready')__r266CardEvents.push(args[2]);(window.__r277FlowEvents ||= []).push(args);");
replace('{error:e.message,currentCase,clickEvidence,results,errors,violations}', '{error:e.message,failures:e.failures||[{error:e.message}],topbar:e.topbarGeometry,currentCase,clickEvidence,results,errors,violations}');
replace("start().catch(e=>{fs.mkdirSync(out,{recursive:true});","start().catch(e=>{fs.mkdirSync(out,{recursive:true});if(e.code==='WAIT_STALLED'||e.code==='WAIT_HARD_LIMIT'||e.failures?.some(f=>/no new progress|total limit/.test(f.error)))fs.writeFileSync(path.join(out,'stalled-state.json'),JSON.stringify({error:e.message,failures:e.failures,topbar:e.topbarGeometry},null,2));");
const m=new Module(__filename,module);m.filename=__filename;m.paths=module.paths;m._compile(source,__filename);
