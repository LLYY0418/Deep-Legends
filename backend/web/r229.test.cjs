'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('fs'),path=require('path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');const {compile,escapeHTML}=require('./r188-harness.cjs');
const gameplay=fs.readFileSync(path.join(__dirname,'gameplay.js'),'utf8'),app=fs.readFileSync(path.join(__dirname,'app.js'),'utf8'),html=fs.readFileSync(path.join(__dirname,'index.html'),'utf8');
function elements(d){return Object.fromEntries([...d.querySelectorAll('[id]')].map(e=>[e.id.replace(/-([a-z])/g,(_,c)=>c.toUpperCase()),e]));}
test('R229 search button shows only the selected region/server name',()=>{
 const dom=new JSDOM(html),el=elements(dom.window.document),state={status:{}};
 try { const f=compile(app,['searchRegion','searchServerID','applySearchRegion','updateSearchRegionLabel','updateSearchRegionStatus','setCNRegionExpanded','setRiotRegionExpanded'],{state,el,savePreference:()=>{}});
 f.applySearchRegion('jp1');assert.equal(el.playerSearchRegionLabel.textContent,'日服');
 state.status={connected:true,clientRegion:'jp1'};f.applySearchRegion('riot-follow');f.updateSearchRegionLabel();assert.equal(el.playerSearchRegionLabel.textContent,'日服');
 f.applySearchRegion('cn','HN10');assert.equal(el.playerSearchRegionLabel.textContent,'黑色玫瑰');
 state.status={connected:true,clientRegion:'TENCENT',serverName:'艾欧尼亚'};f.applySearchRegion('cn');f.updateSearchRegionLabel();assert.equal(el.playerSearchRegionLabel.textContent,'艾欧尼亚');
 state.status={connected:false};f.updateSearchRegionLabel();assert.equal(el.playerSearchRegionLabel.textContent,'国服');
 }finally{dom.window.close()}
});
test('R229 disconnect cancels self work, hides self and preserves searched foreign tabs and empty group switching',async()=>{
 const aborted=[],cleared=[],rendered=[];const current={key:'current',current:true,region:'jp1',regionResolved:true,data:{matches:[1]},currentGame:{data:{}},playerRef:'old',quotaRetry:{timer:4},appendTimer:5,matchTierRetryTimer:6,overviewRequestToken:1};
 const search={key:'search',region:'jp1',data:{matches:[2]}};const state={status:{connected:false},tabs:[current,search],controllers:new Map(['overview:current','overview-more:current','opgg-season:current','current-game:current','overview:search'].map(k=>[k,{abort:()=>aborted.push(k)}])),activeTabs:{players:'current',kr:'current',pro:''},activeGroup:'kr',tabHistories:{players:[],kr:[],pro:[]},currentGameTimer:7};
 const f=compile(gameplay,['connected','tabGroup','riotTab','resetTencentTabsAfterDisconnect','activeTab','playerGroupCount','selectPlayerGroup','visiblePlayerGroups','tabReady','loadOverview','loadOverviewCurrentGame','scheduleOverviewCurrentGame'],{state,PLAYER_GROUPS:{players:"国服",kr:"外服",pro:"职业"},clearTimeout:id=>cleared.push(id),renderPlayerTabs:()=>{},renderOverview:g=>rendered.push(g),selectPlayerTab:key=>rendered.push(key),document:{hidden:false,hasFocus:()=>true},api:()=>assert.fail('disconnected self request')});
 f.resetTencentTabsAfterDisconnect();assert.equal(current.data,null);assert.equal(current.currentGame,null);assert.equal(current.closed,true);assert.equal(current.playerRef,'');assert.equal(current.overviewRequestToken,2);
 assert.deepEqual(aborted,['overview:current','overview-more:current','opgg-season:current','current-game:current']);assert(!aborted.includes('overview:search'));assert(cleared.includes(4)&&cleared.includes(5)&&cleared.includes(6)&&cleared.includes(7));assert.equal(search.data.matches[0],2);
 assert.equal(f.playerGroupCount('kr'),1);assert.equal(f.activeTab('kr'),search);assert.equal(state.activeGroup,'kr');
 f.selectPlayerGroup('players');assert.equal(state.activeGroup,'players');f.selectPlayerGroup('pro');assert.equal(state.activeGroup,'pro');assert.deepEqual(rendered,['players','pro']);
 assert.equal(await f.loadOverview(current),false);await f.loadOverviewCurrentGame(current);f.scheduleOverviewCurrentGame(current);
});
test('R229 stale current-game response cannot write after disconnect and reconnect to same reference',async()=>{
 let finish;const tab={key:'current',current:true,region:'jp1',overlay:true,overviewRequestToken:1,data:{player:{playerRef:'same'}}};const state={status:{connected:true},section:'overview',controllers:new Map()};
 const f=compile(gameplay,['loadOverviewCurrentGame','riotTab','connected'],{state,document:{hidden:false,hasFocus:()=>true},overviewSupplementTarget:()=>({key:'same',body:{}}),api:()=>new Promise(r=>finish=r),updateCurrentGameCard:()=>{}});
 const pending=f.loadOverviewCurrentGame(tab,true);tab.overviewRequestToken=2;tab.closed=true;state.status.connected=false;tab.currentGame=null;tab.currentGamePending='';tab.overviewRequestToken=3;tab.closed=false;state.status.connected=true;
 finish({status:'none'});await pending;assert.equal(tab.currentGame,null);assert.equal(tab.currentGamePending,'');
});
test('R229 empty groups expose only their own launchers, searched and connected pages hide them',()=>{
 const dom=new JSDOM(html),el=elements(dom.window.document),state={section:'overview',overviewTabIsCurrent:true,status:{connected:false},installationsLoaded:true,installations:['tcls','wegame','riot'].map(id=>({id,name:id,available:true}))};
 try {const f=compile(app,['renderLaunchpad'],{state,el,escapeHTML,launchOfficialLogin:()=>{},launchDetectedClient:()=>{}});const ids=()=>[...el.launcherList.querySelectorAll('[data-client-id]')].map(e=>e.dataset.clientId);
 f.renderLaunchpad(state.status);assert.deepEqual(ids(),['tcls','wegame']);state.overviewGroup='kr';f.renderLaunchpad(state.status);assert.deepEqual(ids(),['riot']);state.overviewTabIsCurrent=false;f.renderLaunchpad(state.status);assert.equal(el.clientLaunchpad.hidden,true);state.overviewTabIsCurrent=true;f.renderLaunchpad({connected:true});assert.equal(el.clientLaunchpad.hidden,true);state.overviewGroup='players';state.installations=[];f.renderLaunchpad({connected:false});assert.deepEqual(ids(),['tcls','wegame']);assert([...el.launcherList.querySelectorAll('[data-client-id]')].every(b=>b.disabled));assert.doesNotMatch(el.launcherList.textContent,/undefined/);
 }finally{dom.window.close()}
});
test('R229 client exiting is suppressed for 15s; cold startup and new launch still show',()=>{
 let now=100_000;const logs=[],shown=[],hidden=[];const state={status:{connected:true,identityReady:true}};const f=compile(app,['updateReadingOverlay'],{state,Date:{now:()=>now},STATUS_INTERVAL:5000,showReadingOverlay:(...v)=>shown.push(v),hideReadingOverlay:r=>hidden.push(r),window:{reportFlowDiagnostic:(...v)=>logs.push(v)}});
 f.updateReadingOverlay();state.status={connected:false,clientDiscovery:'credentials-unreadable'};f.updateReadingOverlay();assert.equal(shown.length,0);assert.equal(logs.at(-1)[1],'skip');assert.equal(logs.at(-1)[2].skip_reason,'client-exiting');now+=14999;f.updateReadingOverlay();assert.equal(shown.length,0);
 now++;f.updateReadingOverlay();assert.equal(shown.length,1);state.clientExitingUntil=now+15_000;state.lastClientLaunchAt=now;state.launchOverlayPending=true;f.updateReadingOverlay();assert.equal(shown.length,2);
 state.clientExitingUntil=0;state.launchOverlayPending=false;f.updateReadingOverlay();assert.equal(shown.length,3);
});
test('R229 reconnecting the same JP account reloads self and stores its client group',()=>{
 const loads=[],stored=[];const current={key:'current',current:true,closed:true,region:'jp1',regionResolved:true,playerRef:'',data:null,label:'Fixture#JP1',icon:1};
 const state={status:{connected:false},tabs:[current],activeTabs:{players:'',kr:'current',pro:''},activeGroup:'kr',section:'overview'};
 const f=compile(gameplay,['updateStatus','connected','tabGroup','riotTab','summonerLabel'],{state,localStorage:{setItem:(k,v)=>stored.push([k,v])},renderPlayerTabs:()=>{},loadOverview:(tab,force)=>loads.push([tab.key,force]),ensurePerks:()=>{},ensureItems:()=>{},ensureSummonerSpells:()=>{},scheduleBeaconPoll:()=>{},loadLive:()=>{}});
 f.updateStatus({connected:true,identityReady:true,clientRegion:'jp1',summoner:{gameName:'Fixture',tagLine:'JP1',profileIconId:1}});
 assert.equal(current.closed,false);assert.equal(current.data,null);assert.deepEqual(loads,[['current',true]]);assert.deepEqual(stored,[['lol-loot-last-client-player-group','kr']]);
});
