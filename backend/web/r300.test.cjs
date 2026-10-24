'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require(require.resolve('jsdom',{paths:[path.join(__dirname,'../../desktop')]}));
const fixture=require('../testdata/r298-review-catalog.json');
const clone=v=>JSON.parse(JSON.stringify(v));
const tick=()=>new Promise(setImmediate);
async function editor(options={}) {
 const dom=new JSDOM('<div id="suite-config-root"></div>',{url:'http://127.0.0.1',runScripts:'outside-only',pretendToBeVisual:true}),w=dom.window;
 w.HTMLDialogElement.prototype.showModal=function(){this.open=true;};
 w.HTMLDialogElement.prototype.close=function(){this.open=false;this.dispatchEvent(new w.Event('close'));};
 let saved={version:1,champions:{'1':{ranked:{runes:{primaryStyleId:8000,subStyleId:8100,selectedPerkIds:[1,2,3,4,12,13,5008,5008,5001]},items:{core:[],situational:[]}}}}};
 if(options.saved)saved=clone(options.saved);
 if(options.runeIDs)saved.champions['1'].ranked.runes.selectedPerkIds=clone(options.runeIDs);
 if(options.legacyHextech)saved.champions['1'].hextech=clone(options.legacyHextech);
 const catalog=clone(fixture.catalog);catalog.clientConnected=true;catalog.itemsStatus='ready';catalog.spellsStatus='ready';
 catalog.styles.reverse();catalog.styles.find(s=>s.id===8000).slots[0].perks[0].longDesc='每10分钟获得属性。<br>10分钟：+8法术强度<br/>20分钟：+24法术强度<p>30分钟：+48法术强度</p><script>bad()</script>';
 catalog.statModSlots[0].perks[0].longDesc='获得9适应之力（5.4攻击力或9法术强度）。';
 catalog.items.push({...catalog.items[0],id:3002,name:'第二件装备'});
 Object.assign(catalog,options.catalog||{});
 const calls=[],writes=[];
 w.deepLegendsRuntime={escapeHTML:v=>String(v??'').replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('"','&quot;')};w.deepLegendsSections={register(){}};
 w.fetch=async(url,opts={})=>{
  calls.push({url,method:opts.method});let value={};
  if(url==='/api/auto-config/settings')value=saved;
  else if(url==='/api/auto-config/catalog')value=catalog;
  else if(url==='/api/champions/catalog')value={champions:[{id:1,key:'Annie',nameZh:'安妮',imageSource:'gtimg',imagePath:'/images/lol/act/img/champion/Annie.png'},{id:2,key:'Olaf',nameZh:'奥拉夫'}]};
  else if(url==='/api/settings/summoner-keys')value={keys:options.keys||['','']};
  else if(url.startsWith('/api/gameplay/champion-lanes'))value={'1':[{position:'middle',rate:.7},{position:'top',rate:.3}]};
  else if(url.startsWith('/api/champions/detail')){
   if(new URL(url,'http://fixture').searchParams.get('mode')==='ranked'&&new URL(url,'http://fixture').searchParams.get('position')==='all')return {ok:false,text:async()=>'invalid champion detail filter'};
   if(options.detail) return options.detail(url,opts);
   value={runes:[{primaryStyle:{id:8000},subStyle:{id:8100},selected:[1,2,3,4,12,13,5008,5008,5001].map(id=>({id}))}],build:{summonerSpells:[{assets:[{id:4},{id:14}]}],coreItems:[{assets:[{id:3000}]}]}};
  }
  else if(url==='/api/auto-config/copy'){const body=JSON.parse(opts.body);for(const target of body.targets){saved.champions['1'][target]=clone(saved.champions['1'][body.from]);if(target==='hextech')delete saved.champions['1'][target].runes;}saved.version++;value=saved;}
  else if(url.startsWith('/api/auto-config/champions/')){
   if(options.beforeSave)await options.beforeSave(url,opts);
   if(options.saveFails)return {ok:false,text:async()=>'保存失败，请重试'};
   const mode=url.split('/').at(-1);if(opts.method==='DELETE')delete saved.champions['1'][mode];else{const body=JSON.parse(opts.body);writes.push({mode,body});saved.champions['1'][mode]=body;}saved.version++;value=saved;
  }
  return {ok:true,json:async()=>clone(value),text:async()=>''};
 };
 w.eval(fs.readFileSync(process.env.R298_AUTO_CONFIG_SOURCE||path.join(__dirname,'auto-config.js'),'utf8'));await w.deepLegendsAutoConfig.load();
 const click=async selector=>{const node=w.document.querySelector(selector);assert(node,selector);node.click();await tick();await tick();};
 return {w,click,calls,writes,catalog,get saved(){return clone(saved);},close:()=>w.close()};
}
test('R300 portraits honor catalog source; compact toolbar has a single import and save state',async()=>{
 const h=await editor();try{const d=h.w.document,portrait=d.querySelector('.ac-ed-head img');assert(portrait.src.includes('/api/champion-asset?source=gtimg'));assert(!portrait.src.includes('/api/image?'));assert.equal(d.querySelectorAll('.ac-ed-head [data-act="import"]').length,0);assert.equal(d.querySelectorAll('[data-act="import"]').length,1);assert.equal(d.querySelector('.ac-foot'),null);assert(d.querySelector('.ac-buildname [data-act="save"]'));assert(d.querySelector('.ac-tabs [data-save-state]'));assert(!d.querySelector('.ac-ed-head').textContent.includes('正在编辑'));portrait.dispatchEvent(new h.w.Event('error'));assert(portrait.src.endsWith('/image-unavailable.svg'));await h.click('[data-tab="kit"]');assert(d.querySelector('.ac-section-head [data-save-state]'));assert(d.querySelector('.ac-section-head [data-act="save"]'));}finally{h.close();}
});
test('R300 consecutive right-slot picks keep right selected and preserve left; used-spell swap remains',async()=>{
 const h=await editor();try{await h.click('[data-tab="kit"]');await h.click('[data-spell="4"]');await h.click('[data-slot="1"]');await h.click('[data-spell="14"]');await h.click('[data-spell="6"]');assert.equal(h.w.document.querySelector('.ac-slot.is-active').dataset.slot,'1');await h.click('[data-act="save"]');assert.deepEqual(h.saved.champions['1'].ranked.spells,{spell1Id:4,spell2Id:6});await h.click('[data-spell="4"]');await h.click('[data-act="save"]');assert.deepEqual(h.saved.champions['1'].ranked.spells,{spell1Id:6,spell2Id:4});}finally{h.close();}
});
function starterPool(){return [
 {id:2003,name:'生命药水',priceTotal:50,categories:['Consumable'],maps:{11:true,12:true}},
 {id:1055,name:'多兰之刃',priceTotal:450,categories:['Lane'],maps:{11:true}},
 {id:1001,name:'鞋子',priceTotal:300,categories:['Boots'],maps:{11:true,12:true}},
 ...Array.from({length:7},(_,i)=>({id:3000+i,name:'成装'+i,priceTotal:3000,categories:['Damage'],from:[1001],to:[],maps:{11:true,12:true}}))
 ].map(i=>({inStore:true,displayInItemSets:true,iconPath:'/fixture-icon/'+i.id,from:[],to:[],...i}));}
test('R300 starter defaults, budget rejection, consumable count, count save and 500/1400 limits',async()=>{
 const h=await editor({catalog:{items:starterPool()}});try{await h.click('[data-tab="kit"]');const d=h.w.document;assert.equal(d.querySelector('.ac-build-block.is-active').dataset.itemGroup,'starter');assert.equal(d.querySelectorAll('[data-item="3000"]').length,0);await h.click('[data-item="1055"]');await h.click('[data-item="1001"]');assert.match(d.querySelector('.ac-budget-error').textContent,/超出出门预算（剩 50 金币）/);assert.equal(d.querySelector('[data-build="starter:1"] img'),null);await h.click('[data-build="starter:0"] [data-remove]');for(let i=0;i<10;i++)await h.click('[data-item="2003"]');assert.equal(d.querySelector('.ac-build-block.is-active').dataset.itemGroup,'core');await h.click('[data-act="save"]');assert.deepEqual(h.saved.champions['1'].ranked.items.starter,[{id:2003,count:10}]);await h.click('[data-item-group="starter"]');assert.equal(d.querySelector('[data-item="2003"]').getAttribute('aria-disabled'),'true');await h.click('[data-item="2003"]');assert.match(d.querySelector('.ac-budget-error').textContent,/剩 0 金币/);assert.equal(d.querySelector('[data-build="starter:0"] b').textContent,'×10');await h.click('[data-mode="aram"]');await h.click('[data-act="start"]');await h.click('[data-tab="kit"]');assert.match(d.querySelector('[data-item-group="starter"] .gold').textContent,/1400/);assert.equal(d.querySelector('[data-item="1055"]'),null);}finally{h.close();}
});
test('R300 core full advances to situational, full situational remains; drag rejects invalid moves atomically',async()=>{
 const h=await editor({catalog:{items:starterPool()}});try{await h.click('[data-tab="kit"]');await h.click('[data-item-group="core"]');for(let id=3000;id<3006;id++)await h.click(`[data-item="${id}"]`);assert.equal(h.w.document.querySelector('.ac-build-block.is-active').dataset.itemGroup,'situational');const data={setData(){}};for(const [type,selector]of [['dragstart','[data-build="core:0"]'],['drop','[data-build="starter:0"]']]){const e=new h.w.Event(type,{bubbles:true,cancelable:true});Object.defineProperty(e,'dataTransfer',{value:data});h.w.document.querySelector(selector).dispatchEvent(e);}await h.click('[data-act="save"]');assert.deepEqual(h.saved.champions['1'].ranked.items.core,[3000,3001,3002,3003,3004,3005]);assert.deepEqual(h.saved.champions['1'].ranked.items.starter,[]);await h.click('[data-build="core:0"]');const e=new h.w.KeyboardEvent('keydown',{key:'Delete',bubbles:true,cancelable:true});h.w.document.querySelector('[data-build="core:0"]').dispatchEvent(e);await h.click('[data-act="save"]');assert.deepEqual(h.saved.champions['1'].ranked.items.core,[3001,3002,3003,3004,3005]);}finally{h.close();}
});
function liveModule(settings){const dom=new JSDOM('',{url:'http://127.0.0.1',runScripts:'outside-only'}),w=dom.window,state={section:'overview',autoConfigSettings:settings},reports=[];let callback;w.deepLegendsGameAutoConfig={state,renderLive(){}};w.deepLegendsSections={register(){}};w.reportFlowDiagnostic=(...args)=>reports.push(args);w.PerformanceObserver=class{static supportedEntryTypes=['layout-shift'];constructor(fn){callback=fn;}observe(){}disconnect(){}};w.eval(fs.readFileSync(path.join(__dirname,'auto-config-live.js'),'utf8'));return {w,state,reports,shift:values=>callback({getEntries:()=>values.map(value=>({value}))}),close:()=>w.close()};}
test('R300 custom source follows hover, ranked position/fallback, mode and missing configuration',()=>{
 const h=liveModule({enabled:true,runes:true,spells:true,items:true,champions:{1:{ranked:{spells:{spell1Id:4,spell2Id:14}},'ranked-middle':{items:{starter:[{id:2003,count:2}]}},normal:{items:{core:[3000]}}}}});try{const m=h.w.deepLegendsAutoConfigLive,d={gameId:1,queueId:420,gameMode:'CLASSIC',mapId:11,players:[{isCurrent:true,championPickIntent:1,position:'middle'}]};let c=m.prepare(d);assert.equal(c.modeKey,'ranked-middle');assert.equal(h.state.runeSourceTab,'custom');assert.equal(h.state.selectedRecommendation,'custom');d.players[0].position='top';c=m.prepare(d);assert.equal(c.modeKey,'ranked');d.queueId=430;assert.equal(m.prepare(d).modeKey,'normal');d.players[0].championPickIntent=2;assert.equal(m.prepare(d),null);assert.equal(h.state.runeSourceTab,'opgg');assert.equal(h.state.selectedRecommendation,'opgg');d.queueId=1700;assert.equal(m.prepare(d),null);}finally{h.close();}
});
test('R300 layout diagnostic counts only live page and emits numeric aggregates',()=>{const h=liveModule({});try{h.shift([.2]);h.state.section='live';h.shift([.002,.003]);h.w.dispatchEvent(new h.w.Event('pagehide'));assert.deepEqual(JSON.parse(JSON.stringify(h.reports)),[['live_layout_shift','aggregated',{layoutShiftTotal:.005,layoutShiftMax:.003,layoutShiftCount:2}]]);}finally{h.close();}});
test('R300 tags stay within name columns without adding grid cells; roster has marked stickers only',async()=>{
 const dom=new JSDOM(`<div class="match-team-list"><button data-player-ref="a"><span class="game-icon"></span><span class="match-player-name">长名字</span></button><button data-player-ref="b"><span class="game-icon"></span><span class="match-player-name">未标记</span></button></div><table><thead><tr><th>玩家</th><th>KDA</th></tr></thead><tbody><tr><td><button class="participant-link" data-player-ref="a"><span class="participant-name">名字</span></button></td><td>1/2/3</td></tr></tbody></table><div class="arena-detail-player"><button class="participant-link" data-player-ref="a"><span class="participant-name">斗魂玩家</span></button><span>海克斯</span><span>评分</span><span>KDA</span><span>伤害</span><span class="participant-subline">装备</span></div>`,{url:'http://127.0.0.1',runScripts:'outside-only'}),w=dom.window;
 w.CSS={escape:s=>s};w.deepLegendsRuntime={escapeHTML:s=>String(s??'')};w.deepLegendsSections={register(){}};w.deepLegendsPlayerTagContext={sources:()=>['a','b'].map(playerRef=>({playerRef,gameName:playerRef,taggable:true})),live:()=>null,name:p=>p.gameName,avatar:()=>''};const catalog=[{id:'n',name:'负面',group:'neg'},{id:'p',name:'正面',group:'pos'},{id:'m',name:'自定义',group:'mine'}];w.fetch=async()=>({ok:true,json:async()=>({version:1,catalog,records:{a:{tags:['n','p','m'],note:'全部备注'},b:{tags:[],note:''}}})});
 try{w.eval(fs.readFileSync(path.join(__dirname,'player-tags.js'),'utf8'));await tick();await tick();const d=w.document;assert.equal(d.querySelectorAll('.match-team-list .pmark-add').length,0);assert.equal(d.querySelectorAll('.match-team-list .pmark').length,1);assert.equal(d.querySelector('tbody tr').children.length,d.querySelector('thead tr').children.length);assert.equal(d.querySelector('.arena-detail-player').children.length,6);for(const link of d.querySelectorAll('.participant-link')){const cell=link.closest('.participant-identity');assert(cell.querySelector('.ptag-name-line .ptag-btn'));const host=link.closest('.arena-detail-player')?.querySelector('.participant-subline')||cell;assert.equal(host.querySelectorAll('.ptag-row .ptag').length,3);assert.equal(host.querySelector('.is-more').textContent,'+1');assert(host.querySelector('.ptag-row').dataset.tooltip.includes('全部备注'));assert(cell.querySelector('.ptag-detail').classList.contains('is-neg'));}w.deepLegendsPlayerTags.refresh();await tick();assert.equal(d.querySelectorAll('tbody .ptag-btn').length,1);}finally{w.dispatchEvent(new w.Event("deep-legends:dispose"));await tick();w.close();}
});

test('R300 recommendation import rejects excessive quantities and preserves unsupported-mode controls',async()=>{
 const h=await editor({catalog:{items:[{...starterPool()[0],priceTotal:0}]},detail:async()=>({ok:true,json:async()=>({build:{starterItems:[{assets:[{id:2003,count:90},{id:2003,count:20},{id:2003,count:-2},{id:2003,count:1.5}]}]}})})});
 try{await h.click('[data-act="import"]');assert.deepEqual(h.saved.champions['1'].ranked.items.starter,[{id:2003,count:90}]);for(const mode of ['aram','urf','event']){await h.click(`[data-mode="${mode}"]`);assert(h.w.document.querySelector('[data-act="import"]').disabled);assert.equal(h.saved.champions['1'][mode],undefined);}}finally{h.close();}
});

test('R300 settings backup preserves starter quantities and rejects invalid starter arrays',()=>{
 const {sanitize}=require('./settings-preferences.js');const backup=starter=>({autoConfig:{version:1,champions:{1:{normal:{items:{starter,core:[3000],situational:[3001]}}}}}});
 const source=backup([{id:2003,count:10}]);assert.deepEqual(sanitize(JSON.parse(JSON.stringify(sanitize(source)))),source);
 for(const starter of [[{id:2003,count:0}],[{id:2003,count:100}],[{id:2003,count:1.5}],[{id:0,count:1}],Array.from({length:7},(_,i)=>({id:1000+i,count:1}))])assert.throws(()=>sanitize(backup(starter)));
 const old={autoConfig:{champions:{1:{ranked:{items:{core:[3000],situational:[]}}}}}};assert.deepEqual(sanitize(old),old);
});
