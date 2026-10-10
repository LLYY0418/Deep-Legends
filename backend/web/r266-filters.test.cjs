const {install:installDialog,q,qa}=require('./filter-dialog-fixture.cjs');
'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const af=require(process.env.R266_FILTER_SOURCE || './history-filters.js');
function fixture(){
 const dom=new JSDOM('<main><div id="filters"></div><div class="match-list"><article>unchanged</article></div></main>',{url:'http://fixture'}),root=dom.window.document.querySelector('main');installDialog(dom);
 const rows=Array.from({length:1000},(_,i)=>({gameId:i,result:i%2?'loss':'win',createdAt:Date.now()-i*1000,participants:[{championId:i%5+1,championName:['阿狸','劫','亚索','瑟提','卡莎'][i%5]}]}));
 const tab={advancedConditions:{},data:{matches:rows,pagination:{hasMore:false}}};let changes=0,computes=0;
 const ctx={rows:()=>tab.data.matches,version:()=>tab.data.matches,subject:m=>m.participants[0],tags:()=>[],cursor:()=>tab.data.matches.length,storage:dom.window.localStorage,icon:(id,name)=>`<figure><img src="/fake/${id}" alt="${name}"></figure>`,container:()=>root,optionComputed:()=>computes++,change:()=>{changes++;q(root,'.match-list').replaceChildren(dom.window.document.createElement('article'));},prepare(){},error(){}};
 q(root,'#filters').innerHTML=af.render(tab,()=>ctx);af.bind(root,tab,()=>ctx);
 return {dom,root,tab,ctx,get changes(){return changes},get computes(){return computes}};
}
test('R266 P1 draft changes stay inside modal; close applies once and restores focus',async()=>{
 const h=fixture(),{root,dom,tab}=h;let mutations=0;new dom.window.MutationObserver(records=>mutations+=records.length).observe(q(root,'.match-list'),{childList:true,subtree:true});
 q(root,'[data-af-open]').click();assert.equal(q(root,'[data-af-menu]').hidden,false);assert.deepEqual([...qa(root,'[role=tab]')].map(n=>n.textContent),['条件','常用']);
 q(root,'[data-af-category="hero"]').click();const first=q(root,'[data-af-option="1"]'),img=first.querySelector('img');
 for(let i=1;i<=5;i++)q(root,`[data-af-option="${i}"]`).click();await Promise.resolve();assert.equal(mutations,0);assert.equal(h.changes,0);assert.deepEqual(tab.advancedConditions,{});assert.equal(q(root,'[data-af-option="1"]'),first);assert.equal(first.querySelector('img'),img);assert.match(q(root,'[data-af-summary="hero"]').textContent,/阿狸、劫/);
 q(root,'[data-af-not="true"]').click();assert.match(q(root,'[data-af-summary="hero"]').textContent,/不是 · 阿狸/);
 q(root,'[data-af-close]').click();await Promise.resolve();assert.equal(h.changes,1);assert.equal(mutations,1);assert.equal(tab.advancedConditions.hero.values.length,5);assert.equal(dom.window.document.activeElement,q(root,'[data-af-open]'));dom.window.close();
});
test('R266 P1 background batches keep option nodes, images and order; options compute once per version',()=>{
 const h=fixture(),{root,tab,ctx}=h;q(root,'[data-af-open]').click();q(root,'[data-af-category="hero"]').click();const nodes=[...qa(root,'[data-af-option]')],imgs=nodes.map(n=>n.querySelector('img')),before=h.computes;
 q(root,'[data-af-option="1"]').click();af.refresh(root,tab,ctx);assert.equal(h.computes,before);
 for(let batch=0;batch<3;batch++){tab.data.matches=[...tab.data.matches,...Array.from({length:20},(_,i)=>({...tab.data.matches[4],gameId:1000+batch*20+i}))];af.refresh(root,tab,ctx);const fresh=[...qa(root,'[data-af-option]')];assert.deepEqual(fresh,nodes);fresh.forEach((node,i)=>assert.equal(node.querySelector('img'),imgs[i]));}
 assert.equal(h.computes,before+3);assert.match(nodes[4].querySelector('small').textContent,/260 场/);h.dom.window.close();
});
test('R266 P1 saved legacy filters apply, rename, delete; saved count lives inside page',()=>{
 const h=fixture(),{root,ctx}=h;ctx.storage.setItem('deep-legends-history-presets-v1',JSON.stringify([{name:'旧版',conditions:{hero:{values:['1'],not:false}}}]));q(root,'[data-af-open]').click();q(root,'[data-af-page="saved"]').click();assert.match(q(root,'.af-saved-heading').textContent,/已保存 1 \/ 12/);q(root,'[data-af-rename="0"]').click();q(root,'[data-af-rename-input]').value='新名';q(root,'[data-af-rename-save="0"]').click();assert.equal(af.readPresets(ctx.storage)[0].name,'新名');q(root,'[data-af-apply="0"]').click();assert.equal(h.changes,1);assert.deepEqual(h.tab.advancedConditions.hero.values,['1']);q(root,'[data-af-open]').click();q(root,'[data-af-page="saved"]').click();q(root,'[data-af-delete="0"]').click();assert.equal(af.readPresets(ctx.storage).length,0);h.dom.window.close();
});
test('R266 P2 find appends three real batches through progress without rebuilding the open hero grid',async()=>{
 const h=fixture(),{root,tab,ctx}=h;tab.advancedConditions={hero:{values:['999'],not:false}};tab.data.pagination.hasMore=true;
 q(root,'[data-af-open]').click();q(root,'[data-af-category="hero"]').click();const nodes=[...qa(root,'[data-af-option]')],imgs=nodes.map(n=>n.querySelector('img'));let batches=0,redraws=0;
 ctx.isActive=()=>true;ctx.progress=()=>af.refresh(root,tab,ctx);ctx.render=()=>{redraws++;af.refresh(root,tab,ctx,true);};ctx.load=async count=>{assert.equal(count,20);tab.data.matches=[...tab.data.matches,...Array.from({length:20},(_,i)=>({...tab.data.matches[4],gameId:2000+batches*20+i}))];if(++batches===3)tab.data.pagination.hasMore=false;return true;};
 assert(await af.find(tab,ctx));assert.equal(batches,3);assert.equal(redraws,0);assert.deepEqual([...qa(root,'[data-af-option]')],nodes);nodes.forEach((node,i)=>assert.equal(node.querySelector('img'),imgs[i]));h.dom.window.close();
});
