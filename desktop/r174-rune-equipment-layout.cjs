// R174: production rune record markup/CSS at three viewport widths.
// Run: node desktop/r174-rune-equipment-layout.cjs
"use strict";
const { spawn } = require("node:child_process");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const http = require("node:http");
const os = require("node:os");
const path = require("node:path");

const root = path.resolve(__dirname, "..");
const web = path.join(root, "backend", "web");
const output = process.env.R174_LAYOUT_OUTPUT || path.join(root, "docs", "r174-validation");
const chrome = process.env.CHROME_BIN || (process.platform === "darwin" ? "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" : "chromium");
const source = fs.readFileSync(path.join(web, "gameplay.js"), "utf8");
const temp = fs.mkdtempSync(path.join(os.tmpdir(), "r174-runes-"));
let browser, socket, server;

function functionSource(name) {
  const start = source.indexOf("function " + name + "(");
  assert.ok(start >= 0, name + " missing");
  const bodyStart = source.indexOf("{", start);
  let depth = 0, quote = "", escaped = false;
  for (let index = bodyStart; index < source.length; index += 1) {
    const character = source[index];
    if (quote) {
      if (escaped) escaped = false;
      else if (character === "\\") escaped = true;
      else if (character === quote) quote = "";
      continue;
    }
    if (character === "'" || character === '"' || character.charCodeAt(0) === 96) {
      quote = character;
      continue;
    }
    if (character === "{") depth += 1;
    if (character === "}" && --depth === 0) return source.slice(start, index + 1);
  }
  assert.fail(name + " braces unbalanced");
}



function iconFigure(kind,id,name,size) {
 const color=kind==="spell"?(Number(id)===4?"#c69b2b":"#758caf"):"#657777";
 const svg="<svg xmlns='http://www.w3.org/2000/svg' width='24' height='24'><rect width='24' height='24' rx='4' fill='"+color+"'/><text x='12' y='16' font-size='8' text-anchor='middle' fill='white'>"+id+"</text></svg>";
 return "<img class='game-icon is-"+size+"' alt='"+name+"' src='data:image/svg+xml,"+encodeURIComponent(svg).replaceAll("'", "%27")+"'>";
}

const names = ["renderSpecialistPlayers","proRuneRecordLabel","renderRuneEquipment","renderItemIcon","renderSummonerSpellIcon"];
const script = [
 'const state={live:{},specialistPlayerTabs:new Map(),selectedRecommendation:"starter-spells",summonerSpells:{spells:[{id:4,name:"闪现"},{id:11,name:"惩戒"},{id:7,name:"治疗"}]}};',
 'const escapeHTML = value => String(value??"").replaceAll("&","&amp;").replaceAll("<","&lt;").replaceAll(">","&gt;").replaceAll(String.fromCharCode(34),"&quot;");',
 'const plainText = value => value;',
 'const positionLabel = value => value, livePositionDisplay = value => value;',
 'const proRequestTarget = () => ({key:"fixture"}), specialistRequestTarget = proRequestTarget;',
 'const runeConfigurationTitle = () => "符文组合";',
 'const renderUnifiedRuneBoard = () => "<div class=fixture-board>符文布局夹具</div>";',
 iconFigure.toString(),
 ...names.map(functionSource),
 'const final=[3006,3072,3031,3085,3036,3139,3364];',
 'const specialist=[{key:"final-spells",playerName:"样例玩家",position:"中路",itemIds:final,spell1Id:4,spell2Id:11},{key:"starter-spells",playerName:"样例玩家",position:"中路",itemIds:final,starterItemIds:[1055,2003,2003,1086,1001,1056],spell1Id:4,spell2Id:11},{key:"spells-only",playerName:"样例玩家",position:"中路",itemIds:[],spell1Id:4,spell2Id:7}];',
 'const pro=[{key:"baseline",playerName:"职业样例",title:"职业样例",position:"中路",itemIds:[3006],recordGames:1,recordWins:1},{key:"pro-opening",playerName:"职业样例",title:"职业样例",position:"中路",itemIds:final,starterItemIds:[1055,2003],recordGames:1,recordWins:1}];',
 'document.querySelector("#specialist").innerHTML=renderSpecialistPlayers(specialist,"specialist");document.querySelector("#professional").innerHTML=renderSpecialistPlayers(pro,"pro");'
].join("\n");
const page = '<!doctype html><html lang="zh-CN" data-theme="dark"><head><meta charset="utf-8"><link rel="stylesheet" href="/app.css"><link rel="stylesheet" href="/gameplay.css"><style>body{margin:0;padding:24px;background:var(--bg)}main{width:calc(100vw - 420px);max-width:900px;margin:auto}h1{font-size:18px}h2{font-size:14px;margin:14px 0}.fixture-board{height:42px;padding:10px;color:var(--muted);font-size:10px}</style></head><body><main><h1>R174 符文装备行布局夹具</h1><h2>绝活哥</h2><section id="specialist"></section><h2>职业选手（无技能）</h2><section id="professional"></section></main></body></html>';
async function main() {
 fs.mkdirSync(output,{recursive:true});
 server=http.createServer((request,response)=>{
  if(request.url==="/"){response.setHeader("Content-Type","text/html");response.end(page);return;}
  if(request.url==="/app.css"||request.url==="/gameplay.css"){response.setHeader("Content-Type","text/css");response.end(fs.readFileSync(path.join(web,request.url.slice(1))));return;}
  response.statusCode=404;response.end();
 });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  browser = spawn(chrome, ["--headless=new", "--no-first-run", "--no-default-browser-check", "--remote-debugging-port=0", "--user-data-dir=" + temp, "about:blank"], { stdio: ["ignore", "ignore", "pipe"] });
  const url = await new Promise((resolve, reject) => {
    let stderr = "";
    const timer = setTimeout(() => reject(new Error("Chrome startup timeout")), 20000);
    browser.once("error", reject);
    browser.stderr.on("data", (bytes) => {
      stderr += bytes;
      const found = stderr.match(/DevTools listening on (ws:\/\/[^\s]+)/);
      if (found) { clearTimeout(timer); resolve(found[1]); }
    });
    browser.once("exit", (code) => reject(new Error("Chrome exited " + code + ": " + stderr.slice(-500))));
  });
  socket = new WebSocket(url);
  await new Promise((resolve, reject) => { socket.addEventListener("open", resolve, { once: true }); socket.addEventListener("error", reject, { once: true }); });
  let sequence = 0;
  const pending = new Map();
  socket.addEventListener("message", (event) => {
    const message = JSON.parse(event.data);
    if (!pending.has(message.id)) return;
    const [resolve, reject] = pending.get(message.id);
    pending.delete(message.id);
    message.error ? reject(new Error(JSON.stringify(message.error))) : resolve(message.result);
  });
  const send = (method, params = {}, sessionId) => new Promise((resolve, reject) => {
    const id = ++sequence;
    pending.set(id, [resolve, reject]);
    socket.send(JSON.stringify({ id, method, params, sessionId }));
  });
  const { targetId } = await send("Target.createTarget", { url: "about:blank" });
  const { sessionId } = await send("Target.attachToTarget", { targetId, flatten: true });
  const call = (method, params = {}) => send(method, params, sessionId);
  const evaluate = async (expression) => {
    const result = await call("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true });
    if (result.exceptionDetails) throw new Error(JSON.stringify(result.exceptionDetails));
    return result.result.value;
  };
  const screenshot = async (name) => {
    const result = await call("Page.captureScreenshot", { format: "png" });
    fs.writeFileSync(path.join(output, name), Buffer.from(result.data, "base64"));
  };

  await call("Page.enable"); await call("Runtime.enable");
  await call("Page.navigate", { url: "http://127.0.0.1:" + server.address().port + "/" });
  await evaluate("new Promise(resolve => { if (document.readyState === 'complete') resolve(); else window.addEventListener('load', resolve, {once:true}); })");
  await evaluate("document.fonts.ready.then(() => true)");
  await evaluate(script);
  const measurements = [];
  for (const theme of ["dark", "light"]) {
    await evaluate("document.documentElement.dataset.theme=" + JSON.stringify(theme));
    for (const width of [1440, 1100, 820]) {
      await call("Emulation.setDeviceMetricsOverride", { width, height: 1300, deviceScaleFactor: 1, mobile: false });
      const measured = await evaluate(`(() => {
        const rect = node => { const r=node.getBoundingClientRect(); return {left:r.left,right:r.right,top:r.top,bottom:r.bottom,width:r.width,height:r.height}; };
        return [...document.querySelectorAll(".specialist-game-items")].map(row => {
          const spells = row.querySelector(".specialist-game-spells"), group = row.querySelector(":scope > div:not(.specialist-game-spells)");
          return {key:row.closest("[data-rune-choice]").dataset.runeChoice,row:rect(row),spells:spells&&rect(spells),group:group&&rect(group),
            icons:[...row.querySelectorAll(".item-option-button")].map(rect),
            spellCount:spells?.children.length||0, last:row.lastElementChild.className,
            overflow:row.scrollWidth-row.clientWidth, wrap:spells&&getComputedStyle(spells).flexWrap, flex:spells&&getComputedStyle(spells).flex,
            onlySpells:row.children.length===1&&!!spells};
        });
      })()`);
      for (const row of measured) {
        assert.ok(row.overflow<=1, width+" equipment overflows: "+JSON.stringify(row));
        if (row.spells) {
          assert.equal(row.last,"specialist-game-spells");
          assert.equal(row.spellCount,2);
          assert.equal(row.wrap,"nowrap");
          assert.equal(row.flex,"0 0 auto");
          assert.ok(Math.abs(row.row.right-row.spells.right-10)<=1,"spells not aligned to right padding: "+JSON.stringify(row));
          assert.ok(row.spells.top>=row.row.top&&row.spells.bottom<=row.row.bottom,"spells moved outside row");
          if (row.group) assert.ok(row.group.right<=row.spells.left-7,"inventory overlaps spells");
        }
      }
      const spellsOnly = measured.find(row=>row.key==="spells-only"), baseline=measured.find(row=>row.key==="baseline");
      assert.ok(spellsOnly.onlySpells,"spell-only row has an orphan label/group");
      assert.equal(spellsOnly.row.height,baseline.row.height,"spell-only row increased minimum equipment height");
      const dense=measured.find(row=>row.key==="starter-spells");
      const tops=new Set(dense.icons.slice(0,-2).map(r=>Math.round(r.top)));
      if(width===820) assert.ok(tops.size>1,"narrow inventory did not wrap");
      measurements.push({theme,width,rows:measured});
      await screenshot("rune-equipment-"+theme+"-"+width+".png");
    }
  }
  fs.writeFileSync(path.join(output,"layout-measurements.json"),JSON.stringify(measurements,null,2)+"\n");
  console.log("R174 three-width dark/light layout and spell-only height checks PASS");
}
main().catch(error=>{console.error(error);process.exitCode=1;}).finally(()=>{
 socket?.close();browser?.kill();server?.close();
 fs.rmSync(temp,{recursive:true,force:true,maxRetries:8,retryDelay:100});
});
