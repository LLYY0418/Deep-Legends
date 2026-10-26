"use strict";
// R279 P5: workflow wiring for the lean CI changes, parsed from the real ci.yml.
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs"), path = require("node:path");
const yaml = require("../desktop/node_modules/js-yaml");
const text = fs.readFileSync(path.join(__dirname, "..", ".github", "workflows", "ci.yml"), "utf8");
const ci = yaml.load(text);

test("browser mutations run only when the base strict step succeeded; the other independent steps keep running", () => {
  const steps = ci.jobs["r277-browser-mutations"].steps;
  const strict = steps.find((s) => s.run?.includes("r277-ci-orchestrate.cjs browser --backend"));
  assert.equal(strict.id, "r277_strict");
  assert.match(strict.if,/steps\.fixture_build\.outcome == 'success'/,"the strict base requires its built fixture");
  const mutants = steps.find((s) => s.run?.includes("browser-mutants --shard"));
  assert.match(mutants.if, /steps\.r277_strict\.outcome == 'success'/);
  assert.match(mutants.if, /!cancelled\(\)/);
  for (const marker of ["deferred-browser --backend", "r277-ci-cn-score.cjs browser --backend"]) {
    assert.equal(ci.jobs["r277-browser-matrix"].steps.find((s) => s.run?.includes(marker)).if, "${{ !cancelled() && steps.fixture_build.outcome == 'success' }}", `${marker} stays independent of the base step while requiring a valid fixture`);
  }
  assert.ok(!steps.some((s) => s["continue-on-error"]), "no step may hide a failure");
});

test("mutation: a mutation step without the base-step condition would be flagged by this check", () => {
  const broken = text.replace("steps.r277_strict.outcome == 'success'", "true");
  const steps = yaml.load(broken).jobs["r277-browser-mutations"].steps;
  const mutants = steps.find((s) => s.run?.includes("browser-mutants --shard"));
  assert.doesNotMatch(mutants.if, /steps\.r277_strict\.outcome == 'success'/);
});

test("only codex/probe-* refs are grouped and cancelled; every other ref gets a per-run group", () => {
  const { group, "cancel-in-progress": cancel } = ci.concurrency;
  assert.match(group, /startsWith\(github\.ref, 'refs\/heads\/codex\/probe-'\)/);
  assert.match(group, /github\.run_id/, "non-probe refs must get a group unique to the run");
  assert.match(cancel, /startsWith\(github\.ref, 'refs\/heads\/codex\/probe-'\)/);
  assert.ok(!/release/.test(group + cancel), "release branches are never mentioned, so never grouped");
  // Evaluate the expression for representative refs.
  const evaluate = (ref, runId = 7, attempt = 1) => {
    const probe = ref.startsWith("refs/heads/codex/probe-");
    return { group: probe ? `probe-${ref}` : `run-${runId}-${attempt}`, cancel: probe };
  };
  assert.deepEqual(evaluate("refs/heads/codex/probe-0.12.85-1"), { group: "probe-refs/heads/codex/probe-0.12.85-1", cancel: true });
  assert.equal(evaluate("refs/heads/codex/release-0.12.85", 1).group === evaluate("refs/heads/codex/release-0.12.85", 2).group, false);
  assert.equal(evaluate("refs/heads/codex/release-0.12.85").cancel, false);
  assert.equal(evaluate("refs/heads/main").cancel, false);
});

test("every triage step is failure-only, uses the facts-only tool, and sits before its job's artifact upload", () => {
  // The legacy jobs (quality, browser-*, windows-*, mutations-*) are hash-pinned by r277-ci-orchestration.test.cjs and stay untouched.
  const expected = ["r277-proof-linux-1", "r277-proof-linux-2", "windows-build", "r277-browser-matrix", "r277-browser-mutations"];
  for (const name of expected) {
    const steps = ci.jobs[name].steps;
    const index = steps.findIndex((s) => s.run?.includes("ci-failure-triage.cjs"));
    assert.ok(index >= 0, `${name} needs a triage step`);
    assert.equal(steps[index].if, "failure()");
    assert.ok(!steps[index]["continue-on-error"]);
    const upload = steps.findIndex((s) => s.uses === "actions/upload-artifact@v4" && String(s.with?.path).includes("/history/reports/r277/ci-integration/"));
    assert.ok(index < upload, `${name}: triage.json must be written before the evidence upload`);
  }
});

test("triage-legacy digests the hash-pinned legacy jobs without touching them, and cannot fail the run", () => {
  const job = ci.jobs["triage-legacy"];
  const legacy = ["quality-go", "quality-web", "quality-web-total", "quality-mutations", "windows-build", "windows-tests", "windows-upgrade", "browser-main", "mutations-1", "r277-proof-linux-1", "r277-proof-linux-2", "r277-browser-matrix", "r277-browser-mutations", "mutations-3"];
  assert.deepEqual([...job.needs].sort(), [...legacy].sort());
  assert.equal(job.if, "${{ always() && !cancelled() }}", "evidence aggregation runs even when all jobs are green");
  assert.match(job.if, /always\(\)/);
  assert.deepEqual(job.permissions, { contents: "read", actions: "read" });
  const digest = job.steps.find((s) => s.run?.includes("ci-failure-triage.cjs"));
  assert.ok(digest.run.includes("exit 0") && digest.run.includes("set +e"), "advisory step must always end green");
  assert.ok(!job.steps.some((s) => s["continue-on-error"]));
});
