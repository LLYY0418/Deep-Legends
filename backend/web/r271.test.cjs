'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const {JSDOM}=require('../../desktop/node_modules/jsdom'),{install}=require('./filter-dialog-fixture.cjs'),{compile}=require('./r188-harness.cjs');
function historyFixture(file){
 const dom=new JSDOM('<main></main>',{url:'http://fixture'});install(dom);const root=dom.window.document.querySelector('main'),module={exports:{}};
 vm.runInNewContext(fs.readFileSync(file,'utf8'),{module,globalThis:dom.window,window:dom.window,document:dom.window.document,requestAnimationFrame:fn=>fn(),cancelAnimationFrame(){},ResizeObserver:class{observe(){}disconnect(){}},setTimeout,clearTimeout,Date,Map,Set,console});
 const af=module.exports.render?module.exports:dom.window.deepLegendsHistoryFilters,tab={advancedConditions:{},data:{matches:[]}};let ready=false,release,clicks=0;
 const styles=new Promise(r=>release=()=>{ready=true;r()});const ctx={rows:()=>[],version:()=>tab.data.matches,subject:()=>({}),tags:()=>[],cursor:()=>0,storage:dom.window.localStorage,container:()=>root,stylesReady:()=>ready,ensureStyles:()=>styles,change(){},isActive:()=>true};ctx.render=()=>{root.innerHTML=af.render(tab,()=>ctx);af.bind(root,tab,()=>ctx);};ctx.render();
 const evaluate=async expression=>Function('document',`return ${expression}`)(dom.window.document);
 const click=async selector=>{clicks++;root.querySelector(selector).click();};
 const until=async expression=>{for(let n=0;n<12;n++){if(await evaluate(expression))return;release();await Promise.resolve();}throw Error('ASSERTION FAILED: legacy queued open ended closed: '+expression);};
 return {dom,tab,root,af,ctx,evaluate,click,until,release,get clicks(){return clicks;}};
}
for(const version of [80,81])for(const probe of [265,266,269])test(`R271 r${probe} opens published ${version} once while CSS is pending`,async()=>{
 const file=process.env[`R271_HISTORY_${version}`] || path.join(__dirname,`../../scripts/fixtures/r271/history-filters-0.12.${version}.cjs`);const h=historyFixture(file);
 try{const source=fs.readFileSync(path.join(process.env.R271_PERSISTED_DIR || path.join(__dirname,'../../scripts'),`r${probe}-persisted-ui.cjs`),'utf8'),block=source.match(/const openFilter=async\(\)=>\{([\s\S]*?)\n \};/);assert(block,'actual persisted open function must be exercised');
  const open=Function('evaluate','click','until','phase',`return async()=>{${block[1]}}`)(h.evaluate,h.click,h.until,'write-presets');await open();assert.equal(h.tab.advancedMenu.open,true);assert.equal(h.root.querySelector('[data-af-menu]').hidden,false);assert.equal(h.clicks,1,'one queued open intent must not be toggled closed');await open();assert.equal(h.clicks,1,'an already visible menu remains open without toggling');
 }finally{h.dom.window.close();}
});
test('R271 committing a filter keeps the overview shell and flushes deferred data through the full renderer',()=>{
 const dom=new JSDOM('<main><section class="summoner-strip"></section><aside class="career-column"></aside><div class="match-filterbar"><div data-af-conditions></div></div><div class="match-list"></div></main>',{url:'http://fixture'});const root=dom.window.document.querySelector('main'),tab={data:{matches:[]},advancedConditions:{}};root._afBoundTab=root._overviewViewTab=tab;let full=0,filtered=0,canceled=0;
 const noop=()=>{},af={active:()=>false,renderConditions:()=>'<div data-af-conditions>changed</div>'};
 const deps={document:dom.window.document,window:dom.window,localStorage:dom.window.localStorage,globalThis:{deepLegendsHistoryFilters:af},state:{overlay:[],destroyed:false},activeTab:()=>tab,overviewGroupForSection:()=> 'self',connected:()=>true,overviewContainer:()=>root,cancelAdvancedMatchSearch:()=>canceled++,renderFilteredMatchView:()=>filtered++,rerenderTab:()=>full++,prepareImages:noop,filteredMatches:()=>[],matchSubject:noop,matchDataTags:noop,positionIcon:noop,renderMultiKillTag:noop,ensureHistoryFilterStyles:noop,showToast:noop};
 const f=compile(fs.readFileSync(process.env.R271_GAMEPLAY_SOURCE || path.join(__dirname,'gameplay.js'),'utf8'),['advancedFilterContext'],deps);
 for(let i=0;i<5;i++)f.advancedFilterContext(tab).change();assert.equal(filtered,5,'filter commits must update the match view without rebuilding the career/header shell');assert.equal(full,0);assert.equal(tab.matchViewRevision,5);assert.equal(canceled,5);assert.equal(tab.filteredVisibleCount,20);
 tab.overviewRenderDeferred=true;f.advancedFilterContext(tab).change();assert.equal(full,1,'an asynchronous data update deferred while editing must be rendered');assert.equal(tab.overviewRenderDeferred,false);root._overviewViewTab={};f.advancedFilterContext(tab).change();assert.equal(full,2,'a reused view must take the complete ownership-safe render path');dom.window.close();
});
test('R271 the real PowerShell setup collector preserves failure diagnostics and seven version launches',()=>{
 const {spawnSync}=require('node:child_process'),root=path.resolve(__dirname,'../..');
 const args=['-NoProfile','-File',path.join(root,'scripts/r271-installer-diagnostics.test.ps1')];
 if(process.env.R271_RUN_SOURCE)args.push('-RunSource',process.env.R271_RUN_SOURCE);
 if(process.env.R271_HELPER_SOURCE)args.push('-HelperSource',process.env.R271_HELPER_SOURCE);
 const r=spawnSync(process.env.PWSH_BIN || 'pwsh',args,{cwd:root,encoding:'utf8',timeout:15000});assert.ifError(r.error);assert.equal(r.status,0,r.stdout+r.stderr);assert.match(r.stdout,/PASS R271 actual Run-Setup/);
});
