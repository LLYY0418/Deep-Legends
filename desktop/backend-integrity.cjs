"use strict";
const fs = require("node:fs");
const crypto = require("node:crypto");
function backendHash(file) {
  if (!fs.lstatSync(file).isFile()) throw new Error("软件文件校验失败，请重新安装官方版本");
  const fd = fs.openSync(file, "r"), hash = crypto.createHash("sha256"), chunk = Buffer.alloc(1024 * 1024);
  try { let count; while ((count = fs.readSync(fd, chunk, 0, chunk.length, null)) > 0) hash.update(chunk.subarray(0, count)); }
  finally { fs.closeSync(fd); }
  return hash.digest("hex");
}
function verifyBackend(file, expected) {
  if (!/^[a-f0-9]{64}$/.test(expected || "") || backendHash(file) !== expected) throw new Error("软件文件校验失败，请重新安装官方版本");
  return true;
}
module.exports = { backendHash, verifyBackend };
