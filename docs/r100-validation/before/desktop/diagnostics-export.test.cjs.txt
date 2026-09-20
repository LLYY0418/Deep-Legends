"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const { EventEmitter } = require("node:events");
const path = require("node:path");
const fs = require("node:fs");
const { attachDiagnosticsExport } = require("./diagnostics-export.cjs");

function harness(onCompleted = () => {}, attach = attachDiagnosticsExport) {
  const session = new EventEmitter(), sender = {}, files = new Set();
  let directory = path.resolve("export-fixture"), available = true;
  const endpoint = "http://127.0.0.1:8795/api/diagnostics/log";
  const detach = attach({ session, sender, onCompleted,
    getBaseURL: () => "http://127.0.0.1:8795", getDirectory: () => directory,
    fileSystem: { statSync() { if (!available) throw Error("missing"); return { isDirectory: () => true }; }, existsSync: (p) => files.has(p) },
    now: () => new Date(2026, 8, 8, 10, 0) });
  function download({ contents = sender, urls = [endpoint], throws = false } = {}) {
    const item = new EventEmitter();
    item.getURLChain = () => urls;
    item.getURL = () => urls.at(-1);
    item.getSavePath = () => item.destination || "";
    item.setSaveDialogOptions = (value) => { item.dialogOptions = value; };
    item.setSavePath = (value) => { if (throws) throw Error("interrupted"); item.destination = value; };
    session.emit("will-download", {}, item, contents);
    return item;
  }
  return { session, files, detach, download, endpoint, get directory() { return directory; }, set directory(v) { directory = v; }, set available(v) { available = v; } };
}

test("R87 diagnostics support an optional independent export directory with bounded collision-free names", () => {
  const h = harness();
  const first = h.download();
  assert.equal(first.destination, path.join(h.directory, "lol-loot-diagnostics-0908-1000.jsonl"));
  const second = h.download();
  assert.match(second.destination, /-2\.jsonl$/);
  h.files.add(first.destination);
  first.emit("done", {}, "completed");
  assert.match(h.download().destination, /-3\.jsonl$/);
  h.directory = path.resolve("new-export-directory");
  assert.equal(path.dirname(h.download().destination), h.directory, "selection must not be captured at window creation");
  h.detach();
  assert.equal(h.session.listenerCount("will-download"), 0);
  assert.equal(h.download().destination, undefined);
});

test("R71 diagnostics do not redirect foreign downloads, redirects, or missing directory selections", () => {
  const h = harness();
  for (const options of [
    { contents: {} }, { urls: ["https://example.com/api/diagnostics/log"] },
    { urls: [h.endpoint, "http://127.0.0.1:8795/other"] },
    { urls: [h.endpoint + "?path=private"] }, { urls: [] },
  ]) assert.equal(h.download(options).destination, undefined);
  h.directory = "";
  assert.equal(h.download().destination, undefined);
  h.directory = "relative";
  assert.equal(h.download().destination, undefined);
  h.directory = path.resolve("deleted-directory");
  h.available = false;
  assert.equal(h.download().destination, undefined);
  h.detach();
});

test("R71 interrupted downloads release reservations and the hook is packaged and disposed", () => {
  const h = harness();
  assert.doesNotThrow(() => h.download({ throws: true }));
  const next = h.download();
  assert.match(next.destination, /1000\.jsonl$/);
  next.emit("done", {}, "cancelled");
  assert.equal(h.download().destination, next.destination);
  h.detach();
  const main = fs.readFileSync(path.join(__dirname, "main.cjs"), "utf8");
  assert.match(main, /getDefaultDirectory:.*app\.getPath\("downloads"\)/);
  assert.doesNotMatch(main.slice(main.indexOf("attachDiagnosticsExport({"), main.indexOf("ipcMain.removeHandler(\"desktop-diagnostics-open-folder\")")), /windowShareExportController/);
  assert.match(main, /once\("destroyed", removeDiagnosticsExport\)/);
  assert.ok(require("./package.json").build.files.includes("diagnostics-export.cjs"));
});

test("only completed diagnostics downloads enable opening the exported file folder", () => {
  const completed=[];
  const h=harness((file)=>completed.push(file));
  const interrupted=h.download(); interrupted.emit("done", {}, "interrupted");
  assert.deepEqual(completed,[]);
  const success=h.download(); success.emit("done", {}, "completed");
  assert.deepEqual(completed,[success.destination]);
  h.detach();
});

test("R87 P8 first export and unusable directories still report the actual Save dialog path", () => {
  function check(attach = attachDiagnosticsExport) {
   for (const directory of ["", "relative", path.resolve("deleted")]) {
    const completed=[];const h=harness(file=>completed.push(file),attach);h.directory=directory;h.available=false;
    const item=h.download();
    assert.equal(item.listenerCount("done"),1);
    assert.equal(item.destination,undefined);
    const chosen=path.resolve("user-selected-diagnostics.jsonl");item.getSavePath=()=>chosen;
    item.emit("done",{},"completed");assert.deepEqual(completed,[chosen]);h.detach();
   }
  }
  check();
  const source=fs.readFileSync(path.join(__dirname,"diagnostics-export.cjs"),"utf8");
  const changed=source.replace('    let reserved;', '    if (!getDirectory()) return;\n    let reserved;');
  assert.notEqual(changed,source,"first-export mutation must apply");
  const module={exports:{}};Function("require","module",changed)(require,module);
  assert.throws(()=>check(module.exports.attachDiagnosticsExport),{name:"AssertionError"});
});

test("R87 P8 cancellation, invalid save paths and disposed windows never emit completion", () => {
  for (const mode of ["cancelled","interrupted","empty","throw","detached"]) {
    const completed=[];const h=harness(file=>completed.push(file));h.directory="";
    const item=h.download();
    item.getSavePath=()=>{if(mode==='throw')throw Error('disposed');return mode==='empty'?'':path.resolve('saved.jsonl')};
    if(mode==='detached')h.detach();
    item.emit('done',{},['cancelled','interrupted'].includes(mode)?mode:'completed');
    assert.deepEqual(completed,[],mode);assert.equal(item.listenerCount('done'),0);h.detach();
  }
});
