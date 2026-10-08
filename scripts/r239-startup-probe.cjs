"use strict";
const { evidencePath, requireEvidence } = require('./local-evidence.cjs');
if (require.main === module && process.env.DEEP_LEGENDS_TEST_LICENSE !== "1" && !["--backend", "--probe-backend"].includes(process.argv[2])) throw Error("Explicit license test switch required: DEEP_LEGENDS_TEST_LICENSE=1");

// Real Electron windows + real renderer, isolated synthetic backend/userData.
// Measures warm macOS starts; it does not represent Windows cold-install timing.
const fs = require("node:fs"), path = require("node:path"), os = require("node:os"), http = require("node:http");
const { spawn } = require("node:child_process");
const root = path.resolve(__dirname, "..");
if (process.argv[2] === "--backend") {
  const state = process.env.R239_PROBE_STATE, web = path.join(root, "backend/web");
  const server = http.createServer((req, res) => {
    const name = new URL(req.url, "http://fixture").pathname;
    const json = value => { res.setHeader("Content-Type", "application/json"); res.end(JSON.stringify(value)); };
    if (name === "/api/license/status") return json({ state, generation: 1 });
    if (name === "/api/quit") { res.end(); return; }
    if (name.startsWith("/api/diagnostics/startup")) {
      let body = ""; req.on("data", chunk => { body += chunk; });
      req.on("end", () => { fs.appendFileSync(process.env.R239_PROBE_RECORD, JSON.stringify({ api: name, value: JSON.parse(body) }) + "\n"); res.writeHead(204); res.end(); }); return;
    }
    if (name === "/api/events") { res.writeHead(200, { "Content-Type": "text/event-stream" }); res.write(": fixture\n\n"); return; }
    if (name === "/api/status") return json({ connected: false, installations: [] });
    if (name.startsWith("/api/")) return json({});
    const file = path.resolve(web, name === "/" ? "index.html" : "." + name);
    if (!file.startsWith(web + path.sep) || !fs.existsSync(file) || !fs.statSync(file).isFile()) { res.writeHead(404); res.end(); return; }
    res.setHeader("Content-Type", ({ ".js": "text/javascript", ".css": "text/css", ".html": "text/html", ".png": "image/png" })[path.extname(file)] || "application/octet-stream"); res.end(require(path.join(root,"desktop/license-render-fixture.cjs")).licenseFixtureHTML(fs.readFileSync(file)));
  });
  server.listen(0, "127.0.0.1", () => { const baseUrl = `http://127.0.0.1:${server.address().port}`; console.log("LOOT_READY " + JSON.stringify({ baseUrl, bootstrapUrl: baseUrl, token: "synthetic-r239-local-token-".repeat(2) })); });
} else {
  const [label = "after", source = path.join(root, "desktop/main.cjs")] = process.argv.slice(2);
  const out = evidencePath('r239'), samples = [];
  async function sample(state, index, sourcePath = source, phase = label) {
    const temp = fs.mkdtempSync(path.join(os.tmpdir(), "r239-electron-"));
    try {
      const shell = path.join(temp, "desktop"); fs.mkdirSync(shell);
      for (const entry of fs.readdirSync(path.join(root, "desktop"))) if (entry !== "main.cjs" && entry !== "license-build.cjs") fs.symlinkSync(path.join(root, "desktop", entry), path.join(shell, entry));
      fs.copyFileSync(sourcePath, path.join(shell, "main.cjs"));
      fs.writeFileSync(path.join(shell,"license-build.cjs"),'module.exports = Object.freeze({enabled:true});\n');
      const record = path.join(temp, "record.jsonl"), launcher = path.join(temp, "backend");
      // Launcher arguments are --desktop/--no-browser, so select the fixture explicitly.
      fs.writeFileSync(launcher, `#!${process.execPath}\nprocess.argv[2]='--backend';require(${JSON.stringify(__filename)});\n`, { mode: 0o700 });
      const wrapper = path.join(temp, "probe.cjs");
      fs.writeFileSync(wrapper, `const {app}=require('electron'),fs=require('node:fs');
app.setName(${JSON.stringify("R239 startup probe")});
app.setPath('userData',${JSON.stringify(path.join(temp, "userData"))});
let count=0;app.on('browser-window-created',(_e,w)=>{const number=++count;
 if(number!==2)return;
 const events=[];let previous='';const sampling=setInterval(()=>{if(w.isDestroyed())return;const bounds=w.getContentBounds(),visible=w.isVisible(),current=JSON.stringify({bounds,visible});if(current!==previous){previous=current;events.push({kind:'sample',at:Date.now(),bounds,visible});}},8);
 w.once('ready-to-show',()=>setTimeout(async()=>{
  const page=await w.webContents.executeJavaScript('({state:document.documentElement.dataset.license,privacyDisplay:getComputedStyle(document.querySelector("#license-privacy-dialog")).display,zoom:getComputedStyle(document.querySelector("#license-form")).zoom})');
  clearInterval(sampling);fs.appendFileSync(${JSON.stringify(record)},JSON.stringify({native:events,page,visible:w.isVisible(),content:w.getContentBounds(),resizable:w.isResizable(),maximizable:w.isMaximizable()})+'\\n');app.quit();
 },1000));
});
app.whenReady().then(()=>require('electron').session.defaultSession.webRequest.onBeforeRequest({urls:['*://*/*']},(d,cb)=>cb({cancel:!d.url.startsWith('http://127.0.0.1:')})));
require(${JSON.stringify(path.join(shell, "main.cjs"))});
`);
      fs.mkdirSync(path.join(temp, "userData"));
      const result = await new Promise((resolve, reject) => {
        const env = { ...process.env, LOOT_BACKEND: launcher, R239_PROBE_STATE: state, R239_PROBE_RECORD: record }; delete env.ELECTRON_RUN_AS_NODE;
        fs.writeFileSync(path.join(temp, "package.json"), JSON.stringify({name:'r239-probe',version:'1.0.0',main:'probe.cjs'}));
        const child = spawn(require(path.join(root, "desktop/node_modules/electron")), ["--disable-gpu", temp], { env, stdio: ["ignore", "pipe", "pipe"] }); let output = "";
        child.stdout.on("data", c => { output += c; }); child.stderr.on("data", c => { output += c; });
        const timer = setTimeout(() => { child.kill(); reject(Error("Electron probe timeout: " + output.slice(-1200))); }, 25000);
        child.once("error", reject); child.once("exit", code => { clearTimeout(timer); code === 0 ? resolve() : reject(Error("Electron exited " + code + ": " + output.slice(-1200))); });
      });
      const rows = fs.readFileSync(record, "utf8").trim().split("\n").map(JSON.parse);
      const phases = rows.find(r => r.api === "/api/diagnostics/startup")?.value;
      const native = rows.find(r => r.native);
      if (!phases || !native) throw Error("missing real startup evidence");
      const logDirectory = path.join(temp, "userData/logs");
      const statusReads = fs.existsSync(logDirectory) ? fs.readdirSync(logDirectory).flatMap(name => [...fs.readFileSync(path.join(logDirectory, name), "utf8").matchAll(/授权窗口状态 (\{[^\n]+\})/g)].map(match => JSON.parse(match[1]))) : [];
      samples.push({ phase, state, index, phases, statusReads, stages: rows.filter(r => r.api?.endsWith("startup-stage")), ...native });
      if (phase.startsWith("after")) {
        if (!native.visible || !native.native.some(row => row.visible)) throw Error("main window was never visible");
        if (state === "ACTIVE" && native.native.some(row => row.visible && row.bounds.width === 700)) throw Error("ACTIVE startup flashed the small window");
        if (state === "LOCKED" && (native.content.width !== 700 || native.content.height !== 470 || native.resizable || native.maximizable)) throw Error("locked native window policy failed");
      }
      console.log(JSON.stringify(samples.at(-1)));
    } catch (error) {
      const logs = path.join(temp, 'userData/logs');
      if (fs.existsSync(logs)) { fs.mkdirSync(out, {recursive:true}); fs.cpSync(logs, path.join(out, `startup-${label}-failure-logs`), {recursive:true}); }
      throw error;
    } finally { fs.rmSync(temp, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }); }
  }
  (async () => {
    for (const state of ["ACTIVE", "LOCKED"]) for (let n = 1; n <= 3; n++) {
      if (label === "paired") {
        await sample(state, n, source, "before-paired");
        await sample(state, n, path.join(root, "desktop/main.cjs"), "after-paired");
      } else await sample(state, n);
    }
    fs.mkdirSync(out, { recursive: true }); fs.writeFileSync(path.join(out, `startup-${label}.json`), JSON.stringify({ scope: "Real Electron/macOS warm startup; synthetic local status, no real codes or external network", samples }, null, 2) + "\n");
  })().catch(e => { console.error(e); process.exitCode = 1; });
}
