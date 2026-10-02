const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const source = fs.readFileSync(path.join(__dirname, "gameplay.js"), "utf8");
const championSource = fs.readFileSync(path.join(__dirname, "champions.js"), "utf8");
const styles = fs.readFileSync(path.join(__dirname, "gameplay.css"), "utf8");

test("R166 DPM 780/840 renders the gray outline above the yellow fill with smaller dots", () => {
  const body = source.slice(source.indexOf("  function radarPoint("), source.indexOf("\tfunction renderRecentRanked("));
  const renderAbility = Function("escapeHTML", "number", `${body}\nreturn renderAbility;`)(String, String);
  const metrics = ["kda", "killParticipation", "damageShare", "dpm", "csm", "gpm", "vspm"].map((key) => ({
    key, label: key, player: key === "dpm" ? 780 : 1, baseline: key === "dpm" ? 840 : 1,
    playerScore: key === "dpm" ? 42.3 : 50, grade: key === "dpm" ? "B" : "B+",
  }));
  const html = renderAbility({ metrics, sampleGames: 10, baselineGames: 10, positionLabel: "上路" });
  const player = html.indexOf('class="ability-radar-player"');
  const baseline = html.indexOf('class="ability-radar-baseline"');
  assert.ok(player >= 0 && baseline > player, "gray outline must draw after yellow fill");
  assert.match(styles, /\.ability-radar-baseline\s*\{[^}]*fill:\s*none/s);
  assert.equal((html.match(/ r="3"/g) || []).length, 7);
  assert.doesNotMatch(html, / r="4"/);
  assert.match(styles, /\.ability-radar-points circle\s*\{[^}]*stroke-width:\s*1\.5/s);
});

test("R166 mayhem hover waits 150 ms and rapid travel starts at most one RSC prefetch", () => {
  const setup = championSource.slice(championSource.indexOf("  let mayhemHoverTimer = 0;"), championSource.indexOf("  function readSetting("));
  const listeners = championSource.slice(championSource.indexOf('  root.addEventListener("pointerover"'), championSource.indexOf('  root.addEventListener("input"'));
  const handlers = new Map();
  const calls = [];
  const timers = new Map();
  let nextTimer = 1;
  const rows = Array.from({ length: 10 }, (_, index) => ({ championId: index + 1, key: `hero-${index + 1}` }));
  const root = { addEventListener: (type, handler) => handlers.set(type, handler) };
  const setTimer = (handler, delay) => { const id = nextTimer++; timers.set(id, { handler, delay }); return id; };
  const clearTimer = (id) => timers.delete(id);
  Function("root", "state", "rankingRows", "championMeta", "fetch", "setTimeout", "clearTimeout", `${setup}\n${listeners}`)(
    root, { mode: "aram-mayhem", mayhemView: "champions" }, () => rows, () => null,
    (url) => { calls.push(url); return Promise.resolve({ ok: true }); }, setTimer, clearTimer,
  );
  const rowElement = (id) => ({ dataset: { championRow: String(id) }, isConnected: true, closest: () => rowElement.current, contains: () => false });
  for (let id = 1; id <= 10; id++) {
    const row = rowElement(id); rowElement.current = row;
    handlers.get("pointerover")({ target: row });
    assert.deepEqual([...timers.values()].map((timer) => timer.delay), [150]);
    handlers.get("pointerout")({ target: row, relatedTarget: null });
  }
  assert.equal(calls.length, 0, "short hovers must not fetch");
  const selected = rowElement(1); rowElement.current = selected;
  handlers.get("pointerover")({ target: selected });
  for (const timer of [...timers.values()]) timer.handler();
  assert.equal(calls.length, 1);
  assert.match(calls[0], /mayhem-rsc-prefetch\?champion=hero-1/);
  handlers.get("pointerdown")({ target: selected });
  assert.equal(calls.length, 1, "pointerdown must reuse the same hero prefetch");
});
