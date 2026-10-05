"use strict";

const path = require("node:path"), readline = require("node:readline");
const { spawn, spawnSync } = require("node:child_process");
const { goTests, nodeTests, selectTests, requiredWindowsBackendNames } = require("./verify-ci-test-filters.cjs");
const root = path.resolve(__dirname, "..");

function verifyGoEvents(events, expected, required) {
  const terminal = new Map();
  let packagePasses = 0;
  for (const event of events) {
    if (event.Action === "fail") throw new Error(`Go failed: ${event.Package}/${event.Test || "package"}`);
    if (event.Test && !event.Test.includes("/") && ["pass", "skip"].includes(event.Action)) terminal.set(`${event.Package}/${event.Test}`, event.Action);
    if (!event.Test && event.Action === "pass") packagePasses++;
  }
  if (!packagePasses || !expected.length || !required.length) throw new Error("Go test inventory or package pass is empty");
  for (const name of expected) if (!terminal.has(name)) throw new Error(`Go test did not finish: ${name}`);
  for (const name of required) if (terminal.get(name) !== "pass") throw new Error(`required Windows Go test did not pass: ${name}`);
  return { expected: expected.length, passed: [...terminal.values()].filter(action => action === "pass").length,
    skipped: [...terminal.values()].filter(action => action === "skip").length, required: required.length, requiredPassed: required };
}

function verifyNodeTap(output, expected) {
  const counts = {};
  for (const name of ["tests", "pass", "fail", "cancelled", "skipped", "todo"]) {
    const rows = [...output.matchAll(new RegExp(`^# ${name} (\\d+)\\r?$`, "gm"))];
    if (rows.length !== 1) throw new Error(`missing or ambiguous Node TAP ${name} count`);
    counts[name] = Number(rows[0][1]);
  }
  if (counts.tests !== expected || counts.pass !== expected || counts.fail || counts.cancelled || counts.skipped || counts.todo) {
    throw new Error(`Node selection was empty, incomplete or skipped: ${JSON.stringify(counts)}; expected ${expected}`);
  }
  return counts;
}

async function runGo(module) {
  if (!["backend", "installer"].includes(module)) throw new Error("Go module must be backend or installer");
  const cwd = module === "backend" ? root : path.join(root, "installer");
  const target = module === "backend" ? "./backend" : "./...";
  const listing = spawnSync("go", ["list", "-f", '{{.ImportPath}}|{{.Dir}}|{{join .TestGoFiles ","}}|{{join .XTestGoFiles ","}}', target], { cwd, encoding: "utf8", timeout: 60000 });
  if (listing.error || listing.status !== 0) throw new Error(listing.error?.message || listing.stderr);
  const declarations = goTests(module);
  const expected = [], required = [];
  for (const row of listing.stdout.trim().split(/\r?\n/)) {
    const [packageName, directory, tests, external] = row.split("|");
    const compiled = new Set(`${tests},${external}`.split(",").filter(Boolean).map(file => path.resolve(directory, file)));
    for (const test of declarations) {
      if (!compiled.has(path.resolve(root, test.file))) continue;
      const name = `${packageName}/${test.name}`;
      expected.push(name);
      // All installer tests must pass. Backend permits unrelated platform skips,
      // but every restored Windows path below must produce an actual pass.
      if (module === "installer" || /^(?:TestUpdate|TestR204|TestR201|TestR198|TestSplitRegistryPath|Test(?:Client|WindowsClient|BuildDetected|LaunchClient|OfficialLogin|MergeClient|ClassifyClient)|TestAcceptFocus(?:Request|Sampler|Canceled|History|ExportInspection)|TestR86EmptyProcess|TestRiotClientCommandLine|TestPowerShellProcessOutput)/.test(test.name)) required.push(name);
    }
  }
  if (process.platform === "win32") {
    const mandatory = module === "backend" ? requiredWindowsBackendNames
      : declarations.filter(test => test.file.endsWith("_windows_test.go")).map(test => test.name);
    for (const name of mandatory) {
      const compiledName = expected.find(test => test.endsWith(`/${name}`));
      if (!compiledName) throw new Error(`required Windows test was excluded from this build: ${name}`);
      if (!required.includes(compiledName)) required.push(compiledName);
    }
  }
  const events = [];
  const started = Date.now();
  const child = spawn("go", ["test", "-count=1", "-json", target], { cwd, stdio: ["ignore", "pipe", "inherit"] });
  const completed = new Promise((resolve, reject) => { child.on("error", reject); child.on("close", resolve); });
  const lines = readline.createInterface({ input: child.stdout });
  for await (const line of lines) {
    const event = JSON.parse(line);
    events.push(event);
    if (event.Output) process.stdout.write(event.Output); // Includes every --- PASS.
  }
  const code = await completed;
  if (code !== 0) throw new Error(`go test exited ${code}`);
  const summary = { module, duration_ms: Date.now() - started, ...verifyGoEvents(events, expected, required) };
  console.log(`R225_GO_VERIFIED ${JSON.stringify(summary)}`);
}

async function runNode(args) {
  const count = args.find(arg => arg.startsWith("--expected-tests="));
  const expected = Number(count?.split("=")[1]);
  if (!Number.isSafeInteger(expected) || expected < 1) throw new Error("--expected-tests must be positive");
  args = args.filter(arg => arg !== count);
  const file = args.find(arg => arg.endsWith(".test.cjs"));
  const pattern = args.find(arg => arg.startsWith("--test-name-pattern="))?.slice("--test-name-pattern=".length);
  const declarations = nodeTests(file);
  const selected = pattern ? selectTests(pattern, declarations, "node") : declarations;
  if (selected.length !== expected) throw new Error(`Node static selection ${selected.length} != expected ${expected}`);
  // This is an independent CLI run, including when its guard is tested by a
  // parent node:test worker. Inheriting that worker context silently runs zero.
  const environment = { ...process.env };
  delete environment.NODE_TEST_CONTEXT;
  const child = spawn(process.execPath, ["--test", "--test-reporter=tap", ...args], { cwd: root, env: environment, stdio: ["ignore", "pipe", "inherit"] });
  const completed = new Promise((resolve, reject) => { child.on("error", reject); child.on("close", resolve); });
  let output = "";
  for await (const chunk of child.stdout) { output += chunk; process.stdout.write(chunk); }
  const code = await completed;
  if (code !== 0) throw new Error(`node --test exited ${code}`);
  console.log(`R225_NODE_VERIFIED ${JSON.stringify(verifyNodeTap(output, expected))}`);
}

module.exports = { verifyGoEvents, verifyNodeTap };
if (require.main === module) {
  const [kind, ...args] = process.argv.slice(2);
  const operation = kind === "go" ? runGo(args[0]) : kind === "node" ? runNode(args) : Promise.reject(new Error("kind must be go or node"));
  operation.catch(error => { console.error(error.stack); process.exitCode = 1; });
}
