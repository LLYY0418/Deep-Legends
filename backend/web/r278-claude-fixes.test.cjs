'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),{JSDOM}=require('../../desktop/node_modules/jsdom');
const {compile,extract}=require('./r188-harness.cjs');
const app=fs.readFileSync(process.env.R278_CLAUDE_APP_SOURCE || __dirname+'/app.js','utf8');
const flush=async()=>{for(let i=0;i<8;i++)await new Promise(setImmediate)};
test('R278 review: startup and client following preserve all saved search preferences; only manual selection writes',()=>{
 const dom=new JSDOM('<button id="search"></button><div id="menu"><button data-region-option="cn" data-server-id=""></button><button data-region-option="cn" data-server-id="HN1"><b>艾欧尼亚</b></button><button data-region-option="kr"></button></div><button id="riot"></button><span id="rs"></span><button id="cn"></button><span id="cs"></span>');
 try{
  const d=dom.window.document,el={playerSearchRegion:d.querySelector('#search'),playerSearchRegionMenu:d.querySelector('#menu'),playerSearchRiotFollowClient:d.querySelector('#riot'),playerSearchRiotFollowStatus:d.querySelector('#rs'),playerSearchFollowClient:d.querySelector('#cn'),playerSearchFollowStatus:d.querySelector('#cs')},writes=[];
  const saved={'search-region':'cn','search-server-id':'HN1','search-region-manual':'true'};
  const deps={el,state:{overviewGroup:'players'},isRiotSearchRegion:r=>r==='kr',RIOT_REGION_LABELS:{kr:'韩服'},setCNRegionExpanded(){},updateSearchRegionLabel(){},savePreference:(key,value)=>{writes.push([key,value]);saved[key]=value},searchRegion:()=>el.playerSearchRegion.dataset.region,window:dom.window,searchClientReady:false,searchClientPlatform:'',searchClientServer:'',searchClientIdentity:''};
  const f=Function(...Object.keys(deps),['applySearchRegion','updateSearchRegionStatus'].map(name=>extract(app,name)).join('\n')+'\nreturn {applySearchRegion,updateSearchRegionStatus};')(...Object.values(deps));
  f.applySearchRegion(saved['search-region'],saved['search-server-id'],false);
  assert.deepEqual(writes,[],'startup must not rewrite saved preferences');
  f.updateSearchRegionStatus({connected:true,clientView:{state:'ready'},clientRegion:'TENCENT',serverId:'HN1',summoner:{playerRef:'one'}});
  assert.equal(el.playerSearchRegion.dataset.serverId,'');assert.deepEqual(writes,[],'following client only changes display');
  f.updateSearchRegionStatus({connected:true,clientView:{state:'ready'},clientRegion:'kr',summoner:{playerRef:'two'}});
  assert.equal(el.playerSearchRegion.dataset.region,'riot-follow');assert.deepEqual(saved,{'search-region':'cn','search-server-id':'HN1','search-region-manual':'true'});
  f.applySearchRegion('cn','HN1');assert.deepEqual(writes,[['search-region','cn'],['search-region-manual','true'],['search-server-id','HN1']]);
 }finally{dom.window.close()}
});
test('R278 review: empty name only marks placeholder, shakes and focuses; duplicate/full/storage failures keep inline messages',()=>{
 const af=require(process.env.R278_CLAUDE_FILTER_MODULE || './history-filters.js'),dom=new JSDOM('<main><input data-af-name><p data-af-save-error hidden></p></main>');
 try{
  const menu=dom.window.document.querySelector('main'),input=menu.querySelector('input'),row=menu.querySelector('p');let raw='[]';const storage={getItem:()=>raw,setItem:(_k,v)=>{raw=v}},tab={advancedConditions:{hero:{values:['103']}}};
  assert.equal(af.saveCurrentPreset(menu,tab,{storage}),false);assert.equal(row.textContent,'');assert.equal(row.hidden,true);assert.equal(input.getAttribute('aria-invalid'),'true');assert(input.classList.contains('af-name-error'));assert.equal(dom.window.document.activeElement,input);
  input.value='same';raw=JSON.stringify([{name:'same',conditions:tab.advancedConditions}]);assert.equal(af.saveCurrentPreset(menu,tab,{storage}),false);assert.match(row.textContent,/同名/);assert.equal(row.hidden,false);
  input.value='new';raw=JSON.stringify(Array.from({length:12},(_,i)=>({name:'p'+i,conditions:tab.advancedConditions})));assert.equal(af.saveCurrentPreset(menu,tab,{storage}),false);assert.match(row.textContent,/12/);
  raw='[]';assert.equal(af.saveCurrentPreset(menu,tab,{storage:{getItem:()=>raw,setItem(){throw Error('fixture storage failure')}}}),false);assert.match(row.textContent,/保存失败/);
 }finally{dom.window.close()}
});
test('R278 review: unread popup waits for input focus and non-native modal idle',async()=>{
 const api=require(process.env.R278_CLAUDE_ANNOUNCEMENTS_MODULE || './announcements.js'),dom=new JSDOM('<button id="announcements-button"></button><input><section role="dialog" aria-modal="true" hidden></section><dialog id="announcements-dialog"><button data-announcements-close></button><div data-announcements-content></div></dialog>',{url:'https://fixture.test'}),w=dom.window;
 try{
  Object.defineProperty(w.document,'hidden',{value:false});w.AbortSignal=AbortSignal;w.setInterval=()=>0;w.clearInterval=()=>{};w.fetch=async path=>({ok:true,json:async()=>path.includes('/phase')?{phase:'None'}:{source:'fixture',announcements:[{id:'one',revision:'1',popup:true,title:'公告',summary:'内容',content:'正文',images:[],updatedAt:'2026-10-14T00:00:00+08:00'}]}});
  const dialog=w.document.querySelector('dialog');dialog.showModal=function(){this.open=true};dialog.close=function(){this.open=false};const input=w.document.querySelector('input'),modal=w.document.querySelector('section');input.focus();api.mount(w);await flush();assert.equal(dialog.open,false,'must not steal input focus');
  modal.hidden=false;input.blur();await flush();assert.equal(dialog.open,false,'non-native modal must block popup');modal.hidden=true;await flush();assert.equal(dialog.open,true,'popup resumes when idle');
 }finally{w.close()}
});
test('R278 review: announcement assets are requested only on opening and modal waits for CSS and script onload',async()=>{
 const dom=new JSDOM('<button id="announcements-button"></button><dialog id="announcements-dialog"><button data-announcements-close></button><div data-announcements-content></div></dialog>',{url:'https://fixture.test',runScripts:'outside-only'}),w=dom.window;
 try{
  Object.defineProperty(w.document,'hidden',{value:false});w.setInterval=()=>0;w.clearInterval=()=>{};w.eval(fs.readFileSync(__dirname+'/section-loader.js','utf8'));
  const dialog=w.document.querySelector('dialog');dialog.showModal=function(){this.open=true};dialog.close=function(){this.open=false};
  const notice={id:'one',revision:'1',popup:false,title:'公告',summary:'内容',content:'正文',images:[],updatedAt:'2026-10-14T00:00:00+08:00'};
  const f=compile(app,['initAnnouncements'],{document:w.document,window:w,localStorage:w.localStorage,MutationObserver:w.MutationObserver,queueMicrotask:w.queueMicrotask.bind(w),setInterval:()=>0,clearInterval(){},api:async path=>path.includes('/phase')?{phase:'None'}:{source:'fixture',announcements:[notice]}});f.initAnnouncements();await flush();assert.equal(w.document.querySelectorAll('link,script').length,0);
  w.document.querySelector('button').click();await flush();const link=w.document.querySelector('link');assert.equal(link.getAttribute('href'),'/announcements.css');assert.equal(w.document.querySelector('script'),null);assert.equal(dialog.open,false);
  link.onload();await flush();const script=w.document.querySelector('script');assert.equal(script.getAttribute('src'),'/announcements.js');w.eval(fs.readFileSync(__dirname+'/announcements.js','utf8'));assert.equal(dialog.open,false,'module evaluation alone cannot open modal');script.onload();await flush();assert.equal(dialog.open,true);
 }finally{w.close()}
});
test('R278 P7: automatic sidebar collapse follows measured compact level without writing saved preference',()=>{
 const dom=new JSDOM('<main class="app-frame"></main><header class="topbar" data-topbar-compact="0"></header><button id="toggle"></button><button id="setting"></button>');
 try{
  const d=dom.window.document,frame=d.querySelector('main');let width=1920,saved='expanded';Object.defineProperty(frame,'clientWidth',{get:()=>width});
  const f=compile(app,['applySidebar'],{document:d,window:dom.window,preference:()=>saved,el:{sidebarToggle:d.querySelector('#toggle'),settingsSidebarToggle:d.querySelector('#setting')}});
  f.applySidebar();assert.equal(d.documentElement.dataset.sidebar,'expanded');
  width=450;d.querySelector("header").dataset.topbarCompact="4";f.applySidebar();assert.equal(d.documentElement.dataset.sidebar,'collapsed');assert.equal(d.querySelector('#toggle').getAttribute('aria-expanded'),'false');assert.equal(saved,'expanded');
  width=1920;d.querySelector("header").dataset.topbarCompact="0";f.applySidebar();assert.equal(d.documentElement.dataset.sidebar,'expanded');assert.equal(d.querySelector('#toggle').getAttribute('aria-expanded'),'true');
  saved='collapsed';f.applySidebar();assert.equal(d.documentElement.dataset.sidebar,'collapsed');
 }finally{dom.window.close()}
});
test('R278 P7: query viewport pans with pointer, preserves dragging, and reveals keyboard-focused tag',()=>{
 const dom=new JSDOM('<div class="player-search-query"><input id="name"><input id="tag"></div>');
 try{
  const d=dom.window.document,view=d.querySelector('div'),name=d.querySelector('#name'),tag=d.querySelector('#tag');
  Object.defineProperties(view,{clientWidth:{value:100},scrollWidth:{value:200}});view.getBoundingClientRect=()=>({left:0,width:100});view.scrollTo=({left})=>{view.scrollLeft=left};
  const f=compile(app,['setupSearchQueryViewport'],{el:{playerSearchName:name,playerSearchTag:tag},document:d});f.setupSearchQueryViewport();
  view.dispatchEvent(new dom.window.MouseEvent('mousemove',{clientX:55}));assert(Math.abs(view.scrollLeft-50)<.01);
  view.dispatchEvent(new dom.window.MouseEvent('mousemove',{clientX:95,buttons:1}));assert(Math.abs(view.scrollLeft-50)<.01,'dragging must preserve text selection');
  view.dispatchEvent(new dom.window.MouseEvent('mouseleave'));assert.equal(view.scrollLeft,0);
  tag.focus();assert.equal(view.scrollLeft,100);view.dispatchEvent(new dom.window.MouseEvent('mouseleave'));assert.equal(view.scrollLeft,100);
  name.focus();assert.equal(view.scrollLeft,0);
 }finally{dom.window.close()}
});
