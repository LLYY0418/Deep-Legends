'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {compile,escapeHTML}=require('./r188-harness.cjs');
const source=fs.readFileSync(__dirname+'/gameplay.js','utf8');
test('R288 ranked whole-season header shows only the ranked upstream boundary',()=>{
 const f=compile(source,['renderRanks'],{escapeHTML,renderRankMMRPopover:()=>'',riotTab:()=>false,renderRankMilestones:()=>''});
 const render=progress=>f.renderRanks([],[],[],[],progress);
 const capped=render({upstreamCapped:true,rankedOldestAt:new Date('2026-04-26T12:00:00+08:00').getTime()});
 assert.match(capped,/当前整个赛季/);assert.match(capped,/官方接口仅提供 4月26日 之后的对局/);assert.match(capped,/class="season-data-note"/);
 for(const progress of [{},{upstreamCapped:true,mayhemOldestAt:Date.now()},{upstreamCapped:false,rankedOldestAt:Date.now()}])assert(!render(progress).includes('season-data-note'));
});
