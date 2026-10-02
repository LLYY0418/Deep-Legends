'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const {JSDOM} = require('../../desktop/node_modules/jsdom');
const {read, extract, compile, fixture} = require('./r188-harness.cjs');
const source = read('gameplay.js', process.env.R188_SOURCE_DIR);
const appSource = read('app.js', process.env.R188_SOURCE_DIR);
function h(options = {}) { return fixture({...options, gameplaySource: source}); }
function dom() {
  const d = new JSDOM('<body><main id="live-content"></main></body>', {url: 'http://localhost', pretendToBeVisual: true, runScripts: 'outside-only'});
  d.window.matchMedia = () => ({matches: false});
  d.window.eval(extract(appSource, 'setupFloatingTooltips') + '\nsetupFloatingTooltips();');
  return d;
}
function tooltip(d, f, players) {
  const main = d.window.document.querySelector('main');
  main.innerHTML = f.renderLivePremadeTag(players[0], players, 69);
  main.querySelector('[data-tooltip]').dispatchEvent(new d.window.Event('pointerover', {bubbles: true}));
  return d.window.document.querySelector('#global-tooltip');
}
function roster(n, source = 'session') {
  return Array.from({length: n}, (_, i) => ({premadeGroup: '1', premadeSize: n, premadeSource: source, gameName: `测试成员${i + 1}`, tagLine: '188', championId: i ? 0 : 103, championName: i ? '未知英雄' : '阿狸', profileIconId: 42}));
}
test('R188 direct premade tooltip uses two plain text rows and no images or separator', () => {
  const f = h(), d = dom();
  try {
    const t = tooltip(d, f, roster(2));
    assert.equal(t.querySelector('.tooltip-title').textContent, '组队 2 人');
    assert.equal(t.querySelectorAll('.tooltip-roster-player').length, 2);
    assert.equal(t.querySelectorAll('img').length, 0);
    assert.doesNotMatch(t.textContent, /·|客户端直接|最近战绩/);
    assert.equal(t.querySelector('.tooltip-roster-name').textContent, '测试成员1#188');
    assert.equal(t.querySelector('.tooltip-roster-champion').textContent, '阿狸');
    const rows = JSON.parse(d.window.document.querySelector('[data-tooltip-roster]').dataset.tooltipRoster);
    assert.deepEqual(Object.keys(rows[0]).sort(), ['champion', 'name']);
  } finally { d.window.close(); }
});
test('R188 inferred title is a count with no methodology body, both/lobby stay direct', () => {
  for (const origin of ['inferred', 'both', 'lobby']) {
    const d = dom();
    try {
      const t = tooltip(d, h(), roster(3, origin));
      assert.equal(t.querySelector('.tooltip-title').textContent, `${origin === 'inferred' ? '预组队' : '组队'} 3 人`);
      assert.equal(t.querySelector('.tooltip-body'), null);
      assert.doesNotMatch(t.textContent, /最近战绩|共同出现|客户端直接/);
    } finally { d.window.close(); }
  }
});
test('R188 zero and negative champion IDs leave hero column empty even with intent/name', () => {
  for (const id of [0, -1, -3]) {
    const players = roster(2); players[1].championId = id; players[1].championPickIntent = 69;
    const d = dom();
    try {
      const t = tooltip(d, h(), players);
      assert.equal(t.querySelectorAll('.tooltip-roster-champion')[1].textContent, '');
      assert.doesNotMatch(t.textContent, /未知英雄|英雄待确认/);
    } finally { d.window.close(); }
  }
});
test('R188 mask names uses the existing maskedPlayerName policy', () => {
  const f = h(); f.state.settings.maskNames = true;
  const d = dom();
  try { const t = tooltip(d, f, roster(2)); assert.deepEqual([...t.querySelectorAll('.tooltip-roster-name')].map(e => e.textContent), ['玩家 01', '玩家 02']); assert.doesNotMatch(t.textContent, /测试成员|#188/); }
  finally { d.window.close(); }
});
test('R188 matchup is after tabs outside insight and visible on runes/build/insight', () => {
  for (const active of ['runes', 'build', 'insight']) {
    const f = h({active, locked: true}), d = dom();
    try {
      const main = d.window.document.querySelector('main'); main.innerHTML = f.renderRecommendationArea(f.data);
      const tabs = main.querySelector('.recommendation-tabs'), slot = main.querySelector('[data-lane-matchup-slot]');
      assert.equal(tabs.nextElementSibling, slot);
      assert.equal(slot.parentElement.className, 'recommendation-tab-row');
      assert.ok(slot.querySelector('[data-lane-matchup-card]'));
      assert.equal(main.querySelector('#recommendation-panel-insight [data-lane-matchup-card]'), null);
      assert.equal(slot.closest('[hidden]'), null);
      assert.match(slot.textContent, /对位 阿狸偏劣势 47.6%/);
      assert.doesNotMatch(slot.textContent, /对位克制建议|这局对线|胜率约/);
      assert.equal(f.events.at(-1).placement, 'tab-row');
    } finally { d.window.close(); }
  }
});
test('R188 preselection caps four candidates at three, narrow rule hides the third', () => {
  const f = h(), d = dom();
  try {
    d.window.document.querySelector('main').innerHTML = f.renderRecommendationArea(f.data);
    const options = [...d.window.document.querySelectorAll('.lane-matchup-option')];
    assert.equal(options.length, 3);
    assert.deepEqual(options.map(e => e.dataset.tooltip), ['奥莉安娜', '乐芙兰', '维克托']);
    assert.ok(options.every(e => e.getAttribute('aria-label') === e.dataset.tooltip && e.textContent.trim().startsWith('+')));
    const css = read('gameplay.css');
    const narrow = css.slice(css.indexOf('@container recommendation-area (max-width: 700px)'));
    const rule = narrow.match(/\.lane-matchup-option:nth-child\(n\+3\)\s*\{[^}]*\}/)[0];
    const style = d.window.document.createElement('style'); style.textContent = rule; d.window.document.head.append(style);
    assert.equal(options.filter(e => d.window.getComputedStyle(e).display !== 'none').length, 2);
    f.data.players[0].championId = 69; f.data.players[0].championLocked = true;
    assert.doesNotMatch(f.renderLaneMatchupCard(f.data), /lane-matchup-option/);
  } finally { d.window.close(); }
});
test('R188 notice follows matchup slot without entering panels', () => {
  const f = h({notice: '我的小队'}), d = dom();
  try { d.window.document.querySelector('main').innerHTML = f.renderRecommendationArea(f.data); assert.deepEqual([...d.window.document.querySelector('.recommendation-tab-row').children].map(e => e.className || 'slot'), ['recommendation-tabs', 'slot', 'live-roster-notice']); }
  finally { d.window.close(); }
});
test('R188 pending to success and hidden preserve tab buttons, focused tab, and fixed slot', () => {
  const f = h({pending: true}), d = dom();
  try {
    const document = d.window.document, main = document.querySelector('main');
    main.innerHTML = `<div data-live-body>${f.renderRecommendationArea(f.data)}</div>`;
    const tabs = [...main.querySelectorAll('[role=tab]')], slot = main.querySelector('[data-lane-matchup-slot]');
    tabs[0].focus();
    const patch = compile(source, ['liveBodyChrome', 'captureLiveScroll', 'restoreLiveScroll', 'stampLiveRows', 'preserveLiveImages', 'patchLiveRosterPanel', 'updateLivePanels'], {document, nodes: {liveContent: main}, bindLivePanelScope() {}, applyRenderedMetricStyles() {}, prepareImages() {}, recordLiveRenderRebuild() {}}).updateLivePanels;
    assert.equal(slot.children.length, 0);
    f.pair.status = 'succeeded'; f.pair.data = {winRate: 47.6};
    f.state.laneMatchupCandidates.forEach(entry => { entry.rows = f.candidates.rows; });
    assert.equal(patch(f.renderRecommendationArea(f.data)), true);
    tabs.forEach((tab, i) => assert.equal(main.querySelectorAll('[role=tab]')[i], tab));
    assert.ok(slot.querySelector('[data-lane-matchup-card]'));
    assert.equal(document.activeElement, tabs[0]);
    assert.equal(main.querySelector('[data-lane-matchup-slot]'), slot);
    f.data.phase = 'InProgress'; assert.equal(patch(f.renderRecommendationArea(f.data)), true);
    assert.equal(slot.children.length, 0);
    assert.equal(main.querySelector('[role=tab]'), tabs[0]);
  } finally { d.window.close(); }
});
test('R188 InProgress and unlocked enemy hide matchup', () => {
  const f = h(); f.data.phase = 'InProgress'; assert.doesNotMatch(f.renderRecommendationArea(f.data), /data-lane-matchup-card/);
  f.data.phase = 'ChampSelect'; f.data.players[1].championLocked = false; assert.equal(f.renderLaneMatchupCard(f.data), '');
});
test('R188 placement passes both actual diagnostic senders', async () => {
  const sent = [], fields = {mode: 'a', placement: 'tab-row', shown: true, ownLocked: true, enemyChampionId: 103};
  compile(source, ['recordItemSetClientDiagnostic'], {fetch: async (_, options) => sent.push(JSON.parse(options.body))}).recordItemSetClientDiagnostic('lane_matchup_card', 'render', fields);
  const d = dom();
  try {
    vm.runInNewContext(read('runtime.js'), {window: d.window, document: d.window.document, fetch: async (_, options) => {sent.push(JSON.parse(options.body)); return {ok: true, status: 204};}, setTimeout: () => 1, clearTimeout() {}, setInterval: () => 1, clearInterval() {}, Date, URL, URLSearchParams, AbortController, console});
    d.window.reportFlowDiagnostic('lane_matchup_card', 'render', fields);
    await new Promise(setImmediate);
    assert.equal(sent.length, 2); assert.ok(sent.every(body => body.placement === 'tab-row' && body.mode === 'a' && body.shown === true));
  } finally { d.window.close(); }
});
