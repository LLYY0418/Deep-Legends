"use strict";
const { evidencePath, requireEvidence } = require('./local-evidence.cjs');
if (require.main === module && process.env.DEEP_LEGENDS_TEST_LICENSE !== "1" && !["--backend", "--probe-backend"].includes(process.argv[2])) throw Error("Explicit license test switch required: DEEP_LEGENDS_TEST_LICENSE=1");

// Real Electron/production main + isolated synthetic HTTP backend. No license
// files, real codes, external network, or private-key files are used.
const fs = require("node:fs"), path = require("node:path"), os = require("node:os"), http = require("node:http"), assert = require("node:assert/strict");
const { spawn } = require("node:child_process");
const root = path.resolve(__dirname, ".."), out = process.env.R245_ELECTRON_OUTPUT || evidencePath('r245/electron');
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
async function probe({ phase, state = "ACTIVE", sourceRoot = root, transition = false, rendererDelay = 0, index = 1, flipBeforeAck = false, scale = 1.5, mode = "normal", fractionalRead = false }) {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), "r240-electron-")), directory = path.join(out, `${phase}-${state}-${index}`);
  fs.mkdirSync(directory, { recursive: true }); const rows = []; let current = state, generation = 1, nativeReads = 0;
  const web = path.join(sourceRoot, "backend/web");
  const server = http.createServer((req, res) => {
    const name = new URL(req.url, "http://fixture").pathname, json = value => { res.setHeader("Content-Type", "application/json"); res.end(JSON.stringify(value)); };
    if (name === "/api/license/status") {
      if (req.headers["x-local-token"] && ++nativeReads === 2 && flipBeforeAck) { current = "LOCKED"; generation++; }
      const reply = () => json({ state: current, generation, message: current === "REVOKED" ? "注册码已停用" : "请输入注册码激活软件" });
      if (!req.headers["x-local-token"] && rendererDelay) setTimeout(reply, rendererDelay); else reply(); return;
    }
    if (name === "/api/license/activate") { req.resume(); current = "ACTIVE"; generation++; json({ state: current, generation }); return; }
    if (name === "/fixture/state") { current = new URL(req.url, "http://fixture").searchParams.get("state"); generation++; json({}); return; }
    if (name.startsWith("/api/diagnostics/")) {
      if (name.endsWith("/log")) { res.setHeader("Content-Type", "application/x-ndjson"); res.end(rows.map(JSON.stringify).join("\n") + "\n"); return; }
      let body = ""; req.on("data", c => { body += c; }); req.on("end", () => { const value = JSON.parse(body); rows.push({ api: name, value }); res.writeHead(204); res.end(); }); return;
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
    const shell = path.join(temp, "desktop"), userData = path.join(temp, "userData"), record = path.join(temp, "record.json"); fs.mkdirSync(shell); fs.mkdirSync(userData); fs.writeFileSync(path.join(userData,"window-bounds.json"),JSON.stringify({x:90,y:60,width:1201,height:781,maximized:false}));
    for (const name of fs.readdirSync(path.join(root, "desktop"))) {
      const source = path.join(sourceRoot, "desktop", name);
      if (name.endsWith(".cjs") && fs.existsSync(source)) fs.copyFileSync(source, path.join(shell, name));
      else if (["assets", "node_modules"].includes(name)) fs.symlinkSync(path.join(root, "desktop", name), path.join(shell, name), process.platform === "win32" ? "junction" : "dir");
    }
    fs.writeFileSync(path.join(shell,"license-build.cjs"),'module.exports = Object.freeze({enabled:true});\n');
    const launcher = path.join(temp, "synthetic-backend");
    fs.writeFileSync(launcher, "synthetic fixture marker");
    const wrapper = `const {app,BrowserWindow,screen}=require('electron'),fs=require('node:fs');
// Substitute only the fixture backend child, keeping production main and all
// BrowserWindow APIs real. Node.exe works without a shell on Windows too.
const cp=require('node:child_process'),nativeSpawn=cp.spawn;
cp.spawn=function(command,args,options){return command===${JSON.stringify(launcher)}?nativeSpawn(${JSON.stringify(process.execPath)},[${JSON.stringify(__filename)},'--backend'],options):nativeSpawn(command,args,options)};
app.setName('R240 native probe');app.setPath('userData',${JSON.stringify(userData)});
const origin=${JSON.stringify(origin)},record=${JSON.stringify(record)},directory=${JSON.stringify(directory)},result={shows:[],samples:[],platform:process.platform};
let main,shown=0,sampling,cover;
result.nativeCalls=[];let fractionalEnabled=false;
// Equivalent DIP conversion injection: physical pixels rounded at the display
// scale and divided back. Real Electron still performs every native operation.
for(const name of ['getBounds','getNormalBounds','getContentBounds']){const original=BrowserWindow.prototype[name];BrowserWindow.prototype[name]=function(...args){const rect=original.apply(this,args);if(this===main && fractionalEnabled)for(const k of ['x','y','width','height'])rect[k]=Math.round(rect[k]*${scale})/${scale};return rect}}
process.on('unhandledRejection',error=>{result.transitionError=error.name;});

for(const name of ['setBounds','setContentBounds','setMinimumSize']){const original=BrowserWindow.prototype[name];BrowserWindow.prototype[name]=function(...args){if(this===main)result.nativeCalls.push({name,args});if(${fractionalRead} && this===main && name==='setBounds' && args[0].height>600 && !Object.values(args[0]).every(Number.isInteger)){result.rejectedFractionalRestore=true;return}try{return original.apply(this,args)}catch(error){result.nativeErrors=(result.nativeErrors||[]).concat(error.name);throw error}}}

const sleep=ms=>new Promise(r=>setTimeout(r,ms));
const until=async(fn,limit=5000)=>{const start=Date.now();while(!await fn()){if(Date.now()-start>limit)throw Error('native fixture timeout');await sleep(10)}};
const getDOM=async w=>w.webContents.executeJavaScript(${JSON.stringify(domExpression)});
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
  sampling=setInterval(()=>{if(!w.isDestroyed())result.samples.push({at:Date.now(),visible:w.isVisible(),opacity:w.getOpacity(),bounds:w.getContentBounds(),maximized:w.isMaximized()})},8);
 });
});
app.whenReady().then(()=>require('electron').session.defaultSession.webRequest.onBeforeRequest({urls:['*://*/*']},(d,cb)=>cb({cancel:!d.url.startsWith(origin+'/')})));
require(${JSON.stringify(path.join(shell, "main.cjs"))});
(async()=>{await until(()=>main && main.isVisible());await until(()=>result.shows.length>0);
 result.scale={requested:${scale},screen:screen.getDisplayMatching(main.getBounds()).scaleFactor,dpr:await main.webContents.executeJavaScript('devicePixelRatio'),method:'--force-device-scale-factor'};
 await sleep(100);main.setBounds({x:90,y:60,width:1201,height:781});await sleep(100);
 if(${JSON.stringify(mode)}==='maximized'){main.maximize();await sleep(500)}
 fractionalEnabled=${fractionalRead};result.fractionalRead=${fractionalRead};
 result.beforeLock={bounds:main.getContentBounds(),normal:main.getNormalBounds(),maximized:main.isMaximized()};
 if(${JSON.stringify(mode)}==='minimized')main.minimize();
 if(${JSON.stringify(mode)}==='occluded'){cover=new BrowserWindow({show:true,alwaysOnTop:true,...main.getBounds(),webPreferences:{sandbox:true}});await cover.loadURL('data:text/html,<body style="background:navy">R245 occluder</body>');cover.focus();await sleep(100);result.occluded={coverBounds:cover.getBounds(),mainFocused:main.isFocused()}}
 await main.webContents.executeJavaScript('fetch("/fixture/state?state=REVOKED").then(()=>window.deepLegendsLicense.poll())');await sleep(1800);
 result.locked={bounds:main.getContentBounds(),maximized:main.isMaximized(),minimized:main.isMinimized(),content:await getDOM(main)};
 await main.webContents.executeJavaScript("document.querySelector('#license-code').value='00000000000000000000';document.querySelector('#license-form').requestSubmit()");await sleep(1800);
 result.restored={bounds:main.getContentBounds(),normal:main.getNormalBounds(),maximized:main.isMaximized(),content:await getDOM(main),opacity:main.getOpacity()};
 if(cover)cover.destroy();if(main.isMinimized())main.restore();
 fs.writeFileSync(directory+'/restored.png',(await main.webContents.capturePage()).toPNG());
 result.saved=JSON.parse(fs.readFileSync(${JSON.stringify(path.join(userData,'window-bounds.json'))},'utf8'));
 clearInterval(sampling);fs.writeFileSync(record,JSON.stringify(result));app.quit();
})().catch(error=>{result.error=error.stack;fs.writeFileSync(record,JSON.stringify(result));app.quit()});
`;
    fs.writeFileSync(path.join(temp, "probe.cjs"), wrapper); fs.writeFileSync(path.join(temp, "package.json"), JSON.stringify({ name: "r240-probe", version: "1.0.0", main: "probe.cjs" }));
    const env = { ...process.env, LOOT_BACKEND: launcher, R240_ORIGIN: origin }; delete env.ELECTRON_RUN_AS_NODE;
    await new Promise((resolve, reject) => {
      const child = spawn(require(path.join(root, "desktop/node_modules/electron")), ["--disable-gpu", `--force-device-scale-factor=${scale}`, temp], { env, stdio: ["ignore", "pipe", "pipe"] }); let output = "";
      child.stdout.on("data", c => { output += c; }); child.stderr.on("data", c => { output += c; });
      const timer = setTimeout(() => { child.kill(); reject(Error("real Electron timeout: " + output.slice(-2000))); }, 30000);
      child.once("error", reject); child.once("exit", code => { clearTimeout(timer); code === 0 ? resolve() : reject(Error("Electron exited " + code + ": " + output.slice(-2000))); });
    });
    const expectedState = flipBeforeAck ? "LOCKED" : state;
    const result = { phase, state, expectedState, index, ...JSON.parse(fs.readFileSync(record)), diagnostics: rows, phases: rows.find(r => r.api.endsWith("/startup"))?.value };
    fs.writeFileSync(path.join(directory, "result.json"), JSON.stringify(result, null, 2));
    fs.writeFileSync(path.join(directory, "diagnostic-export.jsonl"), rows.filter(r => r.value.event).map(r => JSON.stringify(r.value)).join("\n") + "\n");
    if (result.error) throw Error(result.error);
    if (phase === "after" || phase === "after-fractional") {
      assert.equal(result.locked.content.state,"locked");assert.equal(Math.round(result.locked.bounds.width),860);assert.equal(Math.round(result.locked.bounds.height),580);
      assert.equal(result.restored.content.state,"active");assert.equal(result.restored.opacity,1);
      for(const k of ['x','y','width','height'])assert.equal(Math.round(result.restored.normal[k]),Math.round(result.beforeLock.normal[k]),`R245 restore ${k}`);
      assert.equal(result.restored.maximized,result.beforeLock.maximized);
      for(const call of result.nativeCalls)for(const value of typeof call.args[0]==='object'?Object.values(call.args[0]):call.args)assert.ok(Number.isInteger(value),'R245 native geometry integer');
      for(const k of ['x','y','width','height'])assert.ok(Number.isInteger(result.saved[k]),'R245 saved geometry integer');
    }
    return result;
  } finally { server.closeAllConnections(); await new Promise(resolve => server.close(resolve)); fs.rmSync(temp, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }); }
}
module.exports = { probe };
if (require.main === module) {
  const phase=process.argv[2]||"baseline";
  (async()=>{const results=[];for(const scale of [1.25,1.5,1.75])for(const mode of phase.includes('fractional')?['normal']:['normal','maximized','minimized','occluded'])results.push(await probe({phase,scale,mode,fractionalRead:phase.includes('fractional'),index:results.length+1}));
    fs.mkdirSync(out,{recursive:true});fs.writeFileSync(path.join(out,phase+'.json'),JSON.stringify(results,null,2));
    console.log(JSON.stringify(results.map(r=>({index:r.index,scale:r.scale,before:r.beforeLock,locked:r.locked.bounds,restored:r.restored,occluded:r.occluded,nativeErrors:r.nativeErrors,nonIntegerCalls:r.nativeCalls.filter(c=>!(typeof c.args[0]==='object'?Object.values(c.args[0]):c.args).every(Number.isInteger)),telemetry:r.diagnostics.filter(d=>d.value.licenseWindow).map(d=>d.value.licenseWindow)}))));
  })().catch(error=>{console.error(error);process.exitCode=1});
}
}
