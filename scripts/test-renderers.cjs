"use strict";

// R222: use Node's real file workers, retaining every assertion while recording
// file wall times (including startup) and enforcing the worklist's time budgets.
const { run } = require("node:test");
const fs = require("node:fs"), path = require("node:path"), os = require("node:os");
const root = path.resolve(__dirname, "..");
const scope = process.argv[2] || "all";
if (!["all", "desktop"].includes(scope)) throw new Error("scope must be all or desktop");
const directories = scope === "desktop" ? ["desktop"] : ["backend/web", "desktop", "scripts"];
const files = directories.flatMap(directory => fs.readdirSync(path.join(root, directory))
  .filter(name => name.endsWith(".test.cjs")).sort().map(name => path.join(root, directory, name)));
const fileNames = new Set(files);
const timings = [];
// Two-core private runners must still overlap test files.
let summary;
let failed = false;
(async () => {
  for await (const event of run({ files, concurrency: Math.max(2, os.availableParallelism()) })) {
    const data = event.data;
    if (event.type === "test:stdout" || event.type === "test:stderr") process.stdout.write(data.message);
    if (event.type === "test:pass" && !fileNames.has(data.name)) console.log(`PASS ${data.name}`);
    if (event.type === "test:fail") {
      failed = true;
      console.error(`FAIL ${data.name}: ${data.details.error?.stack || data.details.error}`);
      if (data.details.error?.cause) console.error(data.details.error.cause);
    }
    if (event.type === "test:complete" && fileNames.has(data.name)) {
      const row = { file: path.relative(root, data.file).replaceAll(path.sep, "/"), duration_ms: data.details.duration_ms };
      timings.push(row);
      if (row.duration_ms > 90000) { failed = true; console.error(`File exceeded 90s: ${JSON.stringify(row)}`); }
    }
    if (event.type === "test:summary" && !data.file) summary = data;
  }
  if (!summary?.success || summary.duration_ms > 240000 || timings.length !== files.length) failed = true;
  const result = { scope, platform: process.platform, concurrency: Math.max(2, os.availableParallelism()),
    file_budget_ms: 90000, suite_budget_ms: 240000, files: timings, summary, success: !failed };
  if (process.env.R222_NODE_TIMING_OUTPUT) fs.writeFileSync(process.env.R222_NODE_TIMING_OUTPUT, JSON.stringify(result, null, 2) + "\n");
  console.log(JSON.stringify(result, null, 2));
  process.exitCode = failed ? 1 : 0;
})().catch(error => { console.error(error); process.exitCode = 1; });
