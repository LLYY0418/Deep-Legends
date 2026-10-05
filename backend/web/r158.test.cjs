'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { JSDOM } = require('../../desktop/node_modules/jsdom');
const flush = () => new Promise(setImmediate);

const source = fs.readFileSync(path.join(__dirname, 'gameplay.js'), 'utf8');
function extract(name) {
  const start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, name);
  let depth = 0;
  for (let i = source.indexOf('{', start); i < source.length; i++) {
    if (source[i] === '{') depth++;
    if (source[i] === '}' && --depth === 0) return source.slice(start, i + 1);
  }
  assert.fail(`unbalanced ${name}`);
}
function compile(names, deps) {
  names = require("./r211-harness-support.cjs").expand(source, names, deps);
  return Function(...Object.keys(deps), names.map(extract).join('\n') + `\nreturn {${names.join(',')}}`)(...Object.values(deps));
}

function overviewFixture(realQueue = false) {
  const dom = new JSDOM('<main></main>', { url: 'http://localhost', runScripts: 'outside-only' });
  const container = dom.window.document.querySelector('main');
  const imageWrites = [];
  if (realQueue) {
    const descriptor = Object.getOwnPropertyDescriptor(dom.window.HTMLImageElement.prototype, 'src');
    Object.defineProperty(dom.window.HTMLImageElement.prototype, 'src', {
      ...descriptor, set(value) { imageWrites.push([this, value]); descriptor.set.call(this, value); },
    });
    dom.window.eval(fs.readFileSync(path.join(__dirname, 'image-queue.js'), 'utf8'));
  }
  const tab = (key, icon = key) => ({
    key, region: 'kr', openMatches: new Set(), matchViewRevision: 0,
    data: {
      player: { playerRef: key, gameName: key, profileIconId: icon, backgroundSource: 'ddragon', backgroundPath: `${key}.jpg` },
      matches: [{ gameId: 1, icon: `${key}-match` }], ranks: [{ icon: `${key}-rank` }],
      pagination: { hasMore: false }, capabilities: [],
    },
  });
  const a = tab('a'), b = tab('b');
  const state = { tabs: [a, b], overlay: [], settings: { maskNames: false } };
  const admissions = [];
  const prepareImages = root => {
    if (realQueue) return;
    for (const img of root.querySelectorAll('img[data-queued-src]')) {
      if (img.dataset.imageReady) continue;
      admissions.push(img.dataset.queuedSrc);
      img.src = img.dataset.queuedSrc;
      img.dataset.imageReady = 'true';
    }
  };
  const renderMatch = match => `<article class="match-entry" data-match-id="${match.gameId}"><img data-queued-src="/${match.icon}"></article>`;
  const ui = compile(['activateOverviewView', 'renderOverviewBodyContent', 'renderOverviewBody', 'reconcileFilteredMatchList'], {
    state, document: dom.window.document, window: {}, console,
    matchTierScope: current => current.data.player.playerRef,
    filteredMatches: rows => rows, matchListEmptyContent: () => '', matchSentinelShouldHide: () => false,
    riotTab: () => false, maskedProfileIcon: () => '',
    iconFigure: (_kind, icon) => `<img class="summoner-avatar" data-queued-src="/avatar-${icon}">`,
    playerLabel: player => player.gameName, summonerProChip: () => '', summonerRegionChip: () => '',
    renderSummonerHighlights: () => '', paginationCopyFor: () => '',
    renderCareerSections: data => `<section class="career-card"><img data-queued-src="/${data.ranks[0].icon}"></section>`,
    renderMatchFilters: () => '', renderMatch, escapeHTML: String, number: String,
    bindOverviewContent: () => {}, applyRenderedMetricStyles: () => {}, prepareImages,
    updateFriendPresenceChips: () => {}, scheduleOverviewCurrentGame: () => {},
    ensurePerks: () => {}, ensureItems: () => {}, ensureSummonerSpells: () => {}, observeMatchTierVisibility: () => {},
    emptyState: (_title, error) => `<p>${error}</p>`, loadOverview: () => {},
  });
  const images = () => ({
    avatar: container.querySelector('.summoner-avatar'),
    art: container.querySelector('.summoner-strip-art'),
    career: container.querySelector('.career-column img'),
    match: container.querySelector('.match-list img'),
  });
  return { dom, container, state, a, b, ui, admissions, imageWrites, images };
}

test('R158 same player and A-B-A overview renders keep loaded image nodes', () => {
  const fixture = overviewFixture();
  const { dom, a, b, ui, admissions, images, container } = fixture;
  try {
    ui.renderOverviewBody(container, a);
    const first = images(), loaded = admissions.length;
    assert.equal(loaded, 4);
    ui.renderOverviewBody(container, a);
    for (const kind of Object.keys(first)) assert.equal(images()[kind], first[kind], `same player ${kind}`);
    assert.equal(admissions.length, loaded);
    ui.renderOverviewBody(container, b);
    ui.renderOverviewBody(container, a);
    for (const kind of Object.keys(first)) assert.equal(images()[kind], first[kind], `returned player ${kind}`);
    assert.equal(admissions.length, loaded + 4, 'returning to A must not enter the image queue again');
    assert.equal(container.dataset.matchTierScope, 'a');
  } finally { dom.window.close(); }
});

test('R158 real image queue does not admit loaded A images after B-A switch', async () => {
  const fixture = overviewFixture(true);
  const { dom, a, b, ui, container, imageWrites, images } = fixture;
  try {
    ui.renderOverviewBody(container, a);
    await flush();
    const old = images();
    assert.equal(imageWrites.length, 4);
    for (const img of Object.values(old)) img.dispatchEvent(new dom.window.Event('load'));
    ui.renderOverviewBody(container, b);
    await flush();
    assert.equal(imageWrites.length, 8);
    for (const img of Object.values(images())) img.dispatchEvent(new dom.window.Event('load'));
    ui.renderOverviewBody(container, a);
    await flush();
    for (const kind of Object.keys(old)) assert.equal(images()[kind], old[kind]);
    assert.equal(imageWrites.length, 8, 'no new src assignment or image admission on return');
  } finally {
    dom.window.dispatchEvent(new dom.window.Event('deep-legends:dispose'));
    dom.window.close();
  }
});

test('R158 changed player data replaces stale art, career and match images', () => {
  const fixture = overviewFixture();
  const { dom, a, b, ui, admissions, images, container } = fixture;
  try {
    ui.renderOverviewBody(container, a);
    const old = images();
    ui.renderOverviewBody(container, b);
    a.data = {
      ...a.data,
      player: { ...a.data.player, gameName: 'Updated', profileIconId: 'new', backgroundPath: 'new.jpg' },
      ranks: [{ icon: 'new-rank' }], matches: [{ gameId: 1, icon: 'new-match' }],
    };
    ui.renderOverviewBody(container, a);
    for (const kind of Object.keys(old)) assert.notEqual(images()[kind], old[kind], `changed ${kind}`);
    assert.match(container.textContent, /Updated/);
    assert.ok(admissions.includes('/new-match') && admissions.includes('/new-rank'));
  } finally { dom.window.close(); }
});

test('R158 inactive overview DOM cache has a four-player limit', () => {
  const fixture = overviewFixture();
  const { dom, a, state, ui, images, container } = fixture;
  try {
    ui.renderOverviewBody(container, a);
    const oldAvatar = images().avatar;
    for (let index = 0; index < 5; index++) {
      const next = { ...a, key: `extra-${index}`, data: {
        ...a.data, player: { ...a.data.player, playerRef: `extra-${index}`, profileIconId: `extra-${index}` },
      } };
      state.tabs.push(next);
      ui.renderOverviewBody(container, next);
    }
    assert.equal(container._overviewViews.size, 4);
    ui.renderOverviewBody(container, a);
    assert.notEqual(images().avatar, oldAvatar, 'evicted player is rendered again');
  } finally { dom.window.close(); }
});

test('R158 closing an inactive player releases its retained DOM', () => {
  const fixture = overviewFixture();
  const { dom, a, b, state, ui, container } = fixture;
  try {
    ui.renderOverviewBody(container, a);
    ui.renderOverviewBody(container, b);
    assert.equal(container._overviewViews.has(a), true);
    a.closed = true;
    state.tabs.splice(state.tabs.indexOf(a), 1);
    ui.renderOverviewBody(container, b);
    assert.equal(container._overviewViews.has(a), false);
  } finally { dom.window.close(); }
});
