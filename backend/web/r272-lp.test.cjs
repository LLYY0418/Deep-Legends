'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {compile}=require('./r188-harness.cjs');
const source=fs.readFileSync(process.env.R272_GAMEPLAY_SOURCE || __dirname+'/gameplay.js','utf8');
test('LP change replaces same-revision match while retaining unaffected card',()=>{
 const dom=new JSDOM('<div id="list"></div>'),list=dom.window.document.querySelector('#list');
 const tab={data:{matches:[{gameId:1,queueId:420,revision:'same'},{gameId:2,queueId:420,revision:'stable'}]},openMatches:new Set()};
 const deps={window:dom.window,document:dom.window.document,filteredMatches:rows=>rows,bindOverviewContent:()=>{},applyRenderedMetricStyles:()=>{},prepareImages:()=>{},advancedFilterContext:()=>({}),matchListEmptyContent:()=>'',recordItemSetClientDiagnostic:()=>{},renderMatch:m=>`<article class="match-entry" data-match-id="${m.gameId}"><span class="result-word">${m.lpDelta==null?'':`<b class="lp-delta">−${Math.abs(m.lpDelta)} LP</b>`}</span></article>`};
 const fn=compile(source,['reconcileFilteredMatchList'],deps).reconcileFilteredMatchList;
 fn(list,tab);const unaffected=list.children[1];
 tab.data.matches=[{gameId:1,queueId:420,revision:'same',lpDelta:-22},tab.data.matches[1]];
 fn(list,tab);assert.equal(list.querySelector('.lp-delta')?.textContent,'−22 LP');assert.equal(list.children[1],unaffected);dom.window.close();
});
