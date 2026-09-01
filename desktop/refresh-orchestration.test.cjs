"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require("jsdom");

const WEB = process.env.DEEP_LEGENDS_WEB_ROOT || path.join(__dirname, "..", "web");
const SCRIPTS = ["demo-data.js", "app.js", "gameplay.js"];
const gameplaySource = fs.readFileSync(path.join(WEB, "gameplay.js"), "utf8");

function functionSource(source, name) {
  let start = source.indexOf(`function ${name}(`);
  assert.notEqual(start, -1, `${name} source not found`);
  if (source.slice(Math.max(0, start - 6), start) === "async ") start -= 6;
  const parametersStart = source.indexOf("(", start);
  let parameterDepth = 0;
  let bodyStart = -1;
  for (let index = parametersStart; index < source.length; index += 1) {
    if (source[index] === "(") parameterDepth += 1;
    if (source[index] === ")" && --parameterDepth === 0) {
      bodyStart = source.indexOf("{", index + 1);
      break;
    }
  }
  assert.notEqual(bodyStart, -1, `${name} body start not found`);
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
  const dependencyNames = Object.keys(dependencies);
  const factory = Function(
    ...dependencyNames,
    `"use strict";\n${names.map((name) => functionSource(source, name)).join("\n")}\nreturn { ${names.join(", ")} };`,
  );
  return factory(...dependencyNames.map((name) => dependencies[name]));
}

function bootDemoApp({ withEventSource = false } = {}) {
  const html = fs.readFileSync(path.join(WEB, "index.html"), "utf8");
  const errors = [];
  const eventSources = [];
  const dom = new JSDOM(html, { url: "http://127.0.0.1:1/?demo", runScripts: "outside-only", pretendToBeVisual: true });
  const w = dom.window;
  w.onerror = (message, source, line, column, error) => { errors.push(String((error && error.stack) || message)); };
  w.addEventListener("unhandledrejection", (event) => { errors.push(String((event.reason && event.reason.stack) || event.reason)); });
  w.IntersectionObserver = class { observe() {} unobserve() {} disconnect() {} };
  w.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} };
  w.matchMedia = () => ({ matches: false, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} });
  w.scrollTo = () => {};
  w.HTMLElement.prototype.scrollIntoView = () => {};
  w.Element.prototype.scrollTo = function () {};
  w.structuredClone = globalThis.structuredClone;
  w.Response = globalThis.Response;
  w.Headers = globalThis.Headers;
  w.Request = globalThis.Request;
  w.fetch = (input, init) => {
    const url = requestURL(input);
    if (url.startsWith("/")) return Promise.resolve(new w.Response(null, { status: 204 }));
    return globalThis.fetch(input, init);
  };
  if (withEventSource) {
    w.EventSource = class {
      constructor(url) { this.url = url; this.closed = false; eventSources.push(this); }
      close() { this.closed = true; }
    };
  }
  w.console.error = (...args) => { errors.push(args.map((value) => (value && value.stack) || String(value)).join(" ")); };
  for (const file of SCRIPTS) w.eval(fs.readFileSync(path.join(WEB, file), "utf8"));
  w.document.dispatchEvent(new w.Event("DOMContentLoaded", { bubbles: true }));
  return { window: w, errors, eventSources };
}

async function waitFor(predicate, message, timeout = 5000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    if (predicate()) return;
    await new Promise((resolve) => setTimeout(resolve, 20));
  }
  assert.fail(message);
}

function requestURL(input) {
  return typeof input === "string" ? input : input?.url || "";
}

test("manual reread recovers a stuck overview and refreshes overview plus live without forcing the overlay", { concurrency: false }, async () => {
  const { window: w, errors } = bootDemoApp();
  try {
    await waitFor(() => w.document.querySelector("#overview-content .summoner-strip"), "initial overview did not load");
    const previousFetch = w.fetch;
    const overviewRequests = [];
    let liveRequests = 0;
    let holdFirstOverview = true;
    w.fetch = (input, init = {}) => {
      const url = requestURL(input);
      const pathname = url.split("?")[0];
      if (pathname === "/api/gameplay/overview") {
        overviewRequests.push(url);
        if (holdFirstOverview) {
          holdFirstOverview = false;
          return new Promise((resolve, reject) => {
            const abort = () => reject(new w.DOMException("Aborted", "AbortError"));
            if (init.signal?.aborted) abort();
            else init.signal?.addEventListener("abort", abort, { once: true });
          });
        }
      }
      if (pathname === "/api/gameplay/live") liveRequests += 1;
      return previousFetch(input, init);
    };

    w.document.getElementById("overview-refresh").click();
    await waitFor(() => overviewRequests.length === 1, "the deliberately stuck overview request did not start");
    w.document.getElementById("refresh").click();
    assert.equal(w.document.getElementById("app-frame").hasAttribute("inert"), false, "manual reread must not force the full-screen overlay");
    await waitFor(() => overviewRequests.length >= 2, "soft reset did not clear tab.loading for a forced overview request");
    await waitFor(() => liveRequests >= 1, "manual reread did not request the live endpoint");
    await waitFor(() => !w.document.getElementById("refresh").classList.contains("is-loading"), "manual reread did not finish");
    await new Promise((resolve) => setTimeout(resolve, 250));
    assert.ok(overviewRequests.some((url) => /[?&]force=1(?:&|$)/.test(url)), `forced overview URL missing: ${overviewRequests.join(", ")}`);
    assert.deepEqual(errors, [], `manual reread raised errors:\n${errors.join("\n")}`);
  } finally {
    w.close();
  }
});

test("gameplay soft reset clears stalled pagination and recommendation failures while preserving touched tabs", { concurrency: false }, () => {
  let aborted = 0;
  const tab = {
    loading: true,
    loadingMore: true,
    paginationStalls: 3,
    paginationBackoffMs: 8000,
    nextAutoAppendAt: 123,
    error: "stuck",
    data: { pagination: { hasMore: true, autoPaused: true, pauseReason: "stalled", moreError: "failed" } },
  };
  const state = {
    controllers: new Map([["overview:current", { abort() { aborted += 1; } }]]),
    tabs: [tab],
    liveLoading: true,
    liveError: "failed",
    liveGameGeneration: 0,
    liveRecommendations: new Map([["old", {}]]),
    specialistRunes: new Map([["old", []]]),
    liveRecommendationFailures: new Map([["old", { at: 1 }]]),
    liveRecommendationFlights: new Map(),
    specialistRuneFailures: new Map([["old", { at: 1 }]]),
    specialistRuneFlights: new Map(),
    livePositionOverride: new Map(),
    liveRecommendationSkipDiagnostics: new Set(),
    specialistRuneSkipDiagnostics: new Set(),
    liveRosterRenderDiagnostics: new Set(),
    specialistPlayerTabs: new Map(),
    recommendationTab: "build",
    recommendationTabTouched: true,
    runeSourceTab: "specialist",
    liveBuildExpanded: true,
    selectedRecommendation: "specialist",
  };
  const { softResetGameplayState } = compileFunctions(
    gameplaySource,
    ["resetLiveGameScopedState", "softResetGameplayState"],
    { state },
  );
  softResetGameplayState();
  assert.equal(aborted, 1);
  assert.equal(state.controllers.size, 0);
  assert.deepEqual(
    { loading: tab.loading, loadingMore: tab.loadingMore, stalls: tab.paginationStalls, backoff: tab.paginationBackoffMs, appendAt: tab.nextAutoAppendAt, error: tab.error },
    { loading: false, loadingMore: false, stalls: 0, backoff: 0, appendAt: 0, error: "" },
  );
  assert.deepEqual(tab.data.pagination, { hasMore: true, autoPaused: false, pauseReason: "", moreError: "" });
  assert.equal(state.liveLoading, false);
  assert.equal(state.liveError, "");
  assert.equal(state.liveRecommendationFailures.size, 0);
  assert.equal(state.specialistRuneFailures.size, 0);
  assert.equal(state.recommendationTab, "build");
  assert.equal(state.runeSourceTab, "specialist");
});

test("collection-dirty is accepted by the SSE whitelist and refreshes status", { concurrency: false }, async () => {
  const { window: w, errors, eventSources } = bootDemoApp({ withEventSource: true });
  try {
    await waitFor(() => w.document.querySelector("#overview-content .summoner-strip"), "initial overview did not load");
    let statusRequests = 0;
    const previousFetch = w.fetch;
    w.fetch = (input, init) => {
      if (requestURL(input).split("?")[0] === "/api/status") statusRequests += 1;
      return previousFetch(input, init);
    };
    const source = eventSources.at(-1);
    assert.ok(source?.onmessage, "SSE source was not initialized");
    source.onmessage({ data: "collection-dirty" });
    await waitFor(() => statusRequests >= 1, "collection-dirty was silently dropped instead of refreshing status");
    await new Promise((resolve) => setTimeout(resolve, 250));
    assert.deepEqual(errors, [], `collection-dirty handling raised errors:\n${errors.join("\n")}`);
  } finally {
    w.close();
  }
});

test("dirty collection rescans on entry and view changes without duplicate refresh requests", { concurrency: false }, async () => {
  const { window: w, errors, eventSources } = bootDemoApp({ withEventSource: true });
  try {
    await waitFor(() => w.document.querySelector("#overview-content .summoner-strip"), "initial overview did not load");
    let collectionDirty = false;
    let completedStatusRequests = 0;
    let refreshRequests = 0;
    const previousFetch = w.fetch;
    w.fetch = async (input, init) => {
      const pathname = requestURL(input).split("?")[0];
      if (pathname === "/api/status") {
        const response = await previousFetch(input, init);
        const payload = await response.json();
        payload.collectionDirty = collectionDirty;
        completedStatusRequests += 1;
        return new w.Response(JSON.stringify(payload), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (pathname === "/api/refresh") {
        refreshRequests += 1;
        return new w.Response(null, { status: 202 });
      }
      return previousFetch(input, init);
    };
    const source = eventSources.at(-1);

    collectionDirty = true;
    source.onmessage({ data: "collection-dirty" });
    await waitFor(() => completedStatusRequests >= 1, "dirty status did not reach the shell");
    w.document.querySelector('[data-section="favorites"]').click();
    await waitFor(() => refreshRequests === 1, "entering collection did not start the deferred rescan");
    w.document.querySelector('[data-view="all"]').click();
    w.document.querySelector('[data-view="owned"]').click();
    await new Promise((resolve) => setTimeout(resolve, 100));
    assert.equal(refreshRequests, 1, "view changes duplicated an in-flight collection rescan");

    collectionDirty = false;
    source.onmessage({ data: "collection-dirty" });
    await waitFor(() => completedStatusRequests >= 2, "clean status did not release the local rescan gate");
    collectionDirty = true;
    source.onmessage({ data: "collection-dirty" });
    await waitFor(() => completedStatusRequests >= 3, "second dirty status did not reach the shell");
    w.document.querySelector('[data-view="chromas"]').click();
    await waitFor(() => refreshRequests === 2, "changing collection view did not start a deferred rescan");
    await new Promise((resolve) => setTimeout(resolve, 250));
    assert.deepEqual(errors, [], `deferred collection rescan raised errors:\n${errors.join("\n")}`);
  } finally {
    w.close();
  }
});
