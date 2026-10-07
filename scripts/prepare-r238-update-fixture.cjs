"use strict";
// Public, pinned release input for the default update compatibility test.
// Keep the 111 MB installer out of Git; CI must obtain and verify it before tests.
const fs = require("node:fs/promises"), path = require("node:path"), crypto = require("node:crypto"), assert = require("node:assert/strict");
const directory = path.resolve(__dirname, "../docs/history/reports/release-0.12.76/anonymous-assets");
const name = "Deep-Legends-Setup-0.12.76-public.exe";
const expectedHash = "01014312b60e591a05bfc87f86e098adf6c5fc5e59520db5aedac5b5dba02ff3";
const origin = "https://github.com/LLYY0418/Deep-Legends/releases/download/v0.12.76/";
async function download(file) {
  const response = await fetch(origin + file, { signal: AbortSignal.timeout(120000) });
  assert.equal(response.status, 200, "published 0.12.76 input unavailable: " + file);
  return Buffer.from(await response.arrayBuffer());
}
(async () => {
  await fs.mkdir(directory, { recursive: true });
  const manifestBytes = await download("latest.json"), manifest = JSON.parse(manifestBytes);
  assert.equal(manifest.schema, 1); assert.equal(manifest.version, "0.12.76");
  assert.equal(manifest.asset.name, name); assert.equal(manifest.asset.size, 111854592);
  assert.equal(manifest.asset.sha256, expectedHash); assert.equal(manifest.asset.url, origin + name);
  let bytes;
  try { bytes = await fs.readFile(path.join(directory, name)); } catch (error) { if (error.code !== "ENOENT") throw error; }
  bytes ||= await download(name);
  assert.equal(bytes.length, 111854592);
  assert.equal(crypto.createHash("sha256").update(bytes).digest("hex"), expectedHash);
  await fs.writeFile(path.join(directory, name), bytes);
  await fs.writeFile(path.join(directory, "latest.json"), manifestBytes);
  console.log(JSON.stringify({ version: manifest.version, public: true, size: bytes.length, sha256: expectedHash, directory }));
})().catch(error => { console.error(error); process.exitCode = 1; });
