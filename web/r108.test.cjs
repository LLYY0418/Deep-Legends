"use strict";
const test=require("node:test"), assert=require("node:assert/strict"), fs=require("node:fs"), path=require("node:path");
const source=fs.readFileSync(path.join(__dirname,"gameplay.js"),"utf8");
function extract(name) {
  let start=source.indexOf(`function ${name}(`); assert.ok(start>=0,name);
  if(source.slice(start-6,start)==="async ")start-=6;
  return source.slice(start,source.indexOf("\n  }",start)+4);
}
function compile(names,deps) { return Function(...Object.keys(deps),names.map(extract).join("\n")+`\nreturn {${names.join(",")}};`)(...Object.values(deps)); }
const noop=()=>{};
const payload=(count=20,beg=0,filter="all")=>({player:{playerRef:"fixture",region:"kr"},matches:Array.from({length:count},(_,i)=>({gameId:beg+i})),pagination:{begIndex:beg,count,hasMore:count===20,filter,serverFiltered:filter!=="all"}});
function fixture(api) {
  const jobs=[],state={settings:{matchCount:20},controllers:new Map()},toasts=[];
  const deps={state,Date:{now:()=>100000},setTimeout:(fn,ms)=>{jobs.push({fn,ms});return jobs.length;},clearTimeout:noop,
    tabReady:()=>true,tabGroup:()=>"kr",riotTab:()=>true,rerenderTab:noop,loadOPGGSeasonSummary:noop,loadOverviewCurrentGame:noop,syncOverviewSupplementRefs:noop,rememberTabPlayerRef:noop,playerLabel:()=>"Fixture",renderCapabilitySettings:noop,
    appendOverviewMatches:noop,showLoadingMoreState:noop,showToast:x=>toasts.push(x),AUTO_PAGE_DELAY_MS:0,MAX_BROWSE_MATCHES:1000,
    normalizedPagination:(p,beg)=>({...p.pagination,nextBegIndex:beg+p.pagination.count}),api};
  return {...compile(["loadOverview"],deps),deps,jobs,toasts,state};
}
for(const append of [false,true]) test(`R108 quota recovery resumes the correct ${append?"next":"incomplete first"} page`,async()=>{
  const calls=[];const h=fixture(async(url,options)=>{calls.push(JSON.parse(options.body));return payload(20,append?20:0)});
  const tab={key:"fixture",region:"kr",playerRef:"fixture",matchFilter:"all",data:payload(append?20:5),nextBegIndex:append?20:0,initialPagePending:!append,quotaRetry:{append,retryAt:99999}};
  assert.equal(await h.loadOverview(tab,false,true,true),true);
  assert.equal(calls.length,1);assert.equal(calls[0].begIndex,append?20:0);
  assert.equal(tab.data.matches.length,append?40:20);assert.equal(tab.nextBegIndex,append?40:20);
  assert.equal(tab.initialPagePending,false);assert.equal(tab.quotaRetry,null);
});
test("R108 active quota avoids useless manual calls and loading state never offers another load button",async()=>{
  const h=fixture(async()=>{throw Error("must not request during cooldown")});
  const tab={key:"fixture",quotaRetry:{retryAt:110000},data:payload()};
  assert.equal(await h.loadOverview(tab,false,true,true),false);assert.equal(h.toasts.length,1);
  const ui=compile(["paginationCopyFor","matchSentinelShouldHide","matchListEmptyContent"],{Date:{now:()=>100000},escapeHTML:x=>x,number:String,matchFilterDisplayLabel:()=>"海克斯大乱斗",emptyState:(a,b)=>a+b});
  for(const extra of [{quotaRetry:{retryAt:110000}},{filterPaging:true},{loadingMore:true}]) {
    const row={data:payload(0),...extra};row.data.pagination.hasMore=true;
    assert.equal(ui.matchSentinelShouldHide(row,false),true);
    assert.doesNotMatch(ui.matchListEmptyContent(row,null,"empty"),/data-load-more|继续向下滚动/);
    assert.doesNotMatch(ui.paginationCopyFor(row),/data-load-more|继续向下滚动/);
  }
});
test("R108 changing modes never merges another queue and returning restores its cached cursor",async()=>{
  let calls=0;
  const h=fixture(async(url,options)=>{calls++;const filter=JSON.parse(options.body).matchFilter;return payload(0,0,filter)});
  const tab={key:"fixture",region:"kr",playerRef:"fixture",matchFilter:"all",data:payload(40),nextBegIndex:40,openMatches:new Set(),matchDetailTabs:new Map()};
  tab.data.pagination.hasMore=true;
  const methods=compile(["loadOverview","updateMatchFilter"],{...h.deps,rememberMatchScrollTop:noop,matchObserverKey:()=>"observer",renderFilteredMatchView:noop,autoLoadMatchFilter:()=>{throw Error("must not crawl a server-filtered empty mode")}});
  await methods.updateMatchFilter(tab,"hextech-aram");
  assert.equal(calls,1);assert.equal(tab.data.matches.length,0);assert.equal(tab.nextBegIndex,0);assert.equal(tab.data.pagination.hasMore,false);
  await methods.updateMatchFilter(tab,"all");
  assert.equal(calls,1);assert.equal(tab.data.matches.length,40);assert.equal(tab.nextBegIndex,40);
  await methods.updateMatchFilter(tab,"hextech-aram");
  assert.equal(calls,1);assert.equal(tab.data.matches.length,0);assert.equal(tab.data.pagination.serverFiltered,true);
});
test("R108 special-mode fallback pauses after a bounded number of pages",async()=>{
  let calls=0;const tab={matchFilter:"more:special",data:payload(0)};tab.data.pagination.hasMore=true;
  const {autoLoadMatchFilter}=compile(["autoLoadMatchFilter"],{state:{settings:{matchCount:20}},filteredMatches:()=>[],renderFilteredMatchView:noop,
    loadOverview:async()=>{calls++;tab.nextBegIndex=calls*20;return true},setTimeout,Date});
  await autoLoadMatchFilter(tab);
  assert.equal(calls,3);assert.equal(tab.data.pagination.autoPaused,true);assert.equal(tab.filterPaging,false);
});
