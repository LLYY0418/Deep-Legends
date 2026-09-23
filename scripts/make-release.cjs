"use strict";
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const { sourceFingerprint } = require("../desktop/source-fingerprint.cjs");
const { verifyPublicReleaseBuild } = require("../desktop/release-build.cjs");
const { setupArtifactName, checksumArtifactName } = require("../desktop/artifact-names.cjs");
const repo = "LLYY0418/Deep-Legends";
const sha256 = (bytes) => crypto.createHash("sha256").update(bytes).digest("hex");

function latestNotes(changelog, version) {
  const section = changelog.match(/^##\s+(\S+)\s+—\s+\d{4}-\d{2}-\d{2}\s*\n([\s\S]*?)(?=^##\s|$(?![\s\S]))/m);
  if (!section || section[1] !== version || !section[2].trim()) throw new Error("CHANGELOG 最新一节必须与发布版本一致且包含更新内容");
  return section[2].trim();
}
function releaseName(name) {
  if (/\s/.test(name) || path.basename(name) !== name) throw new Error(`发布资产名不能包含空格或路径：${name}`);
  return name;
}
function makeRelease({ root = path.resolve(__dirname, ".."), fingerprint, publishedAt = new Date().toISOString(), minSupported = "0.9.0" } = {}) {
  const version = JSON.parse(fs.readFileSync(path.join(root, "desktop", "package.json"), "utf8")).version;
  const versionPattern = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;
  if (!versionPattern.test(version) || !versionPattern.test(minSupported)) throw new Error("发布版本号不合法");
  fingerprint ||= sourceFingerprint(root);
  if (!/^[0-9a-f]{12}$/.test(fingerprint)) throw new Error("构建指纹必须是 12 位小写十六进制");
  if (!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?Z$/.test(publishedAt) || Number.isNaN(Date.parse(publishedAt))) throw new Error("发布日期必须是 RFC3339 UTC 时间");
  const notes = latestNotes(fs.readFileSync(path.join(root, "CHANGELOG.md"), "utf8"), version);
  const assets = [
    [setupArtifactName(version, "public"), releaseName(`Deep-Legends-Setup-${version}-public.exe`)],
  ].map(([input, name]) => ({ input, name, bytes:fs.readFileSync(path.join(root, "dist", "desktop", input)) }));
  for (const asset of assets) if (!asset.bytes.length) throw new Error(`发布资产为空：${asset.name}`);
  verifyPublicReleaseBuild(root, fingerprint, assets);
  const setup = assets[0];
  const manifest = { schema:1, version, fingerprint, publishedAt, minSupported, notes, asset:{ name:setup.name, size:setup.bytes.length, sha256:sha256(setup.bytes), url:`https://github.com/${repo}/releases/download/v${version}/${setup.name}` } };
  assets.push({ name:"latest.json", bytes:Buffer.from(JSON.stringify(manifest,null,2)+"\n") });
  const sums = assets.map(({name,bytes})=>`${sha256(bytes)}  ${name}`).join("\n")+"\n";
  const destination = path.join(root,"dist","release");
  const staging = fs.mkdtempSync(path.join(root,"dist",".release-"));
  try {
    for (const {name,bytes} of assets) fs.writeFileSync(path.join(staging,name),bytes);
    fs.writeFileSync(path.join(staging,checksumArtifactName("public")),sums);
    fs.rmSync(destination,{recursive:true,force:true});
    fs.renameSync(staging,destination);
  } finally {fs.rmSync(staging,{recursive:true,force:true});}
  return manifest;
}
module.exports = {makeRelease,latestNotes,releaseName};
if (require.main === module) {
  try {
    const manifest=makeRelease({fingerprint:process.argv[2]});
    console.log(`已生成 dist/release/ 的三个发布文件（Setup、latest.json、SHA256SUMS-public.txt）：v${manifest.version} · ${manifest.fingerprint}`);
    console.log(`Setup SHA-256: ${manifest.asset.sha256}`);
  } catch(error) {console.error(error.message);process.exitCode=1;}
}
