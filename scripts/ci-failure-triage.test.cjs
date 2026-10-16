"use strict";
// R279 P5: the facts-only triage summary. Every fixture below is a failure shape that CI really produced.
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs"), path = require("node:path"), os = require("node:os");
const { annotations, classify, parseLog, triage, markdown, parseArgs, relFile } = require("./ci-failure-triage.cjs");

test("categories: wait timeout, watchdog, assertion, crash and other are told apart", () => {
  assert.equal(classify("full app did not mount: no observable progress for 15000 ms; last progress {\"nodes\":3}"), "wait-timeout");
  assert.equal(classify("condition timeout: document.title"), "wait-timeout");
  assert.equal(classify("hard limit 120000 ms reached"), "watchdog");
  assert.equal(classify("File exceeded 90s: {\"file\":\"x\"}"), "watchdog");
  assert.equal(classify("AssertionError [ERR_ASSERTION]: Expected values to be strictly equal"), "assertion");
  assert.equal(classify("panic: runtime error: invalid memory address"), "crash");
  assert.equal(classify("Target closed"), "crash");
  assert.equal(classify("something unexpected happened"), "other");
  assert.equal(classify(""), "other");
});

test("renderer timing JSON: failing case, file and category; in-diff is computed against the change set", () => {
  const data = { success: false, failures: [{ file: "backend/web/r192.test.cjs", name: "R192 hydrate", message: "Error: full app did not mount" }, { file: "desktop/pro-players.test.cjs", name: "tabs", message: "AssertionError: 1 !== 2" }], files: [] };
  const report = triage({ job: "quality", base: "x", json: ["t.json"], changed: new Set(["desktop/pro-players.test.cjs"]) }, { json: () => data });
  assert.equal(report.failureCount, 2);
  const byFile = Object.fromEntries(report.failures.map((f) => [f.file, f]));
  assert.deepEqual([byFile["backend/web/r192.test.cjs"].category, byFile["backend/web/r192.test.cjs"].inDiff], ["wait-timeout", false]);
  assert.deepEqual([byFile["desktop/pro-players.test.cjs"].category, byFile["desktop/pro-players.test.cjs"].inDiff], ["assertion", true]);
  assert.equal(byFile["backend/web/r192.test.cjs"].evidence, "t.json");
});

test("unknown base: in-diff is reported as unknown (null), never guessed", () => {
  const data = { failures: [{ file: "scripts/a.test.cjs", name: "a", message: "x" }] };
  const report = triage({ job: "j", json: ["t.json"], changed: null }, { json: () => data });
  assert.equal(report.failures[0].inDiff, null);
  assert.match(markdown(report), /在本次改动内：未知/);
});

test("log shapes: renderer FAIL lines, TAP not ok blocks, Go --- FAIL and panics", () => {
  const log = [
    "FAIL R264 countdown: AssertionError: expected 2 秒",
    "    at file:///work/scripts/r264-browser.cjs:155:9",
    "--- FAIL: TestSomething (0.01s)",
    "    thing_test.go:12: boom",
    "not ok 3 - overview tab",
    "  ---",
    "  location: '/work/desktop/overview.test.cjs:44:1'",
    "  error: timeout waiting",
    "panic: runtime error: index out of range",
  ].join("\n");
  const rows = parseLog(log, "job.log");
  assert.deepEqual(rows.map((r) => r.kind), ["node-test", "go-test", "node-test", "go-test"]);
  assert.equal(rows[0].file, "scripts/r264-browser.cjs");
  assert.equal(rows[2].file, "desktop/overview.test.cjs");
  assert.equal(classify(rows[3].message + rows[3].name), "crash");
});

test("failure.json from a guard is summarised with its scenario and error", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "triage-"));
  try {
    fs.mkdirSync(path.join(dir, "strict"));
    fs.writeFileSync(path.join(dir, "strict", "failure.json"), JSON.stringify({ scenario: "p7-1280-1.75", error: "{\"width\":1280,\"zoom\":1.75} overlap assertion" }));
    fs.writeFileSync(path.join(dir, "step.log"), "all good\n");
    const report = triage({ job: "browser", evidence: [dir], changed: new Set() });
    assert.equal(report.failureCount, 1);
    assert.equal(report.failures[0].name, "p7-1280-1.75");
    assert.equal(report.failures[0].category, "assertion");
    assert.ok(report.failures[0].evidence.endsWith("failure.json"));
  } finally { fs.rmSync(dir, { recursive: true, force: true }); }
});

test("it makes no root-cause claim and never mentions CI-KNOWN-ISSUES; duplicates collapse", () => {
  const data = { failures: [{ file: "a.test.cjs", name: "n", message: "m" }, { file: "a.test.cjs", name: "n", message: "m" }] };
  const report = triage({ job: "j", json: ["t.json"], changed: new Set() }, { json: () => data });
  assert.equal(report.failureCount, 1);
  assert.doesNotMatch(JSON.stringify(report) + markdown(report), /根因是|已知问题条目|CI-KNOWN-ISSUES 匹配/);
  assert.match(report.note, /不判断根因/);
});

test("an unreadable or empty input produces an empty, harmless report", () => {
  const report = triage({ job: "j", json: ["missing.json"], log: ["missing.log"], changed: new Set() }, { json: () => null, text: () => { throw new Error("ENOENT"); } });
  assert.equal(report.failureCount, 0);
  assert.match(markdown(report), /没有在给定的日志/);
});

test("end to end: the CLI exits 0 even for a red job and writes triage.json", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "triage-cli-"));
  try {
    fs.writeFileSync(path.join(dir, "r.json"), JSON.stringify({ success: false, failures: [{ file: "scripts/x.test.cjs", name: "x", message: "AssertionError" }] }));
    const out = path.join(dir, "triage.json"), summary = path.join(dir, "summary.md");
    const result = require("node:child_process").spawnSync(process.execPath, [path.join(__dirname, "ci-failure-triage.cjs"), "--job", "t", "--json", path.join(dir, "r.json"), "--out", out, "--summary", summary], { encoding: "utf8" });
    assert.equal(result.status, 0, result.stderr);
    assert.equal(JSON.parse(fs.readFileSync(out, "utf8")).failures[0].category, "assertion");
    assert.match(fs.readFileSync(summary, "utf8"), /CI 失败分诊（t）/);
    const bad = require("node:child_process").spawnSync(process.execPath, [path.join(__dirname, "ci-failure-triage.cjs"), "--nope"], { encoding: "utf8" });
    assert.equal(bad.status, 0, "advisory tool must never fail a job");
  } finally { fs.rmSync(dir, { recursive: true, force: true }); }
});

test("argument parsing and path normalisation", () => {
  assert.deepEqual(parseArgs(["--job", "q", "--log", "a", "--log", "b", "--evidence", "d", "--base", "origin/main", "--out", "o", "--summary", "s", "--annotate"]), { job: "q", base: "origin/main", log: ["a", "b"], json: [], evidence: ["d"], out: "o", summary: "s", annotate: true });
  assert.equal(relFile("C:\\w\\desktop\\a.test.cjs"), "desktop/a.test.cjs");
  assert.equal(relFile("/runner/work/repo/scripts/x.cjs"), "scripts/x.cjs");
});

test("annotations: one ::error line per failure, escaped, capped at 10, no newlines inside", () => {
  const failures = Array.from({ length: 12 }, (_, i) => ({ kind: "node-test", name: `case ${i}: a,b`, file: "scripts/x.test.cjs", inDiff: i % 2 === 0, category: "assertion", message: `line1\nline2 100%` }));
  const lines = annotations({ failures });
  assert.equal(lines.length, 10);
  for (const line of lines) {
    assert.match(line, /^::error title=triage node-test%3A case \d+%3A a%2Cb::category=assertion; file=scripts\/x\.test\.cjs; in-diff=(yes|no); line1 line2 100%25$/);
    assert.ok(!/\n/.test(line));
  }
  assert.match(annotations({ failures: [{ kind: "go-test", name: "T", file: null, inDiff: null, category: "other", message: "m" }] })[0], /file=unknown; in-diff=unknown/);
});

test("log shapes: file budget lines and bare error lines are kept, structured failures sort first", () => {
  const log = ["Error: browser guard exploded", "File exceeded 90s: {\"file\":\"desktop/pro-players.test.cjs\",\"duration_ms\":120000}", "FAIL a case: AssertionError: x"].join("\n");
  const report = triage({ job: "j", log: ["l"], changed: new Set(["desktop/pro-players.test.cjs"]) }, { text: () => log });
  assert.deepEqual(report.failures.map((f) => f.kind), ["node-test", "node-test", "error-line"]);
  const budget = report.failures.find((f) => f.file === "desktop/pro-players.test.cjs");
  assert.equal(budget.category, "watchdog");
  assert.equal(budget.inDiff, true);
});

test("GitHub log decoration: ##[error] lines are read as error lines", () => {
  const rows = parseLog("##[error]AssertionError [ERR_ASSERTION]: boom\n##[error]Process completed with exit code 1.\n", "l");
  assert.deepEqual(rows.map((r) => r.name), ["AssertionError [ERR_ASSERTION]: boom", "Process completed with exit code 1."]);
});

test('released baseline is resolved from the candidate ancestry; stale main is not assumed',()=>{
 const {resolveBase}=require('./ci-failure-triage.cjs');
 let args;assert.equal(resolveBase('release-base',(_,a)=>{args=a;return 'v0.12.85\n';}),'v0.12.85');
 assert.deepEqual(args,['describe','--tags','--match','v*','--abbrev=0']);
 assert.equal(resolveBase('release-base',()=>{throw Error('tag unavailable');}),null);
 assert.equal(resolveBase('explicit-ref'),'explicit-ref');
 const report=triage({base:'release-base',json:['x'],changed:new Set()},{exec:()=> 'v0.12.85\n',json:()=>({failures:[{file:'backend/web/app.js',name:'old product',message:'x'}]})});
 assert.equal(report.base,'v0.12.85');assert.equal(report.failures[0].inDiff,false);
});

test('failure JSON retains the actual assertion file from Linux and Windows stacks',()=>{
 const {parseFailureJson}=require('./ci-failure-triage.cjs');
 for(const stack of ['Error: bad\n    at run (/home/runner/work/repo/scripts/r272-browser-checks.cjs:33:558)',String.raw`Error: bad
    at run (D:\a\repo\scripts\r272-browser-checks.cjs:33:558)`]){
  assert.equal(parseFailureJson({error:'bad',stack},'failure.json')[0].file,'scripts/r272-browser-checks.cjs');
 }
});


function offlineTriageFixture() {
  const main=path.resolve('/fixture/main'),offline=path.join(main,'r275-offline');
  const files=new Map();
  files.set(path.join(main,'browser-job.json'),[{name:'r275-offline',status:'passed',exitCode:0}]);
  files.set(path.join(offline,'result.json'),{blockedUpstreamControl:true,championMutant:'FAIL',spellsMutant:'FAIL',fixed:'PASS'});
  files.set(path.join(offline,'fixed/result.json'),{errors:[],violations:[]});
  for(const [name,endpoint] of [['no-champion-fixture','/api/champion-asset'],['no-spells-fixture','/api/gameplay/summoner-spells']]){
    const errors=[{source:'network',level:'error',text:'Failed to load resource: the server responded with a status of 404 (Not Found)',url:'http://127.0.0.1:1234'+endpoint}];
    files.set(path.join(offline,name,'failure.json'),{error:JSON.stringify(errors)+'\n\n1 !== 0\n',errors,violations:[]});
  }
  const real=path.join(main,'r266-browser/failure.json');files.set(real,{error:'click long tasks 171',stack:'AssertionError: click long tasks 171\n at run (/work/scripts/r266-browser.cjs:117:124)'});
  const json=[...files.keys()].filter(f=>f.endsWith('failure.json'));
  return {files,main,offline,json,real,report:()=>triage({job:'quick',json,changed:new Set()},{json:f=>files.get(f)||null})};
}

test('verified offline negative controls stay visible as expected evidence while the actual failure stays red',()=>{
  const f=offlineTriageFixture(),r=f.report();
  assert.equal(r.failureCount,1,'expected offline negative controls must not inflate actual failures');
  assert.equal(r.failures[0].name,'r266-browser');
  assert.equal(r.expectedNegatives.length,2);
  assert.deepEqual(r.expectedNegatives.map(n=>n.name).sort(),['no-champion-fixture','no-spells-fixture']);
  assert(r.expectedNegatives.every(n=>n.evidence.endsWith('failure.json')&&n.parentEvidence.endsWith('browser-job.json')));
  assert.equal(annotations(r).length,1);assert.match(annotations(r)[0],/r266-browser/);
  assert.match(markdown(r),/已验证预期负例：2/);
});

test('unverified negative controls and unrelated errors must remain failures',async t=>{
  const variants=[
    ['missing parent',f=>f.files.delete(path.join(f.main,'browser-job.json'))],
    ['failed parent',f=>f.files.get(path.join(f.main,'browser-job.json'))[0].status='failed'],
    ['parent exit1',f=>f.files.get(path.join(f.main,'browser-job.json'))[0].exitCode=1],
    ['missing terminal proof',f=>f.files.delete(path.join(f.offline,'result.json'))],
    ['fixed failed',f=>f.files.get(path.join(f.offline,'result.json')).fixed='FAIL'],
    ['mutant survived',f=>f.files.get(path.join(f.offline,'result.json')).spellsMutant='PASS'],
    ['control absent',f=>f.files.get(path.join(f.offline,'result.json')).blockedUpstreamControl=false],
    ['missing fixed result',f=>f.files.delete(path.join(f.offline,'fixed/result.json'))],
    ['fixed has errors',f=>f.files.get(path.join(f.offline,'fixed/result.json')).errors.push({text:'TypeError'})],
    ['fixed CSP failure',f=>f.files.get(path.join(f.offline,'fixed/result.json')).violations.push({blockedURI:'x'})],
    ['unrelated error',f=>{for(const n of ['no-champion-fixture','no-spells-fixture'])f.files.get(path.join(f.offline,n,'failure.json')).errors.push({text:'TypeError: unrelated'});}],
    ['wrong endpoint',f=>{for(const n of ['no-champion-fixture','no-spells-fixture'])f.files.get(path.join(f.offline,n,'failure.json')).errors[0].url='http://127.0.0.1:1234/api/other';}],
    ['wrong status',f=>{for(const n of ['no-champion-fixture','no-spells-fixture'])f.files.get(path.join(f.offline,n,'failure.json')).errors[0].text='Failed to load resource: the server responded with a status of 500';}],
    ['unexpected assertion',f=>{for(const n of ['no-champion-fixture','no-spells-fixture'])f.files.get(path.join(f.offline,n,'failure.json')).error='TypeError: unrelated';}],
    ['negative CSP failure',f=>{for(const n of ['no-champion-fixture','no-spells-fixture'])f.files.get(path.join(f.offline,n,'failure.json')).violations.push({blockedURI:'x'});}],
    ['untrusted host',f=>{for(const n of ['no-champion-fixture','no-spells-fixture'])f.files.get(path.join(f.offline,n,'failure.json')).errors[0].url='http://example.invalid/api/champion-asset';}],
    ['empty errors',f=>{for(const n of ['no-champion-fixture','no-spells-fixture'])f.files.get(path.join(f.offline,n,'failure.json')).errors=[];}],
    ['malformed errors',f=>{for(const n of ['no-champion-fixture','no-spells-fixture'])f.files.get(path.join(f.offline,n,'failure.json')).errors=null;}],
  ];
  for(const [name,change] of variants)await t.test(name,()=>{const f=offlineTriageFixture();change(f);if(name!=='unexpected assertion')for(const n of ['no-champion-fixture','no-spells-fixture']){const d=f.files.get(path.join(f.offline,n,'failure.json'));if(Array.isArray(d.errors))d.error=JSON.stringify(d.errors)+'\n\n'+d.errors.length+' !== 0\n';}const r=f.report();assert.equal(r.failureCount,3,'unverified or unrelated failures must never be suppressed');assert.equal(r.expectedNegatives.length,0);});
});


test('the verified missing champion fixture may return either observed 400 or 404 on its exact endpoint',()=>{
  const f=offlineTriageFixture(),d=f.files.get(path.join(f.offline,'no-champion-fixture','failure.json'));
  d.errors[0].text='Failed to load resource: the server responded with a status of 400 (Bad Request)';d.error=JSON.stringify(d.errors)+'\n\n1 !== 0\n';
  const r=f.report();assert.equal(r.failureCount,1);assert.equal(r.expectedNegatives.length,2);assert.equal(r.failures[0].name,'r266-browser');
});
