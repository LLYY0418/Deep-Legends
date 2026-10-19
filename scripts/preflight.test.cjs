"use strict";
// R279 P2: the change-aware selector. Synthetic repositories pin each rule; the recall test replays
// REAL historical failures (scripts/preflight-recall-cases.json) and the mutation tests prove the
// recall check fails when a rule is removed.
const test = require("node:test");
const assert = require("node:assert/strict");
const { buildIndex, select, goDeclarations, R277_STRICT, WINDOWS_ONLY_NOTE } = require("./lib/preflight-select.cjs");
const { evaluate, loadIndex } = require("./preflight-recall.cjs");
const { parseArgs } = require("./preflight.cjs");

function repo(files, guards = ["r117", "r264"]) {
  return buildIndex({ files: Object.keys(files), read: (file) => files[file], guards });
}

const synthetic = {
  "backend/web/app.js": "// product",
  "backend/web/app.css": "/* product */",
  "backend/web/other.css": "/* unrelated */",
  "backend/web/harness.cjs": "const fs=require('node:fs');module.exports=()=>fs.readFileSync('backend/web/app.css','utf8');",
  "backend/web/a.test.cjs": "require('node:test');const h=require('./harness.cjs');h();",
  "backend/web/b.test.cjs": "require('node:test');readFile('app.js');",
  "backend/web/c.test.cjs": "require('node:test');// nothing relevant",
  "scripts/lone-tool.cjs": "// not referenced by anything",
  "scripts/ci-only.py": "# run directly by CI",
  ".github/workflows/ci.yml": "steps:\n  - run: python3 scripts/ci-only.py\n",
  "NOTES.md": "notes",
  "backend/alpha.go": "package main\nfunc computeAlphaScore() int { return 1 }\ntype alphaThing struct{}\nfunc (a alphaThing) available() bool { return true }\n",
  "backend/alpha_test.go": "package main\nfunc TestAlpha(t *testing.T) {}\n",
  "backend/beta_test.go": "package main\nfunc TestBeta(t *testing.T) { computeAlphaScore() }\n",
  "backend/gamma_test.go": "package main\nfunc TestGamma(t *testing.T) { available := true; _ = available }\n",
};

test("a product web file selects tests that reference it directly or through a required harness", () => {
  const selection = select(repo(synthetic), ["backend/web/app.css"]);
  assert.deepEqual(selection.nodeTests.map((t) => t.file), ["backend/web/a.test.cjs"]);
  assert.match(selection.nodeTests[0].reasons.join(" "), /经 backend\/web\/harness\.cjs 间接引用|require 依赖/);
  const direct = select(repo(synthetic), ["backend/web/app.js"]);
  assert.deepEqual(direct.nodeTests.map((t) => t.file), ["backend/web/b.test.cjs"]);
});

test("any product web change pulls in the whole browser layer and the Windows-only warning", () => {
  const selection = select(repo(synthetic), ["backend/web/other.css"]);
  assert.deepEqual(selection.nodeTests, []);
  assert.deepEqual(selection.browserGuards.map((g) => g.name).sort(), ["r117", "r264", R277_STRICT].sort());
  assert.deepEqual(selection.windowsOnly, [WINDOWS_ONLY_NOTE]);
  assert.deepEqual(selection.unmapped, [], "browser layer covers an otherwise unreferenced web file");
});

test("a changed test file selects itself", () => {
  const selection = select(repo(synthetic), ["backend/web/c.test.cjs"]);
  assert.deepEqual(selection.nodeTests.map((t) => t.file), ["backend/web/c.test.cjs"]);
});

test("Go: same-name test plus tests that use a function defined in the changed file; method names do not fan out", () => {
  const selection = select(repo(synthetic), ["backend/alpha.go"]);
  assert.deepEqual(selection.goTestFiles.map((g) => g.file), ["backend/alpha_test.go", "backend/beta_test.go"]);
  assert.deepEqual(selection.goTestNames, ["TestAlpha", "TestBeta"]);
  assert.ok(!selection.goTestFiles.some((g) => g.file === "backend/gamma_test.go"), "a local variable named like a method must not select gamma_test.go");
  assert.ok(selection.staticGates.includes("go-vet-backend") && selection.staticGates.includes("gofmt"));
  assert.deepEqual(goDeclarations("package main\nfunc (s matchSpec) accepts() bool { return true }\nfunc helperValue() {}\n"), ["helperValue", "matchSpec"]);
});

test("documentation-only changes need nothing and are not reported as unmapped", () => {
  const selection = select(repo(synthetic), ["NOTES.md"]);
  assert.deepEqual(selection.nodeTests, []);
  assert.deepEqual(selection.unmapped, []);
  assert.equal(selection.recommendProbe, false);
});

test("a script that only CI runs is flagged CI-only; an unreferenced one is unmapped; both recommend a probe", () => {
  const ciOnly = select(repo(synthetic), ["scripts/ci-only.py"]);
  assert.deepEqual(ciOnly.ciOnly, [{ file: "scripts/ci-only.py", workflows: ["ci.yml"] }]);
  assert.equal(ciOnly.recommendProbe, true);
  const lone = select(repo(synthetic), ["scripts/lone-tool.cjs"]);
  assert.deepEqual(lone.unmapped, ["scripts/lone-tool.cjs"]);
  assert.equal(lone.recommendProbe, true);
});

test("a selection above the broad ratio recommends a probe instead of a local full run", () => {
  const files = { "backend/web/app.js": "x" };
  for (let i = 0; i < 5; i++) files[`backend/web/t${i}.test.cjs`] = "require('node:test');// app.js";
  const selection = select(buildIndex({ files: Object.keys(files), read: (f) => files[f], guards: [] }), ["backend/web/app.js"]);
  assert.equal(selection.broad, true);
  assert.equal(selection.recommendProbe, true);
});

test("recall: every recorded historical failure is selected or flagged for the diff that caused it", () => {
  const result = evaluate(loadIndex());
  const missed = result.rows.filter((row) => !row.ok);
  assert.deepEqual(missed, [], `missed: ${missed.map((m) => `${m.id}→${m.missing.join(",")}`).join("; ")}`);
  assert.ok(result.total >= 15, "dataset must keep all recorded failures");
  assert.ok(result.notSelectable.length >= 1, "non-selectable failure classes stay documented");
});

test("mutation: dropping the browser-layer rule or the file-reference scan makes the recall check FAIL", () => {
  const index = loadIndex();
  const noGuards = evaluate({ ...index, guards: [] });
  assert.ok(noGuards.rows.some((row) => !row.ok && row.missing.some((m) => m.startsWith("guard:"))), "browser-layer failures must be missed without the rule");
  const noReferences = evaluate({ ...index, referencedBy: new Map(), scriptRefs: new Map() });
  assert.ok(noReferences.rows.some((row) => !row.ok && row.missing.some((m) => m.startsWith("node:") || m.startsWith("script:"))), "old-contract failures must be missed without the reference scan");
});

test("argument parsing", () => {
  assert.deepEqual(parseArgs(["--list", "--base", "abc", "--files", "a.js, b.js", "--json", "out.json", "--browsers"]), { base: "abc", list: true, json: "out.json", files: ["a.js", "b.js"], browsers: true, help: false });
  assert.throws(() => parseArgs(["--nope"]), /未知参数/);
});

test('explicit browser preflight owns and cleans its dedicated cache and outputs on success or failure',()=>{
 const fs=require('node:fs'),path=require('node:path'),{runBrowsers}=require('./preflight.cjs');
 for(const fail of [false,true]){
  let scratch,cleaned=false;
  const invoke=()=>runBrowsers({browserGuards:[{name:R277_STRICT}]},{runCommand:(label,cmd,args,options)=>{scratch=options.env.PI_SCRATCH_DIR;fs.mkdirSync(path.join(scratch,'r277-browser-gocache'),{recursive:true});if(fail&&args.includes('browser'))throw Error('fixture browser failure');return {ok:true};},cleanCommand:env=>{assert.equal(env.GOCACHE,path.join(scratch,'r277-browser-gocache'));cleaned=true;return {status:0};}});
  if(fail)assert.throws(invoke,/fixture browser failure/);else assert.equal(invoke().length,2);
  assert(cleaned);assert(!fs.existsSync(scratch));
 }
});
