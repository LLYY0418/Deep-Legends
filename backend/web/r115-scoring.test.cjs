const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const source = fs.readFileSync(process.env.R115_SCORING_SOURCE || path.join(__dirname, 'gameplay.js'), 'utf8');
const between = (name, next) => source.slice(source.indexOf(`function ${name}(`), source.indexOf(`function ${next}(`));
// Algorithm assertions moved to backend/r216_score_test.go; UI tests consume
// response fixtures instead of maintaining a second scorer.
test('score tooltip renders backend parts with no methodology',()=>{
 const score={score:6.2,rank:2,total:10,parts:[{key:'kda',value:3,norm:.5},{key:'kp',value:.6,norm:.55}]};
 const chip=new Function(`${between('scoreChip','scorePlacementChip')};return scoreChip;`)();
 assert.match(chip(score),/本局评分 · 全场第/);
 assert.match(chip(score),/data-tooltip-score=/);
 assert.doesNotMatch(chip(score),/公式|基准|中位数|非官方|贡献=|v2/);
});
test('incomplete radar preserves valid points without inventing a closed polygon',()=>{
 const render=new Function('escapeHTML','number',`${between('radarPoint','renderRecentRanked')}; return {renderAbility,abilityMetricValue};`)(String,String);
 const metrics=['kda','kp','damage','dpm','cs','gold','vision'].map((key,i)=>({key,label:key,player:10,baseline:10,playerScore:50,grade:'B+',unavailable:i===6}));
 const markup=render.renderAbility({metrics,sampleGames:3,baselineGames:3});
 assert.equal((markup.match(/<circle /g)||[]).length,6);
 assert.doesNotMatch(markup,/<polygon class="ability-radar-(player|baseline)"/);
 assert.equal(render.abilityMetricValue(metrics[6],'player'),'—');
 assert.equal(render.abilityMetricValue({player:null},'player'),'—');
});
