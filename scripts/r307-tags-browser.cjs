'use strict';
const {browser}=require('./r297-browser-harness.cjs'),fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const out=process.env.R307_REPORT_DIR||'/private/tmp/r307-tags-browser',report={cases:[],restoration:[],errors:[]};
const html=`<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/app.css"><link rel="stylesheet" href="/player-tags.css"></head><body><main id="main-content"><div id="live-content"></div></main><script>
window.deepLegendsRuntime={escapeHTML:v=>String(v??'').replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('"','&quot;')};window.deepLegendsSections={register(){}};window.player={playerRef:'r307-fixture',gameName:'夹具玩家',taggable:true};window.record={tags:['a'],note:''};window.phase='ChampSelect';window.deepLegendsPlayerTagContext={sources:()=>[player],live:()=>({phase,players:[player]}),name:p=>p.gameName,info:()=>'',avatar:()=>'',revealLive(){}};window.fetch=async(_u,o={})=>{const b=JSON.parse(o.body||'{}');if(b.op==='update'){if(window.__hold)await new Promise(r=>window.__release=r);Object.assign(record,{tags:b.tags,note:b.note});return {ok:true,json:async()=>({saved:true})};}return{ok:true,json:async()=>({version:1,catalog:[{id:'a',name:'原标签',group:'mine'},{id:'b',name:'新增标签',group:'pos'}],records:{'r307-fixture':structuredClone(record)}})};};</script><script src="/player-tags.js"></script></body></html>`;
(async()=>{const b=await browser({out,transform:(name,data)=>name==='index.html'?Buffer.from(html):name==='player-tags.js'&&process.env.R307_TAG_SOURCE?fs.readFileSync(process.env.R307_TAG_SOURCE):data});const settle=()=>b.evaluate('new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r)))');try{
 await b.call('Page.navigate',{url:b.origin+'/'});await b.until('window.deepLegendsPlayerTags');
 for(const scene of ['ranked-select','arena-select','in-game','detail']){
 await b.evaluate(`phase='${scene==='in-game'?'InProgress':'ChampSelect'}';document.querySelector('#live-content').innerHTML=${JSON.stringify('<div class="live-teams"><article class="live-player"><div class="live-player-identity"><button data-player-ref="r307-fixture">玩家</button></div></article></div>')};${scene==='detail'?`document.querySelector('#live-content').innerHTML='<div class="match-detail"><div class="participant-identity"><button class="participant-link" data-player-ref="r307-fixture">玩家</button></div></div>';`:''}deepLegendsPlayerTags.refresh();`);await settle();
 for(const wait of [false,true])for(const close of ['x','done','esc','mask'])for(const entry of ['button','chips']){
 const id=[scene,wait,close,entry].join('-');await b.evaluate(`document.querySelector('[data-ptag-open]').click()`);await b.until('document.querySelector(".ptag-modal")');
 await b.evaluate(`(()=>{document.querySelector('[data-ptag-toggle="b"]').click();const input=document.querySelector('.ptag-modal input');input.value=${JSON.stringify(id)};input.dispatchEvent(new Event('input',{bubbles:true}));})()`);if(wait)await b.evaluate('deepLegendsPlayerTags.flush()');
 await b.evaluate(close==='x'?`document.querySelector('header [data-ptag-close]').click()`:close==='done'?`document.querySelector('footer [data-ptag-close]').click()`:close==='esc'?`document.querySelector('.ptag-modal').dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}))`:`document.querySelector('.ptag-scrim').dispatchEvent(new MouseEvent('mousedown',{bubbles:true}))`);
 assert(await b.evaluate('!document.querySelector(".ptag-modal")'),'close must be immediate');await b.evaluate('deepLegendsPlayerTags.flush()');await settle();
 const saved=await b.evaluate(`record.note===${JSON.stringify(id)}`);
 await b.evaluate(`document.querySelector('${entry==='button'?'[data-ptag-open]':'[data-ptag-chips]'}')?.click()`);await settle();const reopened=await b.evaluate('!!document.querySelector(".ptag-modal")');report.cases.push({scene,wait,close,entry,saved,reopened});if(reopened)await b.evaluate(`document.querySelector('[data-ptag-close]').click()`);
 }
 }
 await b.evaluate(`document.querySelector('#live-content').innerHTML='<article class="live-player"><div class="live-player-identity"><button data-player-ref="r307-fixture">玩家</button></div></article>';deepLegendsPlayerTags.refresh()`);await settle();
 await b.evaluate(`document.querySelector('[data-ptag-open]').click()`);await b.until('document.querySelector(".ptag-modal")');
 await b.evaluate(`__hold=true;(()=>{const input=document.querySelector('.ptag-modal input');input.value='inflight';input.dispatchEvent(new Event('input',{bubbles:true}));})();void deepLegendsPlayerTags.flush()`);await b.until('!!window.__release');
 await b.evaluate(`document.querySelector('[data-ptag-close]').click()`);assert(await b.evaluate('!document.querySelector(".ptag-modal")'));
 await b.evaluate(`__hold=false;__release();deepLegendsPlayerTags.flush()`);report.inflight=await b.evaluate("record.note==='inflight'");
 for(const roster of ['match-team-list','arena-team-roster']){
 await b.evaluate(`document.querySelector('#live-content').innerHTML='<div class="${roster}"><button data-player-ref="r307-fixture"><span class="game-icon"></span><span>玩家</span></button></div>';deepLegendsPlayerTags.refresh()`);await settle();
 await b.evaluate(`document.querySelector('[data-player-ref]').innerHTML='<span class="game-icon"></span><span>玩家</span>';deepLegendsPlayerTags.refresh()`);await settle();report.restoration.push({roster,restored:await b.evaluate('!!document.querySelector("[data-ptag-open]")')});
 }
 report.errors=b.errors;fs.writeFileSync(path.join(out,'result.json'),JSON.stringify(report,null,2));
 console.log(JSON.stringify({cases:report.cases.length,lost:report.cases.filter(c=>!c.saved).length,reopenFailures:report.cases.filter(c=>!c.reopened).length,restoration:report.restoration}));
 if(process.env.R307_VERIFY==='1'){assert(report.cases.every(c=>c.saved&&c.reopened));assert(report.restoration.every(c=>c.restored));assert(report.inflight);assert.deepEqual(b.errors,[]);}
}finally{await b.stop();}})().catch(e=>{console.error(e);process.exitCode=1});
