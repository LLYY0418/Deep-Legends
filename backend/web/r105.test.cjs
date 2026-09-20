"use strict";
const test = require("node:test"), assert = require("node:assert/strict"), fs = require("node:fs"), path = require("node:path");
const { JSDOM } = require("../../desktop/node_modules/jsdom");
const suite = fs.readFileSync(process.env.R105_SUITE_SOURCE || path.join(__dirname, "suite.js"), "utf8");
function fn(name) {
  const start = suite.indexOf(`  ${["openFacadeIconPicker","openFacadeBannerPicker","applyFacade"].includes(name) ? "async " : ""}function ${name}(`);
  assert.ok(start >= 0, name);
  const tail = suite.slice(start + 3), next = tail.search(/\n  (?:async )?function /);
  return suite.slice(start, next < 0 ? undefined : start + 3 + next);
}
function harness({ unknown = false, reject = false } = {}) {
  const dom = new JSDOM('<main><button data-facade-icons>头像</button><button data-facade-banners>旗帜</button></main>', { pretendToBeVisual:true });
  const { window }=dom, { document }=window;
  window.HTMLDialogElement.prototype.showModal=function(){this.open=true};
  window.HTMLDialogElement.prototype.close=function(){this.open=false;this.dispatchEvent(new window.Event("close"))};
  const state={facade:{connected:true,summoner:{profileIconId:1}},facadeDraft:{}}, writes=[], toasts=[], frames=[];
  const api=async(url,options)=>{
    if(url==="/api/facade/icons") return {icons:[{id:1,title:"初始",owned:true},{id:2,title:"星光",owned:false,disabled:true,searchTerms:["xingguang","xg"]}],iconOwnershipUnavailable:unknown};
    if(url==="/api/facade/banners") return {banners:[{id:"3",localizedName:"初始",owned:true},{id:"4",localizedName:"星光旗帜",owned:false,imagePath:"/lol-game-data/assets/ASSETS/Regalia/BannerSkins/lny2023.png"}],bannerOwnershipUnavailable:unknown};
    assert.equal(url,"/api/facade/apply"); assert.equal(options.method,"POST");
    const body=JSON.parse(options.body); writes.push(body);
    if(reject) throw new Error("LCU 401 RPC_ERROR");
    return {connected:true,summoner:{profileIconId:body.iconId || 1}};
  };
  const deps={window,document,state,roots:{facade:document.querySelector("main")},api,toast:x=>toasts.push(x),hydrateFacadeDraft(){},renderFacade(){},escapeHTML:s=>String(s).replace(/[<>&"']/g,"_"),imageURL:x=>x,performance,requestAnimationFrame:cb=>frames.push(cb)};
  const methods=Function(...Object.keys(deps),["facadeIconImage","openFacadeIconPicker","openFacadeBannerPicker","applyFacade"].map(fn).join("\n")+"\nreturn {openFacadeIconPicker,openFacadeBannerPicker}")(...Object.values(deps));
  return {dom,window,document,state,writes,toasts,...methods,flush(){while(frames.length)frames.shift()()}};
}
const settle=()=>new Promise(setImmediate);

test("R105 all icons are selectable and only confirmation reaches the write route",async()=>{
  for(const unknown of [false,true]) {
    const h=harness({unknown}); try {
      await h.openFacadeIconPicker(h.document.querySelector("button"));h.flush();
      const d=h.document.querySelector("dialog"); assert.equal(d.className,"facade-picker");
      assert.equal(d.querySelector('[data-picker-owned], [data-picker-group="owned"]'),null);
      assert.equal(d.querySelectorAll("[data-picker-icon]").length,2);
      for(const cell of d.querySelectorAll("[data-picker-icon]")) assert.equal(cell.disabled,false,"some icons are disabled");
      assert.match(d.textContent,/未拥有头像可用于聊天与好友栏/);
      const search=d.querySelector("[data-picker-search]"); search.value="xg"; search.dispatchEvent(new h.window.Event("input"));
      await new Promise(r=>setTimeout(r,170));h.flush();
      assert.equal(d.querySelectorAll("[data-picker-icon]").length,1,"pinyin search lost");
      d.querySelector('[data-picker-icon="2"]').click();await settle();
      assert.equal(h.writes.length,0,"selection must only preview"); assert.equal(d.open,true);
      d.querySelector("[data-picker-apply]").click();await settle();
      assert.deepEqual(h.writes,[{action:"icon",iconId:2}]);assert.equal(h.document.querySelector("dialog"),null);assert.deepEqual(h.toasts,["头像已应用"]);
    } finally {h.dom.window.close()}
  }
});
test("R105 all banners are selectable and a click reaches the write route",async()=>{
  for(const unknown of [false,true]) {
    const h=harness({unknown});try {
      await h.openFacadeBannerPicker(h.document.querySelector("[data-facade-banners]"));
      const d=h.document.querySelector("dialog");assert.ok(d.classList.contains("facade-banner-picker"));
      assert.ok(d.querySelector('[data-banner-group="owned"]'));assert.equal(d.querySelectorAll("[data-banner-id]").length,2);
      for(const cell of d.querySelectorAll("[data-banner-id]")) assert.equal(cell.disabled,false,"some banners are disabled");
      assert.match(d.textContent,/未拥有旗帜可能被客户端拒绝装备/);assert.ok(d.querySelector(".facade-banner-picture[data-queued-src]"));
      d.querySelector('[data-banner-group="owned"]').click();
      assert.deepEqual([...d.querySelectorAll("[data-banner-id]")].map(x=>x.dataset.bannerId),["3"]);
      d.querySelector('[data-banner-group="all"]').click();
      assert.equal(d.querySelectorAll("[data-banner-id]").length,2);
      const search=d.querySelector("[data-banner-search]");assert.ok(search);
      search.value="星光";search.dispatchEvent(new h.window.Event("input"));assert.equal(d.querySelectorAll("[data-banner-id]").length,1);
      assert.doesNotMatch(d.querySelector("[data-banner-id]").textContent,/点击更改/);
      d.querySelector('[data-banner-id="4"]').click();await settle();
      assert.deepEqual(h.writes,[{action:"banner",bannerId:"4"}]);assert.equal(h.document.querySelector("dialog"),null);assert.deepEqual(h.toasts,["旗帜已应用"]);
    } finally {h.dom.window.close()}
  }
});
test("R105 write failures keep both pickers open and show a retryable toast",async()=>{
  for(const kind of ["Icon","Banner"]) {
    const h=harness({reject:true});try {
      await h[`openFacade${kind}Picker`](h.document.querySelector("button"));h.flush();
      const d=h.document.querySelector("dialog"), selector=kind==="Icon"?'[data-picker-icon="2"]':'[data-banner-id="4"]';
      d.querySelector(selector).click();if(kind === "Icon") d.querySelector("[data-picker-apply]").click();await settle();assert.equal(h.writes.length,1);assert.equal(d.open,true);
      assert.match(h.toasts[0],/401 RPC_ERROR/);assert.equal(d.querySelector(selector).disabled,false);
      d.querySelector(selector).click();if(kind === "Icon") d.querySelector("[data-picker-apply]").click();await settle();assert.equal(h.writes.length,2);
    } finally {h.dom.window.close()}
  }
});
test("R105 all 53 reviewed accounts remain available after expanding unranked groups",async()=>{
  const doc=fs.readFileSync(path.join(__dirname,"../../docs/pro-accounts-verification-2026-09-17.md"),"utf8");
  const teams=[];let team;
  for(const line of doc.split("\n")) {
    const heading=line.match(/^## (BLG|IG|T1|HLE|GEN|DK)（/);
    if(heading){team={code:heading[1],name:heading[1],players:[]};teams.push(team);continue}
    if(line.startsWith("## "))team=null;
    if(!team || !line.startsWith("|"))continue;
    const columns=line.split("|"), id=columns[3]?.match(/`([^`]+)`/);if(!id)continue;
    const name=columns[1].trim().replaceAll("*","");let player=team.players.find(x=>x.name===name);
    if(!player){player={key:team.code+"/"+name,name,position:"top",accounts:[]};team.players.push(player)}
    const [gameName,tagLine]=id[1].split("#");player.accounts.push({gameName,tagLine,reviewed:true,dormant:true,rankStatus:"unavailable"});
  }
  const dom=new JSDOM(fs.readFileSync(path.join(__dirname,"index.html"),"utf8"),{url:"http://localhost/",runScripts:"outside-only",pretendToBeVisual:true});
  try {
    const w=dom.window;w.fetch=async()=>({ok:true,json:async()=>({teams,playerCount:33,accountCount:53})});
    w.eval(fs.readFileSync(process.env.R105_PRO_SOURCE || path.join(__dirname,"pro-players.js"),"utf8"));
    w.dispatchEvent(new w.CustomEvent("deep-legends:section",{detail:{name:"pro-players"}}));await settle();
    assert.equal(w.document.querySelectorAll("[data-pro-account]").length,0,"unknown accounts should start folded");
    const keys=[...w.document.querySelectorAll("[data-pro-history]")].map(x=>x.dataset.proHistory);
    for(const key of keys) [...w.document.querySelectorAll("[data-pro-history]")].find(x=>x.dataset.proHistory===key).click();
    const buttons=w.document.querySelectorAll("[data-pro-account]");assert.equal(buttons.length,53);
    for(const team of teams) for(const player of team.players) for(const a of player.accounts) assert.ok([...buttons].some(b=>b.dataset.proAccount===`${player.key}:${a.gameName}#${a.tagLine}`),`${player.name} missing ${a.gameName}`);
    assert.equal(w.document.querySelectorAll(".pro-player-name").length,33);
  } finally {dom.window.close()}
});
