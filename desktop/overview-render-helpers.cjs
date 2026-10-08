"use strict";
const assert = require("node:assert/strict");
const fs = require("node:fs"), path = require("node:path");
const { JSDOM } = require("jsdom");
const WEB = path.join(__dirname, "..", "backend", "web");
const SCRIPTS = ["runtime.js", "demo-data.js", "license-ui.js", "app.js", "favorites-facade.js", "gameplay.js", "champions.js", "friends.js", "suite.js"];
const gameplaySource = fs.readFileSync(process.env.R104_GAMEPLAY_SOURCE || path.join(WEB, "gameplay.js"), "utf8");
const suiteSource = fs.readFileSync(path.join(WEB, "suite.js"), "utf8");
const appStyles = fs.readFileSync(path.join(WEB, "app.css"), "utf8");
const gameplayStyles = fs.readFileSync(path.join(WEB, "gameplay.css"), "utf8");
const suiteStyles = fs.readFileSync(path.join(WEB, "suite.css"), "utf8");

function functionSource(source, name) {
  let start = source.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `${name} source not found`);
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

function compileFunctions(source, names, dependencies) {
  names = require("../backend/web/r211-harness-support.cjs").expand(source, names, dependencies);
  if (names.includes("renderOverviewBodyContent") && !names.includes("renderSelfIdentityHeader")) names = [...names,"renderSelfIdentityHeader"];
  const dependencyNames = Object.keys(dependencies);
  const factory = Function(
    ...dependencyNames,
    `"use strict";\n${names.map((name) => functionSource(source, name)).join("\n")}\nreturn { ${names.join(", ")} };`,
  );
  return factory(...dependencyNames.map((name) => dependencies[name]));
}

function collapsedBeaconStyles(css = appStyles) {
  const dom = new JSDOM(`<html data-sidebar="collapsed"><head><style>${css}</style><style>${gameplayStyles}</style></head><body><button id="section-live" class="section-tab"><span class="section-label">对局</span><span class="live-beacon"></span></button></body></html>`, { pretendToBeVisual: true });
  const label = dom.window.document.querySelector(".section-label");
  const beacon = dom.window.document.querySelector(".live-beacon");
  return {
    dom,
    labelDisplay: dom.window.getComputedStyle(label).display,
    beaconDisplay: dom.window.getComputedStyle(beacon).display,
  };
}

const openDemoWindows = new Set();
function installWindowCleanup(test) {
  test.afterEach(() => {
    for (const w of openDemoWindows) w.close();
  });
}

function bootDemoApp(options = {}) {
  const html = fs.readFileSync(path.join(WEB, "index.html"), "utf8");
  const errors = [];
	const eventSources = [];
  const dom = new JSDOM(html, { url: "http://127.0.0.1:1/?demo", runScripts: "outside-only", pretendToBeVisual: true });
  const w = dom.window;
  openDemoWindows.add(w);
  const mutationObservers = new Set();
  const NativeMutationObserver = w.MutationObserver;
  w.MutationObserver = class extends NativeMutationObserver {
    constructor(callback) {
      super(callback);
      mutationObservers.add(this);
    }
  };
  const closeWindow = w.close.bind(w);
  w.close = () => {
    if (!openDemoWindows.delete(w)) return;
    try {
      w.dispatchEvent(new w.CustomEvent("deep-legends:dispose"));
    } finally {
      // jsdom keeps queued observer microtasks after deleting window.document.
      // Keep real observer behavior during each test, but end it with its window.
      for (const observer of mutationObservers) observer.disconnect();
      mutationObservers.clear();
      closeWindow();
    }
  };
  w.onerror = (message, source, line, column, error) => { errors.push(String((error && error.stack) || message)); };
  w.addEventListener("unhandledrejection", (event) => { errors.push(String((event.reason && event.reason.stack) || event.reason)); });
  // jsdom 未实现的浏览器能力，用最小替身补齐（只影响可见性/尺寸，不影响渲染分支）。
  w.IntersectionObserver = class { observe() {} unobserve() {} disconnect() {} };
  w.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} };
  w.matchMedia = w.matchMedia || (() => ({ matches: false, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} }));
  w.scrollTo = () => {};
  w.HTMLElement.prototype.scrollIntoView = () => {};
  w.Element.prototype.scrollTo = function () {};
  w.structuredClone = globalThis.structuredClone;
  w.fetch = (input, init) => {
    const url = typeof input === "string" ? input : input?.url || "";
    if (new URL(url, w.location.href).pathname === "/api/license/status") {
      return Promise.resolve(new globalThis.Response(JSON.stringify({ state: "ACTIVE", message: "", generation: 1 }), { status: 200 }));
    }
    // The real diagnostic code still serializes its payload; jsdom has no server.
    if (new URL(url, w.location.href).pathname === "/api/diagnostics/client") {
      return Promise.resolve(new globalThis.Response("{}", { status: 200 }));
    }
    return globalThis.fetch(input, init);
  };
  w.Response = globalThis.Response;
  w.Headers = globalThis.Headers;
  w.Request = globalThis.Request;
	if (options.liveEvents) {
	  w.EventSource = class MockEventSource {
		static CLOSED = 2;
		constructor(url) { this.url = url; this.readyState = 1; eventSources.push(this); }
		close() { this.readyState = MockEventSource.CLOSED; }
	  };
	}
	if (options.matchCount) w.localStorage.setItem("lol-loot-match-count", String(options.matchCount));
  // 渲染故障会被 renderOverviewBody 兜住并写进 console.error，这里一并收集。
  w.console.error = (...args) => { errors.push(args.map((value) => (value && value.stack) || String(value)).join(" ")); };

  for (const file of SCRIPTS) {
    let source = fs.readFileSync(path.join(WEB, file), "utf8");
    if (file === "suite.js" && process.env.R137_SUITE_SOURCE) source = fs.readFileSync(process.env.R137_SUITE_SOURCE, "utf8");
    if (file === "suite.js" && options.suiteSourceTransform) source = options.suiteSourceTransform(source);
    if (file === "champions.js" && options.championsSourceTransform) source = options.championsSourceTransform(source);
    if (file === "gameplay.js" && options.gameplaySourceTransform) source = options.gameplaySourceTransform(source);
    if (file === "app.js" && options.championRankings) {
      const originalFetch = w.fetch;
      w.fetch = (url, ...args) => String(url).startsWith("/api/champions/rankings")
        ? Promise.resolve(new w.Response(JSON.stringify({ rows: options.championRankings }))) : originalFetch(url, ...args);
    }
    w.eval(source);
    if (file === "demo-data.js" && options.friendsPayload) {
      const demoFetch = w.fetch;
      w.fetch = (input, init) => {
        const url = typeof input === "string" ? input : input?.url || "";
        options.onRequest?.(url);
        return url.startsWith("/api/social/friends")
          ? Promise.resolve(new w.Response(JSON.stringify(options.friendsPayload()))) : demoFetch(input, init);
      };
    }
    if (file === "demo-data.js" && options.matchCount > 17) {
      const demoFetch = w.fetch;
      w.fetch = async (url, ...args) => {
        const response = await demoFetch(url, ...args);
        if (!String(url).startsWith('/api/gameplay/overview')) return response;
        const payload = await response.json(), original = payload.matches;
        payload.matches = Array.from({length:options.matchCount}, (_,index) => ({...JSON.parse(JSON.stringify(original[index % original.length])),gameId:1000000+index}));
        payload.pagination = {begIndex:0,count:options.matchCount,hasMore:false};
        return new w.Response(JSON.stringify(payload), {status:200});
      };
    }
	if (file === "demo-data.js" && options.facadeStateTransform) {
	  const demoFetch = w.fetch;
	  w.fetch = async (input, init) => {
		const url = typeof input === "string" ? input : input?.url || "";
		if (!url.startsWith("/api/facade/state") || String(init?.method || "GET").toUpperCase() !== "GET") return demoFetch(input, init);
		const response = await demoFetch(input, init);
		const payload = options.facadeStateTransform(await response.json());
		return new w.Response(JSON.stringify(payload), { status: response.status, headers: { "Content-Type": "application/json" } });
	  };
	}
  }
  w.document.dispatchEvent(new w.Event("DOMContentLoaded", { bubbles: true }));
  return { window: w, errors, eventSources };
}

const settled = () => new Promise((resolve) => setTimeout(resolve, 1500));


async function visitTool(w, name) {
  w.document.querySelector(`[data-suite-tab="${name}"]`).click();
  const root = w.document.getElementById(`suite-${name}-root`);
  const deadline = Date.now() + 2500;
  while (root.classList.contains("suite-loading") && Date.now() < deadline) await new Promise((resolve) => setTimeout(resolve, 20));
  assert.equal(root.classList.contains("suite-loading"), false, `${name} did not load`);
  await new Promise((resolve) => setTimeout(resolve, 40));
}

async function bootLiveTab() {
  const boot = bootDemoApp();
  await settled();
  boot.window.document.querySelector('[data-section="live"]').click();
  await settled();
  return boot;
}

module.exports = { WEB, SCRIPTS, gameplaySource, suiteSource, appStyles, gameplayStyles, suiteStyles, functionSource, compileFunctions, collapsedBeaconStyles, bootDemoApp, settled, installWindowCleanup, visitTool, bootLiveTab };
