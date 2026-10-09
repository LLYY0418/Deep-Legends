"use strict";

// Renderer budget gate, option B: GitHub assigns hosted runners whose speed
// differs by ~1.6x for the same commit (same image, every renderer file slower
// by the same factor). This script runs one fixed jsdom workload in the same
// number of concurrent worker processes as scripts/test-renderers.cjs, so the
// gate can compare the renderer suite against the speed of the runner that
// actually executed it. It never touches the renderer tests or their budgets.
const { fork } = require("node:child_process");
const fs = require("node:fs"), os = require("node:os"), path = require("node:path");

const WORKLOAD_VERSION = 1;
const DEFAULT_ITERATIONS = 40;
const DEFAULT_ROUNDS = 3;
const DEFAULT_CONCURRENCY = 5;
const root = path.resolve(__dirname, "..");

// Deterministic DOM work shaped like the overview renderers: 200 match rows,
// selector matching (the main CPU-profile hot spot), class mutation, and
// serialisation. The checksum binds the gate to this exact workload.
function workload(JSDOM, iterations) {
  const rows = [];
  for (let index = 0; index < 200; index++) {
    const items = Array.from({ length: 7 }, (_, slot) => `<li data-item="${index * 7 + slot}"><img alt="" src="item-${slot}.png"></li>`).join("");
    rows.push(`<article class="match ${index % 2 ? "win" : "loss"}" data-id="m${index}"><header><span class="champ">C${index % 160}</span><span class="kda">${index % 13}/${index % 7}/${index % 11}</span></header><ul class="items">${items}</ul><footer><button type="button" data-expand="m${index}">展开</button></footer></article>`);
  }
  const dom = new JSDOM(`<!doctype html><html><body><main id="list">${rows.join("")}</main></body></html>`);
  const document = dom.window.document;
  let checksum = 0;
  for (let pass = 0; pass < iterations; pass++) {
    checksum += document.querySelectorAll("article.match.win [data-item]").length;
    checksum += document.querySelectorAll("main > article:not(.loss) footer button[data-expand]").length;
    for (const node of document.querySelectorAll("article.match")) {
      node.classList.toggle("expanded");
      if (node.matches(".expanded .items, .expanded")) checksum++;
    }
    checksum += document.getElementById("list").innerHTML.length % 997;
  }
  dom.window.close();
  return checksum;
}

function loadJSDOM() {
  return require(path.join(root, "desktop", "node_modules", "jsdom")).JSDOM;
}

function runWorker(iterations) {
  const JSDOM = loadJSDOM();
  workload(JSDOM, 2); // warm the JIT and module caches outside the timed window
  const started = process.hrtime.bigint();
  const checksum = workload(JSDOM, iterations);
  const ms = Number(process.hrtime.bigint() - started) / 1e6;
  process.send({ ms, checksum }, () => process.exit(0));
}

function median(values) {
  const sorted = values.slice().sort((a, b) => a - b);
  const middle = Math.floor(sorted.length / 2);
  return sorted.length % 2 ? sorted[middle] : (sorted[middle - 1] + sorted[middle]) / 2;
}

function oneWorker(iterations) {
  return new Promise((resolve, reject) => {
    const child = fork(__filename, ["--worker", String(iterations)], { stdio: ["ignore", "inherit", "inherit", "ipc"] });
    let message;
    child.on("message", value => { message = value; });
    child.on("error", reject);
    child.on("exit", code => {
      if (code === 0 && message && Number.isFinite(message.ms) && message.ms > 0) resolve(message);
      else reject(new Error(`calibration worker failed: exit=${code} message=${JSON.stringify(message)}`));
    });
  });
}

async function calibrate({ iterations = DEFAULT_ITERATIONS, rounds = DEFAULT_ROUNDS, concurrency = DEFAULT_CONCURRENCY } = {}) {
  const samples = [];
  let checksum;
  for (let round = 0; round < rounds; round++) {
    const results = await Promise.all(Array.from({ length: concurrency }, () => oneWorker(iterations)));
    for (const result of results) {
      if (checksum === undefined) checksum = result.checksum;
      if (result.checksum !== checksum) throw new Error(`calibration checksum diverged: ${result.checksum} != ${checksum}`);
    }
    samples.push(results.map(result => Math.round(result.ms * 1000) / 1000));
  }
  const roundMedians = samples.map(median);
  const cpus = os.cpus();
  return {
    workload_version: WORKLOAD_VERSION,
    iterations,
    rounds,
    concurrency,
    checksum,
    samples_ms: samples,
    round_medians_ms: roundMedians,
    median_ms: median(roundMedians),
    platform: process.platform,
    node: process.version,
    cpu_count: cpus.length,
    cpu_model: cpus[0]?.model || "",
    measured_at: new Date().toISOString(),
  };
}

module.exports = { WORKLOAD_VERSION, DEFAULT_ITERATIONS, DEFAULT_CONCURRENCY, calibrate, median, workload, loadJSDOM };

if (require.main === module) {
  if (process.argv[2] === "--worker") {
    runWorker(Number(process.argv[3]));
  } else {
    const output = process.argv[2];
    const iterations = Number(process.env.RUNNER_CALIBRATION_ITERATIONS || DEFAULT_ITERATIONS);
    calibrate({ iterations }).then(result => {
      const text = JSON.stringify(result, null, 2) + "\n";
      if (output) fs.writeFileSync(output, text);
      process.stdout.write(text);
    }).catch(error => { console.error(error); process.exitCode = 1; });
  }
}
