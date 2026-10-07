'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const {JSDOM} = require('../../desktop/node_modules/jsdom');
const {compile, escapeHTML} = require('./r188-harness.cjs');
const source = fs.readFileSync(process.env.R193_SOURCE_FILE || `${__dirname}/gameplay.js`, 'utf8');
const before = require('../testdata/r193-identity-cards-before.json');
function renderer(code) {
  return compile(code, ['renderLivePlayer', 'renderInsightMatches', 'liveHistoryStateOf', 'liveHistorySettled', 'champSelectEnemyPlaceholder', 'positionLabel', 'maskedPlayerName', 'playerLabel'], {
    state: {settings: {maskNames: false}}, escapeHTML,
    liveDisplayedChampionId: p => p.championId, iconFigure: () => '<img alt="李青">',
    rankTitle: () => '黄金 I', renderLivePremadeTag: () => '', proBadgeAttributes: () => '', renderProIdentityBadge: () => '',
    number: String, percent: n => `${n}%`, kda: String,
  });
}
const current = renderer(source);
const hidden = {hidden: true, position: 'jungle', championId: 64, championName: '李青', historyState: 'unavailable', modeStats: {}, recentGames: []};
const combined = (r, p) => r.renderLivePlayer(p, 0) + r.renderInsightMatches(p);
function dom(html) { const d = new JSDOM(html); return d; }

test('R193 1 hidden identity subtitle contains only position and disappears without position', () => {
  const d = dom(current.renderLivePlayer(hidden, 0));
  try {
    assert.equal(d.window.document.querySelector('.live-player-copy > span').textContent, '打野');
    assert.doesNotMatch(d.window.document.body.textContent, /未定级/);
    for (const position of ['', null, undefined]) {
      const empty = dom(current.renderLivePlayer({...hidden, position}, 0));
      try { assert.equal(empty.window.document.querySelector('.live-player-copy > span'), null); }
      finally { empty.window.close(); }
    }
    assert.doesNotMatch(current.renderLivePlayer({...hidden, rank: {tier: 'gold'}}, 0), /黄金/);
  } finally { d.window.close(); }
});
test('R193 2 hidden combined card contains one client-unavailable message and no history row', () => {
  const html = combined(current, hidden);
  assert.equal(html.split('客户端未公开该玩家').length - 1, 1);
  assert.doesNotMatch(html, /insight-match-row/);
  assert.equal(current.renderInsightMatches({...hidden, historyState: 'pending'}), '');
});
test('R193 3 hidden card omits empty win-rate and KDA columns without em dashes', () => {
  const html = combined(current, hidden), d = dom(html);
  try {
    assert.doesNotMatch(html, /—/);
    assert.equal(d.window.document.querySelectorAll('dl > div').length, 1);
    assert.equal(d.window.document.querySelector('dt').textContent, '当前模式');
    assert.equal(d.window.document.querySelector('.win-rate-value, .live-kda-value'), null);
    const withStats = current.renderLivePlayer({...hidden, recentGames: [{win: true, kills: 2, deaths: 1, assists: 1}, {win: false, kills: 2, deaths: 1, assists: 1}], modeStats: {games: 2, wins: 1, losses: 1, winRate: 50, kda: 3}}, 0);
    assert.match(withStats, /win-rate-value/); assert.match(withStats, /live-kda-value/);
  } finally { d.window.close(); }
});
test('R193 4 unresolved identity, private history and ordinary no-sample cards match baseline', () => {
  for (const {player, html} of before.cards) assert.equal(combined(current, player), html.replace('暂无样本', '本模式暂无战绩'));
  assert.match(combined(current, {...hidden, hidden: false, identityUnresolved: true}), /身份尚未公开/);
});
test('R193 5 ordinary unranked player retains rank subtitle', () => {
  const html = current.renderLivePlayer({...hidden, hidden: false, gameName: '普通玩家'}, 0);
  assert.match(html, /打野 · 未定级/);
  const normal = before.cards[2];
  assert.equal(combined(current, normal.player), normal.html.replace('暂无样本', '本模式暂无战绩'));
});
