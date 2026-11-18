"use strict";

const assert = require("node:assert/strict");
const { execFileSync } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");
const { pathToFileURL } = require("node:url");
const { WEB, functionSource } = require("./overview-render-helpers.cjs");

const root = path.resolve(WEB, "../..");
const read = (name) => fs.readFileSync(path.join(WEB, name), "utf8");
const chrome = process.env.CHROME_BIN ||
  (process.platform === "darwin" ? "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" : "chromium");
const gameplay = read("gameplay.css");
const detail = () => fs.readFileSync(process.env.R320_MATCH_DETAIL_CSS || path.join(WEB, "match-detail.css"), "utf8");
const show = (rev, name) => execFileSync("git", ["show", rev + ":backend/web/" + name], { cwd: root, encoding: "utf8" });
const preMigration = ".team-overview.is-arena > header strong";
const baseline = show("HEAD", "gameplay.css").includes(preMigration)
  ? "HEAD"
  : "f503eba68915fb326b70b3407e973160fdd9f4e1";
const oldGameplay = show(baseline, "gameplay.css");
const oldDetail = show(baseline, "match-detail.css");
const eagerNames = [...read("index.html").matchAll(/<link rel="stylesheet" href="\/([^"]+)"/g)].map((entry) => entry[1]);
const eager = eagerNames.map((name) => name === "gameplay.css" ? gameplay : read(name));
const oldEager = eagerNames.map((name) => name === "gameplay.css" ? oldGameplay : read(name));

function withBrowser(files, page) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "r320-match-styles-"));
  try {
    for (const [name, source] of Object.entries(files)) fs.writeFileSync(path.join(dir, name), source);
    const html = execFileSync(chrome, [
      "--headless=new", "--no-sandbox", "--disable-background-networking", "--no-first-run", "--no-default-browser-check",
      "--window-size=1440,960", "--virtual-time-budget=8000", `--user-data-dir=${path.join(dir, "profile")}`,
      "--dump-dom", pathToFileURL(path.join(dir, page)).href,
    ], { timeout: 30000, maxBuffer: 8 * 1024 * 1024, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
    const result = html.match(/<pre id="result">([A-Za-z0-9+/=]+)<\/pre>/);
    assert.ok(result, "Chromium did not report a result");
    const value = JSON.parse(Buffer.from(result[1], "base64").toString("utf8"));
    assert.equal(value.error, undefined, value.error);
    return value;
  } finally {
    fs.rmSync(dir, { recursive: true, force: true, maxRetries: 4 });
  }
}

function pageScript(run, inputs) {
  return `
    const input = ${JSON.stringify(inputs)};
    function report(result) {
      const bytes = new TextEncoder().encode(JSON.stringify(result));
      document.querySelector("#result").textContent = btoa(Array.from(bytes, byte => String.fromCharCode(byte)).join(""));
    }
    Promise.resolve().then(() => (${run})(input)).then(report, error => report({error: error.stack || String(error)}));
  `;
}

const fixture = `<!doctype html><html data-theme="dark"><meta charset="utf-8"><pre id="result"></pre>
  <main id="fixture"></main><script src="renderer-fixture.js"></script>`;
const rendererFunctions = [
  "ensureMatchDetailRenderer", "renderMatchDetail", "renderMatchOverview", "renderArenaMatchOverview", "renderTeamAnalysis",
  "renderBuild", "renderBuildPlayers", "renderBuildAugments", "buildSubject", "recordBuildSpellShape",
  "renderItemRoute", "renderSkillOrder", "skillPrioritySummary", "renderRuneYield", "renderRuneEffects",
].map((name) => functionSource(read("gameplay.js"), name)).join("\n");

function rendererFixture() {
  const icon = (size = "small", extra = "") =>
    `<span class="game-icon${size ? " is-" + size : ""}${extra}"><img alt="" width="16" height="16"></span>`;
  const numeric = (value) => String(value || 0);
  const escapeHTML = (value) => String(value).replace(/[<>&"]/g, (c) => ({ "<": "&lt;", ">": "&gt;", "&": "&amp;", '"': "&quot;" })[c]);
  const rune = (id, name, eogDescs) => ({ id, name, eogDescs, shortDesc: name });
  const runes = [
    rune(1, "Keystone", ["伤害：@eogvar1@"]), rune(2, "Healing", ["治疗：@eogvar2@"]),
    rune(3, "Income", ["金币：@eogvar3@"]), rune(4, "Utility", []),
  ];
  const player = (id, teamId, augments = []) => ({
    participantId: id, teamId, playerRef: `player-${id}`, championId: id, championName: `Champion ${id}`,
    position: id % 2 ? "top" : "middle", kills: 7, assists: 4, damage: 12000, damageTaken: 15000,
    itemIds: [1, 2, 3, 4, 5, 6, 7], augmentIds: augments, perkIds: [1, 2, 3, 4], primaryStyleId: 10, subStyleId: 20,
    perkStats: runes.map(({ id }) => ({ perkId: id, vars: [21, 22, 23] })),
  });
  const augments = [1, 2, 3, 4, 5, 6];
  const ordinary = { gameId: 10, subjectParticipantId: 1, participants: [player(1, 100), player(2, 100), player(3, 200), player(4, 200)],
    teams: [{ teamId: 100, win: true, kills: 14, damage: 20000 }, { teamId: 200, kills: 8, damage: 16000 }] };
  const arena = Array.from({ length: 6 }, (_, i) => ({
    gameId: 21 + i, arena: true, subjectParticipantId: 1,
    participants: Array.from({ length: 8 }, (_, j) => player(j + 1, Math.floor(j / 2) + 1, augments.slice(0, i + 1))),
  }));
  const matchPlayerGroups = (match) => ({
    arena: match.arena,
    groups: [...new Set(match.participants.map(({ teamId }) => teamId))].map((teamId) => ({
      teamId, subteamId: teamId, placement: teamId, players: match.participants.filter((p) => p.teamId === teamId),
    })),
  });
  const timeline = {
    available: true, itemGroups: [{ minute: 1, events: [{ itemId: 1 }, { itemId: 2, sold: true }] }, { minute: 8, events: [{ itemId: 3 }] }],
    skillOrder: [1, 2, 3, 4].map((slot) => ({ slot, level: slot })),
  };
  const state = {
    matchTimelines: new Map([...arena, ordinary].map(({ gameId }) => [gameId, timeline])),
    augmentDescriptions: new Map([[1, { status: "unavailable" }], [2, { status: "ok", description: "Sample augment description." }]]),
    perks: { perks: runes, styles: [
      { id: 10, name: "Primary", slots: [{ perks: [runes[0]] }, { perks: runes.slice(1, 3) }] },
      { id: 20, name: "Secondary", slots: [{ perks: [] }, { perks: [runes[3]] }] },
    ] },
  };
  const metric = { key: "damage", label: "伤害" };
  const dependencies = {
    state, matchPlayerGroups, escapeHTML, rerenderMatch: () => {}, connected: () => true, riotTab: () => false,
    readMatchScores: () => new Map(), number: numeric, kda: numeric, plainInteger: numeric, compactNumber: numeric, plainText: String,
    iconFigure: (_kind, _id, _name, size) => icon(size, " is-champion-art"),
    renderItemIcons: (ids, size) => ids.map(() => icon(size)).join(""),
    augmentIconFigure: (_id, size) => `<span class="arena-augment-icon">${icon(size)}</span>`,
    playerParticipantName: (p) => p.championName, maskedPlayerName: (p) => p.championName, proBadgeAttributes: () => "", autofillChip: () => "",
    renderMatchScoreCell: () => '<span class="match-score-cell"><span class="match-score-value">8.5</span><span class="match-score-badge"></span></span>',
    isCurrentMatchParticipant: (p, match) => p.participantId === match.subjectParticipantId,
    arenaTeamMeta: (group) => ({ name: `Team ${group.teamId}` }), arenaGroupLabel: (group) => `Team ${group.teamId}`,
    arenaPlacementResult: (_match, placement) => placement < 3 ? "win" : "loss",
    matchModeKind: (match) => match.arena ? "arena" : "standard", positionLabel: String,
    itemIconFigure: (_id, size) => icon(size), perkIconFigure: () => icon("rune"), renderRuneStyleIcon: () => '<svg viewBox="0 0 12 15"></svg>',
    isSummonersRiftMatch: (match) => !match.arena, TEAM_ANALYSIS_METRICS: [metric, { key: "kills", label: "击杀" }],
    TEAM_ANALYSIS_POSITIONS: ["top", "middle"], SKILL_SLOT_LETTERS: { 1: "Q", 2: "W", 3: "E", 4: "R" }, PERK_EFFECT_OVERRIDES: {},
    matchAugmentIDs: (p) => p.augmentIds, ensureAugmentDescriptions: () => {}, matchTimelineKey: (match) => match.gameId,
    spellIconFigure: () => icon(), augmentMetadata: (id) => ({ id, name: `Augment ${id}`, rarity: id % 3 }),
    normalizeAugmentRarity: (tier) => ({ className: ["is-silver", "is-gold", "is-prismatic"][tier], label: "Tier" }),
    renderUnifiedRuneBoard: () => `<div class="unified-rune-board"><section class="unified-rune-column"><header>${icon("rune-style")}</header><span class="rune-option-button is-selected is-linked">${icon("rune")}</span></section></div>`,
  };
  const makeRenderer = (overrides) => {
    const deps = { ...dependencies, ...overrides };
    return Function(...Object.keys(deps), `let matchDetailRenderer = null; ${rendererFunctions}
      return {renderMatchDetail, ensureMatchDetailRenderer, skillPrioritySummary};`)(...Object.values(deps));
  };
  const tabFor = (match, view) => ({ openMatches: [match.gameId], matchDetailTabs: new Map([[String(match.gameId), view]]) });
  return { makeRenderer, ordinary, arena, tabFor };
}

function compare(input) {
  const results = { differences: [], missing: [], rules: [], inventory: [] };
  const sheets = (css) => {
    const sheet = new CSSStyleSheet();
    sheet.replaceSync(css);
    return sheet;
  };
  const rules = (sheet) => {
    const collect = (list, conditions) => Array.from(list).flatMap((rule) => {
      if (rule.type === CSSRule.STYLE_RULE) return [{
        selector: rule.selectorText, declarations: rule.style.cssText, css: rule.cssText, conditions,
        properties: [...rule.style],
      }];
      if (rule.conditionText && rule.cssRules) {
        return collect(rule.cssRules, [...conditions, rule.cssText.slice(0, rule.cssText.indexOf("{")).trim()]);
      }
      return [];
    });
    return collect(sheet.cssRules, []);
  };
  const key = (rule) => JSON.stringify([rule.conditions, rule.selector, rule.declarations]);
  const counts = (list) => {
    const result = new Map();
    for (const rule of list) result.set(key(rule), (result.get(key(rule)) || 0) + 1);
    return result;
  };
  const subtract = (list, existing) => {
    const remaining = counts(existing);
    return list.filter((rule) => {
      const found = remaining.get(key(rule)) || 0;
      if (found) remaining.set(key(rule), found - 1);
      return !found;
    });
  };
  const unrelated = new Set([".pro-identity-chip", ".summoner-strip-copy span, .summoner-strip-copy p", ".summoner-strip-copy .pro-identity-chip"]);
  const beforeGameplay = rules(sheets(input.oldGameplay));
  const afterGameplay = rules(sheets(input.gameplay));
  const beforeDetail = rules(sheets(input.oldDetail));
  const afterDetail = rules(sheets(input.detail));
  const moved = subtract(beforeGameplay, afterGameplay).filter((rule) => !unrelated.has(rule.selector));
  const added = subtract(afterDetail, beforeDetail);
  const unchanged = beforeGameplay.filter((rule) => input.eagerSelectors.includes(rule.selector) && afterGameplay.some((current) => key(current) === key(rule)));
  const audit = [...moved, ...unchanged];
  const allAfter = counts([...afterGameplay, ...afterDetail]);
  for (const rule of moved) {
    if (!added.some((current) => key(rule) === key(current))) results.inventory.push(`missing move: ${rule.conditions} ${rule.css}`);
  }
  for (const rule of subtract(added, moved)) results.inventory.push(`unmatched addition: ${rule.conditions} ${rule.css}`);
  for (const rule of audit) {
    if (allAfter.get(key(rule)) !== 1) results.inventory.push(`expected one destination: ${rule.conditions} ${rule.css}`);
  }
  results.rules = audit.map((rule) => ({ selector: rule.selector, conditions: rule.conditions, css: rule.css, eager: unchanged.includes(rule) }));

  const { makeRenderer, ordinary, arena, tabFor } = rendererFixture();
  return (async () => {
    const renderer = makeRenderer();
    await renderer.ensureMatchDetailRenderer();
    const render = (match, view) => renderer.renderMatchDetail(match, match.participants[0], tabFor(match, view));
    // The current Arena renderer uses arena-detail-team for overview. Keep the
    // older team-overview tones and regular damage headings exercised as well.
    const variants = `<div class="match-detail is-arena-detail">
      <div class="match-overview-teams">${["", " is-first", " is-top", " is-blue", " is-red", " is-blue is-top"].map((tone) =>
        `<section class="team-overview is-arena${tone}"><header><strong>Arena team</strong><span>Team summary</span></header></section>`).join("")}</div>
      <div class="damage-compare"><section class="damage-team is-blue"><h4>Blue team</h4></section><section class="damage-team is-red"><h4>Red team</h4></section></div>
      <div class="damage-compare is-arena"><section class="damage-team is-blue"><h4>First</h4></section><section class="damage-team is-red"><h4>Second</h4></section></div>
      <div class="match-skill-priority"><b class="is-slot-1">Q</b><b class="is-slot-2">W</b><b class="is-slot-3">E</b></div>
    </div>`;
    const markup = [render(ordinary, "overview"), render(ordinary, "team"), render(ordinary, "build"),
      ...arena.map((match) => render(match, "overview")), render(arena[0], "team"), render(arena[5], "build"), variants].join("");
    // Exercise interaction rules with deterministic states and unchanged specificity in both sheets.
    const probe = (selector) => selector.replaceAll(":hover", "[data-r320-hover]").replaceAll(":focus-visible", "[data-r320-focus]");
    const compared = new Set();
    const properties = new Set(audit.flatMap((rule) => rule.properties));
    for (const property of ["font-family", "font-style", "line-height"]) properties.add(property);
    const names = [...properties];
    for (const property of getComputedStyle(document.body)) {
      if (names.some((name) => property.startsWith(name + "-"))) properties.add(property);
    }
    const createFrame = (styles, width, surface) => {
      const frame = document.createElement("iframe");
      frame.style.cssText = `display:block;width:${width}px;height:200px;border:0`;
      document.body.append(frame);
      const doc = frame.contentDocument;
      for (const css of styles) {
        const style = doc.createElement("style");
        style.textContent = css;
        doc.head.append(style);
      }
      doc.body.innerHTML = `<main class="${surface === "live" ? "live-match-dialog-content" : "matches-column"}">${markup}</main>`;
      doc.querySelector(".rune-effects .eff")?.classList.add("is-hover");
      const enableProbes = (list) => {
        for (const rule of list) {
          if (rule.selectorText) {
            const selector = probe(rule.selectorText);
            if (selector !== rule.selectorText) rule.selectorText = selector;
          }
          if (rule.cssRules) enableProbes(rule.cssRules);
        }
      };
      for (const sheet of doc.styleSheets) enableProbes(sheet.cssRules);
      return frame;
    };
    const selectors = [...new Set(audit.flatMap((rule) => rule.selector.split(/, (?=[^)]*(?:\(|$))/)))];
    const getTargets = (doc, selector) => {
      const pseudo = selector.match(/::[\w-]+$/)?.[0] || null;
      const base = pseudo ? selector.slice(0, -pseudo.length) : selector;
      return [...doc.querySelectorAll(probe(base))].map((element) => ({ element, pseudo }));
    };
    const areas = [
      [1280, ["light", "dark", "azure", "emerald", "violet", "crimson", "aurora", "oled"]],
      [980, ["light", "dark"]],
      [820, ["light", "dark"]],
      [660, ["light", "dark"]],
    ];
    for (const { surface, before: beforeStyles, after: afterStyles } of input.surfaces) for (const [width, themes] of areas) {
      const before = createFrame(beforeStyles, width, surface);
      const after = createFrame(afterStyles, width, surface);
      for (const theme of themes) {
        const snapshots = [before, after].map((frame) => {
          const doc = frame.contentDocument;
          doc.documentElement.dataset.theme = theme;
          const targets = new Map();
          for (const state of ["rest", "hover", "focus"]) {
            for (const node of doc.querySelectorAll(".match-detail-tabs button, .damage-sort-toggle, .team-analysis-metrics button, .build-player")) {
              node.toggleAttribute("data-r320-hover", state === "hover");
              node.toggleAttribute("data-r320-focus", state === "focus");
            }
            for (const selector of selectors) {
              if (state === "hover" && !selector.includes(":hover") || state === "focus" && !selector.includes(":focus-visible")) continue;
              const matches = getTargets(doc, selector);
              if (matches.length) compared.add(selector);
              for (const [index, { element, pseudo }] of matches.entries()) {
                const computed = frame.contentWindow.getComputedStyle(element, pseudo);
                const id = `${state} ${selector} [${index}]`;
                targets.set(id, Array.from(properties, (property) => computed.getPropertyValue(property)));
              }
            }
          }
          return targets;
        });
        for (const [id, prior] of snapshots[0]) {
          const current = snapshots[1].get(id);
          const differences = namesForDifference(prior, current, properties);
          if (differences.length) results.differences.push({ surface, width, theme, id, differences });
        }
      }
      before.remove();
      after.remove();
    }
    results.missing = selectors.filter((selector) => !compared.has(selector));
    results.selectorCount = compared.size;
    return results;
  })();

  function namesForDifference(prior, current, properties) {
    return [...properties].flatMap((property, index) =>
      prior[index] === current?.[index] ? [] : [[property, prior[index], current?.[index]]]);
  }
}

function stylesheetGate(input) {
  const { makeRenderer, ordinary, tabFor } = rendererFixture();
  const host = document.querySelector("#fixture");
  const tab = tabFor(ordinary, "overview");
  const append = document.head.append.bind(document.head);
  let held, sheetReady = false, moduleStartedBeforeCSS = false, richBeforeCSS = false;
  let resolveRich;
  const rich = new Promise((resolve) => { resolveRich = resolve; });
  document.head.append = (node) => {
    if (node.dataset.sectionStyle === "match-detail") {
      held = node;
      node.href = "./match-detail.css";
      node.addEventListener("load", () => { sheetReady = Boolean(node.sheet); }, { capture: true });
      return;
    }
    if (node.dataset.sectionModule === "match-details") {
      moduleStartedBeforeCSS = !sheetReady;
      node.src = "./match-details.js";
    }
    append(node);
  };
  const renderer = makeRenderer({ rerenderMatch: () => {
    host.innerHTML = renderer.renderMatchDetail(ordinary, ordinary.participants[0], tab);
    if (!host.querySelector(".match-table")) return;
    richBeforeCSS = !sheetReady;
    resolveRich();
  } });
  host.innerHTML = renderer.renderMatchDetail(ordinary, ordinary.participants[0], tab);
  const style = () => {
    const computed = getComputedStyle(host.querySelector(".match-detail"));
    return [computed.paddingRight, computed.paddingBottom, computed.borderTopWidth, computed.borderTopStyle, computed.borderTopColor];
  };
  return new Promise((resolve) => setTimeout(resolve, 100)).then(async () => {
    const old = document.createElement("style");
    old.textContent = input.oldGameplay;
    document.head.append(old);
    const priorSkeleton = style();
    old.remove();
    const before = {
      sheetPending: Boolean(held && !held.isConnected && !sheetReady),
      skeleton: Boolean(host.querySelector('[role="status"]')),
      earlyModule: Boolean(document.querySelector('[data-section-module="match-details"]')),
      earlyMarkup: Boolean(host.querySelector(".match-detail-head, .team-overview, .match-table")),
      skeletonStyle: style(),
    };
    append(held);
    await rich;
    return { before, priorSkeleton, moduleStartedBeforeCSS, richBeforeCSS, sheetReady,
      tableLayout: getComputedStyle(host.querySelector(".match-table")).tableLayout,
      skeletonReplaced: !host.querySelector('[role="status"]'), finalStyle: style() };
  });
}

test("R320 Chromium preserves computed match-detail styles and verifies the migration inventory", () => {
  const candidate = detail();
  const playerTags = read("player-tags.css");
  const result = withBrowser({
    "fixture.html": fixture + '<script src="runtime.js"></script><script src="match-details.js"></script><script src="compare.js"></script>',
    "runtime.js": read("runtime.js"),
    "match-details.js": 'window.deepLegendsSections = { register() {}, load: () => Promise.resolve() };\n' + read("match-details.js"),
    "renderer-fixture.js": `const rendererFunctions = ${JSON.stringify(rendererFunctions)};\n${rendererFixture}`,
    "compare.js": pageScript(compare, {
      oldGameplay, oldDetail, gameplay, detail: candidate,
      eagerSelectors: [".match-detail"],
      surfaces: [
        { surface: "overview", before: [...oldEager, oldDetail], after: [...eager, candidate] },
        { surface: "live", before: [...oldEager, playerTags, read("shared-cards.css"), oldDetail, read("live-arena.css")],
          after: [...eager, playerTags, read("shared-cards.css"), candidate, read("live-arena.css")] },
      ],
    }),
  }, "fixture.html");
  if (process.env.R320_STYLE_AUDIT_REPORT) fs.writeFileSync(process.env.R320_STYLE_AUDIT_REPORT, JSON.stringify(result, null, 2));
  assert.deepEqual(result.missing, [], "Every migrated selector must match the fixture");
  assert.deepEqual(result.differences.slice(0, 12), [], `${result.differences.length} computed style differences`);
  assert.deepEqual(result.inventory, [], "Every moved rule must retain its selector, conditions, and declarations exactly once");
  assert.equal(result.rules.length, 209, "The migration must account for all 209 original style rules");
  console.log(`Compared ${result.rules.length} rules / ${result.selectorCount} selector branches across 28 surface, theme and width combinations`);
});

test("R320 Chromium styles the loading placeholder and waits for CSS before inserting match details", () => {
  const stylesheet = eager.map((css) => `<style>${css}</style>`).join("");
  const result = withBrowser({
    "fixture.html": fixture.replace('<pre id="result">', stylesheet + '<pre id="result">') +
      '<script src="section-loader.js"></script><script src="gate.js"></script>',
    "renderer-fixture.js": `const rendererFunctions = ${JSON.stringify(rendererFunctions)};\n${rendererFixture}`,
    "section-loader.js": read("section-loader.js"),
    "match-details.js": read("match-details.js"),
    "match-detail.css": detail(),
    "gate.js": pageScript(stylesheetGate, { oldGameplay }),
  }, "fixture.html");
  assert.deepEqual(result.before, { sheetPending: true, skeleton: true, earlyModule: false, earlyMarkup: false, skeletonStyle: result.priorSkeleton });
  assert.equal(result.sheetReady, true);
  assert.equal(result.moduleStartedBeforeCSS, false);
  assert.equal(result.richBeforeCSS, false);
  assert.equal(result.skeletonReplaced, true);
  assert.equal(result.tableLayout, "fixed");
  assert.deepEqual(result.finalStyle, result.priorSkeleton);
});
