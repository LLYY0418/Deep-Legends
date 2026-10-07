"use strict";
const test = require("node:test"), assert = require("node:assert/strict"), fs = require("node:fs"), os = require("node:os"), path = require("node:path");
const { EventEmitter } = require("node:events");
const { lockedWindowBounds, createLicenseWindowController } = require("./license-window.cjs");
const { writeWindowBounds } = require("./window-bounds-store.cjs");
const normal = { x: 120, y: 80, width: 1200, height: 780, maximized: false };
const area = { x: -1920, y: 40, width: 1920, height: 1040 };
function fixture(t, bounds = normal, options = {}) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "r240-bounds-")); t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const file = path.join(dir, "window-bounds.json"); writeWindowBounds(file, bounds);
  const original = fs.readFileSync(file, "utf8"), scales = [], records = [], calls = [];
  let controller;
  class Window extends EventEmitter {
    constructor() { super(); this.bounds = { ...bounds }; this.normal = { ...bounds }; this.max = false; this.full = false; this.opacity = 1; this.resizable = true; this.maximizable = true; this.visible = false; }
    isDestroyed() { return false; }
    isMaximized() { return this.max; }
    isFullScreen() { return this.full; }
    getBounds() { return { ...this.bounds }; }
    getContentBounds() { return this.getBounds(); }
    getNormalBounds() { return { ...this.normal }; }
    setOpacity(value) { this.opacity = value; calls.push(["opacity", value]); }
    geometry(name) { calls.push([name, this.opacity]); assert.equal(this.opacity, 0, "P2 geometry must be invisible before resize or unmaximize"); }
    setMinimumSize(...value) { this.min = value; }
    setResizable(value) { this.resizable = value; }
    setMaximizable(value) { this.maximizable = value; }
    setBounds(value) { this.geometry("setBounds"); this.normal = { ...value }; this.bounds = { ...value }; controller.persistBounds(); }
    setContentBounds(value) {
      this.geometry("setContentBounds");
      if (this.max || this.full) return; // Windows can ignore geometry until leave events.
      if (this.dropFirstBounds) { this.dropFirstBounds = false; return; }
      this.bounds = { ...value }; controller.persistBounds();
    }
    maximize() { this.geometry("maximize"); this.max = true; this.bounds = { ...area }; }
    unmaximize() {
      this.geometry("unmaximize");
      if (this.noExitEvent) { this.max = false; return; }
      setTimeout(() => { this.max = false; this.bounds = { ...this.normal }; this.emit("unmaximize"); }, 25);
    }
    setFullScreen(value) {
      this.geometry("fullscreen");
      if (value) { this.full = true; this.bounds = { ...area }; }
      else setTimeout(() => { this.full = false; this.bounds = { ...this.normal }; this.emit("leave-full-screen"); }, 25);
    }
    show() { this.visible = true; }
  }
  const w = new Window();
  controller = createLicenseWindowController({ window: w, normalBounds: bounds, getWorkArea: () => area, getDisplayScale: () => 1.25,
    onScale: active => scales.push(active), writeBounds: value => writeWindowBounds(file, value), recordState: value => records.push(value), eventTimeout: 60, renderTimeout: 60, ...options });
  return { controller, w, file, original, scales, records, calls };
}
test("R240 every non-ACTIVE state has fixed centered 860x580 DIP without screen adaptation", async t => {
  for (const state of ["LOCKED", "NETWORK_LOCKED", "REPLACED", "REVOKED", "DEVICE_ERROR"]) {
    const { controller: c, w } = fixture(t); await c.setState(state);
    assert.deepEqual(w.bounds, { x: -1390, y: 270, width: 860, height: 580 });
    assert.deepEqual(w.min, [0, 0]); assert.equal(w.resizable, false); assert.equal(w.maximizable, false);
  }
  assert.deepEqual(lockedWindowBounds({ x: 0, y: 0, width: 3840, height: 2160 }), { x: 1490, y: 790, width: 860, height: 580 });
});
test("R240 pending/locked/transition persistence never alters window-bounds.json", async t => {
  const { controller: c, file, original } = fixture(t);
  assert.equal(c.persistBounds(), false); await c.setState("LOCKED");
  assert.equal(c.persistBounds(), false); assert.equal(fs.readFileSync(file, "utf8"), original);
  await c.setState("ACTIVE"); assert.equal(fs.readFileSync(file, "utf8"), original);
  assert.equal(c.persistBounds(), true);
});
for (const mode of ["normal", "maximized", "fullscreen"]) test(`R240 ${mode} runtime lock waits for native exit and restores saved state`, async t => {
  const { controller: c, w, records, file, original } = fixture(t);
  await c.setState("ACTIVE");
  w.normal = { x: 190, y: 110, width: 1400, height: 860 }; w.bounds = { ...w.normal };
  if (mode === "maximized") { w.max = true; w.bounds = { ...area }; }
  if (mode === "fullscreen") { w.full = true; w.bounds = { ...area }; }
  await c.setState("REVOKED");
  assert.equal(w.bounds.width, 860, "P3 must wait unmaximize before content sizing"); assert.equal(w.bounds.height, 580);
  assert.equal(w.max, false); assert.equal(w.full, false); assert.equal(w.resizable, false);
  assert.equal(fs.readFileSync(file, "utf8"), original);
  const lock = records.at(-1); assert.equal(lock.wasMaximized, mode === "maximized"); assert.equal(lock.wasFullscreen, mode === "fullscreen"); assert.equal(lock.waitTimedOut, false); assert.equal(lock.sizeMismatch, false); assert.equal(lock.displayScale, 1.25);
  await c.setState("ACTIVE");
  assert.equal(w.max, mode === "maximized"); assert.equal(w.full, mode === "fullscreen");
  assert.equal(w.normal.width, 1400); assert.equal(w.normal.height, 860);
});
test("R240 native event timeout continues and actual size mismatch retries exactly once", async t => {
  const { controller: c, w, records, calls } = fixture(t); await c.setState("ACTIVE");
  w.max = true; w.noExitEvent = true; w.dropFirstBounds = true;
  await c.setState("NETWORK_LOCKED");
  assert.equal(records.at(-1).waitTimedOut, true); assert.equal(records.at(-1).retried, true);
  assert.equal(records.at(-1).sizeMismatch, false); assert.equal(w.bounds.width, 860);
  assert.equal(calls.filter(c => c[0] === "setContentBounds").length, 2);
});
test("R240 render handshake holds opacity until acknowledged and timeout is bounded", async t => {
  let release; const { controller: c, w } = fixture(t, normal, { waitForRender: () => new Promise(resolve => { release = resolve; }) });
  const transition = c.setState("ACTIVE"); await new Promise(resolve => setImmediate(resolve));
  assert.equal(w.opacity, 0); assert.equal(c.showInitial(), false); release({}); await transition;
  assert.equal(c.showInitial(), true); assert.equal(w.opacity, 1);
  const f = fixture(t, normal, { waitForRender: () => new Promise(() => {}) }); const record = await f.controller.setState("LOCKED");
  assert.equal(record.renderTimedOut, true); assert.equal(f.w.opacity, 1);
});
test("R240 valid ACTIVE startup never applies the small window", async t => {
  const { controller: c, w, calls } = fixture(t); await c.setState("ACTIVE"); await c.setState("ACTIVE");
  assert.equal(calls.some(c => c[0] === "setContentBounds"), false); assert.deepEqual(w.bounds, { x:normal.x,y:normal.y,width:normal.width,height:normal.height });
});

for (const drops of [1, 2]) test(`R243 startup network recovery repairs ${drops} ignored native restores`, async t => {
  const f=fixture(t,normal,{getInitialBounds:()=>({width:1500,height:900})}),{controller:c,w,records}=f;
  await c.setState("NETWORK_LOCKED");const original=w.setBounds.bind(w);let remaining=drops;
  w.setBounds=value=>{if(remaining-->0)return;original(value)};
  await c.setState("ACTIVE");assert.ok(w.getContentBounds().width>=780 && w.getContentBounds().height>=600,"ACTIVE minimum guard must repair ignored native sizing");
  assert.equal(records.at(-1).fallback_used,true);assert.equal(records.at(-1).sizeMismatch,false);
  assert.equal(w.bounds.width,drops===1?1200:1500);assert.equal(w.bounds.height,drops===1?780:900);
  assert.equal(require("node:fs").readFileSync(f.file,"utf8"),f.original);
});
test("R243 complete native rectangle and measured ACTIVE content avoid locked inset arithmetic",async t=>{
 const {controller:c,w,records}=fixture(t,normal,{normalBounds:{width:1200,height:780}});
 w.bounds={x:50,y:60,width:1200,height:780};
 await c.setState("NETWORK_LOCKED");
 const read=w.getContentBounds.bind(w);w.getContentBounds=()=>c.isActive() && w.bounds.height===580?{...read(),height:0}:read();
 await c.setState("ACTIVE");assert.equal(records.at(-1).requestedContent.height,780);assert.ok(Number.isFinite(records.at(-1).requestedContent.width));
});

for (const scale of [1.25,1.5,1.75]) test(`R245 ${scale} native calls, telemetry and persistence use integer DIP and preserve the saved rectangle`,async t=>{
 const f=fixture(t,normal,{getDisplayScale:()=>scale}),{controller:c,w,records}=f;
 await c.setState('ACTIVE');
 const fractional={x:Math.round(191*scale)/scale,y:Math.round(111*scale)/scale,width:Math.round(1201*scale)/scale,height:Math.round(781*scale)/scale};
 w.bounds={...fractional};w.normal={...fractional};
 const calls=[];for(const method of ['setBounds','setContentBounds','setMinimumSize']){const original=w[method].bind(w);w[method]=(...args)=>{calls.push([method,args]);return original(...args)}}
 await c.setState('REVOKED');await c.setState('ACTIVE');
 for(const [method,args] of calls)for(const value of method==='setMinimumSize'?args:Object.values(args[0]))assert.ok(Number.isInteger(value),'R245 native geometry must be integer');
 for(const key of ['x','y','width','height'])assert.equal(w.normal[key],Math.round(fractional[key]),'R245 must restore saved rectangle');
 assert.equal(records.at(-1).native_error,null);assert.equal(records.at(-1).displayScale,scale);
 for(const key of ['x','y','width','height'])assert.ok(Number.isInteger(records.at(-1).actualContent[key]));
 c.persistBounds();const saved=JSON.parse(fs.readFileSync(f.file,'utf8'));for(const key of ['x','y','width','height'])assert.ok(Number.isInteger(saved[key]));
});
for(const throws of [1,2])test(`R245 ${throws} native restore exceptions recover through the saved then initial fallback`,async t=>{
 const {controller:c,w,records}=fixture(t,normal,{getInitialBounds:()=>({width:1500,height:900})});await c.setState('REVOKED');
 const original=w.setBounds.bind(w);let remaining=throws;w.setBounds=value=>{if(remaining-->0)throw new TypeError('test secret not exported');original(value)};
 await c.setState('ACTIVE');assert.equal(w.opacity,1);assert.equal(w.bounds.width,throws===1?1200:1500);assert.equal(w.bounds.height,throws===1?780:900);
 assert.equal(records.at(-1).native_error,'TypeError');assert.equal(records.at(-1).fallback_used,true);assert.equal(records.at(-1).sizeMismatch,false);
});
test('R245 minimized zero getBounds keeps the pre-lock normal rectangle',async t=>{
 const {controller:c,w}=fixture(t);await c.setState('ACTIVE');w.isMinimized=()=>true;const original=w.getBounds.bind(w);w.getBounds=()=>({...original(),width:0,height:0});
 await c.setState('REVOKED');w.isMinimized=()=>false;w.getBounds=original;await c.setState('ACTIVE');assert.equal(w.normal.width,1200);assert.equal(w.normal.height,780);
});
for(const method of ['setContentBounds','setMinimumSize'])test(`R245 ${method} throws once without interrupting the transition`,async t=>{
 const {controller:c,w,records}=fixture(t);await c.setState('ACTIVE');const original=w[method].bind(w);let once=true;
 w[method]=(...args)=>{if(once){once=false;throw new RangeError('private-native-message')}return original(...args)};
 await c.setState('REVOKED');assert.equal(w.bounds.width,860);assert.equal(w.bounds.height,580);assert.equal(w.opacity,1);assert.equal(records.at(-1).native_error,'RangeError');await c.setState('ACTIVE');assert.equal(w.normal.width,1200);
});
test('R245 renderer callback rejection still finishes visible and records the render failure',async t=>{
 const {controller:c,w,records}=fixture(t,normal,{waitForRender:async()=>{throw new Error('test render failure')}});await c.setState('REVOKED');assert.equal(w.opacity,1);assert.equal(records.at(-1).renderTimedOut,true);
});
