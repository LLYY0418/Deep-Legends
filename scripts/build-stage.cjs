"use strict";

const { spawn } = require("node:child_process");
const { performance } = require("node:perf_hooks");
const { constants } = require("node:os");

// Timing only: preserve the command, environment, output and failure status.
// Never print arguments; build commands may carry private configuration.
function runStage({ label, command, args = [], intervalMs = 15000, log = console.log }) {
  const started = performance.now();
  const report = status => log(`[build ${new Date().toISOString()}] ${label}: ${status} (${((performance.now() - started) / 1000).toFixed(1)}s)`);
  report("started");
  return new Promise(resolve => {
    const child = spawn(command, args, { stdio: "inherit" });
    const heartbeat = setInterval(() => report("still running"), intervalMs);
    heartbeat.unref();
    const interrupt = () => child.kill("SIGINT");
    const terminate = () => child.kill("SIGTERM");
    process.on("SIGINT", interrupt);
    process.on("SIGTERM", terminate);
    let finished = false;
    const finish = (code, detail) => {
      if (finished) return;
      finished = true;
      clearInterval(heartbeat);
      process.removeListener("SIGINT", interrupt);
      process.removeListener("SIGTERM", terminate);
      report(code === 0 ? "completed" : `failed: ${detail}`);
      resolve(code);
    };
    child.once("error", error => finish(1, error.code || "could not start command"));
    child.once("exit", (code, signal) => finish(code ?? (128 + (constants.signals[signal] || 1)), signal || `exit ${code}`));
  });
}

module.exports = { runStage };
if (require.main === module) {
  const [label, command, ...args] = process.argv.slice(2);
  if (!label || !command) {
    console.error("usage: node scripts/build-stage.cjs <stage-label> <command> [args...]");
    process.exitCode = 2;
  } else {
    runStage({ label, command, args }).then(code => { process.exitCode = code; });
  }
}
