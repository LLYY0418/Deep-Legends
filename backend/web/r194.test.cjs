'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const {JSDOM} = require('../../desktop/node_modules/jsdom');
const {compile, escapeHTML} = require('./r188-harness.cjs');
const source = fs.readFileSync(`${__dirname}/gameplay.js`, 'utf8');
const fixture = require('../testdata/r194-quickplay-anonymous-live.json');
function harness(data) {
  const timers = [], calls = [];
  const state = {section: 'live', live: data, settings: {liveOrder: 'position', maskNames: false, liveRefresh: true, liveInterval: 3}, beacon: {phase: 'InProgress'}, liveRetryAttempts: 0};
  const names = ['playerLabel', 'maskedPlayerName', 'liveHistoryStateOf', 'liveHistorySettled', 'renderLivePlayer', 'renderInsightMatches', 'champSelectEnemyPlaceholder', 'livePremadeRoster', 'clusterPremadePlayers', 'orderLivePlayers', 'insightTeamLayout', 'isSummonersRiftMatch', 'renderLiveInsights', 'liveClientPositionsPending', 'liveSnapshotComplete', 'liveAutoRefreshStopped', 'renderLiveRefreshStatus', 'scheduleLiveRefresh', 'syncLiveRetryBudget'];
  const functions = compile(source, names, {
    state, document: {hidden: false}, escapeHTML, SUMMONERS_RIFT_QUEUE_IDS: [420, 440],
    liveDisplayedChampionId: p => p.championId, renderLivePremadeTag: () => '', renderProIdentityBadge: () => '', proBadgeAttributes: () => '',
    iconFigure: (_, id, name) => `<img data-champion-id="${id}" alt="${escapeHTML(name)}">`,
    positionLabel: p => ({top: '上路', jungle: '打野', middle: '中路', bottom: '下路', utility: '辅助'}[p] || ''),
    rankTitle: () => '', number: String, percent: String, kda: String, insightScore: () => '',
    liveAugmentRecommendationSource: () => '', recordLiveRosterRendered: () => {}, renderLiveRosterNoticeBars: () => '', renderLiveTeamPortraitTags: () => '', renderLiveRecentPositions: () => '',
    isARAMRelatedMatch: () => false, arenaLivePlayerGroups: () => [], liveGamePhase: () => true,
    clearTimeout: () => {}, connected: () => true, liveRefreshDelayMs: () => 3000,
    setTimeout: (fn, delay) => {timers.push({fn, delay}); return timers.length;}, loadLive: (...args) => calls.push(args),
  });
  return {state, timers, calls, ...functions};
}

test('R194 actual backend quickplay response renders our anonymous TOP among five cards', () => {
  assert.equal(fixture.queueId, 480);
  const h = harness(fixture), d = new JSDOM(h.renderLiveRefreshStatus(fixture) + h.renderLiveInsights(fixture));
  try {
    const doc = d.window.document, own = doc.querySelector('.live-team.is-blue');
    assert.equal(own.querySelector('h3').textContent, '我方');
    const cards = [...own.querySelectorAll('.live-player')];
    assert.equal(cards.length, 5);
    assert.equal(doc.querySelectorAll('.live-team.is-red .live-player').length, 5);
    const top = cards[0];
    assert.equal(top.querySelector('.live-player-name').textContent, '隐藏玩家');
    assert.equal(top.querySelector('.live-player-name').disabled, true);
    assert.equal(top.querySelector('[data-player-ref]'), null);
    assert.equal(top.querySelector('.live-player-copy > span').textContent, '上路');
    assert.equal(top.querySelector('img').getAttribute('data-champion-id'), '1');
    assert.doesNotMatch(top.textContent, /未定级|—/);
    assert.equal(top.textContent.split('客户端未公开该玩家').length - 1, 1);
    assert.equal(top.nextElementSibling.tagName, 'ARTICLE');
    assert.doesNotMatch(doc.querySelector('.live-refresh-status').textContent, /数据尚未完整/);
    assert.equal(h.liveSnapshotComplete(fixture), true);
  } finally { d.window.close(); }
});

test('R194 quickplay nine-player snapshots keep five-second retries until recovered', () => {
  const missing = {...fixture, players: fixture.players.filter(p => !p.hidden)}, h = harness(missing);
  assert.equal(h.liveSnapshotComplete(missing), false);
  h.scheduleLiveRefresh();
  assert.equal(h.timers.length, 1);
  assert.equal(h.timers[0].delay, 5000);
  h.timers[0].fn();
  assert.deepEqual(h.calls, [[false, 'interval']]);
  h.state.liveRetryAttempts = 8;
  assert.match(h.renderLiveRefreshStatus(missing), /数据尚未完整/);
  h.state.live = fixture; h.state.liveRetryAttempts = 0;
  h.scheduleLiveRefresh();
  assert.equal(h.timers.length, 1, 'complete roster must not add another timer');
});

test('R194 browser roster gate mirrors backend registered 5v5 PvP queue groups', () => {
  const groups = fs.readFileSync(`${__dirname}/../queue_groups.go`, 'utf8');
  const allowed = new Set(['solo', 'flex', 'match', 'aram', 'hextech-aram', 'clash', 'urf']);
  const h = harness(fixture), nine = fixture.players.filter(p => !p.hidden);
  for (const match of groups.matchAll(/\{(\d+), "[^"]+", "([^"]+)"/g)) {
    const queueId = Number(match[1]), expectedComplete = !allowed.has(match[2]);
    assert.equal(h.liveSnapshotComplete({...fixture, queueId, players: nine}), expectedComplete, `queue ${queueId}: ${match[2]}`);
  }
  assert.equal(h.liveSnapshotComplete({...fixture, queueId: 999999, players: nine}), true);
});
