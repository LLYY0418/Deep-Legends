"use strict";
const assert=require("node:assert/strict"),fs=require("node:fs"),path=require("node:path");
const { JSDOM } = require(require.resolve("jsdom", { paths: [path.join(__dirname, "..", "..", "desktop")] }));
const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");
const source = fs.readFileSync(process.env.R196_APP_SOURCE || process.env.R186_APP_SOURCE || path.join(__dirname, "app.js"), "utf8");
const ids = ["update-background", "update-ready-toast", "update-ready-title", "update-ready-copy", "update-ready-later", "update-ready-apply", "update-button", "update-dialog", "update-dialog-title", "update-notes", "update-meta", "update-progress", "update-progress-fill", "update-progress-percent", "update-progress-hint", "update-alert", "update-start", "update-later", "update-cancel", "update-apply", "update-release-link", "update-dialog-close", "settings-update-check", "settings-update-feedback"];
const available = { supported: true, current: "0.11.2", latest: "0.12.0", state: "available", portable: false, notes: "### 新增\n- **更新**和`代码`\n- [日志](https://example.com/log)", sizeBytes: 100 * 1024 * 1024, publishedAt: "2026-09-11T12:00:00Z", progress: {} };
function harness(script = source, respond) {
  const dom = new JSDOM(html, { url: "http://127.0.0.1:8787/", runScripts: "outside-only", pretendToBeVisual: true });
  const w = dom.window, requests = [], streams = [];
  w.matchMedia = () => ({ matches: false, addEventListener(){}, removeEventListener(){} });
  w.ResizeObserver = w.IntersectionObserver = class { observe(){} disconnect(){} unobserve(){} };
  w.HTMLElement.prototype.scrollTo = function(){};
  w.HTMLDialogElement.prototype.showModal = function(){this.open=true;};
  w.HTMLDialogElement.prototype.close = function(){this.open=false;};
  w.fetch = (url, options) => { requests.push([url,options]);if(url === "/api/diagnostics/client")return Promise.resolve({ok:true,status:204});return respond?.(url, options) || new Promise(()=>{}); };
  w.Headers = Headers;
  w.deepLegendsLicense = { isActive: () => true, poll() { w.dispatchEvent(new w.CustomEvent("deep-legends:license",{detail:{active:true}})); return Promise.resolve(); } };
  w.EventSource = class {
    static CLOSED = 2;
    constructor(url){this.url=url;this.listeners=new Map();streams.push(this);}
    addEventListener(name,fn){this.listeners.set(name,fn);}
    close(){}
    emit(name,data){this.listeners.get(name)?.({data:JSON.stringify(data)});}
  };
  try {
    w.eval(fs.readFileSync(path.join(__dirname, "runtime.js"), "utf8"));
    w.eval(script.replace(/\}\)\(\);\s*$/, 'window.updateProbe = { renderUpdateStatus, renderUpdateNotes, updateUI, renderUpdateDialog, showUpdateCheckFeedback };})();'));
  } catch(error) {dom.window.close();throw error;}
  return { w, dom, requests, streams, probe:w.updateProbe, get:id=>w.document.getElementById(id), close:()=>w.close() };
}
function renderScenarios(h) {
  for (const state of ["available", "downloading", "verifying", "ready", "failed", "applying"]) h.probe.renderUpdateStatus({ ...available, state, error:state==="failed"?"最后线路超时":"",progress:{receivedBytes:42,totalBytes:100,bytesPerSecond:10,etaSeconds:6} });
}

module.exports={source,ids,available,harness,renderScenarios};
