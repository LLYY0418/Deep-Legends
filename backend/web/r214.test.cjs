'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {read,compile}=require('./r188-harness.cjs');
const {fixture}=require('./r190-harness.cjs');
const source=process.env.R214_GAMEPLAY_SOURCE?fs.readFileSync(process.env.R214_GAMEPLAY_SOURCE,'utf8'):read('gameplay.js');

test('R214 selected player descriptions load once and redraw only the intersecting build',async()=>{
 const dom=new JSDOM('<section></section>'),container=dom.window.document.querySelector('section'),requests=[],redraws=[];
 let finishOther;
 const match={gameId:214,subjectParticipantId:1,participants:[{participantId:1,teamId:100,championId:1,augmentIds:[1,2]},{participantId:2,teamId:200,championId:2,augmentIds:[3,4]}]};
 const unrelated={gameId:215,subjectParticipantId:1,participants:[{participantId:1,augmentIds:[9]}]};
 const tab={key:'self',data:{matches:[match,unrelated]},openMatches:new Set(['214','215']),matchDetailTabs:new Map([['214','build'],['215','build']])};
 const redraw=(_tab,id)=>{redraws.push(id);assert.equal(id,'214');container.innerHTML=f.renderBuild(match,match.participants[0],tab);controls.bindMatchDetailControls(container,tab);};
 const f=fixture(source,{riotTab:()=>false,connected:()=>false,rerenderMatch:redraw,api:async url=>{
  const ids=new URL(url,'http://test').searchParams.get('ids');requests.push(ids);
  if(ids==='3,4')await new Promise(resolve=>{finishOther=resolve;});
  return{items:ids.split(',').map(id=>({id:Number(id),status:'ok',description:`说明文字 ${id}`}))};
 }});
 f.state.matchTimelines.set("fixture",{available:true,participants:match.participants.map(p=>({participantId:p.participantId,itemGroups:[],skillOrder:[]}))});
 f.state.tabs=[tab];f.state.augmentDescriptions.set(9,{status:'ok',description:'无关'});
 const controls=compile(source,['bindMatchDetailControls'],{...f.deps,bindRuneEffectLinks:f.bindRuneEffectLinks,ensureBuildData:f.ensureBuildData,overviewContainer:()=>container,rerenderMatch:redraw});
 container.innerHTML=f.renderBuild(match,match.participants[0],tab);controls.bindMatchDetailControls(container,tab);
 await new Promise(setImmediate);
 assert.deepEqual(requests,['1,2']);assert.match(container.textContent,/说明文字 1/);assert.equal(container.querySelectorAll('.skel').length,0);
 container.querySelector('[data-build-player="2"]').click();
 assert.deepEqual(requests,['1,2','3,4']);assert.ok(container.querySelector('.skel'));
 // Repaint while loading cannot issue another request.
 redraw(tab,'214');assert.deepEqual(requests,['1,2','3,4']);
 finishOther();await new Promise(setImmediate);
 assert.match(container.textContent,/说明文字 3/);assert.match(container.textContent,/说明文字 4/);assert.equal(container.querySelectorAll('.skel').length,0);
 assert.ok(redraws.length>=3);assert.ok(redraws.every(id=>id==='214'));
 container.querySelector('[data-build-player="1"]').click();await new Promise(setImmediate);
 assert.match(container.textContent,/说明文字 1/);assert.deepEqual(requests,['1,2','3,4']);
 dom.window.close();
});

test('R214 score tooltip uses safe DOM rows and actual normalized widths without methodology',()=>{
 const dom=new JSDOM('<main></main>',{pretendToBeVisual:true}),w=dom.window;
 w.matchMedia=()=>({matches:false});
 const scores=compile(source,['scoreChip']);
 const record={score:5.8,rank:2,total:2,parts:[{key:'kda',label:'KDA',value:3,norm:.5},{key:'kp',label:'参团',value:1,norm:.5},{key:'damage',label:'伤害',value:10000,norm:.4},{key:'gold',label:'经济',value:9000,norm:.5},{key:'cs',label:'分均补刀',value:5,norm:.5}]};
 const main=w.document.querySelector('main');main.innerHTML=scores.scoreChip(record);
 const setup=compile(read('app.js'),['setupFloatingTooltips'],{document:w.document,window:w,Element:w.Element,Node:w.Node,requestAnimationFrame:()=>1,cancelAnimationFrame:()=>{}});
 setup.setupFloatingTooltips();main.firstElementChild.dispatchEvent(new w.Event('pointerover',{bubbles:true}));
 const tooltip=w.document.querySelector('#global-tooltip');
 assert.equal(tooltip.hidden,false);assert.match(tooltip.textContent,/本局评分 · 全场第 2/);assert.doesNotMatch(tooltip.textContent,/公式|基准|中位数|非官方|贡献=|v2|权重/);
 const rows=[...tooltip.querySelectorAll('.tooltip-score-row')];assert.equal(rows.length,record.parts.length);assert.equal(rows.length,5);
 rows.forEach((row,i)=>{assert.equal(row.firstChild.textContent,record.parts[i].label);assert.equal(row.querySelector('i').style.width,`${Math.round(record.parts[i].norm*10000)/100}%`);});
 assert.equal(rows[0].querySelector('i').style.width,'50%');assert.ok(parseFloat(rows[2].querySelector('i').style.width)<50);
 // Hostile strings stay text; malformed numeric data gets the plain fallback.
 main.firstElementChild.dataset.tooltipScore=JSON.stringify({value:'6.0',parts:[{label:'<img src=x>',value:'<script>x</script>',norm:.5}]});
 main.firstElementChild.dispatchEvent(new w.Event('pointerout',{bubbles:true}));main.firstElementChild.dispatchEvent(new w.Event('pointerover',{bubbles:true}));
 assert.equal(tooltip.querySelector('img,script'),null);assert.match(tooltip.textContent,/<img src=x>/);
 main.firstElementChild.dataset.tooltipScore=JSON.stringify({value:'6.0',parts:[{label:'错误',value:'1',norm:4}]});
 main.firstElementChild.dispatchEvent(new w.Event('pointerout',{bubbles:true}));main.firstElementChild.dispatchEvent(new w.Event('pointerover',{bubbles:true}));
 assert.equal(tooltip.querySelectorAll('.tooltip-score-row').length,0);
 dom.window.close();
});
