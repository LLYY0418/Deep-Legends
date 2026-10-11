'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{compile,escapeHTML}=require('./r188-harness.cjs');
const source=fs.readFileSync(process.env.R272_GAMEPLAY_SOURCE || __dirname+'/gameplay.js','utf8');
const api=compile(source,['scoreBadgeChip','scorePlacementChip','scoreRankChip','ordinalLabel','renderMatchTags','fitMatchTags'],{state:{matchTimelines:new Map()},escapeHTML,number:String,matchDataTags:()=>Array.from({length:12},(_,i)=>({kind:'damage',priority:20000-i,label:'data'+i,description:'synthetic'})),renderMultiKillTag:()=>'<b>五杀</b>',matchTimelineKey:()=>'',matchKeywordMetadata:()=>null,bindMatchTagsPopover:()=>({anchor:null,close(){}})});
function fixture(record,result='win'){const subject={participantId:1,deaths:1,pentaKills:1};return new JSDOM(api.renderMatchTags({result,gameId:1,participants:[subject]},subject,record,{}));}
test('score rank or badge is first and exclusive for every scored mode',()=>{
 for(const rank of [1,2,5,10]) {const d=fixture({rank,total:16,score:7});try{const first=d.window.document.querySelector('[data-match-tag]');assert(first);assert.equal(first.textContent,({1:'1st',2:'2nd',5:'5th',10:'10th'})[rank]);assert(first.hasAttribute('data-match-tag-pinned'));assert.match(first.innerHTML,/本场评分排名：全场 16 人中的第/);if(rank<=3)assert(first.querySelector('.is-rank-'+rank));}finally{d.window.close();}}
 for(const badge of ['MVP','SVP']) {const d=fixture({badge,rank:2,total:10,score:8});try{assert.equal(d.window.document.querySelector('[data-match-tag]').textContent,badge);assert.equal(d.window.document.querySelector('.match-rank-chip'),null);}finally{d.window.close();}}
});
test('remake, absent score, absent rank or total produce no score placement',()=>{
 for(const [record,result] of [[{rank:2,total:10},'remake'],[null,'win'],[{total:10},'win'],[{rank:2},'win']]) {const d=fixture(record,result);try{assert.equal(d.window.document.querySelector('.match-rank-chip,.match-badge-chip'),null);}finally{d.window.close();}}
});
test('crowded narrow row preserves the score label before multikill and all data tags',()=>{
 const d=fixture({rank:2,total:10,score:7}),doc=d.window.document,row=doc.querySelector('.match-tags-collection');try{Object.defineProperty(row,'clientWidth',{value:35});for(const n of row.querySelectorAll('[data-match-tag]'))n.getBoundingClientRect=()=>({width:55});api.fitMatchTags(doc);assert.equal(row.querySelector('[data-match-tag]').textContent,'2nd');assert.equal(row.querySelector('[data-match-tag]').hidden,false);assert.equal([...row.querySelectorAll('[data-match-tag]')].filter(n=>!n.hidden).length,1);assert.equal(row.querySelector('[data-match-tags-more]').hidden,false);}finally{d.window.close();}
});
