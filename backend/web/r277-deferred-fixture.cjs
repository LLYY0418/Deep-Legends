'use strict';
// Legacy function-extraction fixtures test rich markup synchronously. Join the
// real moved bodies here, not in initial production assets; dedicated deferred
// tests separately execute the real asynchronous loader and recovery path.
const fs=require('node:fs'),path=require('node:path');
const rich=fs.readFileSync(path.join(__dirname,'match-details.js'),'utf8');
const history=fs.readFileSync(path.join(__dirname,'history-filters.js'),'utf8'),live=fs.readFileSync(path.join(__dirname,'live-arena.js'),'utf8');
const historyNames=['ensureHistoryFilterSearch','advancedHistoryState','advancedHistoryHasMore','loadAdvancedHistory','advancedFilterContext'],liveNames=['renderLivePlayer','renderLiveInsights','liveConfidenceText','liveEvidenceSuffix','renderLiveARAMKitCombos','renderLiveRecentPositions'];
const names=['recordBuildSpellShape','matchTableRows','matchTableShell','renderMatchOverview','renderArenaMatchOverview','isARAMRelatedMatch','teamAnalysisMetricsFor','teamAnalysisMembers','teamAnalysisMetricValue','teamAnalysisMetricLabel','renderTeamAnalysis','renderItemRoute','skillPrioritySummary','renderSkillOrder','perkEffectLines','selectedRuneEffects','runeEffectTotals','renderRuneYield','renderRuneEffects'];
const cache=new Map();
function join(source){
 if(cache.has(source))return cache.get(source);const original=source;
 const {extract}=require('./r188-harness.cjs');
 for(const [text,group,label]of [[history,historyNames,'history'],[live,liveNames,'live']])for(const name of group){if(source.includes(`R309 deferred ${label} renderer: ${name}`)){const old=extract(source.replaceAll('R309 deferred','R309 bridge'),name).replaceAll('R309 bridge','R309 deferred');source=source.replace(old,extract(text,name));}}
 for(const name of names){const body=extract(rich,name);if(source.includes(`function ${name}(`)){const old=extract(source.replace('R277 deferred detail renderer: '+name,'R277 bridge'),name).replace('R277 bridge','R277 deferred detail renderer: '+name);if(old.includes('R277 deferred detail renderer:'))source=source.replace(old,body);}else if(/\}\)\(\);\s*$/.test(source))source=source.replace(/\}\)\(\);\s*$/,body+'\n})();');else source+='\n'+body;}
 if(cache.size>=8)cache.delete(cache.keys().next().value);cache.set(original,source);return source;
}
module.exports={join};
