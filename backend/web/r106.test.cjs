"use strict";
const test = require("node:test"), assert = require("node:assert/strict"), fs = require("node:fs"), path = require("node:path");
const { JSDOM } = require("../../desktop/node_modules/jsdom");
const read = name => fs.readFileSync(path.join(__dirname, name), "utf8");
function functionSource(source, name) {
  const start = source.indexOf(`  function ${name}(`);
  assert.ok(start >= 0, name);
  const tail = source.slice(start + 3), next = tail.search(/\n  (?:async )?function /);
  return source.slice(start, next < 0 ? undefined : start + 3 + next);
}
function images(t, kind) {
  const dom = new JSDOM(`<main><figure><img data-${kind}-image data-queued-src="/api/image?path=fixture"></figure></main>`, {url:"http://localhost/",runScripts:"outside-only"});
  const w=dom.window, img=w.document.querySelector("img"); img.loading="lazy";
  let observer;
  w.IntersectionObserver=class {constructor(cb){this.cb=cb;this.targets=[];observer=this} observe(img){this.targets.push(img)} disconnect(){} };
  if(kind==="game") {
    const source=read("gameplay.js"), start=source.indexOf("  function gameImageLoaded("), end=source.indexOf("\n\twindow.deepLegendsGameIcons", start);
    w.eval(source.slice(start,end)+"\nwindow.prepareFixtureImages=prepareImages;");
  } else {
    w.eval(functionSource(read("champions.js"),"prepareImages")+"\nwindow.prepareFixtureImages=prepareImages;");
  }
  t.after(()=>{w.dispatchEvent(new w.Event("deep-legends:dispose"));w.close()});
  return {w,img,observer:()=>observer};
}
for(const kind of ["game","champion"]) test(`R106 ${kind} icons without src remain visible and enter the real lazy queue`,t=>{
  const {w,img,observer}=images(t,kind);
  assert.equal(img.complete,true);assert.equal(img.getAttribute("src"),null);
  w.prepareFixtureImages(w.document.querySelector("main"));assert.equal(img.hidden,false);
  w.eval(read("image-queue.js"));assert.ok(observer().targets.includes(img));
  observer().cb([{target:img,isIntersecting:true}]);assert.equal(img.getAttribute("src"),"/api/image?path=fixture");
  img.hidden=true;img.dispatchEvent(new w.Event("load"));
  assert.equal(img.hidden,false);assert.ok(img.parentElement.classList.contains("has-loaded-image"));
});
test("R106 reopening a successfully loaded icon bypasses cold queue slots and can reattach",t=>{
  const dom=new JSDOM("<main></main>",{url:"http://localhost/",runScripts:"outside-only"}),w=dom.window;
  t.after(()=>{w.dispatchEvent(new w.Event("deep-legends:dispose"));w.close()});
  w.eval(read("image-queue.js"));
  const add=url=>{const img=w.document.createElement("img");w.document.querySelector("main").append(img);w.deepLegendsQueueImage(img,url);return img};
  const first=add("/warm.jpg");first.dispatchEvent(new w.Event("load"));first.remove();
  for(let i=1;i<=6;i++)add(`/cold-${i}.jpg`);
  const queued=add("/cold-7.jpg");assert.equal(queued.getAttribute("src"),null);
  const reopened=add("/warm.jpg");assert.equal(reopened.getAttribute("src"),"/warm.jpg");
  reopened.dispatchEvent(new w.Event("load"));reopened.removeAttribute("src");
  w.deepLegendsQueueImage(reopened,"/warm.jpg");assert.equal(reopened.getAttribute("src"),"/warm.jpg");
});
test("R106 an unset career background selects a real hero without inventing an applied or pending background",()=>{
  const state={facade:{profile:{backgroundSkinId:0},skins:[{id:134001,championId:134,name:"Fixture"},{id:103001,championId:103}]}};
  const source=read("suite.js"), methods=Function("state",functionSource(source,"hydrateFacadeDraft")+functionSource(source,"facadeVisibleSkins")+"return {hydrateFacadeDraft,facadeVisibleSkins}")(state);
  methods.hydrateFacadeDraft();assert.equal(state.facadeDraft.hero,"134");
  assert.equal(methods.facadeVisibleSkins(state.facade.skins,state.facadeDraft).length,1);
  assert.equal(state.facade.profile.backgroundSkinId,0,"draft must not mutate actual background");
  assert.equal(state.facadeDraft.skinId,0,"browsing a hero must not create a pending skin selection");
  state.facade.profile.backgroundSkinId=103001;methods.hydrateFacadeDraft(true);
  assert.equal(state.facadeDraft.hero,"103");assert.equal(state.facadeDraft.skinId,103001);
});
test("R106 only ranked accounts start expanded and timestamps show only last match start",async t=>{
  const dom=new JSDOM(read("index.html"),{url:"http://localhost/",runScripts:"outside-only",pretendToBeVisual:true}),w=dom.window;
  t.after(()=>w.close());
  const account=(gameName,rankStatus,extra={})=>({gameName,tagLine:"KR1",rankStatus,...extra});
  w.fetch=async()=>({ok:true,json:async()=>({teams:[{code:"IG",name:"IG",players:[{key:"ig/test",name:"Fixture",accounts:[account("Ranked","ranked",{tier:"DIAMOND",lp:75,checkedAt:"2026-09-17T13:00:00Z",lastMatchAt:"2026-09-16T10:00:00Z",lastMatchAtKnown:true}),account("Unranked","unranked"),account("Failed","unavailable",{checkedAt:"2026-09-17T13:00:00Z",checkFailed:true})]}]}]})});
  w.eval(read("pro-players.js"));w.dispatchEvent(new w.CustomEvent("deep-legends:section",{detail:{name:"pro-players"}}));await new Promise(setImmediate);
  assert.equal(w.document.querySelectorAll("[data-pro-account]").length,1);
  assert.match(w.document.querySelector("[data-pro-account]").textContent,/最近对局/);
  w.document.querySelector("[data-pro-history]").click();
  assert.equal(w.document.querySelectorAll("[data-pro-account]").length,3);
  assert.doesNotMatch(w.document.querySelector("#pro-players-content").textContent,/已检查|检查未成功/);
});
