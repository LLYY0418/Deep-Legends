"use strict";
const assert = require("node:assert/strict");
const fs = require("node:fs");
const test = require("node:test");
const { JSDOM } = require("../../desktop/node_modules/jsdom");

const html = fs.readFileSync(`${__dirname}/index.html`, "utf8");
const script = fs.readFileSync(process.env.R320_LADDER_JS || `${__dirname}/ladder.js`, "utf8");
const flush = () => new Promise(setImmediate);

function snapshot(page, tier) {
  return {
    page, tier, fetchedAt: new Date().toISOString(),
    challengerCutoffLP: 1862, challengerCount: 300, grandmasterCutoffLP: 1371, grandmasterCount: 700,
    totalSummoners: 2050,
    rows: Array.from({ length: 100 }, (_, index) => ({
      rank: (page - 1) * 100 + index + 1, gameName: `Fixture ${page}-${index}`, tagLine: "KR1",
      profileIconId: 23, tier: tier || "challenger", lp: 2500 - index, level: 80, wins: 51, losses: 20, winRate: 72,
      champions: [{ championId: 103, name: "九尾妖狐" }], proMark: index < 2,
      ...(index === 0 ? { proPlayer: { team: "T1", name: "Reviewed" } } : {}),
    })),
  };
}

function fixture() {
  const dom = new JSDOM(html, { url: "http://127.0.0.1", runScripts: "outside-only", pretendToBeVisual: true });
  const w = dom.window;
  const requests = [], clocks = new Map();
  let timer = 0, intercept, renderSnapshot = snapshot;
  w.setTimeout = (fn, ms) => {
    const id = ++timer;
    clocks.set(id, { fn, ms });
    return id;
  };
  w.clearTimeout = (id) => clocks.delete(id);
  w.fetch = async (url, options) => {
    const parsed = new URL(url, w.location.href);
    requests.push({ url: parsed, options });
    const special = intercept?.(parsed, options);
    if (special) return special;
    const page = Number(parsed.searchParams.get("page"));
    const tier = parsed.searchParams.get("tier");
    const data = parsed.pathname.endsWith("/find") ? { page: 3, rank: 202 }
      : parsed.pathname.endsWith("/prewarm") ? { playerRef: "player_prewarmed" } : renderSnapshot(page, tier);
    return { ok: true, status: 200, json: async () => data };
  };
  w.eval(script);
  const dispatch = (name, detail) => w.dispatchEvent(new w.CustomEvent(`deep-legends:${name}`, { detail }));
  const get = (selector) => w.document.querySelector(selector);
  const runClock = (ms) => {
    for (const [id, clock] of clocks) {
      if (clock.ms === ms) {
        clocks.delete(id);
        void clock.fn();
        break;
      }
    }
  };
  return { w, requests, clocks, get, dispatch, runClock, record: (fn) => { renderSnapshot = fn; }, intercept: (fn) => { intercept = fn; },
    open: () => dispatch("section", { name: "ladder" }), leave: () => dispatch("section", { name: "overview" }) };
}

test("R320 ladder starts on user intent, shows 12 skeletons and renders exactly 100 rows", async (t) => {
  const f = fixture();
  t.after(() => f.w.close());
  await flush();
  assert.deepEqual(f.requests, []);
  f.open();
  assert.equal(f.w.document.querySelectorAll(".ladder-skeleton").length, 12);
  assert.equal(f.get("#ladder-rows").getAttribute("aria-busy"), "true");
  await flush();
  assert.equal(f.w.document.querySelectorAll("[data-ladder-row]").length, 100);
  assert.equal(f.w.document.querySelectorAll(".pro-mark").length, 2);
  assert.equal(f.w.document.querySelectorAll(".pro-name").length, 2);
  assert.equal(f.w.document.querySelectorAll(".pro-tag").length, 1);
  assert.match(f.get(".pro-tag").textContent, /T1Reviewed/);
  assert.equal(f.get(".pro-mark").getAttribute("aria-label"), "职业选手");
  assert.match(f.get(".ladder-avatar").getAttribute("data-queued-src"), /^\/api\/image\?/);
  assert.match(f.get(".ladder-champions img").getAttribute("data-queued-src"), /champion-icons%2F103\.png/);
  assert.equal(f.get(".ladder-champions img").dataset.tooltip, "九尾妖狐");
  assert.equal(f.get("#ladder-range").textContent, "#1 ~ #100");
  assert.equal(f.get("#ladder-total").textContent, "2,050");

  f.runClock(2000);
  await flush();
  assert.equal(f.requests[1].url.searchParams.get("page"), "2");
  assert.equal(f.requests[1].url.searchParams.get("prefetch"), "1");
  f.get('[data-ladder-page="2"]').click();
  await flush();
  assert.equal(f.requests.length, 2, "next page reuses prefetched response");
  assert.equal(f.get('[aria-current="page"][data-ladder-page]').textContent, "2");
});

test("R320 stale events patch only changed rows and keep focus", async (t) => {
  const f = fixture();
  t.after(() => f.w.close());
  f.open();
  await flush();
  const first = f.get("[data-ladder-row]");
  const second = first.nextElementSibling;
  first.querySelector("button").focus();
  f.record((page, tier) => {
    const data = snapshot(page, tier);
    data.rows[1].lp++;
    return data;
  });
  f.dispatch("ladder-updated", { page: 1, tier: "" });
  await flush();
  assert.equal(f.get("[data-ladder-row]"), first);
  assert.notEqual(first.nextElementSibling, second);
  assert.equal(f.w.document.activeElement, first.querySelector("button"));
});

test("R320 marks a failed refresh on the timestamp and clears it on recovery", async (t) => {
  const f = fixture();
  t.after(() => f.w.close());
  const css = fs.readFileSync(`${__dirname}/ladder.css`, "utf8");
  assert.match(css, /\.ladder-update time\.is-stale\s*\{\s*color:\s*var\(--muted\)/);
  const data = snapshot(1, "");
  f.record(() => data);
  f.open();
  await flush();
  const row = f.get("[data-ladder-row]");
  const time = f.get("#ladder-time");
  assert.match(time.textContent, /^\d+ 分钟前$/);
  assert.equal(time.classList.contains("is-stale"), false);

  f.record(() => ({ ...data, stale: true, staleReason: "upstream_unavailable" }));
  f.dispatch("ladder-updated", { page: 1, tier: "" });
  await flush();
  assert.match(time.textContent, /^\d+ 分钟前 · 暂时无法更新$/);
  assert.equal(time.classList.contains("is-stale"), true);
  assert.equal(time.dateTime, data.fetchedAt);
  assert.equal(f.get("[data-ladder-row]"), row);
  assert.equal(f.get("#ladder-failed").hidden, true);

  f.record(() => ({ ...data, stale: false }));
  f.get("#ladder-refresh").click();
  await flush();
  assert.match(time.textContent, /^\d+ 分钟前$/);
  assert.equal(time.classList.contains("is-stale"), false);
  assert.equal(f.get("[data-ladder-row]"), row);
});

test("R320 finder jumps to its page, highlights the exact identity, and preserves return state", async (t) => {
  const f = fixture();
  t.after(() => f.w.close());
  f.open();
  await flush();
  f.get('[data-ladder-tier="master"]').click();
  await flush();
  f.get("#ladder-pro-only").click();
  assert.equal(f.w.document.querySelectorAll("[data-ladder-row]").length, 2);
  f.get("#ladder-find input").value = "Fixture 3-1#KR1";
  f.get("#ladder-find").dispatchEvent(new f.w.Event("submit", { cancelable: true }));
  await flush();
  assert.equal(f.get(".is-highlighted .pro-name").textContent, "Fixture 3-1");
  assert.equal(f.get('[data-ladder-tier=""]').getAttribute("aria-pressed"), "true");
  assert.equal(f.get("#ladder-pro-only").getAttribute("aria-checked"), "true");
  const target = f.get(".is-highlighted button");
  const scroll = f.get("#app-scroll");
  scroll.scrollTop = 450;
  target.click();
  f.leave();
  f.open();
  assert.equal(f.w.document.activeElement, target);
  assert.equal(scroll.scrollTop, 450);
  assert.equal(f.get('[aria-current="page"][data-ladder-page]').textContent, "3");
});

test("R320 row hover waits 200 ms, aborts on exit, and hands a warm reference to overview", async (t) => {
  const f = fixture();
  t.after(() => f.w.close());
  f.open();
  await flush();
  const row = f.get("[data-ladder-row]");
  const pointer = (type) => row.dispatchEvent(new f.w.MouseEvent(type, { bubbles: true }));
  const warms = () => f.requests.filter(({ url }) => url.pathname.endsWith("/prewarm"));
  pointer("pointerover");
  assert.equal(warms().length, 0);
  assert([...f.clocks.values()].some(({ ms }) => ms === 200));
  pointer("pointerout");
  f.runClock(200);
  assert.equal(warms().length, 0, "short hover makes no request");
  pointer("pointerover");
  f.runClock(200);
  assert.equal(warms().length, 1);
  pointer("pointerout");
  assert.equal(warms()[0].options.signal.aborted, true);
  pointer("pointerover");
  f.runClock(200);
  await flush();
  let detail;
  f.w.addEventListener("deep-legends:open-player", (event) => { detail = event.detail; });
  row.click();
  assert.equal(detail.region, "kr");
  assert.equal(detail.source, "ladder");
  assert.equal(detail.playerRef, "player_prewarmed");
  assert.equal(detail.proPlayer.name, "Reviewed");
});

test("R320 distant search renders the supplied page without requesting a forbidden page parameter", async (t) => {
  const f = fixture();
  t.after(() => f.w.close());
  const data = snapshot(476, "");
  data.rows[0].tier = "diamond";
  data.rows[0].division = 1;
  f.intercept((url) => {
    if (url.pathname.endsWith("/find")) return { ok: true, status: 200, json: async () => ({ page: 476, rank: 47501, data }) };
  });
  f.open();
  await flush();
  f.get("#ladder-find input").value = "Fixture 476-0#KR1";
  f.get("#ladder-find").dispatchEvent(new f.w.Event("submit", { cancelable: true }));
  await flush();
  assert.equal(f.get(".is-highlighted .ladder-tier").textContent, "钻石 1");
  assert.equal(f.get("#ladder-range").textContent, "#47,501 ~ #47,600");
  assert.equal(f.requests.some(({ url }) => url.searchParams.get("page") === "476"), false);
  assert.equal(f.w.document.activeElement, f.get(".is-highlighted button"));
  f.get("#ladder-refresh").click();
  await flush();
  assert.equal(f.requests.filter(({ url }) => url.pathname.endsWith("/find")).length, 2);
  f.get('[data-ladder-page="100"]').click();
  await flush();
  assert.equal(f.requests.at(-1).url.searchParams.get("page"), "100");
});
