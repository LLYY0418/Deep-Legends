"use strict";
const test=require("node:test"), assert=require("node:assert/strict"), fs=require("node:fs"), path=require("node:path");
const {JSDOM}=require("../../desktop/node_modules/jsdom");
const source=fs.readFileSync(process.env.R101_SUITE_SOURCE || path.join(__dirname,"suite.js"),"utf8");
function functionSource(script, name) {
  const marker = `function ${name}(`;
  const start = script.indexOf(marker);
  assert.notEqual(start, -1, `missing ${name}`);
  const bodyStart = script.indexOf("{", script.indexOf(")", start));
  let depth = 0;
  let quote = "";
  let escaped = false;
  for (let index = bodyStart; index < script.length; index += 1) {
    const char = script[index];
    if (quote) {
      if (escaped) escaped = false;
      else if (char === "\\") escaped = true;
      else if (char === quote) quote = "";
      continue;
    }
    if (char === '"' || char === "'" || char === "`") { quote = char; continue; }
    if (char === "{") depth += 1;
    if (char === "}" && --depth === 0) return script.slice(start, index + 1);
  }
  assert.fail(`unbalanced ${name}`);
}

function compile(names, dependencies = {}, script = source) {
  const keys = Object.keys(dependencies);
  return Function(...keys, `"use strict";\n${names.map((name) => functionSource(script, name)).join("\n")}\nreturn {${names.join(",")}};`)(...keys.map((key) => dependencies[key]));
}

const escapeHTML = (value) => String(value ?? "").replace(/[&<>'"]/g, "_");


const checked=value=>value?" checked":"";
function modalHarness(){
 const dom=new JSDOM('<main id="facade"><button data-facade-icons>头像</button><button data-facade-banners>旗帜</button></main>',{pretendToBeVisual:true});
 const {window}=dom,{document}=window;window.HTMLDialogElement.prototype.showModal=function(){this.open=true};window.HTMLDialogElement.prototype.close=function(){this.open=false;this.dispatchEvent(new window.Event("close"))};
 const state={facade:{connected:true,summoner:{profileIconId:1}},facadeDraft:{}};const frames=[];const roots={facade:document.querySelector("main")};
 const deps={window,document,state,roots,escapeHTML,imageURL:x=>x,performance,requestAnimationFrame:cb=>frames.push(cb)};
 return {dom,...deps,frames,flush(){while(frames.length)frames.shift()()}};
}
// R105 supersedes the owned-only UI contract; keep the R101 modal/catalog checks.
test("R101 icon catalog remains available with known or unknown ownership",async()=>{
 for(const unknown of [false,true]){
  const h=modalHarness();const deps={...h,api:async()=>({icons:[{id:1,title:"头像",owned:false}],iconOwnershipUnavailable:unknown}),applyFacade:async()=>true};
  const open=Function(...Object.keys(deps),`${functionSource(source,"facadeIconImage")} async ${functionSource(source,"openFacadeIconPicker")};return openFacadeIconPicker`)(...Object.values(deps));
  await open(h.document.querySelector("button"));h.flush();const d=h.document.querySelector("dialog");
  assert.equal(d.querySelector("[data-picker-owned]"),null);assert.equal(d.querySelector("[data-picker-icon]").disabled,false);d.close();h.dom.window.close();
 }
});
test("R101 banner catalog keeps localized grouping and restores focus",async()=>{
 const h=modalHarness();const deps={...h,api:async()=>({banners:[{id:"4",localizedName:"乙",owned:false,isTencentOnly:true},{id:"3",localizedName:"甲",owned:true,isTencentOnly:false}]}),applyFacade:async()=>true};
 const open=Function(...Object.keys(deps),`async ${functionSource(source,"openFacadeBannerPicker")};return openFacadeBannerPicker`)(...Object.values(deps));
 await open(h.document.querySelector("[data-facade-banners]"));const d=h.document.querySelector("dialog");
 assert.equal(d.querySelector('[data-banner-id="4"]').disabled,false);assert.match(d.textContent,/国服专属/);
 d.querySelector('[data-banner-group="tencent"]').click();assert.equal(d.querySelectorAll("[data-banner-id]").length,1);
 d.close();h.flush();assert.equal(h.document.activeElement,h.document.querySelector("[data-facade-banners]"));h.dom.window.close();
});
function champHarness(){
 const dom=new JSDOM('<main></main>');const root=dom.window.document.querySelector("main");const config={ban:{enabled:true,strategy:"show-then-lock",lockDelayMs:10000},pick:{enabled:true,strategy:"show-then-lock",lockDelayMs:10000},bench:{}};const saved=[];
 const deps={state:{champSelectCatalog:{}},checked,escapeHTML,champSelectPoolFor:()=>[],champSelectLaneTabsHTML:()=>"",champSelectSlotsHTML:()=>"",champSelectSettings:()=>({enabled:true}),champSelectSelectedConfig:()=>config,champSelectPanelRoot:()=>root,bindChampSelectRailDrag(){},toast(){},saveChampSelect:async()=>{saved.push(structuredClone(config));render()},renderChampSelect:()=>render()};
 const helpers=compile(["renderChampSelectSideCard","champSelectTimeInputHTML","champSelectSideName","champSelectStrategyHTML","bindChampSelectTimeInputs","bindChampSelectControls"],deps);
 function render(){root.innerHTML=["ban","pick"].map(side=>helpers.renderChampSelectSideCard(side,{groupId:"ranked",hasBan:true,banLimit:5,pickLimit:5},config,{})).join("");helpers.bindChampSelectControls()}
 render();return{dom,root,config,saved,render};
}
test("R101 lock wait stays visible and disabled in all strategies",()=>{
 const h=champHarness();for(const strategy of ["show-only","lock-now","show-then-lock"]){for(const side of ["ban","pick"])h.config[side].strategy=strategy;h.render();for(const side of ["ban","pick"]){const card=h.root.querySelector(`.is-${side}`),input=card.querySelector('[data-cs-time="lock"]');assert.ok(input);assert.equal(input.disabled,strategy!=="show-then-lock");assert.equal(input.value,strategy==="show-then-lock"?"10":"0");for(const b of card.querySelectorAll("[data-cs-delay]"))assert.equal(b.disabled,input.disabled)}}h.dom.window.close();
});
test("R101 strategy switch never saves a zero lock wait, even synthetic disabled change",async()=>{
 const h=champHarness();for(const side of ["ban","pick"]){h.root.querySelector(`[data-cs-strategy="${side}"][data-cs-strategy-value="show-only"]`).click();await new Promise(setImmediate);let input=h.root.querySelector(`.is-${side} [data-cs-time="lock"]`);assert.ok(input);input.value="0";input.dispatchEvent(new h.dom.window.Event("change"));await new Promise(setImmediate);assert.equal(h.config[side].lockDelayMs,10000);h.root.querySelector(`[data-cs-strategy="${side}"][data-cs-strategy-value="show-then-lock"]`).click();await new Promise(setImmediate);assert.equal(h.root.querySelector(`.is-${side} [data-cs-time="lock"]`).value,"10")};assert.ok(h.saved.length>=4);for(const save of h.saved)for(const side of ["ban","pick"])assert.equal(save[side].lockDelayMs,10000);h.dom.window.close();
});
test("R101 facade moves cards left and removes only obsolete explanation and accent",()=>{
 const dom=new JSDOM('<main></main>');const root=dom.window.document.querySelector("main");const state={facade:{connected:true,summoner:{profileIconId:71},skins:[]}};
 const helpers=compile(["hydrateFacadeDraft","renderFacade","facadeIconImage"],{state,roots:{facade:root},escapeHTML,imageURL:x=>x,checked,selected:()=>"",facadeVisibleSkins:()=>[],facadeTitleText:()=>"头衔",facadeTitleFilled:()=>true,facadeIdentityDirty:()=>false,facadeResetDirty:()=>false,rankLabel:()=>"大师",facadeChallengeSlots:()=>"",facadeFilmHTML:()=>"",rankOptions:()=>"",bindFacadeControls(){}});helpers.renderFacade();
 assert.deepEqual([...root.querySelector(".facade-left").children].map(e=>[...e.classList].at(-1)),["facade-preview","facade-icon-card","facade-banner-card","facade-write-card"]);assert.ok(root.querySelector(".facade-icon-card .icon-current img"));assert.equal(root.textContent.includes("这是好友悬浮卡与生涯页的示意预览"),false);assert.match(root.querySelector(".facade-commit").textContent,/改动只在左侧预览，确认后才写入客户端。/);assert.equal(root.querySelector('[data-facade-clear="previous-banner"]'),null);assert.equal(root.textContent.includes("挑战旗帜配色"),false);assert.ok(root.querySelector(".facade-controls > .facade-chat-card"));assert.ok(root.querySelector(".facade-controls > .facade-background-card"));dom.window.close();
});
