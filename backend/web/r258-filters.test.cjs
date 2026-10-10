'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm');
const af=require(process.env.R258_FILTER_SOURCE || './history-filters.js');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {extract}=require('./r188-harness.cjs');
const source=fs.readFileSync(process.env.R258_FILTER_GAMEPLAY_SOURCE || __dirname+'/gameplay.js','utf8');
const tags=vm.runInNewContext('('+extract(source,'matchDataTags')+')');
const now=Date.parse('2026-10-08T08:00:00Z');
function match(id=1,extra={}) {return {gameId:id,queueId:420,mapId:11,modeGroup:'solo',result:'win',createdAt:now-86400000,durationSeconds:1500,subjectParticipantId:1,participants:[{participantId:1,playerRef:'self',championId:103,championName:'阿狸',position:'middle',teamId:100,kills:8,deaths:0,assists:8,kda:16,damage:25000,gold:12000,cs:200,score:{value:9,rank:1,badge:'MVP'},tripleKills:0,quadraKills:0,pentaKills:1},{participantId:2,playerRef:'friend',gameName:'朋友',teamId:100,kills:2,deaths:4,assists:0,damage:10000,gold:10000,cs:100},{participantId:3,playerRef:'enemy',gameName:'对手',teamId:200,kills:5,deaths:3,assists:1,damage:15000,gold:11000,cs:150}],...extra};}
const ctx=()=>({subject:m=>m.participants.find(p=>p.participantId===m.subjectParticipantId),tags,now:()=>now,seasonStart:'2026-01-08T00:00:00Z'});
const condition=(values,extra={})=>({values,not:false,...extra});

test('R258 P7 AND categories, OR values, NOT, mode first and remakes excluded',()=>{
 const rows=[match(1),match(2,{result:'loss'}),match(3,{result:'remake'}),match(4,{mapId:12,modeGroup:'aram'})];
 rows[1].participants=rows[1].participants.map(p=>({...p,championId:86}));
 const tab={advancedConditions:{hero:condition(['103','86']),result:condition(['win','loss'])}};
 assert.deepEqual(af.filter(rows,tab,ctx()).map(m=>m.gameId),[1,2,4]);
 tab.advancedConditions.hero.not=true;assert.equal(af.filter(rows,tab,ctx()).length,0);
 tab.advancedConditions={position:condition(['middle'],{not:true})};assert.equal(af.filter(rows,tab,ctx()).length,0);
 const context={globalThis:{deepLegendsHistoryFilters:af},advancedFilterContext:()=>ctx()};
 const filtered=vm.runInNewContext('('+extract(source,'filteredMatches')+')',context);
 assert.deepEqual([...filtered(rows,{matchFilter:'solo',advancedConditions:{result:condition(['win'])}})].map(m=>m.gameId),[1]);
});

test('R258 P7 exact multikills, at least, only three-field absence uses highest fallback',()=>{
 const p={tripleKills:0,quadraKills:0,pentaKills:1,multiKill:5};
 assert.equal(af.multikill(p,3),false);assert.equal(af.multikill(p,3,true),true);assert.equal(af.multikill(p,5),true);
 assert.equal(af.multikill({multiKill:4,doubleKills:0},4),true);assert.equal(af.multikill({multiKill:5},3,true),true);
 assert.equal(af.multikill({tripleKills:0,multiKill:5},5),false);
});

test('R258 P7 options come only from loaded rows and counts/sides/subteams are exact',()=>{
 const rows=[match(1),match(2,{result:'loss'})],c=ctx();
 assert.deepEqual(af.options(rows,'hero',c).map(x=>[x.value,x.count,x.wins]),[['103',2,1]]);
 assert.deepEqual(af.options(rows,'coplayer',c,{side:'team'}).map(x=>[x.value,x.count]),[['friend',2]]);
 assert.deepEqual(af.options(rows,'coplayer',c,{side:'opponent'}).map(x=>x.value),['enemy']);
 rows[0].participants[0].subteamId=1;rows[0].participants[1].subteamId=2;rows[0].participants[2].subteamId=1;
 assert.deepEqual(af.options([rows[0]],'coplayer',c,{side:'team'}).map(x=>x.value),['enemy']);
 assert.equal(af.filter([rows[0]],{advancedConditions:{coplayer:condition(['friend'],{side:'team'})}},c).length,0);
});

test('R258 P7 existing data tags obey ARAM/Arena eligibility, scope and numeric gates',()=>{
 const c=ctx(),rift=match(),aram=match(2,{mapId:12,modeGroup:'aram'}),arena=match(3,{mapId:30,modeGroup:'arena'});
 for(const row of [aram,arena])assert(!af.options([row],'performance',c).some(o=>['tag:participation','tag:cs','tag:gold','tag:tower'].includes(o.value)));
 assert(af.options([rift],'performance',c).some(o=>o.value==='tag:damage'));
 const tab={advancedConditions:{performance:condition(['mvp','svp'],{scope:'all',thresholds:{kda:15,score:8,kp:80}})}};
 assert.equal(af.filter([rift],tab,c).length,1);assert.equal(af.filter([aram],tab,c).length,0);
 rift.participants[0].score.value=null;assert.equal(af.filter([rift],tab,c).length,0);
 tab.advancedConditions.performance=condition(['zero']);assert.equal(af.filter([rift],tab,c).length,1);
});

test('R258 P7 time uses backend season boundary and duration categories partition boundaries',()=>{
 const c=ctx();for(const [duration,value] of [[1199,'short'],[1200,'medium'],[1800,'long'],[2400,'long'],[2401,'verylong']])assert.equal(af.filter([match(1,{durationSeconds:duration})],{advancedConditions:{duration:condition([value])}},c).length,1);
 const tab={advancedConditions:{time:condition(['season'])}};
 assert.equal(af.filter([match(1,{createdAt:'2026-01-07T23:59:59Z'})],tab,c).length,0);
 assert.equal(af.filter([match(1,{createdAt:'2026-01-08T00:00:00Z'})],tab,c).length,1);
});

function searchFixture() {
 const tab={advancedConditions:{hero:condition(['86'])},data:{pagination:{hasMore:true},matches:[]}},c=ctx();let cursor=0,calls=0,renders=0;
 Object.assign(c,{rows:()=>tab.data.matches,cursor:()=>cursor,isActive:()=>true,render:()=>renders++,load:async count=>{assert(count>0 && count<=20);assert(++calls<=20,'search passed 300-scanned limit');cursor+=count;return true;}});
 return {tab,c,get calls(){return calls},get renders(){return renders},setCursor:value=>cursor=value};
}
test('R258 P7 forward search stops at ten and caps scanned at 300, single flight',async()=>{
 let h=searchFixture();await af.find(h.tab,h.c);assert.equal(h.calls,15);assert.equal(h.tab.advancedSearch.scanned,300);assert.equal(h.tab.advancedSearch.running,false);
 h=searchFixture();h.c.load=async count=>{h.setCursor(count);h.tab.data.matches=Array.from({length:10},(_,i)=>{const m=match(i);m.participants[0].championId=86;return m;});return true;};await af.find(h.tab,h.c);assert.equal(h.tab.advancedSearch.scanned,20);
 h=searchFixture();let resolve;h.c.load=()=>new Promise(r=>resolve=r);const first=af.find(h.tab,h.c);assert.equal(await af.find(h.tab,h.c),false);let aborted=0;af.cancel(h.tab,()=>aborted++);resolve(true);await first;assert.equal(aborted,1);assert.equal(h.tab.advancedSearch.scanned,0);assert.equal(h.tab.advancedSearch.running,false);
});

test('R258 P7 switching the actual player tab cancels outstanding search',()=>{
 const a={key:'a',advancedSearch:{running:true,token:1},data:{}},b={key:'b',data:{}};let aborted=0;
 const state={tabs:[a,b],activeTabs:{players:'a'},activeGroup:'players',tabHistories:{players:[]}};
 const context={globalThis:{deepLegendsHistoryFilters:af},state,activeTab:()=>a,tabGroup:()=> 'players',savePlayerScroll(){},renderPlayerTabs(){},renderOverview(){},cancelHistoryRecovery:()=>{},cancelAdvancedMatchSearch:tab=>af.cancel(tab,()=>aborted++)};
 const select=vm.runInNewContext('('+extract(source,'selectPlayerTab')+')',context);select('b');assert.equal(aborted,1);assert.equal(a.advancedSearch.running,false);assert.equal(state.activeTabs.players,'b');
});

test('R258 P7 common presets capped at 12, replacing names, safe reads/writes',()=>{
 const storage=new Map(),store={getItem:k=>storage.get(k),setItem:(k,v)=>storage.set(k,v)};
 for(let i=0;i<12;i++)assert(af.savePreset(store,'筛选'+i,{result:condition(['win'])}));
 assert.equal(af.savePreset(store,'第13条',{hero:condition(['103'])}),false);
 assert(af.savePreset(store,'筛选0',{hero:condition(['103'])}));assert.deepEqual(Object.keys(af.readPresets(store)[0].conditions),['hero']);
 const broken={getItem(){throw Error('blocked')},setItem(){throw Error('blocked')}};assert.deepEqual(af.readPresets(broken),[]);assert.equal(af.savePreset(broken,'测试',{result:condition(['win'])}),false);
});

test('R258 P7 keyboard capsules and add, category menu, saved replacement and highlight cleanup',()=>{
 const dom=new JSDOM('<main></main>',{url:'http://fixture'}),root=dom.window.document.querySelector('main');
 const tab={advancedConditions:{multikill:condition(['5'])},data:{matches:[match()],pagination:{hasMore:false}}},c=ctx();
 let changed=0;
 Object.assign(c,{rows:()=>tab.data.matches,cursor:()=>20,storage:dom.window.localStorage,container:()=>root,render:()=>{root.innerHTML=af.render(tab,()=>c)+af.renderConditions(tab,()=>c)+'<article class="match-entry" data-match-id="1"><b data-af-multikill="5">五杀</b></article>';af.bind(root,tab,()=>c)},change:()=>{changed++;c.render()},stop:()=>af.cancel(tab),find(){}});
 c.render();assert.equal(root.querySelector('[data-af-add]').tagName,'BUTTON');const remove=root.querySelector('[data-af-remove]');remove.focus();remove.click();assert.equal(changed,1);assert.equal(af.active(tab),false);
 root.querySelector('[data-af-add]')?.click();root.querySelector('[data-af-open]').click();assert.equal(root.querySelector('[data-af-menu]').hidden,false);
 root.querySelector('[data-af-category="hero"]').click();assert(root.querySelector('[data-af-option="103"]'));root.querySelector('[data-af-option="103"]').click();assert.equal(tab.advancedConditions.hero,undefined);root.querySelector('[data-af-close]').click();assert.deepEqual(tab.advancedConditions.hero.values,['103']);
 tab.advancedConditions={multikill:condition(['5'])};c.render();af.decorate(root,tab,c);assert(root.querySelector('.af-hit'));tab.advancedConditions={};af.decorate(root,tab,c);assert(!root.querySelector('.af-hit'));
 tab.data.matches[0].participants[0]={...tab.data.matches[0].participants[0],tripleKills:undefined,quadraKills:undefined,pentaKills:undefined,doubleKills:0,multiKill:3};
 tab.advancedConditions={multikill:condition(['3'])};
 c.multiTag=vm.runInNewContext('let multikillGradientSeq=0;('+extract(source,'renderMultiKillTag')+')');
 root.innerHTML='<article class="match-entry" data-match-id="1"><div class="match-badges"></div></article>';
 af.decorate(root,tab,c);assert.equal(root.querySelector('[data-af-multikill="3"]').textContent,'三杀');assert(root.querySelector('.af-hit'));
 tab.advancedConditions={};af.decorate(root,tab,c);assert(!root.querySelector('[data-af-created]'));
 dom.window.close();
});

test('R258 P7 shared hero search accepts Chinese, pinyin, initials and aliases',()=>{
 const runtime=fs.readFileSync(__dirname+'/runtime.js','utf8');const score=vm.runInNewContext('('+extract(runtime,'scoreChampionSearchOption')+')');
 const meta={nameZh:'九尾妖狐',nameEn:'Ahri',slug:'ahri',searchTerms:['阿狸','狐狸','ali','al','jiuweiyaohu','jwyh']};
 for(const query of ['阿狸','狐狸','ali','AL','jwyh','Ahri','九尾'])assert(score(query,103,'阿狸',meta)>0,query);
 assert.equal(score('德玛',103,'阿狸',meta),0);
});

test('saved filters rename inline because Electron has no window.prompt',()=>{
 const dom=new JSDOM('<main></main>',{url:'http://fixture'}),root=dom.window.document.querySelector('main');
 const tab={advancedConditions:{result:condition(['win'])},data:{matches:[match()],pagination:{hasMore:false}}},c=ctx(),errors=[];
 Object.assign(c,{rows:()=>tab.data.matches,cursor:()=>20,storage:dom.window.localStorage,container:()=>root,error:message=>errors.push(message),render:()=>{root.innerHTML=af.render(tab,()=>c)+af.renderConditions(tab,()=>c);af.bind(root,tab,()=>c)},change:()=>c.render(),stop:()=>af.cancel(tab),find(){}});
 assert(af.savePreset(c.storage,'常用A',{result:condition(['win'])}));assert(af.savePreset(c.storage,'常用B',{result:condition(['loss'])}));
 c.render();root.querySelector('[data-af-show-saved]').click();
 assert.doesNotMatch(fs.readFileSync(__dirname+'/gameplay.js','utf8'),/window\.prompt\(/);
 root.querySelector('[data-af-rename="0"]').click();
 const input=root.querySelector('[data-af-rename-input="0"]');assert.equal(dom.window.document.activeElement,input);assert.equal(input.value,'常用A');
 input.value='常用B';root.querySelector('[data-af-rename-save="0"]').click();
 assert.deepEqual(errors,['已有同名常用筛选']);assert.equal(af.readPresets(c.storage)[0].name,'常用A');
 input.value='  排位胜场  ';input.dispatchEvent(new dom.window.KeyboardEvent('keydown',{key:'Enter',bubbles:true}));
 assert.deepEqual(af.readPresets(c.storage).map(p=>p.name),['排位胜场','常用B']);assert(!root.querySelector('[data-af-rename-input]'));
 assert.equal(root.querySelector('[data-af-menu]').hidden,false);
 root.querySelector('[data-af-rename="1"]').click();
 root.querySelector('[data-af-rename-input="1"]').dispatchEvent(new dom.window.KeyboardEvent('keydown',{key:'Escape',bubbles:true}));
 assert(!root.querySelector('[data-af-rename-input]'));assert.equal(root.querySelector('[data-af-menu]').hidden,false);assert.equal(af.readPresets(c.storage)[1].name,'常用B');
 dom.window.close();
});
