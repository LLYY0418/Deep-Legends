"use strict";
const test = require("node:test"), assert = require("node:assert/strict");
const fs = require("node:fs"), os = require("node:os"), path = require("node:path");
const { spawnSync } = require("node:child_process");
const root = path.resolve(__dirname, "..");

test("R86 bash release stops on a real failing Go test, including an independent early-build mutation", { skip: process.platform === "win32" }, () => {
  const goroot = spawnSync("go", ["env", "GOROOT"], { encoding: "utf8" });
  assert.equal(goroot.status, 0, goroot.stderr);
  const go = path.join(goroot.stdout.trim(), "bin", "go");
  const source = fs.readFileSync(path.join(root, "build-desktop.sh"), "utf8");
  function check(source) {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), "r86-release-gate-"));
    try {
      for (const name of ["desktop", "dist/desktop", "bin", ".gomodcache"]) fs.mkdirSync(path.join(directory, name), { recursive: true });
      fs.writeFileSync(path.join(directory, "desktop/package.json"), JSON.stringify({ version: "99.1.2" }));
      fs.writeFileSync(path.join(directory, ".gomodcache/invalid.go"), "intentionally invalid third-party parser fixture");
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
      for(const name of ["desktop","dist/desktop","bin",".gomodcache"])fs.mkdirSync(path.join(directory,name),{recursive:true});
      fs.writeFileSync(path.join(directory,"desktop/package.json"),JSON.stringify({version:"99.1.2"}));
      fs.writeFileSync(path.join(directory,"go.mod"),"module releasegate\n\ngo 1.24\n");
      fs.writeFileSync(path.join(directory,"main.go"),"package main\n\nfunc main() {}\n");
      fs.writeFileSync(path.join(directory,"gate_test.go"),'package main\n\nimport "testing"\n\nfunc TestReleaseGate(t *testing.T) { t.Fatal("R86_INTENTIONAL_FAILURE") }\n');
      fs.writeFileSync(path.join(directory,".gomodcache/invalid.go"),"invalid dependency parser fixture");
      fs.writeFileSync(path.join(directory,"build-desktop-windows.ps1"),source);
      fs.writeFileSync(path.join(directory,"bin/go.cmd"),`@echo off\r\necho %* >> "${path.join(directory,"go-calls")}"\r\n"${go}" %*\r\nexit /b %errorlevel%\r\n`);
      const result=spawnSync("pwsh",["-NoProfile","-File","build-desktop-windows.ps1","-KeyMode","public"],{cwd:directory,encoding:"utf8",timeout:60000,env:{...process.env,PATH:`${path.join(directory,"bin")};${process.env.PATH}`}});
      assert.notEqual(result.status,0);
      assert.match(result.stdout+result.stderr,/R86_INTENTIONAL_FAILURE/);
      assert.match(result.stdout+result.stderr,/Root tests failed/);
      assert.doesNotMatch(fs.readFileSync(path.join(directory,"go-calls"),"utf8"),/^build\b/m);
    } finally {fs.rmSync(directory,{recursive:true,force:true})}
  }
  check(source);
  assert.throws(()=>check(source.replace("go test ./...","go build ./...\n    go test ./...")),{name:"AssertionError"});
});
