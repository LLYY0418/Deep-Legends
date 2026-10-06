"use strict";

// R231 P13: build two public packages from isolated copies of the same sources.
// Only the control copy adds CRCCheck off; this never changes the default setup.
const fs = require("node:fs"), path = require("node:path"), os = require("node:os");
const crypto = require("node:crypto"), { spawnSync } = require("node:child_process");
const root = path.resolve(__dirname, "..");
const output = path.join(root, "dist", "r231-crc-control");
const reports = path.join(root, "docs", "history", "reports", "r231");
const version = require("../desktop/package.json").version;
const baseFingerprint = require("../desktop/source-fingerprint.cjs").sourceFingerprint();
const electronDist = process.env.ELECTRON_DIST;
if (!electronDist || !fs.existsSync(electronDist)) throw new Error("Set ELECTRON_DIST to the cached Windows Electron zip");
fs.mkdirSync(output, { recursive: true });
fs.mkdirSync(reports, { recursive: true });
const tracked = spawnSync("git", ["ls-files", "-z", "--", "backend", "desktop", "installer", "scripts", "go.mod", "go.sum", "build-desktop.sh", "build-desktop-windows.ps1", "build-windows.ps1"], { cwd: root, encoding: "utf8" });
if (tracked.status !== 0) throw new Error("Cannot enumerate build inputs");
const sources = new Set(tracked.stdout.split("\0").filter(Boolean));
sources.add("backend/client_shutdown.go");
// Freeze one source snapshot so both modes remain comparable even when
// another chat edits the shared checkout while NSIS is compressing.
const snapshot = new Map([...sources].filter(relative => {
  const file = path.join(root, relative); return fs.existsSync(file) && fs.statSync(file).isFile();
}).map(relative => [relative, fs.readFileSync(path.join(root, relative))]));
const receipts = [];
for (const mode of ["baseline", "crc-off"]) {
  const copy = fs.mkdtempSync(path.join(os.tmpdir(), "r231-" + mode + "-"));
  const log = fs.openSync(path.join(reports, "build-" + mode + ".log"), "w");
  const startedAt = new Date().toISOString();
  const run = (cmd, args, cwd = copy, extra = {}) => {
    const result = spawnSync(cmd, args, { cwd, env: { ...process.env, DEEP_LEGENDS_KEY_MODE: "public", ...extra }, stdio: ["ignore", log, log] });
    if (result.error || result.status !== 0) throw new Error(`${mode}: ${cmd} failed (${result.error?.message || result.status}); see build-${mode}.log`);
  };
  try {
    for (const [relative, bytes] of snapshot) {
      const to = path.join(copy, relative);
      fs.mkdirSync(path.dirname(to), { recursive: true }); fs.writeFileSync(to, bytes);
    }
    fs.symlinkSync(path.join(root, "desktop", "node_modules"), path.join(copy, "desktop", "node_modules"), "dir");
    fs.mkdirSync(path.join(copy, "desktop", "backend"), { recursive: true });
    if (mode === "crc-off") {
      const include = path.join(copy, "desktop", "nsis", "installer.nsh");
      fs.writeFileSync(include, "; R231 isolated CRC control only\nCRCCheck off\n" + fs.readFileSync(include, "utf8"));
    }
    const fingerprint = require(path.join(copy, "desktop", "source-fingerprint.cjs")).sourceFingerprint(copy);
    if (mode === "baseline" && fingerprint !== baseFingerprint) throw new Error("Isolated baseline differs from working source");
    run("go", ["build", "-buildvcs=false", "-trimpath", "-ldflags", `-s -w -H=windowsgui -buildid= -X main.version=${version} -X main.buildFingerprint=${fingerprint} -X main.riotAPIKey= -X main.riotAPIKeyCipher=`, "-o", "desktop/backend/loot-service.exe", "./backend"], copy, { GOOS: "windows", GOARCH: "amd64", CGO_ENABLED: "0" });
    run("node", ["desktop/verify-build-fingerprint.cjs", "desktop/backend/loot-service.exe", fingerprint]);
    run("node", ["desktop/verify-embedded-riot-key.cjs"]);
    run("node", ["installer/build-shell.cjs", "--uninstall", version]);
    run("node", [path.join(root, "desktop/node_modules/electron-builder/out/cli/cli.js"), "--win", "nsis", "--x64", "--config.win.signExecutable=false", "--config.nsis.artifactName=Deep Legends Setup ${version}-public.${ext}", "--config.electronDist=" + electronDist], path.join(copy, "desktop"), { DEEP_LEGENDS_FINGERPRINT: fingerprint });
    run("node", ["installer/build-shell.cjs", version, fingerprint]);
    run("node", ["desktop/verify-packaged-runtime.cjs", "dist/desktop/win-unpacked/resources/app.asar"]);
    run("node", ["desktop/verify-build-fingerprint.cjs", "dist/desktop/win-unpacked/resources/app.asar.unpacked/backend/loot-service.exe", fingerprint]);
    run("node", ["desktop/release-build.cjs", fingerprint]);
    const name = `Deep Legends Setup ${version}-r231-${mode}-public.exe`;
    const from = path.join(copy, "dist/desktop", `Deep Legends Setup ${version}-public.exe`);
    fs.copyFileSync(from, path.join(output, name));
    const sha256 = crypto.createHash("sha256").update(fs.readFileSync(from)).digest("hex");
    receipts.push({ mode, keyMode: "public", version, fingerprint, startedAt, endedAt: new Date().toISOString(), name, sha256, bytes: fs.statSync(from).size });
    fs.writeFileSync(path.join(output, "receipts.json"), JSON.stringify(receipts, null, 2) + "\n");
    console.log(`${mode} completed: ${name} · ${fingerprint}`);
  } finally {
    fs.closeSync(log); fs.rmSync(copy, { recursive: true, force: true });
  }
}
fs.writeFileSync(path.join(output, "SHA256SUMS-public.txt"), receipts.map(r => `${r.sha256}  ${r.name}`).join("\n") + "\n");
