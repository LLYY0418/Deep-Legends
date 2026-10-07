"use strict";
// Manual local Windows setup + dir pack. Never called by release workflows.
const fs = require("node:fs"), path = require("node:path"), os = require("node:os");
const crypto = require("node:crypto"), { spawnSync } = require("node:child_process");
const root = path.resolve(__dirname, "..");

function stagingOutput(args) {
  if (args.length && !(args.length === 2 && args[0] === "--output")) throw Error("Usage: node scripts/build-license-staging.cjs [--output DIRECTORY]");
  const out = path.resolve(args[1] || path.join(root, "dist/Deep-Legends-staging-public"));
  if (!/-staging-public$/i.test(path.basename(out))) throw Error("STAGING output directory must end with -staging-public");
  if (fs.existsSync(out) && fs.readdirSync(out).length) throw Error("Staging output directory must be empty");
  return out;
}

function build(args = process.argv.slice(2)) {
  const output = stagingOutput(args), snapshot = fs.mkdtempSync(path.join(os.tmpdir(), "deep-legends-STAGING-"));
  const env = { ...process.env, GOFLAGS: "", GOOS: "windows", GOARCH: "amd64", CGO_ENABLED: "0", DEEP_LEGENDS_LICENSE_BUILD: "1", DEEP_LEGENDS_KEY_MODE: "public", CSC_IDENTITY_AUTO_DISCOVERY: "false", CSC_LINK: "", CSC_KEY_PASSWORD: "" };
  const run = (command, argv, cwd = snapshot) => {
    const result = spawnSync(command, argv, { cwd, env, stdio: "inherit" });
    if (result.error) throw result.error;
    if (result.status !== 0) throw Error("Staging build failed: " + command + " (" + result.status + ")");
  };
  const digest = file => crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
  try {
    for (const name of ["go.mod", "go.sum", "backend", "installer", "scripts", "build-desktop.sh", "build-desktop-windows.ps1", "build-windows.ps1", "desktop"]) {
      fs.cpSync(path.join(root, name), path.join(snapshot, name), { recursive: true, filter: file => !file.includes(path.sep + "node_modules") && !file.includes(path.sep + "payload" + path.sep + "files") && !file.includes(path.sep + "_shots") && !file.endsWith(path.sep + "loot-service.exe") && !file.endsWith(path.sep + "backend-digest.cjs") && !file.endsWith(path.sep + "uninstall-shell.exe") });
    }
    const desktop = path.join(snapshot, "desktop"), packed = path.join(snapshot, "dist/desktop");
    fs.symlinkSync(path.join(root, "desktop/node_modules"), path.join(desktop, "node_modules"), process.platform === "win32" ? "junction" : "dir");
    fs.mkdirSync(path.join(desktop, "backend"), { recursive: true });
    fs.writeFileSync(path.join(desktop, "app-title.cjs"), '"use strict";\nmodule.exports = "Deep Legends-STAGING";\n');
    fs.writeFileSync(path.join(desktop, "license-build.cjs"), '"use strict";\nmodule.exports = Object.freeze({ enabled: true });\n');
    const version = require(path.join(root, "desktop/package.json")).version;
    const fingerprint = require(path.join(root, "desktop/source-fingerprint.cjs")).sourceFingerprint(snapshot);
    const backend = path.join(desktop, "backend/loot-service.exe");
    run("go", ["build", "-tags=license,license_staging", "-buildvcs=false", "-trimpath", "-ldflags", `-s -w -H=windowsgui -buildid= -X main.version=${version} -X main.buildFingerprint=${fingerprint} -X main.riotAPIKey= -X main.riotAPIKeyCipher=`, "-o", backend, "./backend"]);
    run(process.execPath, [path.join(snapshot, "installer/build-shell.cjs"), "--uninstall", version]);
    // Keep the existing install shell's executable/folder contract; the window,
    // shortcut, uninstall display name, appId and final artifacts identify staging.
    run(process.execPath, [path.join(root, "desktop/node_modules/electron-builder/cli.js"), "--win", "nsis", "--x64", "--publish=never", "--config.appId=cn.hexcore.lootassistant.STAGING", "--config.nsis.shortcutName=Deep Legends-STAGING", "--config.nsis.uninstallDisplayName=Deep Legends-STAGING", "--config.nsis.artifactName=Deep Legends Setup ${version}-public.${ext}", "--config.beforePack=verify-license-staging-pack.cjs", "--config.win.signExecutable=false", "--config.directories.output=" + packed], desktop);
    run(process.execPath, [path.join(desktop, "verify-packaged-runtime.cjs"), path.join(packed, "win-unpacked/resources/app.asar")]);
    run(process.execPath, [path.join(snapshot, "installer/build-shell.cjs"), version, fingerprint]);
    const asar = require(path.join(root, "desktop/node_modules/@electron/asar"));
    const archive = path.join(packed, "win-unpacked/resources/app.asar");
    if (!asar.extractFile(archive, "app-title.cjs").toString("utf8").includes('module.exports = "Deep Legends-STAGING";')) throw Error("Packaged staging title is missing");
    const entries = asar.listPackage(archive);
    if (entries.some(entry => /(?:testdata|\.test\.|_test\.go|protocol-vectors|verify-license|build-license-staging)/i.test(entry))) throw Error("Packaged runtime contains test or build fixtures");
    const packedBackend = path.join(packed, "win-unpacked/resources/app.asar.unpacked/backend/loot-service.exe");
    require(path.join(desktop, "verify-license-staging-pack.cjs")).verifyStagingBackend(packedBackend);
    const fixedDigest = asar.extractFile(archive, "backend-digest.cjs").toString("utf8");
    if (!fixedDigest.includes(digest(packedBackend))) throw Error("Packaged staging backend digest mismatch");
    fs.mkdirSync(output, { recursive: true });
    const setup = path.join(output, `Deep-Legends-Setup-${version}-staging-public.exe`);
    fs.renameSync(path.join(packed, `Deep Legends Setup ${version}-public.exe`), setup);
    const directory = path.join(output, "Deep-Legends-staging-public");
    fs.renameSync(path.join(packed, "win-unpacked"), directory);
    const receipt = { version, fingerprint, license_build: "STAGING", key_mode: "public", window_title: "Deep Legends-STAGING", license_origin: "https://license-staging.yinxiaobia.net", license_kid: "staging-2026-10", online_updates: "disabled at build time; no production Latest or cache", directory, setup, setup_sha256: digest(setup), backend_sha256: digest(path.join(directory, "resources/app.asar.unpacked/backend/loot-service.exe")), archive_sha256: digest(path.join(directory, "resources/app.asar")) };
    fs.writeFileSync(path.join(output, "staging-build.json"), JSON.stringify(receipt, null, 2) + "\n");
    fs.writeFileSync(path.join(output, "SHA256SUMS-staging-public.txt"), receipt.setup_sha256 + "  " + path.basename(setup) + "\n");
    console.log(JSON.stringify(receipt));
    return receipt;
  } finally {
    fs.rmSync(snapshot, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
  }
}
module.exports = { stagingOutput, build };
if (require.main === module) { try { build(); } catch (error) { console.error(error.message); process.exitCode = 1; } }
