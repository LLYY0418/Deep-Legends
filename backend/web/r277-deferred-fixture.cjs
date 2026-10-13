'use strict';
// Legacy function-extraction fixtures test rich markup synchronously. Join the
// real moved bodies here, not in initial production assets; dedicated deferred
// tests separately execute the real asynchronous loader and recovery path.
const fs=require('node:fs'),path=require('node:path');
const rich=fs.readFileSync(path.join(__dirname,'match-details.js'),'utf8');
const names=['matchTableRows','matchTableShell','renderMatchOverview','renderArenaMatchOverview','isARAMRelatedMatch','teamAnalysisMetricsFor','teamAnalysisMembers','teamAnalysisMetricValue','teamAnalysisMetricLabel','renderTeamAnalysis','renderItemRoute','skillPrioritySummary','renderSkillOrder','perkEffectLines','selectedRuneEffects','runeEffectTotals','renderRuneYield','renderRuneEffects'];
function join(source){
 const {extract}=require('./r188-harness.cjs');
 for(const name of names){const body=extract(rich,name);if(source.includes(`function ${name}(`)){const old=extract(source.replace('R277 deferred detail renderer: '+name,'R277 bridge'),name).replace('R277 bridge','R277 deferred detail renderer: '+name);if(old.includes('R277 deferred detail renderer:'))source=source.replace(old,body);}else if(/\}\)\(\);\s*$/.test(source))source=source.replace(/\}\)\(\);\s*$/,body+'\n})();');else source+='\n'+body;}
 return source;
}
module.exports={join};
