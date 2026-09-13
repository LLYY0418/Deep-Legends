"use strict";
const test = require("node:test"), assert = require("node:assert/strict"), fs = require("node:fs"), path = require("node:path");
const { JSDOM, VirtualConsole } = require("jsdom");
const root = path.join(__dirname, ".."), bridge = fs.readFileSync(path.join(root, "installer/internal/webviewhost/startup.js"), "utf8");

async function render(t, name, mutate = html => html) {
  let html = fs.readFileSync(path.join(root, `installer/ui/${name}.html`), "utf8");
  for (const name of ["LICENSE", "NOTICE"]) html = html.replaceAll(`__${name}__`, fs.readFileSync(path.join(root, `installer/ui/${name.toLowerCase()}.html`), "utf8"));
  html = html.replaceAll("__VERSION__", "0.12.0").replaceAll("__LOGO__", "data:image/png;base64,");
  // Match Document's inline script placement, including escaped Go identifiers.
  html = html.replaceAll("__INIT__", "\\u005f\\u005fINIT\\u005f\\u005f");
  const initial = { path:"D:\\游戏\\Deep Legends", version:"0.12.0", needBytes:42, sizeBytes:42, cacheBytes:0, freeBytes:1000 };
  html = mutate(html).replace("<head>", `<head><script>window.__INIT__=${JSON.stringify(initial)};\n${bridge}</script>`);
  const messages = [], dom = new JSDOM(html, { runScripts:"dangerously", virtualConsole:new VirtualConsole(), beforeParse(w) {
    w.chrome = { webview:{ postMessage:raw => messages.push(JSON.parse(raw)) } };
  } });
  t.after(() => dom.window.close());
  await new Promise(resolve => dom.window.addEventListener("load", resolve, { once:true }));
  return { w:dom.window, messages, initial };
}
for (const name of ["installer", "uninstaller"]) {
  test(`${name} reports ready only after real template initialization with inline state`, async t => {
    const {w,messages,initial} = await render(t,name);
    assert.deepEqual(messages,[{type:"shell-ready",detail:""}]);
    const el=w.document.getElementById(name === "installer" ? "path" : "s-path");
    assert.equal(name === "installer" ? el.value : el.textContent,initial.path);
    assert.ok(w.document.querySelector(".page.on"));
  });
  test(`${name} reports script failure instead of readiness after broken application bootstrap`, async t => {
    const {messages} = await render(t,name,html=>html.replace("window.host =", "throw new Error('startup regression'); window.host ="));
    assert.ok(messages.some(m=>m.type==="shell-error"));
    assert.equal(messages.some(m=>m.type==="shell-ready"),false);
  });
}
