"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const { setupArtifactName } = require("../desktop/artifact-names.cjs");

function installedSize(directory) {
  let total = 0;
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const file = path.join(directory, entry.name);
    if (entry.isDirectory()) total += installedSize(file);
    else if (entry.isFile()) total += fs.statSync(file).size;
    else throw new Error(`Unexpected packaged entry: ${file}`);
  }
  return total;
}

function verifyShell(file, requirePayload = true) {
  if (requirePayload && fs.statSync(file).size <= 50 * 1024 * 1024) {
    throw new Error("Installer shell is <= 50 MB; the embedded NSIS payload is missing");
  }
  const fd = fs.openSync(file, "r");
  try {
    const header = Buffer.alloc(4096);
    fs.readSync(fd, header, 0, header.length, 0);
    const pe = header.readUInt32LE(0x3c);
    if (header.toString("ascii", 0, 2) !== "MZ" || pe + 94 > header.length ||
        header.readUInt32LE(pe) !== 0x4550 || header.readUInt16LE(pe + 4) !== 0x8664 ||
        header.readUInt16LE(pe + 24) !== 0x20b || header.readUInt16LE(pe + 24 + 68) !== 2) {
      throw new Error("Installer shell must be a Windows amd64 PE32+ GUI executable");
    }
  } finally {
    fs.closeSync(fd);
  }
}

// Built before NSIS's two-pass compiler reads customInstall's File directive.
function buildUninstallShell({ projectRoot, version, run = spawnSync }) {
  if (!/^\d+\.\d+\.\d+(?:[-+][\dA-Za-z.-]+)?$/.test(version)) throw new Error("Invalid uninstaller version");
  const artifact = path.join(projectRoot, "desktop/uninstall-shell.exe");
  fs.rmSync(artifact, { force: true });
  try {
    const result = run("go", ["build", "-buildvcs=false", "-trimpath", "-ldflags", `-s -w -H=windowsgui -buildid= -X main.version=${version}`, "-o", artifact, "./uninstall"], {
      cwd: path.join(projectRoot, "installer"),
      env: { ...process.env, GOOS: "windows", GOARCH: "amd64", CGO_ENABLED: "0" },
      stdio: "inherit",
    });
    if (result.error || result.status !== 0) throw new Error(`Uninstaller shell compilation failed: ${result.error?.message || result.status}`);
    verifyShell(artifact, false);
    if (fs.statSync(artifact).size >= 10 * 1024 * 1024) throw new Error("Uninstaller unexpectedly includes a payload");
    return artifact;
  } catch (error) {
    fs.rmSync(artifact, { force: true });
    throw error;
  }
}

// Both release scripts use exactly this sequence, including failure cleanup.
function buildShell({ projectRoot, version, fingerprint, run = spawnSync }) {
  if (!/^\d+\.\d+\.\d+(?:[-+][\dA-Za-z.-]+)?$/.test(version) || !/^[0-9a-f]{12}$/.test(fingerprint)) {
    throw new Error("Invalid installer version or fingerprint");
  }
  const packagedVersion = JSON.parse(fs.readFileSync(path.join(projectRoot, "desktop/package.json"), "utf8")).version;
  if (version !== packagedVersion) throw new Error("Installer and desktop package versions differ");
  const moduleRoot = path.join(projectRoot, "installer");
  const files = path.join(moduleRoot, "payload/files");
  const artifact = path.join(projectRoot, "dist/desktop", setupArtifactName(version, process.env.DEEP_LEGENDS_KEY_MODE || "private"));
  const staging = path.join(projectRoot, "dist/desktop", `.installer-shell-${fingerprint}.exe`);
  const setup = path.join(files, "setup.exe");
  const metadata = path.join(files, "meta.json");
  const unpacked = path.join(projectRoot, "dist/desktop/win-unpacked");
  const installedBytes = installedSize(unpacked);
  if (!(installedBytes > 0) || !fs.statSync(path.join(unpacked, "Deep Legends.exe")).isFile()) {
    throw new Error("Packaged application is missing");
  }
  fs.mkdirSync(files, { recursive: true });
  try {
    // The NSIS artifact was just produced by pack:win-setup.
    fs.renameSync(artifact, setup);
    fs.writeFileSync(metadata, JSON.stringify({ version, fingerprint, installedBytes, exeName: "Deep Legends.exe", productFolder: "Deep Legends" }) + "\n");
    const result = run("go", ["build", "-buildvcs=false", "-trimpath", "-ldflags", "-s -w -H=windowsgui -buildid=", "-o", staging, "."], {
      cwd: moduleRoot,
      env: { ...process.env, GOOS: "windows", GOARCH: "amd64", CGO_ENABLED: "0" },
      stdio: "inherit",
    });
    if (result.error || result.status !== 0) throw new Error(`Installer shell compilation failed: ${result.error?.message || result.status}`);
    verifyShell(staging);
    fs.renameSync(staging, artifact);
    return { artifact, installedBytes };
  } finally {
    fs.rmSync(setup, { force: true });
    fs.rmSync(metadata, { force: true });
    fs.rmSync(staging, { force: true });
  }
}

module.exports = { buildShell, buildUninstallShell, installedSize, verifyShell };
if (require.main === module) {
  if (process.argv[2] === "--uninstall") {
    const artifact = buildUninstallShell({ projectRoot: path.resolve(__dirname, ".."), version: process.argv[3] });
    console.log(`Uninstaller shell built: ${path.basename(artifact)}`);
  } else {
  const result = buildShell({ projectRoot: path.resolve(__dirname, ".."), version: process.argv[2], fingerprint: process.argv[3] });
  console.log(`Installer shell built: ${path.basename(result.artifact)} (${result.installedBytes} installed bytes)`);
  }
}
