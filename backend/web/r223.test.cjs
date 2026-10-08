const {viewStatus,clientView}=require('./r258-client-view-fixture.cjs');
'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {compile,escapeHTML,extract}=require('./r188-harness.cjs');
const app=fs.readFileSync(path.join(__dirname,'app.js'),'utf8'),gameplay=fs.readFileSync(path.join(__dirname,'gameplay.js'),'utf8'),html=fs.readFileSync(path.join(__dirname,'index.html'),'utf8');
function elements(document) {return Object.fromEntries([...document.querySelectorAll('[id]')].map(e=>[e.id.replace(/-([a-z])/g,(_,c)=>c.toUpperCase()),e]));}
function menu() {
 const dom=new JSDOM(html),el=elements(dom.window.document),state={status:{}},preferences=new Map();
 const f=compile(app,['searchRegion','searchServerID','setCNRegionExpanded','setRiotRegionExpanded','applySearchRegion','updateSearchRegionLabel','updateSearchRegionStatus','visibleRegionMenuEntries'],{state,el,savePreference:(k,v)=>preferences.set(k,v)});
 return {dom,el,state,preferences,...f};
}

test('R223 two mutually exclusive menu sections contain eight foreign platforms and follow-client',()=>{
 const f=menu();try {
  const sections=f.el.playerSearchRegionMenu.querySelectorAll('.region-menu-section-toggle');assert.deepEqual([...sections].map(e=>e.textContent),['国服','外服']);
  const choices=[...f.el.playerSearchRiotOptions.querySelectorAll('[data-region-option]')].map(e=>e.dataset.regionOption);
  assert.deepEqual(choices,['riot-follow','kr','jp1','na1','euw1','eun1','tw2','vn2','sg2']);
  f.applySearchRegion('cn');f.setCNRegionExpanded(true);assert.equal(f.el.playerSearchCnOptions.hidden,false);assert.equal(f.el.playerSearchRiotOptions.hidden,true);
  f.setRiotRegionExpanded(true);assert.equal(f.el.playerSearchRiotOptions.hidden,false);assert.equal(f.el.playerSearchCnOptions.hidden,true);assert.equal(f.searchRegion(),'kr');
  f.setCNRegionExpanded(true);assert.equal(f.el.playerSearchRiotOptions.hidden,true);
 }finally {f.dom.window.close();}
});

test('R223 default KR, foreign follow including Brazil, and persistent manual selection',()=>{
 const f=menu();try {
  f.applySearchRegion('kr','',false);assert.equal(f.searchRegion(),'kr');assert.equal(f.el.playerSearchRiotFollowClient.disabled,true);
  f.state.status={connected:true,clientRegion:'br1'};f.updateSearchRegionStatus(f.state.status);
  assert.equal(f.searchRegion(),'br1');assert.equal(f.el.playerSearchRiotFollowClient.disabled,false);assert.equal(f.el.playerSearchRegionLabel.textContent,'巴西');
  f.applySearchRegion('jp1');assert.equal(f.preferences.get('search-region-manual'),'true');assert.equal(f.el.playerSearchRegionLabel.textContent,'日服');
  f.state.status={connected:true,clientRegion:'TENCENT',serverId:'HN10',sgpReady:true,serverName:'黑色玫瑰'};f.updateSearchRegionStatus(f.state.status);
  assert.equal(f.searchRegion(),'jp1');assert.equal(f.el.playerSearchRiotFollowClient.disabled,true);assert.equal(f.el.playerSearchFollowClient.disabled,false);
  f.state.status={connected:true,clientRegion:''};f.updateSearchRegionStatus(f.state.status);assert.equal(f.el.playerSearchFollowClient.disabled,true);
 }finally {f.dom.window.close();}
});

test('R223 unknown current tab stays ungrouped and resolves to JP once',()=>{
 const current={key:'current',current:true,label:'Fixture#JP1',icon:1,region:'',regionResolved:false,playerRefs:new Set()},loads=[];
 const state={controllers:new Map(),status:{connected:true,identityReady:true},tabs:[current],activeTabs:{players:'current',kr:'',pro:''},activeGroup:'players',section:'overview'};
 const f=compile(gameplay,['connected','updateStatus','tabGroup','riotTab','summonerLabel','tabServerLabel','tabServerID','tabServerTitle','summonerRegionChip'],{state,setTimeout:()=>1,clearTimeout:()=>{},activateOverviewTabPanel:()=>{},escapeHTML,CN_SERVER_LABELS:{HN10:'黑色玫瑰'},renderPlayerTabs:()=>{},loadOverview:(tab,force)=>loads.push({key:tab.key,force}),ensurePerks:()=>{},ensureItems:()=>{},ensureSummonerSpells:()=>{},scheduleBeaconPoll:()=>{},loadLive:()=>{}});
 const unknown={connected:true,identityReady:true,clientRegion:'',summoner:{gameName:'Fixture',tagLine:'JP1',profileIconId:1}};
 f.updateStatus(viewStatus(unknown));assert.equal(f.tabGroup(current),'');assert.equal(f.summonerRegionChip(current),'');assert.equal(loads.length,0);
 f.updateStatus(viewStatus({...unknown,clientRegion:'jp1'}));assert.equal(f.tabGroup(current),'kr');assert.equal(state.activeGroup,'kr');assert.match(f.summonerRegionChip(current),/日服/);assert.equal(loads.length,1);
 f.updateStatus(viewStatus({...unknown,clientRegion:'jp1'}));assert.equal(loads.length,1);
 f.updateStatus(viewStatus({...unknown,clientRegion:'TENCENT',serverId:'HN10',sgpReady:true}));assert.equal(f.tabGroup(current),'players');assert.match(f.summonerRegionChip(current),/黑色玫瑰/);assert.equal(loads.length,2);
});

test('R223 cancelled launch leaves cards visible, suppresses overlay and produces no toast or refresh',async()=>{
 const dom=new JSDOM(html),el=elements(dom.window.document),state={section:'overview',status:{connected:false},installationsLoaded:true,installations:[{id:'tcls',available:true,name:'TCLS'},{id:'riot',available:true,name:'Riot'}]},hidden=[];
 try {
  const f=compile(app,['renderLaunchpad','launchOfficialLogin','launchDetectedClient'],{state,el,escapeHTML,api:async()=>({cancelled:true}),hideReadingOverlay:reason=>hidden.push(reason),showToast:()=>assert.fail('cancel toast'),refreshStatus:()=>assert.fail('cancel refresh'),setTimeout:()=>assert.fail('cancel timer'),loadClientInstallations:()=>{}});
  await f.launchOfficialLogin();assert.equal(state.clientLaunched,null);assert.equal(state.clientLaunchInFlight,'');assert.equal(state.overlaySuppressed,undefined);assert.deepEqual(hidden,[]);assert.equal(el.launcherList.hidden,false);
  assert([...el.launcherList.querySelectorAll('button')].filter(b=>b.dataset.clientId!=='wegame').every(b=>!b.disabled));
  state.overviewGroup='kr';state.clientLaunched={id:'riot',at:Date.now()};f.renderLaunchpad(clientView(state.status));assert.equal(el.launcherList.hidden,false);assert.equal(el.launcherList.querySelector('[data-client-id="riot"]').disabled,true);assert.equal(el.launcherList.querySelector('[data-client-id="tcls"]'),null);
  state.clientLaunched.at-=61_000;f.renderLaunchpad(clientView({connected:false,clientDiscovery:'process-not-found'}));assert.equal(state.clientLaunched,null);assert([...el.launcherList.querySelectorAll('button')].filter(b=>b.dataset.clientId!=='wegame').every(b=>!b.disabled));assert.equal(dom.window.document.getElementById('client-launch-reselect'),null);
 }finally {dom.window.close();}
});

test('R223 foreign tabs display region badges and metadata keeps the name-side region chip',()=>{
 const dom=new JSDOM('<nav></nav>'),tabs=dom.window.document.querySelector('nav'),state={tabs:[{key:'jp',region:'jp1',label:'Fixture#JP1'},{key:'br',region:'br1',label:'Fixture#BR1'}],activeTabs:{kr:'jp'},settings:{}};
 try {
  const f=compile(gameplay,['renderPlayerTabWorkspace','tabServerLabel','tabServerID','riotTab'],{state,document:dom.window.document,overviewWorkspace:()=>({tabs}),tabGroup:()=> 'kr',connected:()=>true,escapeHTML,CN_SERVER_LABELS:{},assetPath:()=>'',assetIcon:()=>'',prepareImages:()=>{},requestAnimationFrame:fn=>fn(),updatePlayerTabScrollControls:()=>{}});
  f.renderPlayerTabWorkspace('kr');assert.deepEqual([...tabs.querySelectorAll('.player-tab-region')].map(e=>e.textContent),['日服','巴西']);
  const render=extract(gameplay,'renderOverviewBodyContent');assert.match(render,/summoner-name-meta[^\n]+\$\{playerTag\}\$\{regionChip\}/);assert.doesNotMatch(render,/summoner-level-row[^\n]+\$\{regionChip\}/);
  assert.match(gameplay,/kr: "外服"/);assert.match(fs.readFileSync(path.join(__dirname,'pro-players.js'),'utf8'),/韩服/);
 }finally {dom.window.close();}
});

test('R223 unknown region cannot call ARAMKit and hidden, unfocused or inactive tabs cannot poll current game',async()=>{
 let calls=0;const state={section:'overview',destroyed:false,currentGameTimer:1},tab={key:'jp',region:'jp1'},doc={hidden:false,hasFocus:()=>false};
 const f=compile(gameplay,['loadMayhemRating','mayhemRatingStatus','loadOverviewCurrentGame','scheduleOverviewCurrentGame','riotTab','tabServerID'],{state,document:doc,CN_SERVER_LABELS:{},api:async()=>{calls++;return {};},activeTab:()=>tab,updateCurrentGameCard:()=>{},loadOPGGSeasonSummary:()=>{},clearTimeout:()=>{},setTimeout:()=>{},rerenderTab:()=>{}});
 await f.loadMayhemRating({current:true,regionResolved:false,data:{player:{playerRef:'opaque'}}});assert.equal(calls,0);
 f.scheduleOverviewCurrentGame(tab);await f.loadOverviewCurrentGame(tab);assert.equal(calls,0);
 doc.hidden=true;doc.hasFocus=()=>true;f.scheduleOverviewCurrentGame(tab);await f.loadOverviewCurrentGame(tab);assert.equal(calls,0);
 doc.hidden=false;state.section='champions';f.scheduleOverviewCurrentGame(tab);await f.loadOverviewCurrentGame(tab);assert.equal(calls,0);
});

test('R223 empty career UI contains titles without methodology while actual failures remain',()=>{
 const f=compile(gameplay,['renderAbility','renderRecentRanked','renderPositionStats'],{escapeHTML,number:String,isMayhemQueueId:()=>false,rankedQueueNoun:()=> '排位',rankedQueueLabel:()=> '单双排'});
 const outputs=[f.renderAbility(null,'单双排'),f.renderRecentRanked({games:0},420),f.renderPositionStats([],420)];
 for(const markup of outputs){assert.doesNotMatch(markup,/基于|最多统计|至少需要|需 ≥3|<small>/);assert.match(markup,/<h3>/);}
});
