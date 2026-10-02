"use strict";
const fs = require("node:fs"), path = require("node:path");
const STDERR_LIMIT = 64 * 1024;
const PANIC_KINDS = new Set(["none", "index-out-of-range", "nil-pointer", "slice-bounds", "type-assertion", "closed-channel", "other"]);
const SIGNALS = new Set(["none", "SIGABRT", "SIGALRM", "SIGBUS", "SIGFPE", "SIGHUP", "SIGILL", "SIGINT", "SIGKILL", "SIGPIPE", "SIGQUIT", "SIGSEGV", "SIGTERM", "SIGTRAP", "SIGUSR1", "SIGUSR2"]);
const FRAME = /^main\.(?:\(\*[A-Za-z0-9_]+\)\.)?[A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)*$/;
const FRAME_LINE = /^(main\.(?:\(\*[A-Za-z0-9_]+\)\.)?[A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)*)\(/;
const markerPath = directory => path.join(directory, "desktop-relaunch.json");
const elapsed = value => Number.isSafeInteger(value) ? Math.max(0, value) : 0;
function stderrTail(previous, chunk) {
  const data = Buffer.concat([previous, Buffer.from(String(chunk), "utf8")]);
  return data.subarray(Math.max(0, data.length - STDERR_LIMIT));
}
function backendExitEvidence(code, signal, uptime, stderr) {
  const text = Buffer.from(stderr).subarray(-STDERR_LIMIT).toString("utf8");
  let kind = /^panic:/m.test(text) ? "other" : "none";
  for (const [prefix, name] of [
    ["panic: runtime error: index out of range", "index-out-of-range"],
    ["panic: runtime error: invalid memory address or nil pointer dereference", "nil-pointer"],
    ["panic: runtime error: slice bounds out of range", "slice-bounds"],
    ["panic: interface conversion:", "type-assertion"],
    ["panic: send on closed channel", "closed-channel"],
    ["panic: close of closed channel", "closed-channel"],
  ]) if (text.split(/\r?\n/).some(line => line.startsWith(prefix))) { kind = name; break; }
  const frames = [];
  for (const line of text.split(/\r?\n/)) {
    const match = line.match(FRAME_LINE);
    if (match && frames.length < 8) frames.push(match[1]);
  }
  return normalizeBackendEvidence({event:"desktop_backend_exit", code, signal:signal || "none", uptime_ms:uptime, panic_kind:kind, frames});
}
function normalizeBackendEvidence(raw) {
  if (!raw || typeof raw !== "object") return null;
  if (raw.event === "desktop_relaunch_requested") return {event:raw.event};
  if (raw.event === "desktop_relaunch_completed") return {event:raw.event, interval_ms:elapsed(raw.interval_ms)};
  if (raw.event !== "desktop_backend_exit") return null;
  return {event:raw.event, code:Number.isInteger(raw.code) ? Math.max(-2147483648,Math.min(2147483647,raw.code)) : -1,
    signal:SIGNALS.has(raw.signal) ? raw.signal : "none", uptime_ms:elapsed(raw.uptime_ms),
    panic_kind:PANIC_KINDS.has(raw.panic_kind) ? raw.panic_kind : "other",
    frames:Array.isArray(raw.frames) ? raw.frames.filter(value => typeof value === "string" && FRAME.test(value)).slice(0,8) : []};
}
function writeRelaunchMarker(directory, now = Date.now()) {
  try {
    fs.mkdirSync(directory,{recursive:true,mode:0o700});
    const dir=fs.lstatSync(directory);if(!dir.isDirectory()||dir.isSymbolicLink())return false;
    fs.writeFileSync(markerPath(directory), JSON.stringify({requested_at:Math.floor(now)}),
      {mode:0o600,flag:fs.constants.O_WRONLY|fs.constants.O_TRUNC|fs.constants.O_CREAT|(fs.constants.O_NOFOLLOW||0)});
    return true;
  } catch { return false; }
}
function consumeRelaunchMarker(directory, now = Date.now()) {
  const file = markerPath(directory);
  try {
    const info=fs.lstatSync(file);if(!info.isFile()||info.isSymbolicLink())return null;
    let marker;
    try { if(info.size<=256)marker=JSON.parse(fs.readFileSync(file,"utf8")); }
    finally { fs.unlinkSync(file); }
    if (!marker || !Number.isSafeInteger(marker.requested_at) || marker.requested_at<0 || marker.requested_at>now) return null;
    return normalizeBackendEvidence({event:"desktop_relaunch_completed", interval_ms:Math.floor(now-marker.requested_at)});
  } catch { return null; }
}
module.exports = {backendExitEvidence,normalizeBackendEvidence,stderrTail,writeRelaunchMarker,consumeRelaunchMarker,STDERR_LIMIT};
