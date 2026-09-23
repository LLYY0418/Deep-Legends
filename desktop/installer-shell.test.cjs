"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const os = require("node:os");
const { JSDOM } = require("jsdom");
const { buildShell, buildUninstallShell, verifyShell } = require("../installer/build-shell.cjs");

function fixture(t, keyMode = "private") {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "r80-shell-build-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const name of ["desktop", "dist/desktop/win-unpacked/resources", "installer/payload/files"]) fs.mkdirSync(path.join(root, name), { recursive: true });
  fs.writeFileSync(path.join(root, "desktop/package.json"), '{"version":"0.11.2"}');
  fs.writeFileSync(path.join(root, "installer/payload/files/.gitkeep"), "");
  fs.writeFileSync(path.join(root, "dist/desktop/win-unpacked/Deep Legends.exe"), "application");
  fs.writeFileSync(path.join(root, "dist/desktop/win-unpacked/resources/app.asar"), "resources");
  fs.writeFileSync(path.join(root, `dist/desktop/Deep Legends Setup 0.11.2${keyMode === "public" ? "-public" : ""}.exe`), "original NSIS payload");
  return root;
}

function fakePE(file, size = 51 * 1024 * 1024) {
  const bytes = Buffer.alloc(4096);
  bytes.write("MZ"); bytes.writeUInt32LE(128, 0x3c);
  bytes.writeUInt32LE(0x4550, 128); bytes.writeUInt16LE(0x8664, 132);
  bytes.writeUInt16LE(0x20b, 152); bytes.writeUInt16LE(2, 220);
  fs.writeFileSync(file, bytes);
  fs.truncateSync(file, size);
}

test("installer build embeds the actual NSIS artifact and publishes only a verified GUI shell", (t) => {
  const root = fixture(t);
  const result = buildShell({ projectRoot: root, version: "0.11.2", fingerprint: "aabbccddeeff", keyMode: "private", run(command, args, options) {
    assert.equal(command, "go");
    assert.equal(options.cwd, path.join(root, "installer"));
    assert.equal(options.env.GOOS, "windows"); assert.equal(options.env.GOARCH, "amd64"); assert.equal(options.env.CGO_ENABLED, "0");
    assert.ok(args.includes("-s -w -H=windowsgui -buildid="));
    assert.equal(fs.readFileSync(path.join(root, "installer/payload/files/setup.exe"), "utf8"), "original NSIS payload");
    assert.deepEqual(JSON.parse(fs.readFileSync(path.join(root, "installer/payload/files/meta.json"))), {
      version: "0.11.2", fingerprint: "aabbccddeeff", installedBytes: 20, exeName: "Deep Legends.exe", productFolder: "Deep Legends",
    });
    fakePE(args[args.indexOf("-o") + 1]);
    return { status: 0 };
  } });
  assert.doesNotThrow(() => verifyShell(result.artifact));
  assert.deepEqual(fs.readdirSync(path.join(root, "installer/payload/files")), [".gitkeep"]);
});

for (const failure of ["compiler", "missing-payload", "wrong-format"]) {
  test(`installer build cleans payload and rejects ${failure}`, (t) => {
    const root = fixture(t);
    assert.throws(() => buildShell({ projectRoot: root, version: "0.11.2", fingerprint: "aabbccddeeff", keyMode: "private", run(command, args) {
      const output = args[args.indexOf("-o") + 1];
      if (failure === "compiler") return { status: 1 };
      fakePE(output, failure === "missing-payload" ? 4 * 1024 * 1024 : undefined);
      if (failure === "wrong-format") { const fd = fs.openSync(output, "r+"); fs.writeSync(fd, Buffer.from("XX"), 0, 2, 0); fs.closeSync(fd); }
      return { status: 0 };
    } }), /compilation failed|payload is missing|GUI executable/);
    assert.deepEqual(fs.readdirSync(path.join(root, "installer/payload/files")), [".gitkeep"]);
    assert.equal(fs.existsSync(path.join(root, "dist/desktop/Deep Legends Setup 0.11.2.exe")), false);
    assert.equal(fs.readdirSync(path.join(root, "dist/desktop")).some(name => name.startsWith(".installer-shell-")), false);
  });
}

test("public installer shell uses the public NSIS artifact", (t) => {
  const root = fixture(t, "public");
  const result = buildShell({ projectRoot: root, version: "0.11.2", fingerprint: "aabbccddeeff", keyMode: "public", run(_command, args) {
    assert.equal(fs.readFileSync(path.join(root, "installer/payload/files/setup.exe"), "utf8"), "original NSIS payload");
    fakePE(args[args.indexOf("-o") + 1]);
    return { status: 0 };
  } });
  assert.equal(result.artifact, path.join(root, "dist/desktop/Deep Legends Setup 0.11.2-public.exe"));
  assert.doesNotThrow(() => verifyShell(result.artifact));
});

test("both release entry points wrap NSIS before hashes and before deleting win-unpacked", () => {
  for (const name of ["build-desktop.sh", "build-desktop-windows.ps1"]) {
    const source = fs.readFileSync(path.join(__dirname, "..", name), "utf8");
    const pack = source.lastIndexOf("npm run pack:win-setup");
    assert.ok(source.indexOf("--uninstall") >= 0 && source.indexOf("--uninstall") < pack, "uninstaller must exist before NSIS compiles");
    assert.ok(source.indexOf("uninstall-shell.exe", pack) > pack, "generated uninstaller must be cleaned");
    const wrap = source.indexOf("build-shell.cjs", pack);
    const hash = Math.max(source.lastIndexOf("shasum -a 256"), source.lastIndexOf("Get-FileHash -Algorithm SHA256"));
    assert.ok(pack >= 0 && wrap > pack && hash > wrap, name);
    if (name.endsWith("ps1")) assert.ok(source.indexOf("Remove-Item -Recurse -Force $unpackedDirectory") > wrap);
  }
});

test("small uninstaller builds independently before NSIS without embedding payload", (t) => {
  const root = fixture(t);
  const artifact = buildUninstallShell({ projectRoot: root, version: "0.11.2", run(command, args, options) {
    assert.equal(command, "go");
    assert.equal(args.at(-1), "./uninstall");
    assert.equal(options.cwd, path.join(root, "installer"));
    assert.ok(args.some(arg => arg.includes("-X main.version=0.11.2")));
    assert.deepEqual(fs.readdirSync(path.join(root, "installer/payload/files")), [".gitkeep"]);
    fakePE(args[args.indexOf("-o") + 1], 4 * 1024 * 1024);
    return { status: 0 };
  } });
  assert.doesNotThrow(() => verifyShell(artifact, false));
  assert.equal(artifact, path.join(root, "desktop/uninstall-shell.exe"));
});

for (const failure of ["compiler", "payload", "invalid PE"]) {
  test(`uninstaller build removes partial artifact after ${failure}`, (t) => {
    const root = fixture(t);
    assert.throws(() => buildUninstallShell({ projectRoot: root, version: "0.11.2", run(command, args) {
      const output = args[args.indexOf("-o") + 1];
      if (failure === "compiler") { fs.writeFileSync(output, "partial"); return { status: 1 }; }
      fakePE(output, failure === "payload" ? 100 * 1024 * 1024 : 4 * 1024 * 1024);
      if (failure === "invalid PE") { const fd = fs.openSync(output, "r+"); fs.writeSync(fd, Buffer.from("XX"), 0, 2, 0); fs.closeSync(fd); }
      return { status: 0 };
    } }), /compilation failed|payload|GUI executable/);
    assert.equal(fs.existsSync(path.join(root, "desktop/uninstall-shell.exe")), false);
  });
}

test("uninstaller design supports retain/delete data, every bridge request and all progress states", (t) => {
  const messages = [];
  const initial = { path: "D:\\游戏\\Deep Legends", sizeBytes: 500000000, cacheBytes: 90000000, version: "0.11.2" };
  const html = fs.readFileSync(path.join(__dirname, "../installer/ui/uninstaller.html"), "utf8").replaceAll("__LOGO__", "data:image/png;base64,");
  const dom = new JSDOM(html, { runScripts: "dangerously", beforeParse(window) {
    window.__INIT__ = initial;
    window.chrome = { webview: { postMessage: raw => messages.push(JSON.parse(raw)) } };
  } });
  t.after(() => dom.window.close());
  const w = dom.window, el = id => w.document.getElementById(id);
  assert.equal(el("s-path").textContent, initial.path);
  assert.equal(el("s-version").textContent, initial.version);
  assert.equal(el("s-size").textContent, "477 MB");
  assert.equal(el("s-cache").textContent, "86 MB");
  assert.equal(el("chk-data").dataset.on, "0");
  el("btn-uninstall").click(); assert.deepEqual(messages.pop(), { type: "confirm", deleteData: false });
  el("chk-data").click(); assert.ok(el("btn-uninstall").classList.contains("danger"));
  el("btn-uninstall").click(); assert.deepEqual(messages.pop(), { type: "confirm", deleteData: true });
  el("btn-min").click(); assert.equal(messages.pop().type, "minimize");
  el("grab").dispatchEvent(new w.MouseEvent("mousedown", { button: 0 })); assert.equal(messages.pop().type, "drag");
  el("btn-cancel").click(); assert.equal(messages.pop().type, "close");
  w.host.uninstalling({ path: initial.path });
  assert.ok(el("page-progress").classList.contains("on"));
  const steps = Array.from(el("steps").children);
  assert.ok(steps[0].classList.contains("on"));
  w.host.progress({ percent: 46, stage: "正在删除程序文件…" });
  assert.ok(steps[0].classList.contains("done")); assert.ok(steps[1].classList.contains("on"));
  w.host.progress({ percent: 10 }); assert.equal(el("pct").textContent, "46%");
  w.host.progress({ percent: 97, stage: "正在清理快捷方式与注册表…" });
  assert.ok(steps[2].classList.contains("on"));
  el("btn-close").click(); assert.equal(messages.length, 0);
  w.host.done(); assert.equal(el("pct").textContent, "100%");
  assert.ok(steps.every(step => step.classList.contains("done")));
  assert.equal(el("prog-label").textContent, "卸载完成");
  w.host.failed({ message: "失败", detail: "<script>unsafe</script>" });
  assert.ok(el("page-fail").classList.contains("on"));
  assert.equal(el("fail-code").querySelector("script"), null);
  assert.equal(el("fail-code").textContent, "<script>unsafe</script>");
  el("btn-retry").click(); assert.ok(el("page-confirm").classList.contains("on"));
  el("btn-fail-close").click(); assert.equal(messages.pop().type, "close");
});

test("installer design executes all bridge interactions, terms, progress and failure without changing layout", async (t) => {
  const messages = [];
  const initial = { path: "D:\\游戏\\Deep Legends", needBytes: 500000000, freeBytes: 2000000000, version: "0.11.2" };
  let html = fs.readFileSync(path.join(__dirname, "../installer/ui/installer.html"), "utf8");
  for (const name of ["LICENSE", "NOTICE"]) html = html.replaceAll(`__${name}__`, fs.readFileSync(path.join(__dirname, `../installer/ui/${name.toLowerCase()}.html`), "utf8"));
  html = html.replaceAll("__VERSION__", initial.version).replaceAll("__LOGO__", "data:image/png;base64,");
  const dom = new JSDOM(html, { runScripts: "dangerously", beforeParse(window) {
    window.__INIT__ = initial;
    window.chrome = { webview: { postMessage: (raw) => messages.push(JSON.parse(raw)) } };
  } });
  t.after(() => dom.window.close());
  const { window: w } = dom;
  const el = (id) => w.document.getElementById(id);
  assert.equal(el("path").value, initial.path);
  assert.equal(el("chk-desktop").dataset.on, "1");
  assert.ok(el("btn-install").classList.contains("off"));
  el("lnk-notice").click();
  assert.ok(el("modal").classList.contains("on"));
  assert.match(el("sheet-body").textContent, /LCU/);
  el("sheet-accept").click();
  assert.ok(!el("btn-install").classList.contains("off"));
  el("chk-desktop").click();
  el("btn-install").click();
  assert.deepEqual(messages.pop(), { type: "install", path: initial.path, desktopShortcut: false });
  el("btn-browse").click(); assert.equal(messages.pop().type, "browse");
  el("btn-min").click(); assert.equal(messages.pop().type, "minimize");
  el("grab").dispatchEvent(new w.MouseEvent("mousedown", { button: 0 })); assert.equal(messages.pop().type, "drag");
  el("path").value = "E:\\Games";
  el("path").dispatchEvent(new w.Event("input"));
  await new Promise(resolve => setTimeout(resolve, 260));
  assert.deepEqual(messages.pop(), { type: "path", path: "E:\\Games" });
  w.host.path({ ok: false, error: "磁盘空间不足" });
  assert.ok(el("btn-install").classList.contains("off"));
  w.host.path({ ok: true, path: initial.path, freeBytes: initial.freeBytes });
  w.host.installing({ path: initial.path });
  w.host.progress({ percent: 34, stage: "正在解压程序文件…" });
  w.host.progress({ percent: 10 });
  assert.equal(el("pct").textContent, "34%");
  assert.equal(el("fill").style.width, "34%");
  el("btn-close").click(); assert.equal(messages.length, 0);
  w.host.progress({ percent: 97, stage: "正在优化首次启动…" });
  assert.equal(el("pct").textContent, "97%");
  assert.equal(el("stage").textContent, "正在优化首次启动…");
  w.host.progress({ percent: 98 }); assert.equal(el("pct").textContent, "98%");
  w.host.done(); assert.equal(el("pct").textContent, "100%");
  assert.equal(el("prog-label").textContent, "正在启动 Deep Legends…");
  assert.equal(el("stage").textContent, "安装已完成，正在等待应用窗口…");
  assert.match(html, /\.fill::after\s*\{[^}]*animation:\s*sheen[^}]*infinite/s);
  el("btn-close").click(); assert.equal(messages.length, 0, "handoff must keep installer open");
  assert.ok(el("page-install").classList.contains("on"));
  w.host.failed({ message: "失败", detail: "<script>test</script>" });
  assert.equal(el("fail-code").textContent, "<script>test</script>");
  assert.equal(el("fail-code").querySelector("script"), null);
  el("btn-retry").click(); assert.ok(el("page-setup").classList.contains("on"));
  el("btn-close").click(); assert.equal(messages.pop().type, "close");
});
