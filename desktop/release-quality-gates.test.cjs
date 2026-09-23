"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const fs = require("node:fs"), os = require("node:os"), path = require("node:path");
const { spawnSync } = require("node:child_process");
const root = path.resolve(__dirname, "..");
const { setupArtifactName, checksumArtifactName } = require("./artifact-names.cjs");

test("R136 public outputs have distinct names and private outputs keep release names", () => {
  assert.equal(setupArtifactName("0.12.19", "public"), "Deep Legends Setup 0.12.19-public.exe");
  assert.equal(setupArtifactName("0.12.19", "private"), "Deep Legends Setup 0.12.19.exe");
  assert.equal(checksumArtifactName("public"), "SHA256SUMS-public.txt");
  assert.equal(checksumArtifactName("private"), "SHA256SUMS.txt");
  const bash = fs.readFileSync(path.join(root, "build-desktop.sh"), "utf8");
  assert.match(bash, /--config\.nsis\.artifactName=Deep Legends Setup \$\{version\}-public\.\$\{ext\}/);
  const windows = fs.readFileSync(path.join(root, "build-desktop-windows.ps1"), "utf8");
  assert.match(windows, /--config\.nsis\.artifactName=Deep Legends Setup \$\{version\}-public\.\$\{ext\}/);
});

function writeToolchainFixture(directory) {
  const toolchainTests = path.join(directory, ".gotoolchain", "test");
  fs.mkdirSync(toolchainTests, { recursive: true });
  // Go ships deliberately invalid compiler tests; gofmt must never parse them.
  fs.writeFileSync(path.join(toolchainTests, "char_lit1.go"), "package toolchain\n\nvar invalid = '\\uD800'\n");
}

test("R86 bash release stops on a real failing Go test, including an independent early-build mutation", { skip: process.platform === "win32" }, () => {
  const goroot = spawnSync("go", ["env", "GOROOT"], { encoding: "utf8" });
  assert.equal(goroot.status, 0, goroot.stderr);
  const go = path.join(goroot.stdout.trim(), "bin", "go");
  const source = fs.readFileSync(path.join(root, "build-desktop.sh"), "utf8");
  function check(source) {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), "r86-release-gate-"));
    try {
      for (const name of ["desktop", "dist/desktop", "bin", ".gomodcache", ".gopath/pkg/mod/dependency@v1.0.0"]) fs.mkdirSync(path.join(directory, name), { recursive: true });
      fs.writeFileSync(path.join(directory, "desktop/package.json"), JSON.stringify({ version: "99.1.2" }));
      fs.writeFileSync(path.join(directory, ".gomodcache/invalid.go"), "intentionally invalid third-party parser fixture");
      fs.writeFileSync(path.join(directory, ".gopath/pkg/mod/dependency@v1.0.0/invalid.go"), "intentionally invalid third-party parser fixture");
      writeToolchainFixture(directory);
      fs.writeFileSync(path.join(directory, "go.mod"), "module releasegate\n\ngo 1.24\n");
      fs.writeFileSync(path.join(directory, "main.go"), "package main\n\nfunc main() {}\n");
      fs.writeFileSync(path.join(directory, "gate_test.go"), 'package main\n\nimport "testing"\n\nfunc TestReleaseGate(t *testing.T) { t.Fatal("R86_INTENTIONAL_FAILURE") }\n');
      fs.writeFileSync(path.join(directory, "build-desktop.sh"), source);
      fs.writeFileSync(path.join(directory, "bin/go"), `#!/bin/sh\nprintf '%s\\n' "$*" >> '${directory}/go-calls'\nexec '${go}' "$@"\n`, { mode: 0o700 });
      const result = spawnSync("bash", ["build-desktop.sh"], { cwd: directory, encoding: "utf8", timeout: 60000,
        env: { ...process.env, DEEP_LEGENDS_KEY_MODE: "public", PATH: `${directory}/bin:${process.env.PATH}` } });
      assert.notEqual(result.status, 0);
      assert.match(result.stdout + result.stderr, /R86_INTENTIONAL_FAILURE/);
      assert.doesNotMatch(fs.readFileSync(path.join(directory, "go-calls"), "utf8"), /^build\b/m, "no independent build path may run before failing tests");
    } finally { fs.rmSync(directory, { recursive: true, force: true }); }
  }
  check(source);
  assert.throws(() => check(source.replace('go test ./...', 'go build ./...\ngo test ./...')), { name: "AssertionError" });
});

test("R86 CI uses the package version, installs test dependencies and needs no private Riot key", () => {
  const ci = fs.readFileSync(path.join(root, ".github/workflows/ci.yml"), "utf8");
  assert.match(ci, /require\('\.\/desktop\/package\.json'\)\.version/);
  assert.match(ci, /-Version \$version -KeyMode public/);
  const windows = ci.slice(ci.indexOf("  windows-build:"));
  assert.ok(windows.indexOf("npm ci --prefix desktop") < windows.indexOf("./build-desktop-windows.ps1"));
  assert.match(fs.readFileSync(path.join(root, "build-desktop-windows.ps1"), "utf8"), /if \(-not \$Version\) \{ \$Version = \$package.version \}/);
  const retired = fs.readFileSync(path.join(root, "build-windows.ps1"), "utf8");
  assert.match(retired, /throw "This portable build entry is retired/);
  assert.doesNotMatch(retired, /go build|Compress-Archive|Start-Process/);
});

test("R86 Windows release stops on a real failing Go test and rejects an independent early build", { skip: process.platform !== "win32" ? "requires Windows and PowerShell" : false }, () => {
  const goroot = spawnSync("go", ["env", "GOROOT"], {encoding:"utf8"});
  assert.equal(goroot.status,0,goroot.stderr);
  const go=path.join(goroot.stdout.trim(),"bin","go.exe");
  const source=fs.readFileSync(path.join(root,"build-desktop-windows.ps1"),"utf8");
  function check(source) {
    const directory=fs.mkdtempSync(path.join(os.tmpdir(),"r86-win-gate-"));
    try {
      for(const name of ["desktop","dist/desktop","bin","scripts",".gomodcache",".gopath/pkg/mod/dependency@v1.0.0"])fs.mkdirSync(path.join(directory,name),{recursive:true});
      fs.writeFileSync(path.join(directory,"desktop/package.json"),JSON.stringify({version:"99.1.2"}));
      fs.writeFileSync(path.join(directory,"go.mod"),"module releasegate\n\ngo 1.24\n");
      fs.writeFileSync(path.join(directory,"main.go"),"package main\n\nfunc main() {}\n");
      fs.writeFileSync(path.join(directory,"gate_test.go"),'package main\n\nimport "testing"\n\nfunc TestReleaseGate(t *testing.T) { t.Fatal("R86_INTENTIONAL_FAILURE") }\n');
      fs.writeFileSync(path.join(directory,".gomodcache/invalid.go"),"invalid dependency parser fixture");
      fs.writeFileSync(path.join(directory,".gopath/pkg/mod/dependency@v1.0.0/invalid.go"),"invalid dependency parser fixture");
      writeToolchainFixture(directory);
      fs.writeFileSync(path.join(directory,"build-desktop-windows.ps1"),source);
      fs.copyFileSync(path.join(root,"scripts/normalize-source-line-endings.cjs"),path.join(directory,"scripts/normalize-source-line-endings.cjs"));
      fs.writeFileSync(path.join(directory,"bin/npm.cmd"),"@echo off\r\nexit /b 0\r\n");
      fs.writeFileSync(path.join(directory,"bin/go.cmd"),`@echo off\r\necho %* >> "${path.join(directory,"go-calls")}"\r\n"${go}" %*\r\nexit /b %errorlevel%\r\n`);
      const result=spawnSync("powershell.exe",["-NoProfile","-ExecutionPolicy","Bypass","-File","build-desktop-windows.ps1","-KeyMode","public"],{cwd:directory,encoding:"utf8",timeout:60000,env:{...process.env,PATH:`${path.join(directory,"bin")};${process.env.PATH}`}});
      if (result.error) throw new Error(`Windows PowerShell failed to start: ${result.error.message}`);
      const output = `${result.stdout || ""}${result.stderr || ""}`;
      assert.notEqual(result.status,0);
      assert.match(output,/R86_INTENTIONAL_FAILURE/);
      assert.match(output,/Root tests failed/);
      assert.doesNotMatch(fs.readFileSync(path.join(directory,"go-calls"),"utf8"),/^build\b/m);
    } finally {fs.rmSync(directory,{recursive:true,force:true})}
  }
  check(source);
  assert.throws(()=>check(source.replace("go test ./...","go build ./...\n    go test ./...")),{name:"AssertionError"});
});

test("format gates exclude local toolchains and GOPATH modules while retaining untracked project source", { skip: process.platform === "win32" }, () => {
  const build = fs.readFileSync(path.join(root, "build-desktop.sh"), "utf8");
  const ci = fs.readFileSync(path.join(root, ".github/workflows/ci.yml"), "utf8");
  const scan = build.match(/^unformatted="\$\((.*)\)"$/m)[1];
  const ciScan = ci.match(/unformatted="\$\((.*)\)"/)[1];
  assert.equal(scan, ciScan, "local and CI format checks must agree");
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "format-gopath-"));
  try {
    const dependency = path.join(directory, ".gopath/pkg/mod/dependency@v1.0.0");
    fs.mkdirSync(dependency, { recursive: true });
    fs.writeFileSync(path.join(dependency, "doc.go"), "package dependency\nfunc f( ){ }\n");
    fs.writeFileSync(path.join(directory, "main.go"), "package main\n\nfunc main() {}\n");
    writeToolchainFixture(directory);
    const run = command => {
      const result = spawnSync("bash", ["-e", "-o", "pipefail", "-c", command], { cwd: directory, encoding: "utf8", timeout: 10000 });
      assert.equal(result.status, 0, result.stderr);
      return result.stdout.trim();
    };
    assert.equal(run(scan), "", "toolchain fixtures and dependency formatting must not block a build");
    // Different task numbers and future cache names must follow the same rule.
    for (const cache of [".mut102gopath", ".mut999gopath", ".future-toolchain", "nested/.cache", "node_modules", "vendor", "dist"]) {
      const fixture = path.join(directory, cache, "pkg/mod/toolchain/test/invalid.go");
      fs.mkdirSync(path.dirname(fixture), { recursive: true });
      fs.writeFileSync(fixture, "intentionally invalid compiler fixture");
    }
    assert.equal(run(scan), "", "new cache names must not enter the format gate");
    const unfiltered = spawnSync("gofmt", ["-l", ".mut102gopath/pkg/mod/toolchain/test/invalid.go"], { cwd: directory, encoding: "utf8" });
    assert.notEqual(unfiltered.status, 0, "the reported cache fixture must fail if scanned");
    const windows = fs.readFileSync(path.join(root, "build-desktop-windows.ps1"), "utf8");
    const excluded = new RegExp(windows.split("\n").find(line => line.includes("$goSources = @(")).match(/-notmatch '([^']+)'/)[1]);
    for (const separator of ["/", "\\"]) {
      for (const name of [".mut102gopath", ".mut999gopath", ".future-toolchain", "node_modules", "vendor", "dist"]) {
        assert.ok(excluded.test(`${separator}${name}${separator}invalid.go`), name);
      }
      assert.ok(!excluded.test(`${separator}new source${separator}untracked.go`));
    }
    // No git repository: newly added source must still be checked, including spaces.
    fs.mkdirSync(path.join(directory, "new source"));
    fs.writeFileSync(path.join(directory, "new source/untracked.go"), "package source\nfunc f( ){ }\n");
    assert.equal(run(scan), "./new source/untracked.go");
    fs.writeFileSync(path.join(directory, "new source/untracked.go"), "invalid project syntax");
    const invalidSource = spawnSync("bash", ["-e", "-o", "pipefail", "-c", scan], { cwd: directory, encoding: "utf8", timeout: 10000 });
    assert.notEqual(invalidSource.status, 0, "real project syntax errors must still fail");
  } finally { fs.rmSync(directory, { recursive: true, force: true }); }
});
