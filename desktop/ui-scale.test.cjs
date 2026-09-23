"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const { EventEmitter } = require("node:events");
const scale = require("./ui-scale.cjs");
const scaleSource = fs.readFileSync(path.join(__dirname, "ui-scale.cjs"), "utf8");
const mainSource = fs.readFileSync(path.join(__dirname, "main.cjs"), "utf8");
const storedScalePath = path.join("/user-data", "ui-scale.json");
const cases = [[1366,768,1],[1600,900,1],[1920,1080,1],[2304,1318,1.2],[2560,1440,1.33],[2560,1600,1.33],[3440,1440,1.6],[3840,2160,2],[3840,1600,1.78],[5120,2880,2.5],[7680,4320,2.5],[1024,600,1]];
for (const [width,height,expected] of cases) test(`auto ${width}x${height} = ${expected}`, () => {
  assert.equal(scale.autoScaleFor({width,height}), expected);
});
function loadScale(source) {
  const module = {exports:{}};
  vm.runInNewContext(source, {module});
  return module.exports;
}
test("continuous scale, quantization, clamps and manual-step normalization", () => {
  assert.deepEqual(scale.UI_SCALE_STEPS, [0.9,1,1.1,1.25,1.4,1.5,1.75,2,2.25,2.5]);
  assert.equal(scale.UI_SCALE_BASE_WIDTH, 1920);
  assert.equal(scale.UI_SCALE_BASE_HEIGHT, 900);
  assert.equal(scale.UI_SCALE_MIN, 1);
  assert.equal(scale.UI_SCALE_MAX, 2.5);
  // ★真正执行缩放的是 backend/web/app.js，两边的常量必须逐字一致。
  const appSource = fs.readFileSync(path.join(__dirname, "..", "backend", "web", "app.js"), "utf8");
  assert.match(appSource, /const UI_SCALE_STEPS = \[0\.9, 1, 1\.1, 1\.25, 1\.4, 1\.5, 1\.75, 2, 2\.25, 2\.5\];/);
  assert.match(appSource, /const UI_SCALE_BASE_WIDTH = 1920;/);
  assert.match(appSource, /const UI_SCALE_BASE_HEIGHT = 900;/);
  assert.match(appSource, /const UI_SCALE_MIN = 1;/);
  assert.match(appSource, /const UI_SCALE_MAX = 2\.5;/);
  // 连续：等比放大的窗口拿到等比的倍率，没有档位台阶。
  for (const k of [1, 1.1, 1.25, 1.5, 2, 2.5]) assert.equal(scale.autoScaleFor({width:1920*k,height:900*k}), k);
  // 设置页可选的手动档位，凡是 >= 下限的都必须是自动模式能自然到达的值；
  // 0.9 是只给手动锁档用的，自动模式到不了。
  for (const step of scale.UI_SCALE_STEPS) assert.equal(scale.autoScaleFor({width:1920*step,height:900*step}), Math.max(step, scale.UI_SCALE_MIN));
  assert.equal(scale.autoScaleFor({width:1728,height:810}), 1, "自动模式只放大不缩小");
  // 宽高谁紧就听谁的。
  assert.equal(scale.autoScaleFor({width:3840,height:1600}), 1.78);
  assert.equal(scale.autoScaleFor({width:2000,height:4000}), 1.04);
  // 量化到 1%：不足 1% 的窗口变化不触发重排。
  assert.equal(scale.autoScaleFor({width:1929,height:10000}), 1);
  assert.equal(scale.autoScaleFor({width:1939,height:10000}), 1.01);
  // 上下限。
  assert.equal(scale.autoScaleFor({width:100,height:100}), 1);
  assert.equal(scale.autoScaleFor({width:99999,height:99999}), 2.5);
  assert.equal(scale.autoScaleFor(), 1);
  for (const step of scale.UI_SCALE_STEPS) {
    assert.equal(scale.normalizeScale(step), step);
    assert.equal(scale.normalizeScale(String(step)), step);
  }
  for (const value of ["auto", "", "bad", null, undefined, {}, NaN, Infinity]) assert.equal(scale.normalizeScale(value), "auto");
  for (const [value,expected] of [[-1,0.9],[0,0.9],[99,2.5],[1.04,1],[1.2,1.25],[1.32,1.25]]) assert.equal(scale.normalizeScale(value),expected);
  // 手动档位仍然允许 90%（把界面调小换更多内容），只是自动模式不会选它。
  assert.equal(scale.normalizeScale(0.9), 0.9);
});
test("scale mutants turn the corresponding geometry guards red", () => {
  const widthOnly = loadScale(scaleSource.replace("Math.min(Number(width) / UI_SCALE_BASE_WIDTH, Number(height) / UI_SCALE_BASE_HEIGHT)", "Number(width) / UI_SCALE_BASE_WIDTH"));
  for (const [width,height,expected] of [[3440,1440,1.6],[3840,1600,1.78]]) assert.throws(()=>assert.equal(widthOnly.autoScaleFor({width,height}),expected));
  // 基准改坏了，1920x1080 这条基线用例自己就会红。
  const wrongBase = loadScale(scaleSource.replace("UI_SCALE_BASE_WIDTH = 1920", "UI_SCALE_BASE_WIDTH = 1600"));
  assert.throws(()=>assert.equal(wrongBase.autoScaleFor({width:1920,height:1080}),1));
  const noFloor = loadScale(scaleSource.replace("Math.max(UI_SCALE_MIN, Math.round(raw * 100) / 100)", "Math.round(raw * 100) / 100"));
  assert.throws(()=>assert.equal(noFloor.autoScaleFor({width:1766,height:1010}),1));
  const noCeiling = loadScale(scaleSource.replace("Math.min(UI_SCALE_MAX, ", "((x)=>x)("));
  assert.throws(()=>assert.equal(noCeiling.autoScaleFor({width:99999,height:99999}),2.5));
  const noQuantize = loadScale(scaleSource.replace("Math.round(raw * 100) / 100", "raw"));
  assert.throws(()=>assert.equal(noQuantize.autoScaleFor({width:3840,height:1600}),1.78));
});

function harness({source=mainSource, stored, width=1920, height=1080}={}) {
  const files = new Map(stored === undefined ? [] : [[storedScalePath, stored]]);
  const writes=[], windows=[], zoom=[], minimum=[], overlays=[], timers=new Map();
  let now=0, seq=0;
  const ipcMain = new EventEmitter();
  ipcMain.handlers=new Map();
  ipcMain.handle=(key,fn)=>ipcMain.handlers.set(key,fn);
  ipcMain.removeHandler=key=>ipcMain.handlers.delete(key);
  const nativeTheme=new EventEmitter();
  const screen=new EventEmitter();
  screen.getAllDisplays=()=>[];
  screen.getPrimaryDisplay=()=>({workAreaSize:{width:1920,height:1080}});
  const app=new EventEmitter();
  Object.assign(app,{setAppUserModelId(){},requestSingleInstanceLock:()=>true,whenReady:()=>({then(){}}),getPath:()=>"/user-data",quit(){},isPackaged:true});
  class BrowserWindow extends EventEmitter {
    constructor(options) {
      super(); this.options=options;this.bounds={x:0,y:0,width,height};this.destroyed=false;
      this.url="http://127.0.0.1:8787/";
      this.webContents=new EventEmitter();
      Object.assign(this.webContents,{session:{},isDestroyed:()=>this.destroyed,getURL:()=>this.url,setZoomFactor:value=>zoom.push(value),send:(...args)=>this.sent.push(args),setWindowOpenHandler(){}});
      this.sent=[];windows.push(this);
    }
    isDestroyed(){return this.destroyed;}
    getContentBounds(){return this.bounds;}
    getBounds(){return {...this.bounds,width:this.bounds.width+30,height:this.bounds.height+60};}
    isMaximized(){return false;}
    setMinimumSize(...size){minimum.push(size);}
    setTitleBarOverlay(overlay){overlays.push(overlay);}
    setBackgroundColor(){}
    loadURL(){return Promise.resolve();}
  }
  const electron={app,BrowserWindow,ipcMain,nativeTheme,screen,session:{},shell:{},dialog:{}};
  const fakeFs={
    statSync(file){if(!files.has(file))throw Error("missing");return {size:Buffer.byteLength(files.get(file))};},
    readFileSync(file){return files.get(file);},mkdirSync(){},appendFileSync(){},
    writeFileSync(file,data){writes.push(file);files.set(file,data);},
    renameSync(from,to){files.set(to,files.get(from));files.delete(from);},
  };
  const context=vm.createContext({
    require(name){
      if(name==="electron")return electron;
      if(name==="node:fs")return fakeFs;
      if(name==="./diagnostics-export.cjs")return {attachDiagnosticsExport:()=>()=>{}};
      if(name==="./share-export.cjs")return {createShareExportController:()=>({clear(){}})};
      if(name==="./window-bounds-store.cjs")return {readWindowBounds:()=>null,writeWindowBounds(){}};
      if(name.startsWith("./"))return require(name);
      return require(name);
    },__dirname,process:{on(){},platform:"win32",env:{}},URL,Buffer,console,
    setTimeout(fn,delay){const id=++seq;timers.set(id,{fn,time:now+delay});return id;},
    clearTimeout(id){timers.delete(id);},setImmediate(fn){fn();},
  });
  vm.runInContext(source+`\nglobalThis.probe={titleBarOverlay,applyUiScale,boot(){backendReady={baseUrl:"http://127.0.0.1:8787",bootstrapUrl:"http://127.0.0.1:8787/"};createMainWindow();}};`,context);
  context.probe.boot();
  // 外壳不再调用 setZoomFactor（真正的缩放在渲染进程的 CSS zoom 上），
  // 所以档位序列改从 setMinimumSize 的调用反推：每次跨档都会写一次 [780*Z, 600*Z]。
  return {window:windows[0],files,writes,zoom,minimum,overlays,ipcMain,nativeTheme,screen,probe:context.probe,
    scales(){return minimum.map(([w])=>Math.round(w / 780 * 100) / 100);},resetScales(){minimum.length=0;},
    tick(ms){now+=ms;for(const [id,timer] of timers)if(timer.time<=now){timers.delete(id);timer.fn();}},
    get(){return JSON.parse(JSON.stringify(ipcMain.handlers.get("desktop-scale-get")({sender:windows[0].webContents})));},
    set(mode,value){ipcMain.emit("desktop-scale-set",{sender:windows[0].webContents},mode,value);},
  };
}

test("titlebar, minimum size, reload, resize and display events run through the real main module", () => {
  const h=harness();
  assert.equal(h.probe.titleBarOverlay("dark",false,2).height,112);
  assert.equal(h.probe.titleBarOverlay("dark",false,1.25).height,70);
  assert.equal(h.probe.titleBarOverlay("light",true,2).height,112);
  h.window.webContents.emit("did-finish-load");
  assert.deepEqual(h.scales(),[1]);assert.deepEqual(h.minimum,[[780,600]]);
  assert.deepEqual(h.zoom,[],"the shell must never zoom the page itself");
  h.resetScales();
  for(let i=0;i<20;i++){h.window.emit("resize");h.tick(10);}
  h.tick(299);assert.deepEqual(h.scales(),[]);
  h.tick(1);assert.deepEqual(h.scales(),[]);
  h.window.bounds={width:3840,height:2160};h.window.emit("resize");
  h.tick(299);assert.deepEqual(h.scales(),[]);h.tick(1);
  assert.deepEqual(h.scales(),[2]);assert.deepEqual(h.minimum.at(-1),[1560,1200]);assert.equal(h.overlays.at(-1).height,112);
  h.window.webContents.emit("did-finish-load");h.window.webContents.emit("did-finish-load");
  assert.deepEqual(h.scales(),[2,2,2],"reload must reapply even at the same step");
  h.window.bounds={width:2560,height:1440};h.screen.emit("display-metrics-changed");
  assert.equal(h.scales().at(-1),1.33);
  h.nativeTheme.emit("updated",{notAScale:true});assert.equal(h.overlays.at(-1).height,74);
  h.ipcMain.emit("desktop-modal",{},true);assert.equal(h.overlays.at(-1).height,74);
  // 渲染进程汇报的倍率是权威：外壳按它同步标题栏与最小尺寸。
  h.ipcMain.emit("desktop-scale-applied",{sender:h.window.webContents},2);
  assert.equal(h.scales().at(-1),2);assert.equal(h.overlays.at(-1).height,112);
  h.ipcMain.emit("desktop-scale-applied",{sender:{isDestroyed:()=>false,getURL:()=>"https://untrusted.invalid/"}},2.5);
  assert.equal(h.scales().at(-1),2,"untrusted senders cannot move the shell chrome");
  // 必须用 getContentBounds：getBounds 含边框（+30/+60），会把倍率算大。
  h.window.bounds={width:1920,height:1080};h.screen.emit("display-metrics-changed");assert.equal(h.scales().at(-1),1);
  h.window.emit("closed");assert.equal(h.screen.listenerCount("display-metrics-changed"),0);
  assert.equal(h.nativeTheme.listenerCount("updated"),0);assert.equal(h.ipcMain.listenerCount("desktop-scale-set"),0);
});

test("IPC restores independent preferences, normalizes fixed values and rejects untrusted senders", () => {
  const h=harness({stored:JSON.stringify({mode:"fixed",value:2})});
  h.window.webContents.emit("did-finish-load");assert.deepEqual(h.get(),{mode:"fixed",value:2,auto:1});
  h.set("fixed",1.6);assert.equal(h.get().value,1.5);
  assert.deepEqual(JSON.parse(h.files.get(storedScalePath)),{mode:"fixed",value:1.5});
  assert.ok(h.writes.every(file=>file.endsWith("ui-scale.json.tmp")));
  const count=h.writes.length;h.set("fixed",1.5);assert.equal(h.writes.length,count);
  h.window.bounds={width:3840,height:2160};h.screen.emit("display-metrics-changed");assert.equal(h.get().value,1.5);assert.equal(h.get().auto,2);
  h.set("auto");assert.equal(h.get().value,2);assert.equal(h.get().mode,"auto");
  h.window.url="https://untrusted.invalid/";h.set("fixed",2.5);
  assert.equal(h.get(),null);h.window.url="http://127.0.0.1:8787/";assert.equal(h.get().mode,"auto");assert.equal(h.get().value,2);
  h.ipcMain.emit("desktop-scale-set",{sender:{isDestroyed:()=>false,getURL:()=>h.window.url}},"fixed",2.5);
  assert.equal(h.get().mode,"auto");assert.equal(h.get().value,2);
  h.set("invalid",2);h.set("fixed","broken");assert.equal(h.get().mode,"auto");
  for(const stored of ["invalid", "x".repeat(1025), JSON.stringify({mode:"fixed",value:"bad"})])assert.equal(harness({stored}).get().mode,"auto");
  const restored=harness({stored:h.files.get(storedScalePath)});assert.equal(restored.get().mode,"auto");
});

test("main behavior guards reject titlebar, minimum size, trust, reload, debounce and export mutants", () => {
  function titleGuard(source){const h=harness({source});assert.equal(h.probe.titleBarOverlay("dark",false,2).height,112);}
  function minimumGuard(source){const h=harness({source});h.set("fixed",2);assert.deepEqual(h.minimum.at(-1),[1560,1200]);}
  function trustGuard(source){const h=harness({source});h.window.url="https://untrusted.invalid/";h.set("fixed",2);h.window.url="http://127.0.0.1:8787/";assert.equal(h.get().mode,"auto");assert.equal(h.writes.length,0);}
  function reloadGuard(source){const h=harness({source});h.window.webContents.emit("did-finish-load");h.window.webContents.emit("did-finish-load");assert.deepEqual(h.scales(),[1,1]);}
  function resizeGuard(source){const h=harness({source});h.window.webContents.emit("did-finish-load");h.resetScales();for(let i=0;i<20;i++)h.window.emit("resize");h.tick(300);assert.equal(h.scales().length,0);}
  function appliedGuard(source){const h=harness({source});h.ipcMain.emit("desktop-scale-applied",{sender:{isDestroyed:()=>false,getURL:()=>"https://untrusted.invalid/"}},2);assert.deepEqual(h.scales(),[]);}
  function noPageZoomGuard(source){const h=harness({source});h.window.webContents.emit("did-finish-load");h.set("fixed",2);assert.deepEqual(h.zoom,[],"the shell must not zoom the page");}
  for(const guard of [titleGuard,minimumGuard,trustGuard,reloadGuard,resizeGuard,appliedGuard,noPageZoomGuard])guard(mainSource);
  assert.throws(()=>appliedGuard(mainSource.replace('ipcMain.on("desktop-scale-applied", (event, scale) => {\n    if (!isTrustedRenderer(event.sender) || event.sender !== mainWindow?.webContents) return;','ipcMain.on("desktop-scale-applied", (event, scale) => {')));
  assert.throws(()=>noPageZoomGuard(mainSource.replace("  currentUiScale = scale;\n  syncTitleBar(scale);","  currentUiScale = scale;\n  window.webContents.setZoomFactor(scale);\n  syncTitleBar(scale);")));
  assert.throws(()=>titleGuard(mainSource.replace("Math.round(56 * scale)","56")));
  assert.throws(()=>minimumGuard(mainSource.replace("window.setMinimumSize(Math.round(780 * scale), Math.round(600 * scale));","")));
  assert.throws(()=>trustGuard(mainSource.replace('ipcMain.on("desktop-scale-set", (event, mode, value) => {\n    if (!isTrustedRenderer(event.sender)) return;', 'ipcMain.on("desktop-scale-set", (event, mode, value) => {')));
  assert.throws(()=>reloadGuard(mainSource.replace('mainWindow.webContents.on("did-finish-load",', 'mainWindow.webContents.once("did-finish-load",')));
  assert.throws(()=>resizeGuard(mainSource.replace("if (!force && currentUiScale === scale) return;", "")));
  const h=harness();const exportWindow={isDestroyed:()=>false,setMinimumSize(){assert.fail("export window was scaled");},webContents:{setZoomFactor(){assert.fail("export window was scaled");}}};
  h.probe.applyUiScale(exportWindow,2,true);
});

test("preload exposes scale IPC without leaking the Electron event", async () => {
  const exposed={},ipc=new EventEmitter(),sent=[];
  ipc.invoke=async name=>({name});ipc.send=(...args)=>sent.push(args);
  vm.runInNewContext(fs.readFileSync(path.join(__dirname,"preload.cjs"),"utf8"),{require:()=>({contextBridge:{exposeInMainWorld:(name,value)=>exposed[name]=value},ipcRenderer:ipc})});
  assert.equal((await exposed.desktopScale.get()).name,"desktop-scale-get");exposed.desktopScale.set("fixed",2);assert.deepEqual(sent,[["desktop-scale-set","fixed",2]]);
  let received;const stop=exposed.desktopScale.onChanged(value=>received=value);const payload={mode:"auto",value:2,auto:2};ipc.emit("desktop-scale-changed",{sender:"private"},payload);assert.equal(received,payload);stop();assert.equal(ipc.listenerCount("desktop-scale-changed"),0);
});

test("packaged runtime includes the scaling module", () => {
  assert.ok(require("./package.json").build.files.includes("ui-scale.cjs"));
});
