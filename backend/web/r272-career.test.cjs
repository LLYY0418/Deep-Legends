'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{compile}=require('./r188-harness.cjs');
const recovery=require(process.env.R272_RECOVERY_SOURCE || './history-recovery.js');
const source=fs.readFileSync(process.env.R272_GAMEPLAY_SOURCE || __dirname+'/gameplay.js','utf8');
test('three recovery rounds keep exactly one keyed activity section and share button',()=>{
 const dom=new JSDOM('<main><aside class="career-column"><section data-career-key="activity"><button data-share>share</button></section><section data-career-key="players">before</section></aside></main>'),root=dom.window.document.querySelector('main'),share=root.querySelector('[data-share]');
 for(let i=0;i<3;i++){recovery.patch(root,[['players',`<section data-career-key="players">round ${i}</section>`],['activity','<section data-career-key="activity"><button data-share>share</button></section>']]);assert.equal(root.querySelectorAll('[data-share]').length,1);assert.equal(root.querySelector('[data-share]'),share,'keyed activity/share node must be retained');assert.equal(root.querySelector('.career-column').children.length,2);assert.equal(root.querySelectorAll('[data-career-key="activity"]').length,1);}
 dom.window.close();
});
test('async career data updates its card while retaining scrolled match list',()=>{
 const dom=new JSDOM('<main><aside class="career-column"><section data-career-key="champions">pending</section></aside><div class="match-list"><article>match</article></div></main>'),root=dom.window.document.querySelector('main'),list=root.querySelector('.match-list'),entry=list.firstChild;let fullRenders=0;const noop=()=>{};
 const deps={document:dom.window.document,overviewContainer:()=>root,careerSectionEntries:()=>[['champions','<section data-career-key="champions">ready</section>']],rerenderTab:()=>{fullRenders++;root.innerHTML='reset';},bindOverviewContent:noop,bindPlayerLinks:noop,bindRankHistoryControls:noop,bindRankedQueueControls:noop,bindMayhemRatingControls:noop,bindOverviewShareControls:noop,applyRenderedMetricStyles:noop,prepareImages:noop};
 const fn=compile(source,['updateCareerCards'],deps).updateCareerCards;fn({data:{}},'opgg_season');assert(root.querySelector('[data-career-key="champions"]'),'async update must keep career card');assert.equal(root.querySelector('[data-career-key="champions"]').textContent,'ready');assert.equal(root.querySelector('.match-list'),list);assert.equal(list.firstChild,entry);assert.equal(fullRenders,0);dom.window.close();
});

test('expired reference retries the same append cursor once using remembered Riot ID',async()=>{
 const calls=[],events=[],fn=compile(source,['requestOverviewWithReferenceRetry'],{window:{reportFlowDiagnostic:(...args)=>events.push(args)},state:{lastRiotKeyChangeAt:Date.now()},tabGroup:()=> 'pro'}).requestOverviewWithReferenceRetry;
 const tab={region:'kr',data:{player:{gameName:'Anonymous',tagLine:'TEST'}}},body=JSON.stringify({playerRef:'expired-fixture',begIndex:40,count:20});let result;
 await assert.doesNotReject(async()=>{result=await fn(tab,body,async input=>{calls.push(JSON.parse(input));if(calls.length===1)throw Object.assign(new Error('玩家引用已失效'),{status:404});return {matches:[{gameId:1}]};});});
 assert.equal(calls.length,2);assert.equal(calls[1].begIndex,40);assert.equal(calls[1].playerRef,undefined);assert.equal(calls[1].gameName,'Anonymous');assert.equal(result.matches.length,1);assert.equal(events.length,1);
 let repeated=0;await assert.rejects(fn(tab,body,async()=>{repeated++;throw Object.assign(new Error('玩家引用已失效'),{status:404});}));assert.equal(repeated,2,'one retry only');
});
