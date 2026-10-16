"use strict";
// R279 P3.3: timing hygiene ratchet. New fixed sleeps, fixed-window polling loops and wall-clock threshold
// assertions are rejected; the existing ones are registered in scripts/test-hygiene-baseline.json and may only decrease.
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs"), path = require("node:path");
const { scanSource, scanRepo, compare, CATEGORIES } = require("./lib/test-hygiene-scan.cjs");
const { build } = require("./test-hygiene-baseline.cjs");
const root = path.resolve(__dirname, "..");
const baseline = JSON.parse(fs.readFileSync(path.join(__dirname, "test-hygiene-baseline.json"), "utf8"));

test("the repository has no more fixed sleeps, polling loops or wall-clock assertions than the baseline", () => {
  const current = scanRepo(root);
  const { increases } = compare(baseline.files, current);
  assert.deepEqual(increases, [], `新增了固定等待：${increases.map((i) => `${i.file} ${i.category} ${i.allowed}→${i.actual}`).join("; ")}。请改用 scripts/lib/wait-progress.cjs。`);
});

test("the baseline never lists debt that was already removed (it must be lowered, so it only ever goes down)", () => {
  const { stale } = compare(baseline.files, scanRepo(root));
  assert.deepEqual(stale, [], `基线高于实际，请运行 node scripts/test-hygiene-baseline.cjs --write：${stale.map((s) => `${s.file} ${s.category}`).join("; ")}`);
});

test("every baseline entry names a real file and carries a real reason", () => {
  for (const [file, entry] of Object.entries(baseline.files)) {
    assert.ok(fs.existsSync(path.join(root, file)), `${file} no longer exists; remove the entry`);
    assert.ok(typeof entry.reason === "string" && entry.reason.length >= 12 && !/TODO|待补/.test(entry.reason), `${file} needs a reason`);
    assert.ok(CATEGORIES.some((c) => entry[c] > 0), `${file} has no counts`);
  }
});

test("mutation: a new fixed sleep / fixed polling loop / wall-clock assertion in a fixture is detected", () => {
  const clean = "const x = await waitFor(() => ready, { idleMs: 15000 });\nawait new Promise(r => setTimeout(r, 20));\n";
  assert.deepEqual(scanSource(clean), { sleep: 0, poll: 0, wall: 0, "silent-timeout":0, "early-ready":0 });
  assert.equal(scanSource(`${clean}await new Promise(resolve => setTimeout(resolve, 500));`).sleep, 1);
  assert.equal(scanSource(`${clean}for (let i = 0; i < 100; i++) { if (await ok()) break; await new Promise(r => setTimeout(r, 50)); }`).poll, 1);
  assert.equal(scanSource(`${clean}while (Date.now() < deadline) { await tick(); }`).poll, 1);
  assert.equal(scanSource(`${clean}const t = Date.now(); run(); assert(Date.now() - t < 100);`).wall, 1);
  const files = { "x.cjs": scanSource(`${clean}await new Promise(resolve => setTimeout(resolve, 500));`) };
  assert.equal(compare({}, files).increases.length, 1, "an unregistered file with a fixed sleep must be reported as an increase");
});

test("comments and strings that merely mention the patterns do not count", () => {
  assert.deepEqual(scanSource("// setTimeout(r, 500) used to be here\n/* while (Date.now() < deadline) */\nconst note = 1;\n"), { sleep: 0, poll: 0, wall: 0, "silent-timeout":0, "early-ready":0 });
});

test("the baseline writer refuses to raise a count without an explicit, reviewed reason", () => {
  const previous = { files: { "a.cjs": { sleep: 1, reason: "existing debt with a reason" } } };
  assert.throws(() => build(previous, { "a.cjs": { sleep: 2, poll: 0, wall: 0 } }, ""), /新增固定等待/);
  assert.throws(() => build(previous, { "b.cjs": { sleep: 1, poll: 0, wall: 0 } }, ""), /新增固定等待/);
  const lowered = build(previous, { "a.cjs": { sleep: 1, poll: 0, wall: 0 } }, "");
  assert.equal(lowered.files["a.cjs"].reason, "existing debt with a reason");
  assert.equal(build(previous, { "b.cjs": { sleep: 1, poll: 0, wall: 0 } }, "reviewed: legacy fixture").files["b.cjs"].reason, "reviewed: legacy fixture");
});

test("R281 mutations: silent timeout and initially true app readiness fail the ratchet",()=>{
 const silent="await new Promise(resolve=>{const t=setInterval(check,50);setTimeout(()=>{clearInterval(t);resolve();},5000);});";
 assert.equal(scanSource(silent)['silent-timeout'],1);
 assert.equal(scanSource(silent.replace('resolve();','reject(Error(\"not ready\"));'))['silent-timeout'],0);
 assert.equal(scanSource(silent.replace('()=>{clearInterval(t);resolve();}','()=>resolve()'))['silent-timeout'],1);
 assert.equal(scanSource(silent.replace('()=>{clearInterval(t);resolve();}','resolve'))['silent-timeout'],1);
 assert.equal(scanSource('await new Promise(resolve=>setTimeout(()=>resolve(),400));')['silent-timeout'],0,'a deliberate pause remains sleep debt, not a readiness timeout');
 const early="await until(\"!document.querySelector('#app-frame').hasAttribute('inert')\");\nawait evaluate(\"dispatchEvent(new CustomEvent('deep-legends:navigate'))\");";
 assert.equal(scanSource(early)['early-ready'],1);
 assert.equal(scanSource("await waitAppReady({evaluate});\n"+early)['early-ready'],0);
 assert.equal(scanSource(early.replace('until(', 'until(\n').replace(');\nawait', '\n);\n'+Array(20).fill('noop();').join('\n')+'\nawait'))['early-ready'],1);
 assert.equal(scanSource("await waitAppReady({evaluate});await call(\n'Page.navigate',{url});\n"+early)['early-ready'],1,'new navigation invalidates earlier readiness');
 for(const [name,source] of [['silent',silent],['early',early]])assert(compare({}, {[name+'.cjs']:scanSource(source)}).increases.some(row=>row.category===(name==='silent'?'silent-timeout':'early-ready')));
});
