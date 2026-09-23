'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const read=name=>fs.readFileSync(name==='section-loader.js'&&process.env.R115_LOADER_SOURCE||path.join(__dirname,name),'utf8');
function harness(){
 const dom=new JSDOM('<head></head><body><main id="champions-panel"></main><main id="pro-players-panel"></main><main id="suite-panel"></main><main id="settings-panel"></main></body>',{url:'http://fixture/',runScripts:'outside-only'});
 const w=dom.window;w.eval(read('section-loader.js'));const scripts=()=>[...w.document.querySelectorAll('script[data-section-module]')];const styles=()=>[...w.document.querySelectorAll('link[data-section-style]')];
 const finish=(name,fn=()=>{})=>{w.deepLegendsSections.register(name,fn);styles().find(n=>n.dataset.sectionStyle===name)?.onload();setImmediate(()=>scripts().find(n=>n.dataset.sectionModule===name)?.onload());};
 const fail=(name)=>styles().find(n=>n.dataset.sectionStyle===name)?.onerror();
 return {dom,w,scripts,styles,finish,fail,close:()=>dom.window.close()};
}
test('R115 initial resources retain friends and shared CSS, defer optional modules, and keep demo opt-in',()=>{
 const html=read('index.html');for(const name of ['champions','pro-players','suite'])assert.doesNotMatch(html,new RegExp(`<script src="/${name}\\.js"`));
 assert.match(html,/<script src="\/friends.js"/);assert.equal((html.match(/<link rel="stylesheet"/g) || []).length,5);assert.doesNotMatch(html,/<link rel="stylesheet" href="\/(champions|pro-players|suite|shared-blocks)\.css"/);assert.match(read('app.css'),/\.mayhem-ranking-list/);assert.match(read('app.css'),/container-type:\s*inline-size/);
 assert.doesNotMatch(html,/<script src="\/demo-data.js"/);assert.match(read('runtime.js'),/if \(demo\)/);
 assert.equal(require('../../desktop/package.json').build.nsis.differentialPackage,false);
});
test('R115 module requests are singleflight and repeat entry does not add script nodes',async()=>{
 const h=harness();try{const a=h.w.deepLegendsSections.activate('champions'),b=h.w.deepLegendsSections.activate('champions');assert.equal(h.styles().length,1);assert.equal(h.scripts().length,0);let activated=0;h.finish('champions',()=>activated++);await Promise.all([a,b]);assert.equal(activated,1);await h.w.deepLegendsSections.activate('overview');await h.w.deepLegendsSections.activate('champions');assert.equal(h.scripts().length,1);assert.equal(h.w.document.querySelector('[data-section-loading]'),null);}finally{h.close();}
});
test('R115 failed loading is visible, retryable and does not leave duplicate nodes',async()=>{
 const h=harness();try{const a=h.w.deepLegendsSections.activate('pro-players');h.fail('pro-players');await a;assert.match(h.w.document.querySelector('[role=alert]').textContent,/加载失败.*重新加载/);h.w.document.querySelector('button').click();assert.equal(h.styles().length,1);h.finish('pro-players');await new Promise(setImmediate);assert.equal(h.w.document.querySelector('[role=alert]'),null);}finally{h.close();}
});
test('R115 stale module completion never navigates or loads a page after leaving',async()=>{
 const h=harness();try{let calls=0;const a=h.w.deepLegendsSections.activate('champions');await h.w.deepLegendsSections.activate('overview');h.finish('champions',()=>calls++);await a;assert.equal(calls,0);assert.equal(h.w.document.querySelector('[data-section-loading]'),null);}finally{h.close();}
});
test('R115 Suite loads helpers first, retains navigation tab and replays only the latest state to the new subscriber',async()=>{
 const h=harness();try{
  let oldStatusCalls=0;h.w.addEventListener('deep-legends:status',()=>oldStatusCalls++);
  h.w.dispatchEvent(new h.w.CustomEvent('deep-legends:status',{detail:{connected:false}}));
  h.w.dispatchEvent(new h.w.CustomEvent('deep-legends:status',{detail:{connected:true}}));
  h.w.dispatchEvent(new h.w.CustomEvent('deep-legends:navigate',{detail:{section:'suite',tab:'sweep'}}));
  let state;h.w.deepLegendsSections.listen('deep-legends:status',event=>state=event.detail);assert.equal(state.connected,true);assert.equal(oldStatusCalls,2);
  const a=h.w.deepLegendsSections.activate('suite');assert.equal(h.styles().length,1);assert.equal(h.scripts().length,0);h.finish('champions');await new Promise(setImmediate);let detail;assert.equal(h.styles().length,2);h.finish('suite',e=>detail=e.detail);await a;assert.equal(detail.name,'suite');assert.equal(detail.navigation.tab,'sweep');
 }finally{h.close();}
});
test('R115 broadcast controls use server declarations, disable team-only options for self, and retain default-off choices',()=>{
 const source=read('suite.js'),start=source.indexOf('  function watchRuleControl('),end=source.indexOf('\n  function watchChoiceButtons',start);
 const state={watch:{broadcastOptions:[{key:'camp',label:'阵营位置',template:'SOURCE CAMP'},{key:'teamComposition',label:'己方英雄',template:'SOURCE LINEUP',teamOnly:true},{key:'assignedPosition',label:'我的分路',template:'SOURCE POSITION'}]}};
 const render=Function('state','escapeHTML','watchChoiceButtons',source.slice(start,end)+';return watchRuleControl')(state,String,()=> '');
 const dom=new JSDOM(render({control:'visibility'},{visibility:'self'}));try{const inputs=[...dom.window.document.querySelectorAll('input')];assert.equal(inputs.length,3);assert.equal(inputs[0].checked,true);assert.equal(inputs[1].disabled,true);assert.equal(inputs[1].checked,false);assert.equal(inputs[2].checked,false);assert.match(dom.window.document.body.textContent,/SOURCE CAMP.*SOURCE LINEUP.*SOURCE POSITION/s);}finally{dom.window.close();}
 assert.match(source,/state.watch.rules.positionBroadcast\[input.dataset.watchBroadcast\] = input.checked/);
});
test('R115 shared blocks keep critical geometry while optional CSS waits for its section',()=>{
 const shared=read('app.css'),loader=read('section-loader.js');
 assert.match(shared,/\.mayhem-ranking-list/);assert.match(shared,/\.suite-panel\s*\{[\s\S]*container-type:\s*inline-size/);assert.match(loader,/await Promise\.all\(\(styles\[name\] \|\| \[\]\)\.map\(loadStyle\)\)/);assert.match(loader,/link\.onload/);
});
