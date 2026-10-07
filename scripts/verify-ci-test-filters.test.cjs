"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const fs = require("node:fs"), os = require("node:os"), path = require("node:path");
const { spawnSync } = require("node:child_process");
const { verifyWorkflow, nodeTests } = require("./verify-ci-test-filters.cjs");
const { verifyGoEvents, verifyNodeTap } = require("./run-ci-tests.cjs");
const workflow = fs.readFileSync(path.join(__dirname, "../.github/workflows/ci.yml"), "utf8");

test("R225 static names exclude comments and synthetic source strings", t => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "r225-declarations-"));
  t.after(() => fs.rmSync(directory, {recursive:true, force:true}));
  const file = path.join(directory, "declarations.test.cjs");
  fs.writeFileSync(file, `// test("comment",()=>{});\n/* test("block",()=>{}); */\nconst synthetic='test("string",()=>{})';\nconst template=\`test("template",()=>{})\`;\nconst regex=/test("regex",.*)/;\ntest("real",()=>{});`);
  assert.deepEqual(nodeTests(file).map(test => test.name), ["real"]);
});

test("R225 actual CI filters resolve every alternative and reject nonexistent names", () => {
  const filters = verifyWorkflow(workflow);
  assert.equal(filters.length, 3);
  assert.ok(filters.every(step => step.kind === "node"));
  assert.throws(() => verifyWorkflow(workflow.replace('R86 Windows release|R222 Windows', 'R86 Windows release|R222 Windows|R225 nonexistent test')), /unmatched node test filter: R225 nonexistent test/);
  assert.throws(() => verifyWorkflow(workflow.replace('--expected-tests=1', '--expected-tests=2')), /count 1 != expected 2/);
  const go = "go test ./backend -run '^(TestR204KeySaveAndClear|TestR204KeyRuntime401AndPrivacy)$'";
  assert.equal(verifyWorkflow(go)[0].names.length, 2);
  assert.throws(() => verifyWorkflow(go.replace('TestR204KeySaveAndClear', 'TestR204KeySaveRejectsUnauthorizedAndEncrypts')), /unmatched go test filter/);
  assert.throws(() => verifyWorkflow(go.replace('TestR204KeySaveAndClear', 'TestWindowsApplicationWindowDetection')), /unmatched go test filter/, "a name in the installer module must not satisfy a backend filter");
});

test("R225 static expected counts also cover the unfiltered R82 PowerShell file", () => {
  const entry = verifyWorkflow(workflow).find(step => step.file === "scripts/r82-startup-ab-script.test.cjs");
  assert.equal(entry.pattern, undefined);
  assert.equal(entry.expected, 2);
  assert.equal(entry.names.length, 2);
  const command = "--expected-tests=2 scripts/r82-startup-ab-script.test.cjs";
  assert.ok(workflow.includes(command));
  assert.throws(() => verifyWorkflow(workflow.replace(command, command.replace("=2", "=3"))), /count 2 != expected 3: scripts\/r82-startup-ab-script\.test\.cjs/);
  assert.throws(() => verifyWorkflow(workflow.replace(command, command.replace("=2", "=invalid"))), /cannot parse Node expected count/);
});

test("R225 runtime Go evidence rejects empty, omitted and skipped required tests", () => {
  const expected = ["backend/TestUpdateApply", "backend/TestR204KeySaveAndClear"];
  const events = expected.map(name => ({ Action: "pass", Package: "backend", Test: name.split("/")[1] }));
  events.push({ Action: "pass", Package: "backend" });
  assert.equal(verifyGoEvents(events, expected, expected).required, 2);
  assert.throws(() => verifyGoEvents([], expected, expected), /empty/);
  assert.throws(() => verifyGoEvents(events.slice(1), expected, expected), /did not finish/);
  assert.throws(() => verifyGoEvents([{...events[0], Action: "skip"}, ...events.slice(1)], expected, expected), /did not pass/);
  assert.throws(() => verifyGoEvents([{ Action: "fail", Package: "backend" }], expected, expected), /Go failed/);
});

test("R225 real Node runner proves a selected test passed and rejects an accidental skip", t => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "r225-filter-proof-"));
  t.after(() => fs.rmSync(directory, {recursive: true, force: true}));
  const file = path.join(directory, "proof.test.cjs");
  const run = () => spawnSync(process.execPath, [path.join(__dirname, "run-ci-tests.cjs"), "node", "--expected-tests=1", "--test-name-pattern=R225 selected", file], {encoding:"utf8", timeout:15000});
  fs.writeFileSync(file, 'const test=require("node:test");test("R225 selected",()=>{});test("unselected",()=>{});');
  let result = run();
  assert.equal(result.status, 0, result.stdout + result.stderr);
  assert.match(result.stdout, /R225_NODE_VERIFIED/);
  fs.writeFileSync(file, 'const test=require("node:test");test("R225 selected",{skip:true},()=>{});');
  result = run();
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /incomplete or skipped/);
  assert.throws(() => verifyNodeTap('# tests 0\n# pass 0\n# fail 0\n# cancelled 0\n# skipped 0\n# todo 0\n', 1), /empty, incomplete or skipped/);
});
