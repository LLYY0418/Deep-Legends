'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('../../desktop/node_modules/jsdom');
const {compile,escapeHTML}=require('./r188-harness.cjs');
const gameplay=fs.readFileSync(path.join(__dirname,'gameplay.js'),'utf8'),app=fs.readFileSync(path.join(__dirname,'app.js'),'utf8'),html=fs.readFileSync(path.join(__dirname,'index.html'),'utf8');
const platforms=['br1','eun1','euw1','jp1','kr','la1','la2','me1','na1','oc1','ru','sg2','tr1','tw2','vn2'];

test('R220 every Riot tab is client independent, grouped with Korea, and labelled by its actual platform',()=>{
 const f=compile(gameplay,['riotTab','tabReady','tabGroup','tabServerID','tabServerLabel','tabServerTitle','normalizePlayerTabContext','sameTabServerScope','matchTierScope'],{connected:()=>false,CN_SERVER_LABELS:{HN1:'艾欧尼亚'}});
 for(const region of platforms){const tab={region,playerRef:'opaque'};assert(f.riotTab(tab));assert(f.tabReady(tab));assert.equal(f.tabGroup(tab),'kr');assert.equal(f.normalizePlayerTabContext(tab).group,'kr');assert.equal(f.tabServerID(tab),'');assert.match(f.tabServerTitle(tab),new RegExp(`\\(${region.toUpperCase()}\\)`));assert(f.matchTierScope(tab).startsWith(`${region}:${region}:`));}
 assert.equal(f.tabServerLabel({region:'jp1'}),'日服');assert.equal(f.tabServerTitle({region:'jp1'}),'日服 (JP1)');
 assert.equal(f.sameTabServerScope({region:'jp1'},'kr',''),false);assert.equal(f.riotTab({region:'private'}),false);
});

test('R220 JP status moves the current tab and identityReady reissues its overview once',()=>{
 const current={key:'current',current:true,label:'Fixture#JP1',icon:1,region:'',playerRefs:new Set()},loads=[];
 const state={status:{connected:true,identityReady:false},tabs:[current],activeTabs:{players:'current',kr:'',pro:''},activeGroup:'players',section:'overview'};
 const f=compile(gameplay,['connected','updateStatus','tabGroup','riotTab','summonerLabel'],{state,renderPlayerTabs:()=>{},loadOverview:(tab,force)=>loads.push({key:tab.key,force}),ensurePerks:()=>{},ensureItems:()=>{},ensureSummonerSpells:()=>{},scheduleBeaconPoll:()=>{},loadLive:()=>{}});
 const status={connected:true,identityReady:false,clientRegion:'jp1',summoner:{gameName:'Fixture',tagLine:'JP1',profileIconId:1}};
 f.updateStatus(status);assert.equal(current.region,'jp1');assert.equal(f.tabGroup(current),'kr');assert.equal(state.activeGroup,'kr');assert.equal(state.activeTabs.kr,'current');assert.equal(loads.length,1);
 f.updateStatus({...status,identityReady:true});assert.equal(loads.length,2);assert.equal(loads.at(-1).force,true);
 f.updateStatus({...status,identityReady:true});assert.equal(loads.length,2);
});

test('R220 top search defaults to the connected platform, keeps manual choice, and clears Tencent server IDs',()=>{
 const dom=new JSDOM(html),el={};try{
 for(const id of ['player-search-region','player-search-region-label','player-search-region-menu','player-search-cn-toggle','player-search-cn-options','player-search-follow-client','player-search-follow-status'])el[id.replace(/-([a-z])/g,(_,c)=>c.toUpperCase())]=dom.window.document.getElementById(id);
 const state={status:{connected:true,clientRegion:'jp1'}},preferences=[];
 const f=compile(app,['searchRegion','searchServerID','applySearchRegion','updateSearchRegionLabel','updateSearchRegionStatus','setCNRegionExpanded'],{state,el,savePreference:(...x)=>preferences.push(x)});
 f.updateSearchRegionStatus(state.status);assert.equal(f.searchRegion(),'jp1');assert.equal(el.playerSearchRegionLabel.textContent,'日服');assert.equal(f.searchServerID(),'');
 f.applySearchRegion('na1','HN1');assert.equal(f.searchRegion(),'na1');assert.equal(f.searchServerID(),'');f.updateSearchRegionStatus(state.status);assert.equal(f.searchRegion(),'na1');
 state.status={connected:true,clientRegion:'TENCENT',serverId:'HN1',serverName:'艾欧尼亚'};f.updateSearchRegionStatus(state.status);assert.equal(f.searchRegion(),'');
 for(const region of platforms)assert(el.playerSearchRegionMenu.querySelector(`[data-region-option="${region}"]`),region);
 }finally{dom.window.close();}
});

test('R220 perk detail requests and cache scopes use the actual tab platform',async()=>{
 const state={perkStatsRefreshes:new Map()},requests=[];
 const f=compile(gameplay,['ensureBuildData'],{state,riotTab:t=>platforms.includes(t.region),matchSubject:()=>({participantId:1}),matchAugmentIDs:()=>[],api:async(url,options)=>{requests.push(JSON.parse(options.body));return null;}});
 for(const region of platforms)await f.ensureBuildData({gameId:220,perkStatsStale:true},{region,data:{player:{playerRef:'opaque'}}});
 assert.deepEqual(requests.map(r=>r.region),platforms);assert.equal(state.perkStatsRefreshes.size,platforms.length);
});

test('R220 player links from JP match details preserve JP and its group',()=>{
 const dom=new JSDOM('<button data-player-ref="fixture-ref" data-name="Fixture"></button>');try{
 const opens=[];
 const f=compile(gameplay,['bindPlayerLinks'],{openPlayerOverlay:options=>opens.push(options),openPlayer:(...args)=>opens.push(args),tabServerID:()=>'',playerButtonLabel:()=> 'Fixture',proContextFromButton:()=>({}),riotTab:()=>true});
 f.bindPlayerLinks(dom.window.document.body,{region:'jp1',overlay:false});dom.window.document.querySelector('button').click();
 assert.equal(opens[0][2],'jp1');assert.equal(opens[0][3],'');
 f.bindPlayerLinks(dom.window.document.body,{region:'jp1',overlay:true});dom.window.document.querySelector('button').click();assert.equal(opens.at(-1).region,'jp1');
 }finally{dom.window.close();}
});

test('R220 practice NONE position displays the OPGG primary lane without fallback explanation',()=>{
 const self={isCurrent:true,position:'NONE'},data={gameMode:'PRACTICETOOL',mapId:11,players:[self]},payload={resolvedPosition:'mid',positions:[{position:'mid',roleRate:95}]};
 const f=compile(gameplay,['practicePlayerPosition','liveCurrentPositionChip','specialistPosition'],{state:{},escapeHTML,liveRecommendationTarget:()=>({clientPosition:'NONE'}),liveRecommendationsFor:()=>payload,positionLabel:p=>({middle:'中路'})[p]||p,positionIcon:()=>''});
 assert.equal(f.practicePlayerPosition(self,data),'middle');assert.equal(f.specialistPosition(data),'mid');const markup=f.liveCurrentPositionChip(data);assert.match(markup,/中路/);assert.doesNotMatch(markup,/其他|推断|报告的位置|回退/);
});

test('R220 external capability diagnostics hide Tencent SGP and ARAMKit entry quietly',()=>{
 const state={status:{clientRegion:'jp1'},lastCapabilities:[{name:'match-details',state:'failed',path:'sgp: /match',detail:'腾讯 SGP失败'},{name:'match-history',state:'available',path:'riot: /match',attempts:[{source:'riot',outcome:'success'},{source:'sgp',outcome:'disabled'}]}]},nodes={gameplaySettingsStatus:{innerHTML:''}};
 const f=compile(gameplay,['renderCapabilitySettings','capabilityAttemptSummary','renderRankMMRPopover'],{state,nodes,escapeHTML,number:String,riotTab:t=>t.region==='jp1'});f.renderCapabilitySettings();assert.match(nodes.gameplaySettingsStatus.innerHTML,/Riot API/);assert.doesNotMatch(nodes.gameplaySettingsStatus.innerHTML,/SGP|腾讯/);assert.equal(f.renderRankMMRPopover({region:'jp1'}),'');
});

test('R220 local Riot overview permits the retry budget and still accepts ordinary JSON replies',async()=>{
 const state={controllers:new Map()},requests=[];
 const f=compile(gameplay,['api'],{state,Headers,AbortController,fetch:async(path,options)=>{requests.push({path,accept:options.headers.get('Accept')});return new Response('{"matches":[]}',{headers:{'Content-Type':'application/json'}});},setTimeout:()=>1,clearTimeout:()=>{}});
 const result=await f.api('/api/gameplay/overview?count=10',{onProgress:()=>{}},'current',190000);
 assert.deepEqual(result,{matches:[]});assert.equal(requests[0].accept,'application/x-ndjson');
 assert.match(gameplay,/const payload = tab.current && connected\(\)/);
 assert.match(gameplay,/verification.expectGameId[^\n]+\{onProgress\}, requestKey, timeout/);
});

test('R220 clicking a live JP player opens a JP overview and keeps rune application unchanged',()=>{
 const dom=new JSDOM('<button data-player-ref="fixture-jp-player"></button>');try{
 const opened=[],state={status:{clientRegion:'jp1'}};
 const f=compile(gameplay,['bindLivePanelScope'],{state,bindLiveNode:()=>{},bindRuneWorkspaceControls:()=>{},bindRuneChoiceButtons:()=>{},applyRunes:()=>{},applyItemSet:()=>{},openPlayerOverlay:p=>opened.push(p),playerButtonLabel:()=> 'Fixture',proContextFromButton:()=>({})});
 f.bindLivePanelScope(dom.window.document.body);dom.window.document.querySelector('button').click();assert.equal(opened[0].region,'jp1');assert.equal(opened[0].serverId,'');
 assert.match(gameplay,/仅英雄选择阶段可应用/);
 }finally{dom.window.close();}
});
