'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { JSDOM } = require('../../desktop/node_modules/jsdom');
const read = (name) => fs.readFileSync(path.join(__dirname, name), 'utf8');

test('R318 P1 shared card styles are loaded before first live recommendation render', () => {
  const loader = read('section-loader.js');
  assert.match(loader, /champions: \["patch-notes", "shared-cards", "champions"\]/);
  assert.match(loader, /"live-arena": \["shared-cards", "live-arena"\]/);
  const shared = read('shared-cards.css');
  for (const selector of ['.arena-option-card', '.arena-option-main', '.arena-option-icons', '.mayhem-ranking-list', '.position-icon']) {
    assert.ok(shared.includes(selector), `${selector} missing from shared-cards.css`);
  }
  assert.doesNotMatch(read('champions.css'), /^\.arena-option-card \{/m);
  const gameplay = read('gameplay.js');
  assert.match(gameplay, /loadStyle\(name\)/);
  assert.match(gameplay, /state\.liveCardStylesReady/);
  assert.match(gameplay, /function renderLive\(source = "direct"\)[\s\S]{0,1600}正在准备对局页/);
});

test('R318 P1 cold-start gate never renders an unstyled recommendation frame', async () => {
  const dom = new JSDOM('<!doctype html><html><head></head><body></body></html>', { url: 'http://localhost', runScripts: 'outside-only', pretendToBeVisual: true });
  const { window } = dom;
  try {
    window.eval(read('section-loader.js'));
    const promise = window.deepLegendsSections.loadStyle('shared-cards');
    const link = window.document.querySelector('link[data-section-style="shared-cards"]');
    assert.ok(link, 'shared-cards link created');
    assert.equal(link.getAttribute('href'), '/shared-cards.css');
    link.dispatchEvent(new window.Event('load'));
    await promise;
    assert.equal(window.document.querySelectorAll('link[data-section-style="shared-cards"]').length, 1);
  } finally {
    window.close();
  }
});

test('R318 P2 arena without skills puts three boots on one row', () => {
  const gameplay = read('gameplay.js');
  assert.match(gameplay, /const hasSkillPlan = Boolean\(build\.skillPriority\?\.length \|\| build\.skillOrder\?\.length\)/);
  assert.match(gameplay, /"is-arena-no-skill"/);
  const css = read('gameplay.css') + "\n" + read('live-arena.css');
  assert.match(css, /\.build-summary-bar\.is-arena-no-skill > \.build-summary-boots \.config-option-list \{ grid-template-columns: repeat\(3,minmax\(0,1fr\)\)/);
  assert.match(css, /@container build-recommendation \(max-width: 560px\)[\s\S]{0,160}grid-template-columns: minmax\(0,1fr\)/);
});

test('R318 P3 rank label is muted, rank text keeps the theme color, and fame is self-only', () => {
  global.deepLegendsMatchCards = { live: { rankTitle: (rank) => `${rank.division} ${rank.tier}` } };
  const api = require('./live-arena.js');
  const base = { hidden: false, isCurrent: false, soloRank: { tier: 'PLATINUM', division: 'I' }, flexRank: { tier: 'GOLD', division: 'II' } };
  const stranger = api.identityLine({ ...base, arenaFame: { level: 12, fame: 97312 } }, 'arena', '');
  assert.match(stranger, /class="live-rank-label">单双 </);
  assert.match(stranger, /class="live-rank-label">组排 </);
  assert.match(stranger, /class="live-rank-title"/);
  assert.doesNotMatch(stranger, /live-arena-fame/);
  const self = api.identityLine({ ...base, isCurrent: true, arenaFame: { level: 12, fame: 97312 } }, 'arena', '');
  assert.match(self, /live-arena-fame/);
  assert.match(self, /12级/);
  assert.match(self, /97,312/);
  const noData = api.identityLine({ ...base, isCurrent: true }, 'arena', '');
  assert.doesNotMatch(noData, /live-arena-fame/);
  const css = read('gameplay.css') + "\n" + read('live-arena.css');
  assert.match(css, /\.live-rank-label\{color:var\(--muted\)/);
  assert.match(css, /\.live-arena-fame b\{color:var\(--primary-strong\)/);
});

test('R318 P4 arena detail header is one cell and columns cannot overlap', () => {
  const details = read('match-details.js');
  assert.match(details, /class="arena-column-player">玩家 \/ 装备</);
  assert.doesNotMatch(details, /class="arena-column-items">装备</);
  assert.match(details, /slice\(0, 7\)/);
  const css = read('gameplay.css') + "\n" + read('live-arena.css');
  assert.match(css, /\.arena-detail-columns \{ display: grid; grid-template-columns: minmax\(208px,234px\) var\(--arena-augment-width,104px\) minmax\(54px,1fr\) minmax\(54px,1fr\) minmax\(96px,1\.5fr\)/);
  assert.match(css, /\.arena-detail-augments \{[^}]*grid-auto-columns:26px/);
  for (const [count, width] of [[1, 26], [2, 55], [3, 84], [4, 113], [5, 142], [6, 171]]) {
    assert.match(css, new RegExp(`data-augment-count="${count}"\\] \\{ --arena-augment-width:${width}px`));
  }
});

test('R318 P5 MMR popover aligns its left edge and shifts inside the viewport', async () => {
  const dom = new JSDOM('<div id="host"></div>', { url: 'http://localhost', runScripts: 'outside-only', pretendToBeVisual: true });
  const { window } = dom;
  try {
    window.eval(`${read('app.js').slice(read('app.js').indexOf('function createFloatingLayerLifecycle'), read('app.js').indexOf('window.deepLegendsFloatingLayers='))};window.layers=createFloatingLayerLifecycle();`);
    const document = window.document;
    document.body.innerHTML = '<div class="rank-mmr-control"><button class="rank-mmr-trigger">i</button><div class="rank-mmr-popover">x</div></div>';
    const button = document.querySelector('.rank-mmr-trigger');
    const panel = document.querySelector('.rank-mmr-popover');
    const rect = (left, top, width, height) => ({ left, top, right: left + width, bottom: top + height, width, height, x: left, y: top });
    button.getBoundingClientRect = () => rect(100, 100, 40, 20);
    panel.getBoundingClientRect = () => rect(0, 0, 320, 200);
    Object.defineProperty(window, 'innerWidth', { value: 1280, configurable: true });
    Object.defineProperty(window, 'innerHeight', { value: 800, configurable: true });
    window.eval('window.layers.closeAll()');
    button.dispatchEvent(new window.MouseEvent('pointerover', { bubbles: true }));
    assert.equal(panel.style.left, '100px', 'left edge aligned with trigger');
    window.layers.closeAll();
    button.getBoundingClientRect = () => rect(1100, 100, 40, 20);
    button.dispatchEvent(new window.MouseEvent('pointerover', { bubbles: true }));
    assert.equal(panel.style.left, '952px', 'shifted left to stay 8px from the right edge');
  } finally {
    window.close();
  }
});

test('R318 P6 starter filter is the classic set again without limiting placement', () => {
  const config = require('./auto-config.js');
  const item = (id, price, categories = []) => ({ id, priceTotal: price, inStore: true, displayInItemSets: true, maps: { 11: {}, 12: {} }, categories });
  assert.equal(config.starterFilterAllowed(item(1055, 300), 'ranked'), true);
  assert.equal(config.starterBudget('ranked'), 500);
  assert.equal(config.starterBudget('aram'), 1400);
  assert.equal(config.starterBudget('hextech'), 1400);
  assert.equal(config.starterFilterAllowed(item(3006, 1000), 'ranked'), false);
  assert.equal(config.starterFilterAllowed(item(3006, 1000, ['Boots']), 'aram'), true);
  assert.equal(config.starterFilterAllowed(item(3031, 3000), 'ranked'), false);
  const source = read('auto-config.js');
  assert.match(source, /S\.savedFilters=new Set\(S\.filters\);S\.filters=new Set\(\["Starter"\]\)/);
  assert.match(source, /f==="Starter"\?starterFilterAllowed\(i,S\.mode\)/);
});

test('R318 P7 narrow rune scaling uses width only and is monotonic', () => {
  const { runeFitGeometry } = require('./auto-config.js');
  let previous = 0;
  for (const width of [420, 500, 560, 620, 680, 740, 760, 800, 900, 1100]) {
    const fit = runeFitGeometry(width, 315, false, true);
    assert.ok(fit.scale >= 1 && fit.scale <= 1.3, `${width}: ${fit.scale}`);
    assert.ok(fit.scale + 1e-9 >= previous, `${width}: ${fit.scale} < ${previous}`);
    previous = fit.scale;
  }
  assert.ok(runeFitGeometry(900, 315, false, true).scale > 1);
  assert.equal(runeFitGeometry(420, 315, false, true).scale, 1);
  assert.equal(runeFitGeometry(900, 2000, false, true).f, 1);
  const source = read('auto-config.js');
  assert.match(source, /narrow=width<=820/);
});

test('R318 P8 augment icons use an ordered candidate chain and a text-free placeholder', () => {
  const gameplay = read('gameplay.js');
  assert.match(gameplay, /data-augment-candidates=/);
  assert.match(gameplay, /data-augment-candidate/);
  assert.match(gameplay, /augment-placeholder/);
  assert.match(gameplay, /flushAugmentResolves/);
  assert.match(gameplay, /\/api\/gameplay\/augments\?ids=/);
  const css = read('gameplay.css') + "\n" + read('live-arena.css');
  assert.match(css, /game-icon-fallback\.augment-placeholder/);
  const backend = read('../augment_unified.go');
  for (const source of ['lcu', 'communitydragon', 'opgg', 'bundled']) assert.match(backend, new RegExp(`"${source}"`));
});
