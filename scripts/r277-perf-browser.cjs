'use strict';
const fs=require('node:fs'),path=require('node:path'),Module=require('node:module');
process.env.R272_CHECKS='1';process.env.R266_CASE_ONE='1';process.env.R272_SCENE_ONLY='1';
let source=fs.readFileSync(path.join(__dirname,'r266-browser.cjs'),'utf8');
function replace(a,b){if(source.split(a).length!==2)throw Error('Unexpected perf browser anchor: '+a);source=source.replace(a,b);}
replace("require('./r272-browser-checks.cjs')({evaluate,until,click,call,shot,ready,results,errors})","require('./r277-perf-browser-checks.cjs')({evaluate,until,click,call,shot,ready,results,errors})");
replace('reconcileFilteredMatchList,renderLiveInsights,fitMatchTags}',"reconcileFilteredMatchList,renderLiveInsights,fitMatchTags,renderOverviewBody,loadNextMatchWindow:typeof loadNextMatchWindow===\"function\"?loadNextMatchWindow:null,appendOverviewMatches:typeof appendOverviewMatches===\"function\"?appendOverviewMatches:null}");
replace("body=(await response.text()).replace('  const fixtures = new Map(['","body=(await response.text()).replaceAll('Date.now()', '1728000000000').replace('  const fixtures = new Map(['");
new (require("node:vm").Script)(source,{filename:__filename});
if(process.env.R277_PERF_COMPILE_ONLY==='1'){console.log('transformed perf harness syntax PASS');return;}
const m=new Module(__filename,module);m.filename=__filename;m.paths=module.paths;m._compile(source,__filename);
