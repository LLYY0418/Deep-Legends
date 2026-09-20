"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require(require.resolve("jsdom", { paths: [path.join(__dirname, "..", "desktop")] }));

const html = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");
const source = fs.readFileSync(path.join(__dirname, "app.js"), "utf8");
const styles = fs.readFileSync(path.join(__dirname, "app.css"), "utf8");

function functionSource(name) {
  let start = source.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `${name} must exist in app.js`);
  if (source.slice(Math.max(0, start - 6), start) === "async ") start -= 6;
  const bodyStart = source.indexOf("{", start);
  let depth = 0;
  let quote = "";
  let escaped = false;
  for (let index = bodyStart; index < source.length; index += 1) {
    const character = source[index];
    if (quote) {
      if (escaped) escaped = false;
      else if (character === "\\") escaped = true;
      else if (character === quote) quote = "";
      continue;
    }
    if (character === '"' || character === "'" || character === "`") {
      quote = character;
      continue;
    }
    if (character === "{") depth += 1;
    if (character === "}" && --depth === 0) return source.slice(start, index + 1);
  }
  assert.fail(`${name} body is not balanced`);
}

function compileFunction(name, dependencies) {
  const names = Object.keys(dependencies);
  return Function(...names, `"use strict"; return (${functionSource(name)});`)(...names.map((key) => dependencies[key]));
}

test("总览未连接态只提供客户端入口且不提供密码输入", () => {
  const launchpad = html.match(/<section id="client-launchpad"[\s\S]*?<\/section>/)?.[0] || "";
  assert.match(launchpad, /id="launcher-list"[^>]*aria-label="客户端登录入口"/);
  assert.match(launchpad, /选择登录入口/);
  assert.match(launchpad, /密码、扫码与安全验证只在腾讯官方窗口完成，登录后自动连接。/);
  assert.doesNotMatch(launchpad, /<input|type="password"|name="(?:account|password|token)"/i);
  const removedLauncherName = ["We", "Game"].join("");
  assert.doesNotMatch(launchpad, new RegExp(`<button[^>]+data-client-id=["']${removedLauncherName}`, "i"));
  assert.doesNotMatch(source, new RegExp(`item\\.id\\s*===\\s*["']${removedLauncherName}["']`, "i"));
  assert.doesNotMatch(launchpad, /官方登录/);
  assert.doesNotMatch(styles, /official-login-button|official-login-action/);
});

test("国服纯净入口动作只提交固定 tcls id 并进入等待登录状态", async () => {
  const calls = [];
  const timers = [];
  const state = { clientLaunchInFlight: "", clientLaunched: null, officialLoginMessage: "", status: { connected: false } };
  const el = { officialLogin: { disabled: false } };
  const renderCalls = [];
  const launch = compileFunction("launchOfficialLogin", {
    state,
    el,
    renderLaunchpad: (status) => renderCalls.push(status),
    api: async (...args) => { calls.push(args); },
    showToast: () => {},
    refreshStatus: () => {},
    setTimeout: (callback, delay) => { timers.push({ callback, delay }); return timers.length; },
  });

  await launch();

  assert.equal(calls.length, 1);
  assert.equal(calls[0][0], "/api/client-launch");
  assert.deepEqual(JSON.parse(calls[0][1].body), { id: "tcls" });
  assert.deepEqual(Object.keys(JSON.parse(calls[0][1].body)), ["id"]);
  assert.equal(state.clientLaunchInFlight, "");
  assert.equal(state.clientLaunched.id, "tcls");
  assert.equal(state.officialLoginMessage, "");
  assert.ok(renderCalls.length >= 2);
  assert.deepEqual(timers.map((timer) => timer.delay), [1200]);
});

test("所有客户端入口共享启动锁并在失败后立即解锁", async () => {
  const calls = [];
  const timers = [];
  let resolveOfficial;
  const state = { clientLaunchInFlight: "", clientLaunched: null, officialLoginMessage: "", status: { connected: false } };
  const el = { officialLogin: { disabled: false } };
  const api = (...args) => {
    calls.push(args);
    if (calls.length === 1) return new Promise((resolve) => { resolveOfficial = resolve; });
    return Promise.resolve();
  };
  const dependencies = {
    state,
    el,
    renderLaunchpad: () => {},
    api,
    showToast: () => {},
    refreshStatus: () => {},
    setTimeout: (callback, delay) => { timers.push({ callback, delay }); return timers.length; },
  };
  const launchOfficial = compileFunction("launchOfficialLogin", dependencies);
  const launchAlternative = compileFunction("launchDetectedClient", dependencies);
  const officialPromise = launchOfficial();
  await launchAlternative({ disabled: false, dataset: { clientId: "riot" } });
  assert.equal(calls.length, 1, "alternative launch must be blocked while TCLS is launching");
  resolveOfficial();
  await officialPromise;
  assert.equal(state.clientLaunchInFlight, "");
  assert.equal(state.clientLaunched.id, "tcls");
  assert.deepEqual(timers.map((timer) => timer.delay), [1200]);

  const failingState = { clientLaunchInFlight: "", clientLaunched: null, officialLoginMessage: "", status: { connected: false } };
  const launchFailingAlternative = compileFunction("launchDetectedClient", {
    ...dependencies,
    state: failingState,
    api: async () => { throw new Error("启动失败"); },
  });
  await launchFailingAlternative({ disabled: false, dataset: { clientId: "riot" } });
  assert.equal(failingState.clientLaunchInFlight, "", "failed launch must release the shared lock");
  assert.equal(failingState.clientLaunched, null, "failed launch must not enter the waiting state");
  assert.equal(failingState.officialLoginMessage, "启动失败");
});

test("Riot 入口成功后进入对应等待状态并只安排状态刷新", async () => {
  const calls = [];
  const timers = [];
  const state = { clientLaunchInFlight: "", clientLaunched: null, officialLoginMessage: "", status: { connected: false } };
  const launch = compileFunction("launchDetectedClient", {
    state,
    renderLaunchpad: () => {},
    api: async (...args) => { calls.push(args); },
    showToast: () => {},
    refreshStatus: () => {},
    setTimeout: (callback, delay) => { timers.push({ callback, delay }); return timers.length; },
  });

  await launch({ disabled: false, dataset: { clientId: "riot" } });

  assert.equal(calls.length, 1);
  assert.equal(calls[0][0], "/api/client-launch");
  assert.deepEqual(JSON.parse(calls[0][1].body), { id: "riot" });
  assert.equal(state.clientLaunchInFlight, "");
  assert.equal(state.clientLaunched.id, "riot");
  assert.deepEqual(timers.map((timer) => timer.delay), [3000]);
});

test("启动成功后隐藏入口并按客户端显示等待登录状态", () => {
  const launchpad = html.match(/<section id="client-launchpad"[\s\S]*?<\/section>/)?.[0] || "";
  const dom = new JSDOM(`<!doctype html><body>${launchpad}</body>`);
  const document = dom.window.document;
  const state = {
    section: "overview",
    overviewTabIsCurrent: true,
    status: { connected: false },
    installationsLoaded: true,
    installationLoadError: "",
    officialLoginMessage: "",
    clientLaunchInFlight: "",
    clientLaunched: { id: "tcls", at: 1 },
    installations: [
      { id: "tcls", name: "TCLS", available: true },
      { id: "riot", name: "Riot", available: true },
    ],
  };
  const el = {
    clientLaunchpad: document.getElementById("client-launchpad"),
    launchpadEyebrow: document.getElementById("launchpad-eyebrow"),
    launchpadTitle: document.getElementById("launchpad-title"),
    launchpadDescription: document.getElementById("launchpad-description"),
    clientLaunchReselect: document.getElementById("client-launch-reselect"),
    launcherList: document.getElementById("launcher-list"),
    officialLoginStatus: document.getElementById("official-login-status"),
  };
  let render;
  render = compileFunction("renderLaunchpad", {
    state,
    el,
    escapeHTML: (value) => String(value),
    renderLaunchpad: (data) => render(data),
    launchOfficialLogin: () => {},
    launchDetectedClient: () => {},
    loadClientInstallations: () => {},
  });

  render(state.status);
  assert.equal(el.launcherList.hidden, true);
  assert.equal(el.clientLaunchReselect.hidden, false);
  assert.equal(el.clientLaunchReselect.disabled, false);
  assert.equal(el.launchpadEyebrow.textContent, "已启动");
  assert.equal(el.launchpadTitle.textContent, "正在登录国服客户端");
  assert.match(el.launchpadDescription.textContent, /弹出的腾讯窗口完成登录/);

  state.clientLaunched = { id: "riot", at: 2 };
  render(state.status);
  assert.equal(el.launcherList.hidden, true);
  assert.equal(el.launchpadTitle.textContent, "正在登录 Riot 客户端");
  assert.match(el.launchpadDescription.textContent, /弹出的 Riot 窗口完成登录/);

  el.clientLaunchReselect.click();
  assert.equal(state.clientLaunched, null);
  assert.equal(state.clientLaunchInFlight, "");
  assert.equal(el.launcherList.hidden, false);
  assert.equal(el.clientLaunchReselect.hidden, true);
  assert.equal(el.launchpadTitle.textContent, "选择登录入口");
  const buttons = Array.from(el.launcherList.querySelectorAll("[data-client-id]"));
  assert.equal(buttons.length, 2);
  assert.ok(buttons.every((button) => !button.disabled), "reselected launchers must be clickable");

  state.clientLaunched = { id: "tcls", at: 3 };
  render({ connected: true });
  assert.equal(state.clientLaunched, null, "connected client must clear the waiting state");
  assert.equal(state.clientLaunchInFlight, "");
  assert.equal(el.clientLaunchpad.hidden, true);
});

test("启动入口只渲染可用的 TCLS 与 Riot 客户端并使用统一卡片", () => {
  const render = functionSource("renderLaunchpad");
  assert.match(render, /item\.available && \(item\.id === "tcls" \|\| item\.id === "riot"\)/);
  assert.match(render, /data-client-id=/);
  assert.match(render, /launchOfficialLogin\(button\)/);
  assert.match(render, /国服纯净入口/);
  assert.match(render, /跳过 WeGame，直连国服客户端/);
  assert.match(render, /item\.name/);
  assert.match(styles, /\.launcher-list \{[^}]*grid-template-columns:\s*repeat\(auto-fit/);
  assert.match(styles, /\.launcher-card:disabled \{/);
});

test("安装扫描失败与未安装状态分开显示并允许重试", async () => {
  const state = { installations: [], installationsLoaded: true, installationLoadError: "", officialLoginMessage: "", status: { connected: false } };
  const renderCalls = [];
  const load = compileFunction("loadClientInstallations", {
    state,
    api: async () => { throw new Error("本地接口暂不可用"); },
    renderLaunchpad: (status) => renderCalls.push(status),
  });

  await load();

  assert.equal(state.installationsLoaded, true);
  assert.equal(state.installationLoadError, "本地接口暂不可用");
  assert.ok(renderCalls.length >= 2);
  const render = functionSource("renderLaunchpad");
  assert.match(render, /installationFailed \? `无法检查客户端安装位置/);
  assert.match(render, /无法检查客户端安装位置/);
  assert.match(render, /重新检查安装位置/);
});
