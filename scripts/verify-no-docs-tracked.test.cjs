"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const fs = require("node:fs"), path = require("node:path"), os = require("node:os");
const { spawnSync } = require("node:child_process");
const { scanSource, verify } = require("./verify-no-docs-tracked.cjs");
test("local-note guard rejects source paths with precise lines across languages", () => {
  const cases = [
    ["probe.go", '// os.ReadFile("../docs/comment")\nraw, err := os.ReadFile("../docs/x")', 2],
    ["probe.go", 'filepath.Join("..", "docs", "x")', 1],
    ["probe.cjs", 'path.join(__dirname, "..", "docs", "x")', 1],
    ["probe.js", "readFile('../docs/x')", 1],
    ["probe.py", "# read_text('../docs/comment')\np = root / 'docs/x'", 2],
    ["probe.go", 'os.ReadFile(`../docs/x`)', 1],
    ["probe.cjs", 'fs.readFileSync("..\\\\docs\\\\x")', 1],
    ["probe.js", 'readFile("../do\\u0063s/x")', 1],
    ["probe.js", 'readFile(\n  "../docs/x"\n)', 2],
    ["probe.go", 'os.ReadFile("do" + "cs/x")', 1],
    ["probe.js", 'readFile("../\\u{0064}ocs/x")', 1],
    ["probe.py", 'open("../\\U00000064ocs/x")', 1],
  ];
  for (const [file, source, line] of cases) {
    const found = scanSource(file, source); assert.equal(found.length, 1, source); assert.equal(found[0].line, line);
  }
  assert.deepEqual(scanSource("probe.go", '// ../docs/comment\nraw := "testdata/fixture.json"'), []);
  assert.deepEqual(scanSource("probe.py", '# docs/x\np = "testdata/x"'), []);
});
test("local-note guard rejects tracked notes, forced assistant files and runtime mutants", () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "local-note-guard-"));
  function git(...args) {
    const result = spawnSync("git", args, { cwd: directory, encoding: "utf8" });
    assert.equal(result.status, 0, result.stderr); return result;
  }
  function cli() { return spawnSync(process.execPath, [path.join(__dirname, "verify-no-docs-tracked.cjs"), directory], { encoding: "utf8" }); }
  try {
    git("init", "--quiet");
    fs.writeFileSync(path.join(directory, ".gitignore"), "/docs/\n/AGENTS.md\n/CLAUDE.md\n");
    fs.writeFileSync(path.join(directory, "safe.go"), 'package main\nvar fixture = "testdata/x"\n');
    git("add", ".gitignore", "safe.go"); assert.deepEqual(verify(directory), []); assert.equal(cli().status, 0);
    for (const file of ["docs/probe.md", "AGENTS.md", "CLAUDE.md"]) {
      fs.mkdirSync(path.dirname(path.join(directory, file)), { recursive: true }); fs.writeFileSync(path.join(directory, file), "local only\n");
      git("add", "-f", file); const result = cli(); assert.equal(result.status, 1); assert.ok(result.stderr.includes(`${file}:1`));
      git("rm", "--cached", file); assert.equal(cli().status, 0); assert.ok(fs.existsSync(path.join(directory, file)));
    }
    fs.writeFileSync(path.join(directory, "safe.go"), 'package main\nvar data, err = os.ReadFile("../docs/x")\n');
    const result = cli(); assert.equal(result.status, 1); assert.match(result.stderr, /safe\.go:2/);
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});
test("optional audit copy has a narrow statement exception only", () => {
  const source = fs.readFileSync(path.join(__dirname, "build-no-license-audit.cjs"), "utf8");
  assert.deepEqual(scanSource("scripts/build-no-license-audit.cjs", source), []);
  assert.equal(scanSource("scripts/build-no-license-audit.cjs", source + '\nfs.readFileSync("../docs/x");').length, 1);
  assert.ok(scanSource("probe.cjs", source).length > 0);
});
