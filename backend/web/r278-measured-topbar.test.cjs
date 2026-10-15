'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),{JSDOM}=require('../../desktop/node_modules/jsdom');
const {compile,escapeHTML}=require('./r188-harness.cjs');
const app=fs.readFileSync(process.env.R278_CLAUDE_APP_SOURCE||__dirname+'/app.js','utf8');
test('R278 measured topbar detects child bounds, internal search overflow and pairwise button intersections',()=>{
 const box=(left,width)=>({left,right:left+width,top:0,bottom:42,width,height:42});
 let search={clientWidth:100,scrollWidth:100},children=[{getBoundingClientRect:()=>box(10,100)}],buttons=[box(10,32),box(50,32)];
 const bar={offsetWidth:200,getBoundingClientRect:()=>box(0,200),get children(){return children},querySelector:()=>search,querySelectorAll:()=>buttons.map(b=>({getBoundingClientRect:()=>b}))};
 const f=compile(app,['topbarOverflows'],{getComputedStyle:()=>({paddingLeft:'10',paddingRight:'10'})});
 assert.equal(f.topbarOverflows(bar),false);search.scrollWidth=102;assert.equal(f.topbarOverflows(bar),true);
 search.scrollWidth=100;buttons=[box(10,32),box(41,32)];assert.equal(f.topbarOverflows(bar),true);
 buttons=[box(10,32),box(50,32)];children=[{getBoundingClientRect:()=>box(150,42)}];assert.equal(f.topbarOverflows(bar),true);
});
test('R278 measured compact controller changes at most one level per frame and reserves sixteen pixels before restoring',async()=>{
 const dom=new JSDOM('<main><header class="topbar"></header><aside class="sidebar"></aside></main>'),d=dom.window.document,bar=d.querySelector('header'),frames=[],observers=[];
 let width=700,required=3,minRestoredWidth=700;
 Object.defineProperty(bar,'offsetWidth',{get:()=>width});
 bar.getBoundingClientRect=()=>({width:width*2});
 Object.defineProperty(d.querySelector('aside'),'clientWidth',{value:70});
 Object.defineProperty(d,'fonts',{value:{ready:Promise.resolve()}});
 class Observer {constructor(callback){this.callback=callback;observers.push(this)}observe(){}}
 const levels=[];
 const f=compile(app,['setupTopbarCompact'],{document:d,window:dom.window,requestAnimationFrame:fn=>{frames.push(fn);return frames.length},ResizeObserver:Observer,MutationObserver:Observer,preference:()=> 'expanded',getComputedStyle:()=>({getPropertyValue:()=> '214'}),applySidebar:()=>levels.push(Number(bar.dataset.topbarCompact)),topbarOverflows:n=>Number(n.dataset.topbarCompact)<required || n!==bar && parseFloat(n.style.width)<minRestoredWidth});
 try{
  f.setupTopbarCompact();await Promise.resolve();
  const tick=()=>{const before=Number(bar.dataset.topbarCompact),fn=frames.shift();assert(fn);fn();assert(Math.abs(Number(bar.dataset.topbarCompact)-before)<=1);};
  for(let i=0;i<4;i++)tick();assert.equal(bar.dataset.topbarCompact,'3');assert.equal(bar.dataset.topbarSettled,'true');assert.deepEqual(levels,[1,2,3]);
  required=0;width=715;observers[0].callback();observers[0].callback();assert.equal(frames.length,1,'observer callbacks must coalesce');tick();assert.equal(bar.dataset.topbarCompact,'3','fifteen pixels are insufficient');
  width=716;observers[0].callback();for(let i=0;i<4;i++)tick();assert.equal(bar.dataset.topbarCompact,'0');assert.equal(bar.dataset.topbarSettled,'true');assert.equal(d.querySelectorAll('header').length,1,'measurement copies must be removed');
 }finally{dom.window.close()}
});
test('R278 compact region uses exactly two Unicode characters and retains complete accessible identity',()=>{
 const dom=new JSDOM(fs.readFileSync(__dirname+'/index.html','utf8')),d=dom.window.document;
 try{
  const el=Object.fromEntries([...d.querySelectorAll('[id]')].map(n=>[n.id.replace(/-([a-z])/g,(_,c)=>c.toUpperCase()),n]));
  const f=compile(app,['updateSearchRegionLabel'],{el,state:{status:{}},isRiotSearchRegion:r=>['kr','jp1'].includes(r)});
  for(const [server,full,short] of [['HN1','艾欧尼亚','艾欧'],['HN10','黑色玫瑰','黑色']]){
   el.playerSearchRegion.dataset.region='cn';el.playerSearchRegion.dataset.serverId=server;f.updateSearchRegionLabel();assert.equal(el.playerSearchRegionLabel.textContent,full);assert.equal(el.playerSearchRegionShort.textContent,short);assert.equal(el.playerSearchRegion.title,full);assert.equal(el.playerSearchRegion.getAttribute('aria-label'),full);assert.equal(el.playerSearchRegion.dataset.serverId,server);
  }
  el.playerSearchRegionMenu.querySelector('[data-server-id="HN1"] b').textContent='𠮷野之城';el.playerSearchRegion.dataset.serverId='HN1';f.updateSearchRegionLabel();assert.equal(el.playerSearchRegionShort.textContent,'𠮷野');
 }finally{dom.window.close()}
});
test('R278 approved corrupt contract verifies actual search follow login while retaining every other control',()=>{
 const dom=new JSDOM(fs.readFileSync(__dirname+'/index.html','utf8')),d=dom.window.document,events=[];
 try{
  const el=Object.fromEntries([...d.querySelectorAll('[id]')].map(n=>[n.id.replace(/-([a-z])/g,(_,c)=>c.toUpperCase()),n]));
  dom.window.reportFlowDiagnostic=(event,trigger,fields)=>events.push({event,trigger,fields,searchUI:new Error().stack.includes('updateSearchRegionStatus')});
  const status={connected:true,clientView:{state:'ready'},clientRegion:'TENCENT',serverId:'HN1',summoner:{playerRef:'fixture'}};
  const f=compile(app,['applySearchRegion','updateSearchRegionLabel','updateSearchRegionStatus'],{el,state:{overviewGroup:'players',status},document:d,window:dom.window,isRiotSearchRegion:r=>r==='kr',setCNRegionExpanded(){},savePreference(){throw Error('automatic following must not write preferences')},searchRegion:()=>el.playerSearchRegion.dataset.region});
  f.applySearchRegion('cn','HN1',false);f.updateSearchRegionStatus(status);
  const before={defaultPage:'live',defaultFilter:'solo',matchCount:'20',scale:'1.25',maskNames:true,searchRegion:'cn',searchServer:'HN1'},actual={...before,searchRegion:el.playerSearchRegion.dataset.region,searchServer:el.playerSearchRegion.dataset.serverId},follow={label:el.playerSearchRegionLabel.textContent,events};
  const {assertCorruptControls}=require('../../scripts/r265-persisted-ui.cjs');assertCorruptControls(actual,before,follow);
  assert.throws(()=>assertCorruptControls({...actual,scale:'1'},before,follow),assert.AssertionError);
  assert.throws(()=>assertCorruptControls(actual,before,{...follow,events:[]}),/client_follow_client login/);
 }finally{dom.window.close()}
});
test('R278 approved official-auto contract rejects restored seconds in actual product markup',()=>{
 const source=fs.readFileSync(process.env.R278_CONTRACT_GAMEPLAY_SOURCE||__dirname+'/gameplay.js','utf8'),f=compile(source,['historyServiceState','renderHistoryServiceStatus'],{riotTab:()=>false,escapeHTML,Date});
 const dom=new JSDOM(f.renderHistoryServiceStatus({data:{matches:[],capabilities:[{historyStatus:'official',retryAfter:2}]}}));
 try{
  require('../../scripts/r278-approved-contracts.cjs').assertOfficialOutage({text:dom.window.document.body.textContent,styleCount:dom.window.document.querySelectorAll('.service-outage,.service-outage-compact,.service-outage-inline').length,retryText:dom.window.document.querySelector('[data-gameplay-retry]').textContent});
 }finally{dom.window.close()}
});

test('R278 logical width floor keeps compact level three through sixteen pixels of hysteresis',async()=>{
 const dom=new JSDOM('<main><header class="topbar"></header></main>'),d=dom.window.document,bar=d.querySelector('header'),frames=[],observers=[];
 let width=601;
 Object.defineProperty(bar,'offsetWidth',{get:()=>width});
 bar.getBoundingClientRect=()=>({width:width*2});
 class Observer {constructor(callback){this.callback=callback;observers.push(this)}observe(){}}
 const f=compile(app,['setupTopbarCompact'],{document:d,window:dom.window,requestAnimationFrame:fn=>{frames.push(fn);return frames.length},ResizeObserver:Observer,MutationObserver:Observer,preference:()=> 'expanded',applySidebar(){},topbarOverflows:()=>false});
 const drain=()=>{let count=0;while(frames.length){assert(count++<10,'controller must settle');const before=Number(bar.dataset.topbarCompact);frames.shift()();assert(Math.abs(Number(bar.dataset.topbarCompact)-before)<=1);}assert.equal(bar.dataset.topbarSettled,'true');};
 const resize=value=>{width=value;observers[0].callback();drain();};
 try{
  f.setupTopbarCompact();drain();assert.equal(bar.dataset.topbarCompact,'0','width above 600 without overflow must not force compaction');
  for(const value of [600,599,450]){resize(value);assert(Number(bar.dataset.topbarCompact)>=3,'logical width <=600 must enforce compact level three');}
  for(const value of [601,615,600,616,601,600]){resize(value);assert.equal(bar.dataset.topbarCompact,'3','boundary changes must not oscillate or bypass sixteen logical pixels');}
  resize(617);assert.equal(bar.dataset.topbarCompact,'0','wide space restores one level per frame');
  assert.equal(d.querySelectorAll('header').length,1,'measurement copies must be removed');
 }finally{dom.window.close()}
});

test('R278 approved restart contract follows startup login despite saved manual preference',()=>{
 const dom=new JSDOM(fs.readFileSync(__dirname+'/index.html','utf8')),d=dom.window.document,events=[];
 try{
  const el=Object.fromEntries([...d.querySelectorAll('[id]')].map(n=>[n.id.replace(/-([a-z])/g,(_,c)=>c.toUpperCase()),n]));
  const saved={'search-region':'cn','search-server-id':'HN1','search-region-manual':'true'},writes=[];
  const status={connected:true,clientView:{state:'ready'},clientRegion:'TENCENT',serverId:'HN1',summoner:{playerRef:'fixture'}};
  dom.window.reportFlowDiagnostic=(event,trigger,fields)=>events.push({event,trigger,fields,searchUI:new Error().stack.includes('updateSearchRegionStatus')});
  const f=compile(app,['applySearchRegion','updateSearchRegionLabel','updateSearchRegionStatus'],{el,state:{overviewGroup:'players',status},document:d,window:dom.window,isRiotSearchRegion:r=>r==='kr',setCNRegionExpanded(){},savePreference:(key,value)=>{writes.push([key,value]);saved[key]=value},searchRegion:()=>el.playerSearchRegion.dataset.region});
  f.applySearchRegion(saved['search-region'],saved['search-server-id'],false);el.playerSearchRegion.dataset.manual=saved['search-region-manual'];f.updateSearchRegionStatus(status);
  const before={defaultPage:'live',defaultFilter:'solo',matchCount:'20',scale:'1.25',maskNames:true,searchRegion:'cn',searchServer:'HN1'},actual={...before,searchRegion:el.playerSearchRegion.dataset.region,searchServer:el.playerSearchRegion.dataset.serverId},follow={label:el.playerSearchRegionLabel.textContent,events};
  const {assertRestartControls}=require('../../scripts/r265-persisted-ui.cjs');assertRestartControls(actual,before,follow);
  assert.deepEqual(saved,{'search-region':'cn','search-server-id':'HN1','search-region-manual':'true'});assert.deepEqual(writes,[]);
  assert.throws(()=>assertRestartControls({...actual,scale:'1'},before,follow),assert.AssertionError);
  assert.throws(()=>assertRestartControls(actual,before,{...follow,events:[]}),/restart must record search client_follow_client login/);
 }finally{dom.window.close()}
});

test('R278 approved read contract follows startup login despite saved manual preference',()=>{
 const dom=new JSDOM(fs.readFileSync(__dirname+'/index.html','utf8')),d=dom.window.document,events=[];
 try{
  const el=Object.fromEntries([...d.querySelectorAll('[id]')].map(n=>[n.id.replace(/-([a-z])/g,(_,c)=>c.toUpperCase()),n]));
  const saved={'search-region':'cn','search-server-id':'HN1','search-region-manual':'true'},writes=[];
  const status={connected:true,clientView:{state:'ready'},clientRegion:'TENCENT',serverId:'HN1',summoner:{playerRef:'fixture'}};
  dom.window.reportFlowDiagnostic=(event,trigger,fields)=>events.push({event,trigger,fields,searchUI:new Error().stack.includes('updateSearchRegionStatus')});
  const f=compile(app,['applySearchRegion','updateSearchRegionLabel','updateSearchRegionStatus'],{el,state:{overviewGroup:'players',status},document:d,window:dom.window,isRiotSearchRegion:r=>r==='kr',setCNRegionExpanded(){},savePreference:(key,value)=>{writes.push([key,value]);saved[key]=value},searchRegion:()=>el.playerSearchRegion.dataset.region});
  f.applySearchRegion(saved['search-region'],saved['search-server-id'],false);el.playerSearchRegion.dataset.manual=saved['search-region-manual'];f.updateSearchRegionStatus(status);
  const before={defaultPage:'live',defaultFilter:'solo',matchCount:'20',scale:'1.25',maskNames:true,searchRegion:'cn',searchServer:'HN1'},actual={...before,searchRegion:el.playerSearchRegion.dataset.region,searchServer:el.playerSearchRegion.dataset.serverId},follow={label:el.playerSearchRegionLabel.textContent,events};
  const {assertReadControls}=require('../../scripts/r265-persisted-ui.cjs');assertReadControls(actual,before,follow);
  assert.deepEqual(saved,{'search-region':'cn','search-server-id':'HN1','search-region-manual':'true'});assert.deepEqual(writes,[]);
  assert.throws(()=>assertReadControls({...actual,scale:'1'},before,follow),assert.AssertionError);
  assert.throws(()=>assertReadControls(actual,before,{...follow,events:[]}),/read must record search client_follow_client login/);
 }finally{dom.window.close()}
});
