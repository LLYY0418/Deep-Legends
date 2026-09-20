"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { JSDOM } = require("jsdom");
const WEB = path.join(__dirname, "..", "web");
const settle = () => new Promise(resolve => setTimeout(resolve, 250));

test("live specialist right overview button opens exact KR overview while rune selection and return remain independent", async () => {
  const dom = new JSDOM(fs.readFileSync(path.join(WEB, "index.html"), "utf8"), {
    url: "http://127.0.0.1:1/?demo", runScripts: "outside-only", pretendToBeVisual: true,
  });
  const w = dom.window;
  try {
    const errors = [], opened = [], requests = [];
    w.onerror = (_, __, ___, ____, error) => errors.push(String(error));
    w.IntersectionObserver = class { observe() {} unobserve() {} disconnect() {} };
    w.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} };
    w.matchMedia = () => ({ matches: false, addEventListener() {}, removeEventListener() {} });
    w.scrollTo = () => {};
    w.HTMLElement.prototype.scrollIntoView = () => {};
    w.Element.prototype.scrollTo = () => {};
    w.CSS ||= {}; w.CSS.escape ||= value => String(value).replace(/[^a-zA-Z0-9_-]/g, c => `\\${c}`);
    w.structuredClone = structuredClone;
    w.fetch = fetch; w.Response = Response; w.Headers = Headers; w.Request = Request;
    w.addEventListener("deep-legends:open-player", e => opened.push(e.detail));
    for (const file of ["runtime.js", "demo-data.js", "app.js", "gameplay.js", "champions.js", "friends.js", "suite.js"]) {
      w.eval(fs.readFileSync(path.join(WEB, file), "utf8"));
      if (file !== "demo-data.js") continue;
      const demoFetch = w.fetch;
      w.fetch = async (input, init) => {
        const url = typeof input === "string" ? input : input.url;
        requests.push({url, body: init?.body});
        if (url.startsWith("/api/diagnostics/client")) return new Response("{}");
        const response = await demoFetch(input, init);
        if (url === "/api/gameplay/overview" && init?.method === "POST") {
          const payload = await response.json();
          Object.assign(payload.player, {gameName:'한글 <이름> " &', tagLine:'KR&1', displayName:'한글 <이름> " &#KR&1'});
          return new Response(JSON.stringify(payload), {headers:{"Content-Type":"application/json"}});
        }
        if (!url.startsWith("/api/gameplay/live")) return response;
        const payload = await response.json();
        const row = payload.recommendations.runes.specialists[0];
        // Escaping must preserve the structured Riot ID, not parse a title.
        row.playerName = '한글 <이름> " &'; row.tagLine = 'KR&1'; row.title = '征服者 + 坚决';
        payload.recommendations.runes.specialists.push({...row, key:"specialist-1",playerName:"Second",tagLine:"KR2"});
        return new Response(JSON.stringify(payload), {headers:{"Content-Type":"application/json"}});
      };
    }
    w.document.dispatchEvent(new w.Event("DOMContentLoaded", {bubbles:true}));
    await settle();
    w.document.querySelector('[data-section="live"]').click(); await settle();
    w.document.querySelector('[data-rune-source="specialist"]').click(); await settle();
    const live = w.document.querySelector("#live-content");
    const selectors = live.querySelectorAll('[data-specialist-player]');
    assert.equal(selectors.length, 2);
    selectors[1].click();
    assert.equal(w.document.querySelector("#player-overlay").hidden, true);
    assert.ok(live.querySelector('[data-rune-choice="specialist-1"]'));
    const selectedRune = live.querySelector('[data-rune-choice].is-selected')?.dataset.runeChoice;
    const button = live.querySelector('[data-specialist-overview]');
    assert.equal(button.textContent, "总览");
    assert.equal(selectors[0].textContent, '한글 <이름> " &');
    assert.ok(selectors[0].classList.contains("specialist-player-name"));
    assert.ok(button.classList.contains("specialist-player-overview"));
    assert.equal(button.querySelector("이름"), null);
    button.click(); await settle();
    assert.equal(opened.length, 1);
    assert.equal(opened[0].gameName, '한글 <이름> " &');
    assert.equal(opened[0].tagLine, 'KR&1');
    assert.equal(opened[0].region, 'kr');
    assert.equal(opened[0].source, 'live-specialist');
    assert.equal(w.document.querySelector("#player-overlay").hidden, false);
    assert.match(w.document.querySelector("#player-overlay-title").textContent, /한글 <이름> " &#KR&1/);
    assert.equal(live.querySelector('[data-rune-choice].is-selected')?.dataset.runeChoice, selectedRune);
    w.document.querySelector("#player-overlay-back").click();
    assert.equal(w.document.querySelector("#player-overlay").hidden, true);
    assert.ok(live.querySelector('[data-rune-choice="specialist-1"]'), "return keeps selected specialist runes");
    assert.equal(errors.length, 0, errors.join("\n"));
  } finally { w.close(); }
});
