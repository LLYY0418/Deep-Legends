"use strict";

// R222: use Node's real file workers, retaining every assertion while recording
// file wall times (including startup) and enforcing the worklist's time budgets.
const { run } = require("node:test");
const fs = require("node:fs"), path = require("node:path");
const root = path.resolve(__dirname, "..");
const scope = process.argv[2] || "all";
if (!["all", "desktop"].includes(scope)) throw new Error("scope must be all or desktop");
const directories = scope === "desktop" ? ["desktop"] : ["backend/web", "desktop", "scripts"];
let files = directories.flatMap(directory => fs.readdirSync(path.join(root, directory))
  .filter(name => name.endsWith(".test.cjs")).sort().map(name => path.join(root, directory, name)));
const args = process.argv.slice(3);
let shard;
if (args.length) {
  if (scope !== 'all' || args.length !== 2 || args[0] !== '--shard' || !['1', '2'].includes(args[1])) throw Error('expected all --shard 1 or all --shard 2');
  const sharding = require('./renderer-shards.cjs'), inventory = sharding.discover(root);
  const selected = sharding.plan(inventory)[Number(args[1]) - 1];
  shard = sharding.metadata(selected, inventory);
  files = selected.files.map(file => path.join(root, file));
}
const fileNames = new Set(files);
const timings = [], cases = [];
const {fileBudget,suiteBudget,withinBudgets}=require("./renderer-budgets.cjs");
const file_budget_ms=fileBudget(process.platform);
// Full coverage includes both DOM work and asynchronous fixture waits. Five
// bounded workers let those waits overlap; the desktop-only scope keeps four.
const concurrency = scope === "all" ? 5 : 4;
// One bounded worker pool consumes all discovered files, longest first. The
// previous reserved serial lane became the 203s bottleneck on Linux. Measured
// costs only order execution; they never omit tests or change either budget.
const costs = shard ? require('./renderer-shard-costs.json').costs_ms : require("./renderer-costs.json");
const ordered = files.slice().sort((a,b) => {
  const relative = file => path.relative(root,file).replaceAll(path.sep,"/");
  return (costs[relative(b)] || 1000) - (costs[relative(a)] || 1000) || a.localeCompare(b);
});
const groups = [{ name: "shared-pool", files: ordered, concurrency }];
const summaries = [];
let failed = false;
const failures = [];
(async () => {
  const start = performance.now();
  await Promise.all(groups.map(async group => {
    let groupSummary;
    for await (const event of run({ files: group.files, concurrency: group.concurrency })) {
      const data = event.data;
      if (event.type === "test:stdout" || event.type === "test:stderr") process.stdout.write(data.message);
      if (event.type === "test:pass" && !fileNames.has(data.name)) console.log(`PASS ${data.name}`);
      if (event.type === "test:fail") {
        failed = true;
        failures.push({ file: data.file ? path.relative(root, data.file).replaceAll(path.sep, "/") : "", name: data.name, message: String(data.details.error?.stack || data.details.error).slice(0, 600) });
        console.error(`FAIL ${data.name}: ${data.details.error?.stack || data.details.error}`);
        if (data.details.error?.cause) console.error(data.details.error.cause);
      }
      if (event.type === "test:complete" && fileNames.has(data.name)) {
        const row = { file: path.relative(root, data.file).replaceAll(path.sep, "/"), duration_ms: data.details.duration_ms, group: group.name };
        timings.push(row);
        if (!withinBudgets(process.platform,[row],0)) { failed = true; console.error(`File exceeded ${file_budget_ms/1000}s: ${JSON.stringify(row)}`); }
      }
      if (event.type === "test:complete" && !fileNames.has(data.name) && data.file) cases.push({file:path.relative(root,data.file).replaceAll(path.sep,"/"),name:data.name,nesting:data.nesting,duration_ms:data.details.duration_ms});
      if (event.type === "test:summary" && !data.file) groupSummary = data;
    }
    if (!groupSummary?.success) failed = true;
    summaries.push({ name: group.name, files: group.files.length, concurrency: group.concurrency, summary: groupSummary });
  }));
  const duration_ms = performance.now() - start;
  const counts = {};
  for (const group of summaries) for (const [name, count] of Object.entries(group.summary?.counts || {})) counts[name] = (counts[name] || 0) + count;
  const summary = { success: summaries.length === groups.length && summaries.every(group => group.summary?.success), counts, duration_ms };
  if (!summary.success || !withinBudgets(process.platform,timings,summary.duration_ms) || timings.length !== files.length || new Set(timings.map(row => row.file)).size !== files.length) failed = true;
  const result = { scope, ...(shard ? {shard} : {}), platform: process.platform, runner: require("./lib/runner-diagnostics.cjs").runnerMetadata(), timingComparison: require("./lib/runner-diagnostics.cjs").timingComparison(timings,costs), concurrency, groups: summaries,
    file_budget_ms, suite_budget_ms: suiteBudget, files: timings, cases, failures, summary, success: !failed };
  if (process.env.R222_NODE_TIMING_OUTPUT) fs.writeFileSync(process.env.R222_NODE_TIMING_OUTPUT, JSON.stringify(result, null, 2) + "\n");
  console.log(JSON.stringify(result, null, 2));
  process.exitCode = failed ? 1 : 0;
})().catch(error => { console.error(error); process.exitCode = 1; });
