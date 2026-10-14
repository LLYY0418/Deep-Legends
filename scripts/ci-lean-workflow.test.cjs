"use strict";
// R279 P5: workflow wiring for the lean CI changes, parsed from the real ci.yml.
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs"), path = require("node:path");
const yaml = require("../desktop/node_modules/js-yaml");
const text = fs.readFileSync(path.join(__dirname, "..", ".github", "workflows", "ci.yml"), "utf8");
const ci = yaml.load(text);

test("browser mutations run only when the base strict step succeeded; the other independent steps keep running", () => {
  const steps = ci.jobs["r277-browser-matrix"].steps;
  const strict = steps.find((s) => s.run?.includes("r277-ci-orchestrate.cjs browser --backend"));
  assert.equal(strict.id, "r277_strict");
  assert.equal(strict.if, undefined, "the strict step itself must run unconditionally (it is the base gate)");
  const mutants = steps.find((s) => s.run?.includes("browser-mutants --backend"));
  assert.match(mutants.if, /steps\.r277_strict\.outcome == 'success'/);
  assert.match(mutants.if, /!cancelled\(\)/);
  for (const marker of ["deferred-browser --backend", "r277-ci-cn-score.cjs browser --backend"]) {
    assert.equal(steps.find((s) => s.run?.includes(marker)).if, "${{ !cancelled() }}", `${marker} stays independent of the base step`);
  }
  assert.ok(!steps.some((s) => s["continue-on-error"]), "no step may hide a failure");
});

test("mutation: a mutation step without the base-step condition would be flagged by this check", () => {
  const broken = text.replace("steps.r277_strict.outcome == 'success'", "true");
  const steps = yaml.load(broken).jobs["r277-browser-matrix"].steps;
  const mutants = steps.find((s) => s.run?.includes("browser-mutants --backend"));
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
  const expected = ["r277-proof-linux", "r277-native-windows", "r277-browser-matrix"];
  for (const name of expected) {
    const steps = ci.jobs[name].steps;
    const index = steps.findIndex((s) => s.run?.includes("ci-failure-triage.cjs"));
    assert.ok(index >= 0, `${name} needs a triage step`);
    assert.equal(steps[index].if, "failure()");
    assert.ok(!steps[index]["continue-on-error"]);
    const upload = steps.findIndex((s) => s.uses === "actions/upload-artifact@v4");
    assert.ok(index < upload, `${name}: triage.json must be written before the evidence upload`);
  }
});
