"use strict";

const os = require("node:os");
const { spawn, spawnSync } = require("node:child_process");
const { performance } = require("node:perf_hooks");

// Hints from the 2026-09-19 full-suite timing run. They affect scheduling only:
// every discovered test still runs exactly once, and unknown tests get a small
// equal weight so newly added coverage remains balanced across shards.
const testDurationHints = new Map([
  ["TestR99SeedQuotaDiagnosticsNeverContainIdentity", 60.01],
  ["TestR89DiskBudgetAfter2000Images", 19.84],
  ["TestR96PartialTruthDoesNotEndAsNone", 17.14],
  ["TestR99SeedAnchorSurvivesLRUAndHasOnlyStableField", 13.41],
  ["TestR92RiotMatchConcreteDiskBudget", 11.24],
  ["TestR100AssetHasWholeHandlerDeadline", 8.00],
  ["TestR100OverviewLongRoundUsesFreshSingleWaitBudget", 6.47],
  ["TestR101BannerProbeAlwaysRestores", 6.06],
  ["TestR86WebsocketDropKeepsLiveLCUConnection", 5.04],
  ["TestR102SeedOneAccountFailureDoesNotBlockOthers", 4.09],
  ["TestR100PerksNeverWaitForOptionalAugmentsAndPersist", 3.02],
  ["TestR113BroadcastEventStormIsBoundedAndRearmsNextSession", 3.01],
  ["TestR105_SuccessStatusWithoutChangeIsNotSuccess", 2.61],
  ["TestR64FacadeEventBurstIsThrottled", 2.15],
  ["TestR54CollectionProbeRetriesUntilReady", 2.11],
  ["TestR92RiotMatchByteBudgetAcrossRestart", 2.09],
]);

function testNameHash(name) {
  let hash = 2166136261;
  for (const byte of Buffer.from(name)) {
    hash ^= byte;
    hash = Math.imul(hash, 16777619);
  }
  return hash >>> 0;
}

function partitionTests(names, count) {
  const shards = Array.from({ length: Math.max(1, Math.min(count, names.length || 1)) }, () => ({ names: [], weight: 0 }));
  const ordered = [...names].sort((left, right) =>
    (testDurationHints.get(right) || 0.01) - (testDurationHints.get(left) || 0.01) ||
    testNameHash(left) - testNameHash(right));
  for (const name of ordered) {
    const target = shards.reduce((best, shard) =>
      shard.weight < best.weight || (shard.weight === best.weight && shard.names.length < best.names.length) ? shard : best);
    target.names.push(name);
    target.weight += testDurationHints.get(name) || 0.01;
  }
  return shards.map(shard => shard.names).filter(shard => shard.length > 0);
}

function discoverTests({ command = "go", cwd = process.cwd(), env = process.env } = {}) {
  const result = spawnSync(command, ["test", "-vet=off", "-list", "^(Test|Example|Fuzz)", "."], {
    cwd, env, encoding: "utf8", maxBuffer: 4 * 1024 * 1024,
  });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    process.stdout.write(result.stdout || "");
    process.stderr.write(result.stderr || "");
    throw new Error(`could not list Go tests (exit ${result.status})`);
  }
  return result.stdout.split(/\r?\n/).filter(line => /^(Test|Example|Fuzz)[A-Za-z0-9_]+$/.test(line));
}

function discoverPackages({ command = "go", cwd = process.cwd(), env = process.env } = {}) {
  const result = spawnSync(command, ["list", "./..."], { cwd, env, encoding: "utf8", maxBuffer: 4 * 1024 * 1024 });
  if (result.error) throw result.error;
  if (result.status !== 0) {
    process.stdout.write(result.stdout || "");
    process.stderr.write(result.stderr || "");
    throw new Error(`could not list Go packages (exit ${result.status})`);
  }
  return result.stdout.split(/\r?\n/).filter(Boolean);
}

function configuredShardCount(env = process.env) {
  const available = typeof os.availableParallelism === "function" ? os.availableParallelism() : os.cpus().length;
  const raw = env.GO_TEST_SHARDS ?? String(Math.min(5, available));
  if (!/^[1-9]$|^1[0-6]$/.test(raw)) throw new Error("GO_TEST_SHARDS must be an integer from 1 to 16");
  return Number(raw);
}

async function runShards({ command = "go", cwd = process.cwd(), env = process.env, log = console.error } = {}) {
  const packages = discoverPackages({ command, cwd, env });
  if (packages.length !== 1) {
    log(`[go-test] discovered ${packages.length} packages; using complete unsharded ./... fallback`);
    return new Promise(resolve => {
      const child = spawn(command, ["test", "-vet=off", "./..."], { cwd, env, stdio: "inherit" });
      const interrupt = () => child.kill("SIGINT");
      const terminate = () => child.kill("SIGTERM");
      process.on("SIGINT", interrupt);
      process.on("SIGTERM", terminate);
      const finish = (code, signal) => {
        process.removeListener("SIGINT", interrupt);
        process.removeListener("SIGTERM", terminate);
        resolve(code ?? (128 + (os.constants.signals[signal] || 1)));
      };
      child.once("error", () => finish(1));
      child.once("exit", finish);
    });
  }
  const names = discoverTests({ command, cwd, env });
  if (names.length === 0) throw new Error("no Go tests discovered");
  const shards = partitionTests(names, configuredShardCount(env));
  log(`[go-test] discovered ${names.length} tests; running ${shards.length} shards`);
  const children = new Set();
  let firstFailure = 0;
  const stopChildren = signal => {
    for (const child of children) child.kill(signal);
  };
  const interrupt = () => stopChildren("SIGINT");
  const terminate = () => stopChildren("SIGTERM");
  process.on("SIGINT", interrupt);
  process.on("SIGTERM", terminate);
  try {
    await Promise.all(shards.map((namesInShard, index) => new Promise(resolve => {
      const started = performance.now();
      const label = `shard ${index + 1}/${shards.length}`;
      log(`[go-test] ${label}: started (${namesInShard.length} tests)`);
      const pattern = `^(${namesInShard.join("|")})$`;
      const child = spawn(command, ["test", "-vet=off", "-run", pattern, "."], { cwd, env, stdio: "inherit" });
      children.add(child);
      let finished = false;
      const finish = code => {
        if (finished) return;
        finished = true;
        children.delete(child);
        const elapsed = ((performance.now() - started) / 1000).toFixed(1);
        if (code === 0) log(`[go-test] ${label}: completed (${elapsed}s)`);
        else {
          firstFailure ||= code || 1;
          log(`[go-test] ${label}: failed (exit ${code ?? 1}, ${elapsed}s)`);
          stopChildren("SIGTERM");
        }
        resolve();
      };
      child.once("error", () => finish(1));
      child.once("exit", (code, signal) => finish(code ?? (128 + (os.constants.signals[signal] || 1))));
    })));
  } finally {
    process.removeListener("SIGINT", interrupt);
    process.removeListener("SIGTERM", terminate);
  }
  return firstFailure;
}

module.exports = { configuredShardCount, discoverPackages, discoverTests, partitionTests, runShards, testDurationHints, testNameHash };

if (require.main === module) {
  runShards().then(code => { process.exitCode = code; }).catch(error => {
    console.error(`[go-test] ${error.message}`);
    process.exitCode = 1;
  });
}
