"use strict";
const test=require("node:test");
const assert=require("node:assert/strict");
const fs=require("node:fs");
const path=require("node:path");
const {JSDOM}=require("jsdom");
const {UI_SCALE_STEPS}=require("./ui-scale.cjs");
const web=path.join(__dirname,"..","backend","web");
const appSource=fs.readFileSync(path.join(web,"app.js"),"utf8");
function boot({bridge,preference,migrated,source=appSource}={}) {
  const dom=new JSDOM(fs.readFileSync(path.join(web,"index.html"),"utf8"),{url:"http://127.0.0.1:1/?demo",runScripts:"outside-only",pretendToBeVisual:true});
  const w=dom.window;
  w.IntersectionObserver=class{observe(){}unobserve(){}disconnect(){}};
  w.ResizeObserver=class{observe(){}unobserve(){}disconnect(){}};
  w.matchMedia=()=>({matches:false,addEventListener(){},removeEventListener(){}});
  w.scrollTo=()=>{};w.HTMLElement.prototype.scrollIntoView=()=>{};
  w.Element.prototype.scrollTo=()=>{};
  Object.assign(w,{structuredClone,fetch,Response,Headers,Request});
  require("./license-render-fixture.cjs").installLicenseRenderFixture(w);
  if(bridge)w.desktopScale=bridge;else delete w.desktopScale;
  if(preference)w.localStorage.setItem("lol-loot-ui-scale",preference);
  if(migrated)w.localStorage.setItem("lol-loot-ui-scale-default-auto",migrated);
  for(const file of ["runtime.js","demo-data.js","license-ui.js","app.js"])w.eval(file==="app.js"?source:fs.readFileSync(path.join(web,file),"utf8"));
  w.document.dispatchEvent(new w.Event("DOMContentLoaded"));
  return {w,select:w.document.getElementById("setting-ui-scale"),close(){w.dispatchEvent(new w.CustomEvent("deep-legends:dispose"));w.close();}};
}
const settle=()=>new Promise(resolve=>setImmediate(resolve));
test("appearance scale options exactly match the desktop steps and precede density",async()=>{
  const h=boot();try{
    await settle();
    const values=[...h.select.options].map(option=>option.value);
    assert.equal(values.length,new Set(values).size);
    assert.deepEqual(new Set(values),new Set(["auto",...UI_SCALE_STEPS.map(String)]));
    assert.equal(h.select.closest(".setting-card").nextElementSibling.querySelector("h3").textContent,"界面密度");
  }finally{h.close();}
});
// 缩放做在前端，所以没有桌面外壳（浏览器直连后端预览）时也必须真的缩放：
// 这正是上一版方案的致命缺陷——用户日常就是用浏览器看样式，看不到任何效果。
function resize(w,width,height){
  Object.defineProperty(w,"innerWidth",{value:width,configurable:true});
  Object.defineProperty(w,"innerHeight",{value:height,configurable:true});
  w.dispatchEvent(new w.Event("resize"));
  return new Promise(resolve=>w.requestAnimationFrame(()=>setImmediate(resolve)));
}
test("scaling applies without the desktop bridge and follows the window size",async()=>{
  const verify=async source=>{
    const h=boot({source});
    try{
      await settle();
      assert.equal(h.select.closest(".setting-card").hidden,false,"browser preview must still expose the setting");
      const root=h.w.document.documentElement;
      // jsdom 默认 1024x768，小于基准 → 自动模式不缩小，保持 1。
      assert.equal(root.style.getPropertyValue("--ui-zoom"),"1");
      await resize(h.w,1920,1080);
      assert.equal(root.style.getPropertyValue("--ui-zoom"),"1","1920x1080 是设计基准，必须正好 100%");
      // 连续缩放：不是档位，窗口每宽 1% 倍率就跟着涨 1%。
      await resize(h.w,2560,1440);
      assert.equal(root.style.getPropertyValue("--ui-zoom"),"1.33");
      await resize(h.w,2112,1440);
      assert.equal(root.style.getPropertyValue("--ui-zoom"),"1.1");
      await resize(h.w,3840,2160);
      assert.equal(root.style.getPropertyValue("--ui-zoom"),"2");
      await resize(h.w,1200,900);
      assert.equal(root.style.getPropertyValue("--ui-zoom"),"1","窗口小于基准时只走响应式断点，不压字号");
    }finally{h.close();}
  };
  await verify(appSource);
  // 变异：不写 --ui-zoom / 不监听 resize / 只在有桌面外壳时才缩放，三种都必须被抓住。
  await assert.rejects(()=>verify(appSource.replace('document.documentElement.style.setProperty("--ui-zoom", String(scale));',"")));
  await assert.rejects(()=>verify(appSource.replace('window.addEventListener("resize", scheduleUiScale, { passive: true });',"")));
  await assert.rejects(()=>verify(appSource.replace("  async function setupUiScaleSetting() {","  async function setupUiScaleSetting() {\n    if (!window.desktopScale) return;")));
});
test("desktop preference wins on startup; changes persist and auto labels update the visible custom select",async()=>{
  let receive,unsubscribed=false;const sent=[];
  const h=boot({preference:"auto",bridge:{get:async()=>({mode:"fixed",value:1.75,auto:2}),set:(...args)=>sent.push(args),onChanged:callback=>{receive=callback;return()=>{unsubscribed=true;};}}});
  try{
    assert.equal(sent.length,0,"do not overwrite the shell before get resolves");await settle();
    assert.equal(h.select.closest(".setting-card").hidden,false);assert.equal(h.select.value,"1.75");assert.deepEqual(sent,[["fixed",1.75]]);
    h.select.value="2";h.select.dispatchEvent(new h.w.Event("change",{bubbles:true}));
    assert.equal(h.w.localStorage.getItem("lol-loot-ui-scale"),"2");assert.deepEqual(sent.at(-1),["fixed",2]);
    h.select.value="auto";h.select.dispatchEvent(new h.w.Event("change",{bubbles:true}));
    receive({mode:"auto",value:2,auto:2});
    // 自动档的括号里显示的是"实际生效的档位"，由渲染进程按窗口尺寸算，
    // 不是外壳送来的 payload——否则浏览器预览下这个数字就是假的。
    assert.equal(h.select.options[0].textContent,"自动（当前 100%）");
    assert.equal(h.select._appSelectRoot.querySelector("[data-app-select-trigger] span").textContent,"自动（当前 100%）");
    assert.deepEqual(sent.at(-1),["auto",undefined]);
    await resize(h.w,2560,1440);assert.equal(h.select.options[0].textContent,"自动（当前 133%）");
  }finally{h.close();}
  assert.equal(unsubscribed,true);
});
test("a delayed get cannot undo a user selection made during startup",async()=>{
  let resolve;const sent=[];
  const h=boot({bridge:{get:()=>new Promise(r=>resolve=r),set:(...args)=>sent.push(args),onChanged(){}}});
  try{
    h.select.value="1.75";h.select.dispatchEvent(new h.w.Event("change"));
    resolve({mode:"fixed",value:1,auto:1});await settle();
    assert.equal(h.select.value,"1.75");assert.deepEqual(sent.at(-1),["fixed",1.75]);
  }finally{h.close();}
});

test("R238 browser fixed preference resets once and subsequent fixed choices survive",async()=>{
  const h=boot({preference:"1.25"});
  let preference,migrated;
  try{
    await settle();
    assert.equal(h.select.value,"auto");
    assert.equal(h.w.localStorage.getItem("lol-loot-ui-scale-default-auto"),"1");
    h.select.value="1.25";h.select.dispatchEvent(new h.w.Event("change"));
    preference=h.w.localStorage.getItem("lol-loot-ui-scale");
    migrated=h.w.localStorage.getItem("lol-loot-ui-scale-default-auto");
    assert.equal(preference,"1.25");
  }finally{h.close();}
  const restarted=boot({preference,migrated});
  try{await settle();assert.equal(restarted.select.value,"1.25");}
  finally{restarted.close();}
});

test("R238 shell auto overrides a stale fixed browser preference",async()=>{
  const h=boot({preference:"1.25",bridge:{get:async()=>({mode:"auto",value:1,auto:1}),onChanged(){},set(){}}});
  try{
    await settle();
    assert.equal(h.select.value,"auto");
    assert.equal(h.w.localStorage.getItem("lol-loot-ui-scale"),"auto");
  }finally{h.close();}
});

test("R269 published 225/250 preferences migrate to 200 and retain the migration after restart",async()=>{
 for(const legacy of ["2.25","2.5"]){const h=boot({preference:legacy,migrated:"1"});try{await settle();assert.equal(h.select.value,"2");assert.equal(h.w.localStorage.getItem("lol-loot-ui-scale"),"2");assert.equal(h.w.document.documentElement.style.getPropertyValue("--ui-zoom"),"2");assert(![...h.select.options].some(o=>["2.25","2.5"].includes(o.value)));}finally{h.close()}}
});
