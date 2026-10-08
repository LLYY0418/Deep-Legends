"use strict";
const { evidencePath, requireEvidence } = require('./local-evidence.cjs');
// Real Electron/production main + isolated synthetic HTTP backend. No license
// files, real codes, external network, or private-key files are used.
const fs = require("node:fs"), path = require("node:path"), os = require("node:os"), http = require("node:http"), assert = require("node:assert/strict");
const { spawn } = require("node:child_process");
const root = path.resolve(__dirname, ".."), out = process.env.R248_ELECTRON_OUTPUT || evidencePath('r248/electron');
const domExpression = "(" + (() => {
  const layer = document.querySelector("#license-overlay"), form = document.querySelector("#license-form"), frame = document.querySelector("#app-frame"), expiry=document.querySelector("#setting-license-expiry");
  return { state: document.documentElement.dataset.license, overlayHidden: !layer || getComputedStyle(layer).display === "none", frameHidden: frame.hidden,
    frameInert: frame.hasAttribute("inert"), formVisible: !!form && form.getBoundingClientRect().width > 0 && getComputedStyle(layer).display !== "none",
    expiryVisible: !!expiry && !expiry.hidden && getComputedStyle(expiry).display !== "none" };
}).toString() + ")()";
if (process.argv[2] === "--backend") {
  console.log("LOOT_READY " + JSON.stringify({ baseUrl: process.env.R240_ORIGIN, bootstrapUrl: process.env.R240_ORIGIN, token: "r240-synthetic-token-".repeat(3) }));
  setInterval(() => {}, 1000);
} else {
async function probe({ phase, state = "ACTIVE", sourceRoot = root, transition = false, rendererDelay = 0, index = 1, flipBeforeAck = false, storedBounds = null, frontendAssetDirectory = "" }) {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), "r240-electron-")), directory = path.join(out, `${phase}-${state}-${index}`);
  fs.mkdirSync(directory, { recursive: true }); const rows = [], serverRequests = [], transportEvents = []; let current = state, generation = 1, nativeReads = 0;
  const web = path.join(sourceRoot, "backend/web");
  const server = http.createServer((req, res) => {
    serverRequests.push(req.url);
    const name = new URL(req.url, "http://fixture").pathname, json = value => { res.setHeader("Content-Type", "application/json"); res.end(JSON.stringify(value)); };
    if (name === "/api/license/status") {
      if (req.headers["x-local-token"] && ++nativeReads === 2 && flipBeforeAck) { current = "LOCKED"; generation++; }
      const reply = () => json({ state: current, generation, message: current === "REVOKED" ? "注册码已停用" : "请输入注册码激活软件" });
      if (!req.headers["x-local-token"] && rendererDelay) setTimeout(reply, rendererDelay); else reply(); return;
    }
    if (name === "/fixture/state") { current = new URL(req.url, "http://fixture").searchParams.get("state"); generation++; json({}); return; }
    if (name.startsWith("/api/diagnostics/")) {
      if (name.endsWith("/log")) { res.setHeader("Content-Type", "application/x-ndjson"); res.end(rows.map(JSON.stringify).join("\n") + "\n"); return; }
      let body = ""; req.on("data", c => { body += c; }); req.on("end", () => { const value = JSON.parse(body); rows.push({ api: name, value }); res.writeHead(204); res.end(); }); return;
    }
    if (name === "/api/events") { res.writeHead(200, { "Content-Type": "text/event-stream" }); res.write(": fixture\n\n"); return; }
    if (name === "/api/status") return json({ connected: false, installations: [] });
    if (name.startsWith("/api/")) return json({});
    const relative = name === "/" ? "index.html" : name.slice(1);
    const file = path.resolve(web, frontendAssetDirectory && ["index.html", "license-ui.js"].includes(relative) ? path.join(frontendAssetDirectory, relative) : relative);
    if (!file.startsWith(web + path.sep) || !fs.existsSync(file) || !fs.statSync(file).isFile()) { res.writeHead(404); res.end(); return; }
    res.setHeader("Content-Type", ({ ".js": "text/javascript", ".css": "text/css", ".html": "text/html", ".png": "image/png" })[path.extname(file)] || "application/octet-stream"); res.end(fs.readFileSync(file));
  });
  const originalServerEmit = server.emit;
  server.emit = function(event, ...args) {
    if (event === "clientError") transportEvents.push({event,at:Date.now(),code:args[0]?.code,message:args[0]?.message});
    return originalServerEmit.call(this,event,...args);
  };
  server.on("connection",socket=>{
    transportEvents.push({event:"connection",at:Date.now()});
    socket.on("error",error=>transportEvents.push({event:"socket-error",at:Date.now(),code:error.code,message:error.message}));
    socket.on("close",()=>transportEvents.push({event:"socket-close",at:Date.now()}));
  });
  await new Promise(resolve => server.listen(0, "127.0.0.1", resolve)); const origin = `http://127.0.0.1:${server.address().port}`;
  try {
    const shell = path.join(temp, "desktop"), userData = path.join(temp, "userData"), record = path.join(temp, "record.json"); fs.mkdirSync(shell); fs.mkdirSync(userData);
    if (storedBounds) fs.writeFileSync(path.join(userData,"window-bounds.json"),JSON.stringify(storedBounds));
    for (const name of fs.readdirSync(path.join(root, "desktop"))) {
      const source = path.join(sourceRoot, "desktop", name);
      if (name.endsWith(".cjs") && fs.existsSync(source)) fs.copyFileSync(source, path.join(shell, name));
      else if (["assets", "node_modules"].includes(name)) fs.symlinkSync(path.join(root, "desktop", name), path.join(shell, name), process.platform === "win32" ? "junction" : "dir");
    }
    const launcher = path.join(temp, "synthetic-backend");
    fs.writeFileSync(launcher, "synthetic fixture marker");
    const wrapper = `const {app,BrowserWindow}=require('electron'),fs=require('node:fs');
// Substitute only the fixture backend child, keeping production main and all
// BrowserWindow APIs real. Node.exe works without a shell on Windows too.
const cp=require('node:child_process'),nativeSpawn=cp.spawn;
cp.spawn=function(command,args,options){return command===${JSON.stringify(launcher)}?nativeSpawn(${JSON.stringify(process.execPath)},[${JSON.stringify(__filename)},'--backend'],options):nativeSpawn(command,args,options)};
app.setName('R240 native probe');app.setPath('userData',${JSON.stringify(userData)});
const origin=${JSON.stringify(origin)},record=${JSON.stringify(record)},directory=${JSON.stringify(directory)},result={shows:[],samples:[],networkRequests:[],networkDecisions:[],events:[],platform:process.platform,arch:process.arch,electron:process.versions.electron,execPath:process.execPath};
let main,shown=0,sampling;
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
const until=async(fn,limit=5000)=>{const start=Date.now();while(!await fn()){if(Date.now()-start>limit)throw Error('native fixture timeout');await sleep(10)}};
const getDOM=async w=>w.webContents.executeJavaScript(${JSON.stringify(domExpression)});
const originalShow=BrowserWindow.prototype.show;
BrowserWindow.prototype.show=function(...args){const value=originalShow.apply(this,args);
 if(this.webContents?.getURL().startsWith(origin)){main=this;const w=this,n=++shown;
  // Both requests start in the same call stack as the FIRST native show.
  const page=w.webContents.capturePage(),dom=getDOM(w),bounds=w.getContentBounds();
  Promise.all([page,dom]).then(([image,content])=>{fs.writeFileSync(directory+'/show-'+n+'.png',image.toPNG());
   result.shows.push({number:n,bounds,content,resizable:w.isResizable(),maximizable:w.isMaximizable(),opacity:w.getOpacity(),captureInvokedAtShow:true});
  }).catch(error=>{result.error=error.stack});
 }
 return value;
};
app.on('browser-window-created',(_event,w)=>{
 result.events.push({event:'browser-window-created',at:Date.now(),id:w.id,alwaysOnTop:w.isAlwaysOnTop()});
 w.webContents.on('did-start-navigation',(_event,url,inPlace,isMainFrame)=>result.events.push({event:'did-start-navigation',at:Date.now(),url,inPlace,isMainFrame}));
 w.webContents.on('did-fail-load',(_event,code,description,url,isMainFrame)=>result.events.push({event:'did-fail-load',at:Date.now(),code,description,url,isMainFrame}));
 w.webContents.on('render-process-gone',(_event,details)=>result.events.push({event:'render-process-gone',at:Date.now(),...details}));
 w.webContents.on('did-finish-load',()=>result.events.push({event:'did-finish-load',at:Date.now(),url:w.webContents.getURL()}));
 w.on('ready-to-show',()=>result.events.push({event:'ready-to-show',at:Date.now(),id:w.id}));
 if(w.isAlwaysOnTop())return;
 // URL is not available during construction; select the application page when loaded.
 w.webContents.once('did-finish-load',()=>{if(!w.webContents.getURL().startsWith(origin))return;main=w;
  sampling=setInterval(()=>{if(!w.isDestroyed())result.samples.push({at:Date.now(),visible:w.isVisible(),opacity:w.getOpacity(),bounds:w.getContentBounds(),maximized:w.isMaximized()})},8);
 });
});
app.whenReady().then(()=>require('electron').session.defaultSession.webRequest.onBeforeRequest({urls:['*://*/*']},(d,cb)=>{result.networkRequests.push(d.url);const cancel=!d.url.startsWith(origin+'/');result.networkDecisions.push({url:d.url,resourceType:d.resourceType,cancel,at:Date.now()});cb({cancel})}));
require(${JSON.stringify(path.join(shell, "main.cjs"))});
(async()=>{await until(()=>main && main.isVisible());await until(()=>result.shows.length>0);
 if(${transition}){await sleep(700);main.maximize();await sleep(500);result.maximizedBefore=main.isMaximized();result.beforeLock={bounds:main.getContentBounds(),normal:main.getNormalBounds()};
  await main.webContents.executeJavaScript('fetch("/fixture/state?state=REVOKED").then(()=>window.deepLegendsLicense.poll())');await sleep(1800);result.locked={bounds:main.getContentBounds(),maximized:main.isMaximized(),resizable:main.isResizable(),maximizable:main.isMaximizable(),content:await getDOM(main)};
  await main.webContents.executeJavaScript('fetch("/fixture/state?state=ACTIVE").then(()=>window.deepLegendsLicense.poll())');await sleep(1800);result.restored={bounds:main.getContentBounds(),maximized:main.isMaximized(),content:await getDOM(main)};
 }else await sleep(150);
 clearInterval(sampling);fs.writeFileSync(record,JSON.stringify(result));app.quit();
})().catch(error=>{result.error=error.stack;fs.writeFileSync(record,JSON.stringify(result));app.quit()});
`;
    fs.writeFileSync(path.join(temp, "probe.cjs"), wrapper); fs.writeFileSync(path.join(temp, "package.json"), JSON.stringify({ name: "r240-probe", version: "1.0.0", main: "probe.cjs" }));
    const env = { ...process.env, LOOT_BACKEND: launcher, R240_ORIGIN: origin }; delete env.ELECTRON_RUN_AS_NODE;
    const childOutput = {stdout:"",stderr:""};
    const saveChildOutput=()=>{for(const [stream,value] of Object.entries(childOutput))fs.writeFileSync(path.join(directory,"electron-"+stream+".log"),value);};
    await new Promise((resolve, reject) => {
      const child = spawn(require(path.join(root, "desktop/node_modules/electron")), ["--disable-gpu", temp], { env, stdio: ["ignore", "pipe", "pipe"] }); let output = "";
      child.stdout.on("data", c => { output += c; childOutput.stdout += c; }); child.stderr.on("data", c => { output += c; childOutput.stderr += c; });
      const timer = setTimeout(() => { saveChildOutput(); child.kill(); reject(Error("real Electron timeout: " + output.slice(-2000))); }, 30000);
      child.once("error", error=>{clearTimeout(timer);saveChildOutput();reject(error);}); child.once("exit", code => { clearTimeout(timer); saveChildOutput(); code === 0 ? resolve() : reject(Error("Electron exited " + code + ": " + output.slice(-2000))); });
    });
    const expectedState = flipBeforeAck ? "LOCKED" : state;
    const result = { phase, state, expectedState, index, ...JSON.parse(fs.readFileSync(record)), serverRequests, transportEvents, diagnostics: rows, phases: rows.find(r => r.api.endsWith("/startup"))?.value };
    fs.writeFileSync(path.join(directory, "result.json"), JSON.stringify(result, null, 2));
    fs.writeFileSync(path.join(directory, "diagnostic-export.jsonl"), rows.filter(r => r.value.event).map(r => JSON.stringify(r.value)).join("\n") + "\n");
    if (result.error) throw Error(result.error);
    if (phase === "after") {
      const first = result.shows.find(row => row.number === 1);
      assert.ok(first?.captureInvokedAtShow, "R248 capture must start at first native show");
      assert.equal(first.content.state,"disabled","R248 first frame must show disabled main UI");
      assert.equal(first.content.frameHidden,false,"R248 first frame must show application");
      assert.equal(first.content.overlayHidden,true,"R248 first frame must hide activation overlay");
      assert.equal(first.content.formVisible,false); assert.equal(first.content.expiryVisible,false);
      assert.equal(first.resizable,true); assert.equal(first.maximizable,true); assert.equal(first.opacity,1);
      assert.ok(first.bounds.width!==860 || first.bounds.height!==580,"R248 default cannot use license window geometry");
      assert.equal(serverRequests.filter(url=>url.startsWith("/api/license/")).length,0,"R248 shell and renderer must issue zero license API requests");
      assert.equal(result.networkRequests.filter(url=>/license(?:-staging)?\.yinxiaobia\.net/.test(url)).length,0,"R248 must issue zero authorization network requests");
      assert.equal(rows.filter(row=>String(row.value.event||"").startsWith("license_")).length,0,"R248 must emit zero license diagnostics");
      if(storedBounds){assert.ok(first.bounds.width>=storedBounds.width-40 && first.bounds.width<=storedBounds.width,"R248 restores saved bounds");}
    }
    return result;
  } finally { server.closeAllConnections(); await new Promise(resolve => server.close(resolve)); fs.rmSync(temp, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }); }
}
module.exports = { probe };
if (require.main === module) {
  (async()=>{
    const baseline=process.argv[2];assert.ok(baseline,"v0.12.76 source directory required");
    const results=[];
    for(let index=1;index<=3;index++) {
      results.push(await probe({phase:"before",state:"DISABLED",sourceRoot:baseline,index}));
      results.push(await probe({phase:"after",state:"DISABLED",index}));
    }
    results.push(await probe({phase:"after",state:"DISABLED",index:4,storedBounds:{x:40,y:40,width:1050,height:750,maximized:false}}));
    fs.writeFileSync(path.join(out,"startup.json"),JSON.stringify(results,null,2));
    console.log(JSON.stringify(results.map(r=>({phase:r.phase,index:r.index,first:r.shows[0],phases:r.phases}))));
  })().catch(error=>{console.error(error);process.exitCode=1});
}
}
