"use strict";
if (require.main === module && process.env.DEEP_LEGENDS_TEST_LICENSE !== "1" && !["--backend", "--probe-backend"].includes(process.argv[2])) throw Error("Explicit license test switch required: DEEP_LEGENDS_TEST_LICENSE=1");

// Real Electron/production main + isolated synthetic HTTP backend. No license
// files, real codes, external network, or private-key files are used.
const fs = require("node:fs"), path = require("node:path"), os = require("node:os"), http = require("node:http"), assert = require("node:assert/strict");
const { spawn } = require("node:child_process");
const root = path.resolve(__dirname, ".."), out = process.env.R243_ELECTRON_OUTPUT || path.join(root, "docs/history/reports/r243/electron");
const domExpression = "(" + (() => {
  const layer = document.querySelector("#license-overlay"), form = document.querySelector("#license-form"), frame = document.querySelector("#app-frame");
  const ghost = layer.cloneNode(true); ghost.hidden = false; ghost.style.visibility = "hidden"; ghost.style.pointerEvents = "none"; document.body.append(ghost);
  const icon = ghost.querySelector("img").getBoundingClientRect(); ghost.remove();
  return { state: document.documentElement.dataset.license, overlayHidden: layer.hidden, frameHidden: frame.hidden,
    formVisible: form.getBoundingClientRect().width > 0 && getComputedStyle(layer).display !== "none", icon: { x: icon.x, y: icon.y, width: icon.width, height: icon.height } };
}).toString() + ")()";
if (process.argv[2] === "--backend") {
  console.log("LOOT_READY " + JSON.stringify({ baseUrl: process.env.R240_ORIGIN, bootstrapUrl: process.env.R240_ORIGIN, token: "r240-synthetic-token-".repeat(3) }));
  setInterval(() => {}, 1000);
} else {
async function probe({ phase, state = "ACTIVE", sourceRoot = root, transition = false, rendererDelay = 0, index = 1, flipBeforeAck = false, recovery = false, savedBounds = null, immediate = false, dropRestore = 0, minimize = false, activation = false }) {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), "r240-electron-")), directory = path.join(out, `${phase}-${state}-${index}`);
  fs.mkdirSync(directory, { recursive: true }); const rows = []; let current = state, generation = 1, nativeReads = 0;
  const web = path.join(root, "backend/web");
  const ready=path.join(temp,"issuer-ready"), fixtureLog=path.join(directory,"go-fixture.log");
  const fixture=spawn("go",["test","-tags=license","-count=1","-run","^TestR243ExpiredCacheRecoveryElectronFixture$","./backend"],{cwd:root,env:{...process.env,R243_FIXTURE_READY:ready,R243_FIXTURE_MODE:activation?"activation":immediate?"immediate":"recovery"},stdio:["ignore","pipe","pipe"]});
  let fixtureOutput="";fixture.stdout.on("data",c=>fixtureOutput+=c);fixture.stderr.on("data",c=>fixtureOutput+=c);
  const readyStart=Date.now();while(!fs.existsSync(ready)){if(Date.now()-readyStart>20000)throw Error("Go fixture failed: "+fixtureOutput);await new Promise(r=>setTimeout(r,20))}
  const issuerOrigin=fs.readFileSync(ready,"utf8");
  const server = http.createServer((req, res) => {
    const name = new URL(req.url, "http://fixture").pathname, json = value => { res.setHeader("Content-Type", "application/json"); res.end(JSON.stringify(value)); };
    if (name === "/api/license/status" || name === "/api/license/activate" || name.startsWith("/fixture/")) {
      const proxy=http.request(issuerOrigin+req.url,{method:req.method,headers:req.headers},reply=>{res.writeHead(reply.statusCode,reply.headers);reply.pipe(res)});proxy.on("error",()=>{res.writeHead(503);res.end()});req.pipe(proxy);return;
    }
    if (name.startsWith("/api/diagnostics/")) {
      if (name.endsWith("/log")) { res.setHeader("Content-Type", "application/x-ndjson"); res.end(rows.map(JSON.stringify).join("\n") + "\n"); return; }
      let body = ""; req.on("data", c => { body += c; }); req.on("end", () => { const value = JSON.parse(body); rows.push({ api: name, value });
        if(name.endsWith("/client")) { const proxy=http.request(issuerOrigin+name,{method:"POST",headers:{"Content-Type":"application/json"}},reply=>{res.writeHead(reply.statusCode,reply.headers);reply.pipe(res)});proxy.end(body); }
        else {res.writeHead(204); res.end();} }); return;
    }
    if (name === "/api/events") { res.writeHead(200, { "Content-Type": "text/event-stream" }); res.write(": fixture\n\n"); return; }
    if (name === "/api/status") return json({ connected: false, installations: [] });
    if (name.startsWith("/api/")) return json({});
    const file = path.resolve(web, name === "/" ? "index.html" : "." + name);
    if (!file.startsWith(web + path.sep) || !fs.existsSync(file) || !fs.statSync(file).isFile()) { res.writeHead(404); res.end(); return; }
    res.setHeader("Content-Type", ({ ".js": "text/javascript", ".css": "text/css", ".html": "text/html", ".png": "image/png" })[path.extname(file)] || "application/octet-stream"); res.end(require(path.join(root,"desktop/license-render-fixture.cjs")).licenseFixtureHTML(fs.readFileSync(file)));
  });
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve)); const origin = `http://127.0.0.1:${server.address().port}`;
  try {
    const shell = path.join(temp, "desktop"), userData = path.join(temp, "userData"), record = path.join(temp, "record.json"); fs.mkdirSync(shell); fs.mkdirSync(userData);
    if (savedBounds) fs.writeFileSync(path.join(userData, "window-bounds.json"), JSON.stringify(savedBounds));
    for (const name of fs.readdirSync(path.join(root, "desktop"))) {
      const source = path.join(sourceRoot, "desktop", name);
      if (name.endsWith(".cjs") && fs.existsSync(source)) fs.copyFileSync(source, path.join(shell, name));
      else if (["assets", "node_modules"].includes(name)) fs.symlinkSync(path.join(root, "desktop", name), path.join(shell, name), process.platform === "win32" ? "junction" : "dir");
    }
    fs.writeFileSync(path.join(shell,"license-build.cjs"),'module.exports = Object.freeze({enabled:true});\n');
    const launcher = path.join(temp, "synthetic-backend");
    fs.writeFileSync(launcher, "synthetic fixture marker");
    const wrapper = `const {app,BrowserWindow}=require('electron'),fs=require('node:fs');
// Substitute only the fixture backend child, keeping production main and all
// BrowserWindow APIs real. Node.exe works without a shell on Windows too.
const cp=require('node:child_process'),nativeSpawn=cp.spawn;
cp.spawn=function(command,args,options){return command===${JSON.stringify(launcher)}?nativeSpawn(${JSON.stringify(process.execPath)},[${JSON.stringify(__filename)},'--backend'],options):nativeSpawn(command,args,options)};
app.setName('R240 native probe');app.setPath('userData',${JSON.stringify(userData)});
const origin=${JSON.stringify(origin)},record=${JSON.stringify(record)},directory=${JSON.stringify(directory)},result={shows:[],samples:[],platform:process.platform};
let main,shown=0,sampling;
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
const until=async(fn,limit=5000)=>{const start=Date.now();while(!await fn()){if(Date.now()-start>limit)throw Error('native fixture timeout');await sleep(10)}};
const getDOM=async w=>w.webContents.executeJavaScript(${JSON.stringify(domExpression)});
let drops=${dropRestore};
const nativeSetBounds=BrowserWindow.prototype.setBounds;
BrowserWindow.prototype.setBounds=function(rect,...args){if(this===main && rect.height>600 && drops-->0){result.ignoredNativeRestores=(result.ignoredNativeRestores||0)+1;return}return nativeSetBounds.call(this,rect,...args)};
const originalShow=BrowserWindow.prototype.show;
BrowserWindow.prototype.show=function(...args){const value=originalShow.apply(this,args);
 if(this.webContents?.getURL().startsWith(origin)){main=this;const w=this,n=++shown;
  // Both requests start in the same call stack as the FIRST native show.
  const page=w.webContents.capturePage(),dom=getDOM(w),bounds=w.getContentBounds();
  Promise.all([page,dom]).then(([image,content])=>{fs.writeFileSync(directory+'/show-'+n+'.png',image.toPNG());const scaled=image.resize({width:bounds.width,height:bounds.height}),bitmap=scaled.toBitmap(),size=scaled.getSize();let gold=0;
   const r=content.icon;for(let y=Math.max(0,Math.floor(r.y));y<Math.min(size.height,Math.ceil(r.y+r.height));y++)for(let x=Math.max(0,Math.floor(r.x));x<Math.min(size.width,Math.ceil(r.x+r.width));x++){const i=(y*size.width+x)*4,b=bitmap[i],g=bitmap[i+1],red=bitmap[i+2];if(red>130 && g>80 && b<140 && red>g*1.12)gold++}
   result.shows.push({number:n,bounds,content,activationIconGoldPixels:gold,captureInvokedAtShow:true});
  }).catch(error=>{result.error=error.stack});
 }
 return value;
};
app.on('browser-window-created',(_event,w)=>{if(w.isAlwaysOnTop())return;
 // URL is not available during construction; select the application page when loaded.
 w.webContents.once('did-finish-load',()=>{if(!w.webContents.getURL().startsWith(origin))return;main=w;
  sampling=setInterval(()=>{if(!w.isDestroyed())result.samples.push({at:Date.now(),visible:w.isVisible(),opacity:w.getOpacity(),bounds:w.getContentBounds(),maximized:w.isMaximized()})},25);
 });
});
app.whenReady().then(()=>require('electron').session.defaultSession.webRequest.onBeforeRequest({urls:['*://*/*']},(d,cb)=>cb({cancel:!d.url.startsWith(origin+'/')})));
require(${JSON.stringify(path.join(shell, "main.cjs"))});
(async()=>{await until(()=>main && main.isVisible());await until(()=>result.shows.length>0);
 if(${recovery}){result.locked={bounds:main.getContentBounds(),content:await getDOM(main)};
  if(${activation}) {
    await main.webContents.executeJavaScript("document.querySelector('#license-code').value='00000000000000000000';document.querySelector('#license-form').requestSubmit()");
    await sleep(1000);result.activationBusy=await main.webContents.executeJavaScript("({button:document.querySelector('#license-submit').textContent,disabled:document.querySelector('#license-submit').disabled,message:document.querySelector('#license-message').textContent,inputDisabled:document.querySelector('#license-code').disabled})");
  }
  if(${minimize})main.minimize();
  await until(async()=> (await getDOM(main)).state==='active',22000);await until(()=>main.getOpacity()===1,5000);await sleep(100);
  result.restored={bounds:main.getContentBounds(),normal:main.getNormalBounds(),maximized:main.isMaximized(),content:await getDOM(main)};
  if(main.isMinimized())main.restore();
  fs.writeFileSync(directory+'/restored.png',(await main.webContents.capturePage()).toPNG());
 }else if(${transition}){await sleep(700);main.maximize();await sleep(500);result.maximizedBefore=main.isMaximized();result.beforeLock={bounds:main.getContentBounds(),normal:main.getNormalBounds()};
  await main.webContents.executeJavaScript('fetch("/fixture/state?state=REVOKED").then(()=>window.deepLegendsLicense.poll())');await sleep(1800);result.locked={bounds:main.getContentBounds(),maximized:main.isMaximized(),resizable:main.isResizable(),maximizable:main.isMaximizable(),content:await getDOM(main)};
  await main.webContents.executeJavaScript('fetch("/fixture/state?state=ACTIVE").then(()=>window.deepLegendsLicense.poll())');await sleep(1800);result.restored={bounds:main.getContentBounds(),maximized:main.isMaximized(),content:await getDOM(main)};
 }else await sleep(150);
 clearInterval(sampling);fs.writeFileSync(record,JSON.stringify(result));app.quit();
})().catch(error=>{result.error=error.stack;fs.writeFileSync(record,JSON.stringify(result));app.quit()});
`;
    fs.writeFileSync(path.join(temp, "probe.cjs"), wrapper); fs.writeFileSync(path.join(temp, "package.json"), JSON.stringify({ name: "r240-probe", version: "1.0.0", main: "probe.cjs" }));
    const env = { ...process.env, LOOT_BACKEND: launcher, R240_ORIGIN: origin }; delete env.ELECTRON_RUN_AS_NODE;
    await new Promise((resolve, reject) => {
      const child = spawn(require(path.join(root, "desktop/node_modules/electron")), ["--disable-gpu", temp], { env, stdio: ["ignore", "pipe", "pipe"] }); let output = "";
      child.stdout.on("data", c => { output += c; }); child.stderr.on("data", c => { output += c; });
      const timer = setTimeout(() => { child.kill(); reject(Error("real Electron timeout: " + output.slice(-2000))); }, 30000);
      child.once("error", reject); child.once("exit", code => { clearTimeout(timer); code === 0 ? resolve() : reject(Error("Electron exited " + code + ": " + output.slice(-2000))); });
    });
    const expectedState = immediate ? "ACTIVE" : state;
    const result = { phase, state, expectedState, index, ...JSON.parse(fs.readFileSync(record)), diagnostics: rows, phases: rows.find(r => r.api.endsWith("/startup"))?.value };
    result.cacheEvidence=await (await fetch(issuerOrigin+"/fixture/evidence")).json();
    result.export=await (await fetch(issuerOrigin+"/api/diagnostics/log")).text();
    fs.writeFileSync(path.join(directory,"real-diagnostic-export.jsonl"),result.export);
    fs.writeFileSync(path.join(directory, "result.json"), JSON.stringify(result, null, 2));
    fs.writeFileSync(path.join(directory, "diagnostic-export.jsonl"), rows.filter(r => r.value.event).map(r => JSON.stringify(r.value)).join("\n") + "\n");
    if (result.error) throw Error(result.error);
    if (phase === "after" || phase === "mutation") {
      const first = result.shows.find(row => row.number === 1);
      assert.ok(first?.captureInvokedAtShow, "P2 first show must immediately capturePage");
      assert.equal(first.content.state, expectedState === "ACTIVE" ? "active" : "locked", "P2 first show content must match authenticated state");
      assert.equal(first.content.formVisible, expectedState !== "ACTIVE", "P2 first show activation form visibility");
      assert.equal(first.content.overlayHidden, expectedState === "ACTIVE"); assert.equal(first.content.frameHidden, expectedState !== "ACTIVE");
      if (expectedState === "ACTIVE") assert.equal(first.activationIconGoldPixels, 0, "P2 ACTIVE first-show pixels must contain no activation icon");
      else { assert.ok(first.activationIconGoldPixels > 20, "P2 LOCKED first-show pixels must contain activation icon"); assert.equal(first.bounds.width, 860); assert.equal(first.bounds.height, 580); }
      assert.equal(result.cacheEvidence.expired_cache,true);
      assert.equal(result.cacheEvidence.state,"ACTIVE");
      assert.equal(result.cacheEvidence.renew_requests,activation||immediate?1:2);
      if(activation){assert.equal(result.cacheEvidence.activate_requests,2);assert.equal(result.cacheEvidence.activation_fresh_signed_request,true);assert.deepEqual(result.activationBusy,{button:"正在激活…",disabled:true,message:"正在激活…",inputDisabled:true})}
      assert.ok(result.restored.bounds.width>=780 && result.restored.bounds.height>=600,"ACTIVE minimum guard must restore native window");
      if(savedBounds && !dropRestore){assert.equal(result.restored.normal.width,savedBounds.width);assert.equal(result.restored.normal.height,savedBounds.height);assert.equal(result.restored.normal.x,savedBounds.x);assert.equal(result.restored.normal.y,savedBounds.y);assert.equal(result.restored.maximized,savedBounds.maximized)}
      const recovered=rows.find(r=>r.value.licenseWindow?.fromState==="NETWORK_LOCKED" && r.value.licenseWindow.toState==="ACTIVE")?.value.licenseWindow;
      if(!immediate){assert.ok(recovered,"export transition");assert.equal(recovered.renderTimedOut,false);assert.ok(recovered.elapsedMs<1500,"recovery should not hit render timeout");if(dropRestore)assert.equal(recovered.fallback_used,true)}
      assert.ok(!result.export.includes('license_window_state_invalid'),"normal native telemetry must pass strict validation");
      if (flipBeforeAck) assert.ok(rows.some(row => row.value.licenseWindow?.renderMismatch), "P2 disagreement must reapply Go state and record mismatch");
      if (transition) {
        assert.equal(result.maximizedBefore, true, "P3 fixture must actually maximize the native window");
        assert.equal(result.locked.bounds.width, 860); assert.equal(result.locked.bounds.height, 580);
        assert.equal(result.locked.maximized, false); assert.equal(result.locked.resizable, false); assert.equal(result.locked.maximizable, false);
        assert.equal(result.locked.content.state, "locked"); assert.equal(result.locked.content.formVisible, true);
        assert.equal(result.restored.maximized, true, "P3 reactivation restores native maximization");
        assert.equal(result.restored.content.formVisible, false);
        const lock = rows.find(row => row.value.licenseWindow?.wasMaximized && row.value.licenseWindow.toState === "REVOKED")?.value.licenseWindow;
        assert.ok(lock, "P3 native transition telemetry must be exported"); assert.equal(lock.actualContent.width, 860); assert.equal(lock.actualContent.height, 580);
        assert.equal(lock.renderTimedOut, false); assert.equal(lock.sizeMismatch, false);
      }
    }
    return result;
  } finally {
    try{await fetch(issuerOrigin+"/fixture/done")}catch(_){}
    await new Promise(resolve=>{if(fixture.exitCode!==null)return resolve();fixture.once("exit",resolve);setTimeout(()=>{fixture.kill();resolve()},2000)});
    fs.writeFileSync(fixtureLog,fixtureOutput);
    server.closeAllConnections(); await new Promise(resolve => server.close(resolve)); fs.rmSync(temp, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }); }
}
module.exports = { probe };
if (require.main === module) {
  const mode = process.argv[2] || "baseline";
  (async () => {
    const results = [];
    let baselineRoot;
    if(mode==="baseline"){
      baselineRoot=fs.mkdtempSync(path.join(os.tmpdir(),"r243-r242-source-"));fs.mkdirSync(path.join(baselineRoot,"desktop"));
      const asar=require(path.join(root,"desktop/node_modules/@electron/asar")),archive=path.join(root,"dist/R242-staging-public/Deep-Legends-staging-public/resources/app.asar");
      for(const name of fs.readdirSync(path.join(root,"desktop")))if(name.endsWith(".cjs") && !/test|verify|render|build|diagnostics-20/.test(name)){
        try{fs.writeFileSync(path.join(baselineRoot,"desktop",name),asar.extractFile(archive,name))}catch(_){}
      }
    }
    try {
      const saved={x:90,y:60,width:1200,height:780,maximized:false};
      const scenarios=mode==="mutation"?[{savedBounds:saved,dropRestore:2}]:mode==="baseline"?[{savedBounds:saved},{savedBounds:null},{savedBounds:saved,dropRestore:1,minimize:true}]:[
        {savedBounds:saved,immediate:true},{savedBounds:saved},{savedBounds:{...saved,maximized:true}},{savedBounds:null},
        {savedBounds:saved,dropRestore:1},{savedBounds:saved,dropRestore:2},{savedBounds:saved,minimize:true},{savedBounds:saved,activation:true}];
      for(const scenario of scenarios)results.push(await probe({phase:mode,state:"NETWORK_LOCKED",recovery:true,sourceRoot:baselineRoot||process.env.R243_SOURCE_ROOT||root,index:results.length+1,...scenario}));
    } finally {if(baselineRoot)fs.rmSync(baselineRoot,{recursive:true,force:true})}
    fs.mkdirSync(out, { recursive: true }); fs.writeFileSync(path.join(out, mode + ".json"), JSON.stringify(results, null, 2));
    console.log(JSON.stringify(results.map(r => ({ phase:r.phase,state:r.state,shows:r.shows,maximizedBefore:r.maximizedBefore,locked:r.locked,restored:r.restored,phases:r.phases }))));
  })().catch(error => { console.error(error); process.exitCode = 1; });
}
}
