'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const read=n=>fs.readFileSync(path.join(__dirname,n),'utf8'),flush=()=>new Promise(setImmediate);
function extract(src,name){let start=src.indexOf(`function ${name}(`);assert.ok(start>=0,name);if(src.slice(start-6,start)==='async ')start-=6;return src.slice(start,src.indexOf('\n  }',start)+4)}
function compile(file,names,deps){return Function(...Object.keys(deps),names.map(n=>extract(read(file),n)).join('\n')+`\nreturn {${names.join(',')}}`)(...Object.values(deps))}
function queue(t){const dom=new JSDOM('<main></main>',{url:'http://localhost',runScripts:'outside-only'}),w=dom.window;
 let observer; w.IntersectionObserver=class{constructor(cb){this.cb=cb;observer=this}observe(){}disconnect(){}};
 w.eval(read('image-queue.js'));t.after(()=>{w.dispatchEvent(new w.Event('deep-legends:dispose'));w.close()});
 return {w,observer,add(url,lazy=false){const img=w.document.createElement('img');if(lazy)img.loading='lazy';img.dataset.queuedSrc=url;w.document.querySelector('main').append(img);return img}};
}
test('R111 admitted lazy thumbnails start eagerly and all visible images drain the queue',async t=>{
 const h=queue(t),images=Array.from({length:18},(_,i)=>h.add(`/skin-${i}`,true));await flush();
 assert.equal(images.filter(i=>i.hasAttribute('src')).length,0,'offscreen assets stay lazy');
 h.observer.cb(images.map(target=>({target,isIntersecting:true})));
 assert.equal(images.filter(i=>i.hasAttribute('src')).length,h.w.deepLegendsImageQueueLimit);
 for(const img of images){assert.equal(img.loading,'eager','native lazy must not occupy an active slot');assert.ok(img.hasAttribute('src'));img.dispatchEvent(new h.w.Event('load'))}
 assert.ok(images.every(i=>i.dataset.imageReady==='true'));
});
test('R111 switching an in-flight image to its fallback does not strand the new URL',async t=>{
 const {w,add}=queue(t),img=add('/old');await flush();
 img.dataset.queuedSrc='/new';await flush();
 img.dispatchEvent(new w.Event('error'));await flush();
 assert.equal(img.getAttribute('src'),'/new');img.dispatchEvent(new w.Event('load'));
 assert.equal(img.dataset.imageReady,'true');
});
test('R111 picker chunks and unrelated UI changes do not rescan the whole document',async t=>{
 const {w,add}=queue(t);add('/one');await flush();let queries=0;
 const original=w.document.querySelectorAll.bind(w.document);w.document.querySelectorAll=(...args)=>{queries++;return original(...args)};
 for(let i=0;i<20;i++)w.document.querySelector('main').append(w.document.createElement('span'));
 await flush();assert.equal(queries,0);
});
test('R111 automatic pagination needs downward user intent and stops when its tab is inactive',()=>{
 const dom=new JSDOM('<div id="app-scroll"><main><div data-match-sentinel></div></main></div>'),w=dom.window;
 try {const root=w.document.querySelector('#app-scroll'),container=w.document.querySelector('main'),tab={data:{pagination:{hasMore:true}}},state={},calls=[],timers=[];let cb,current=container;
 const IO=class{constructor(fn){cb=fn}observe(){}disconnect(){}};
 const {bindMatchSentinel:bind}=compile('gameplay.js',['bindMatchSentinel'],{state,document:w.document,window:{IntersectionObserver:IO,requestAnimationFrame:fn=>fn()},IntersectionObserver:IO,matchObserverKey:()=> 'observer',overviewContainer:()=>current,loadOverview:(...a)=>calls.push(a),setTimeout:fn=>{timers.push(fn);return timers.length},clearTimeout:()=>{}});
 bind(container,tab);cb([{isIntersecting:true}]);assert.equal(timers.length,0,'short page/scroll restoration must not fetch');
 root.dispatchEvent(new w.WheelEvent('wheel',{deltaY:-10}));assert.equal(timers.length,0);
 root.dispatchEvent(new w.WheelEvent('wheel',{deltaY:10}));timers.shift()();assert.equal(calls.length,1);
 bind(container,tab);cb([{isIntersecting:true}]);assert.equal(timers.length,0,'new page must not cascade');
 current=null;root.dispatchEvent(new w.WheelEvent('wheel',{deltaY:10}));assert.equal(timers.length,0);
 current=container;state.observer.disconnect();root.dispatchEvent(new w.WheelEvent('wheel',{deltaY:10}));assert.equal(timers.length,0,'cleanup removes input listeners');
 }finally{w.close()}
});
test('R111 read-only detection preserves committed state, hero selection and loaded DOM',async()=>{
 const facade={profile:{backgroundSkinId:136038},chat:{icon:7173}},draft={hero:'62',skinId:0};
 const controller=new AbortController(),state={facade,facadeDraft:draft,facadeController:controller},button={disabled:false},requests=[];
 const {runFacadeProbe}=compile('suite.js',['runFacadeProbe'],{state,api:async(...args)=>{requests.push(args);return {mode:'read-only'}},toast:()=>{}});
 await runFacadeProbe(button);assert.equal(requests.length,1);assert.equal(requests[0][0],'/api/facade/probe');assert.equal(state.facade,facade);assert.equal(state.facadeDraft,draft);assert.equal(button.disabled,false);
 assert.equal(controller.signal.aborted,true);assert.equal(state.facadeController,null,'aborted refresh must not block the next read');
});
test('R111 failed background refresh retains the existing career DOM and draft',async()=>{
 const dom=new JSDOM('<main><img src="/current"><select><option>Wukong</option></select></main>');
 try{const root=dom.window.document.querySelector('main'),img=root.querySelector('img'),draft={hero:'62',skinId:0},facade={skins:[]};const state={connected:true,facade,facadeDraft:draft};
 const {loadFacade}=compile('suite.js',['loadFacade'],{state,roots:{facade:root},api:async()=>{throw Error('transient')},AbortController,setTimeout,clearTimeout,renderFacade:()=>assert.fail('must retain DOM'),toast:()=>{}});
 await loadFacade(true,true,'sse');assert.equal(root.querySelector('img'),img);assert.equal(state.facade,facade);assert.equal(state.facadeDraft,draft);
 }finally{dom.window.close()}
});
