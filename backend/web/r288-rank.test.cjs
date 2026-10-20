'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {compile,escapeHTML}=require('./r188-harness.cjs');
const source=fs.readFileSync(__dirname+'/gameplay.js','utf8');
test('R288 ranked whole-season header shows only the ranked upstream boundary',()=>{
 const f=compile(source,['renderRanks'],{escapeHTML,renderRankMMRPopover:()=>'',riotTab:()=>false,renderRankMilestones:()=>''});
 const render=progress=>f.renderRanks([],[],[],[],progress);
 const capped=render({upstreamCapped:true,rankedOldestAt:new Date('2026-04-26T12:00:00+08:00').getTime()});
 assert.match(capped,/统计的是当前整个赛季的排位数据/);assert.match(capped,/更早的场次暂时无法计入/);assert.match(capped,/<h3>排位<button/);assert.match(capped,/官方接口仅提供 4月26日 之后的对局/);assert.match(capped,/class="season-data-note"/);
 for(const progress of [{},{upstreamCapped:true,mayhemOldestAt:Date.now()},{upstreamCapped:false,rankedOldestAt:Date.now()}]){const html=render(progress);assert(html.includes('season-data-note'));assert(!html.includes('官方接口仅提供'));}
});
