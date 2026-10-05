'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { JSDOM } = require('../../desktop/node_modules/jsdom');
const source = fs.readFileSync(path.join(__dirname, 'gameplay.js'), 'utf8');
function extract(name) {
  const match = source.match(new RegExp('function ' + name + '\\('));
  assert.ok(match, name);
  const start = match.index, body = source.indexOf('{', source.indexOf(')', start));
  let depth = 0, quote = '', escaped = false;
  for (let i = body; i < source.length; i++) {
    const c = source[i];
    if (quote) { if (escaped) escaped = false; else if (c === '\\') escaped = true; else if (c === quote) quote = ''; continue; }
    if (c === '"' || c === "'" || c === '`') { quote = c; continue; }
    if (c === '{') depth++;
    if (c === '}' && --depth === 0) return source.slice(start, i + 1);
  }
  assert.fail(name + ' unbalanced');
}
test('R183 ranked backend response renders an anonymous enemy in middle and stops incomplete retries', () => {
  const response = JSON.parse(fs.readFileSync(path.join(__dirname, '../testdata/r183-ranked-anonymous-live.json'), 'utf8'));
  const names = ['playerLabel', 'maskedPlayerName', 'liveHistoryStateOf', 'liveHistorySettled', 'renderLivePlayer', 'champSelectEnemyPlaceholder', 'livePremadeRoster', 'clusterPremadePlayers', 'orderLivePlayers', 'insightTeamLayout', 'isSummonersRiftMatch', 'renderLiveInsights', 'liveClientPositionsPending', 'liveSnapshotComplete', 'liveAutoRefreshStopped', 'renderLiveRefreshStatus'];
  const deps = {
    state: { settings: { liveOrder: 'position', maskNames: false }, liveRetryAttempts: 8, beacon: { phase: 'InProgress' } },
    SUMMONERS_RIFT_QUEUE_IDS: [420, 440], liveDisplayedChampionId: (p) => p.championId,
    renderLivePremadeTag: () => '', renderProIdentityBadge: () => '', proBadgeAttributes: () => '',
    iconFigure: (_, id, name) => `<img data-champion-id="${id}" alt="${name}" src="/api/champion-icon?id=${id}">`,
    escapeHTML: String, positionLabel: (p) => ({ top: '上路', jungle: '打野', middle: '中路', bottom: '下路', utility: '辅助' }[p] || ''),
    rankTitle: () => '', number: String, percent: String, kda: String,
    liveAugmentRecommendationSource: () => 'classic', recordLiveRosterRendered: () => {},
    renderLiveRosterNoticeBars: () => '', renderLiveTeamPortraitTags: () => '', renderInsightMatches: () => '', renderLiveRecentPositions: () => '',
    isARAMRelatedMatch: () => false, arenaLivePlayerGroups: () => [], liveGamePhase: () => true,
  };
  const functions = Function(...Object.keys(deps), require('./r220-harness-support.cjs').prelude(source,deps)+names.map(extract).join('\n') + ';return {renderLiveInsights, liveSnapshotComplete, renderLiveRefreshStatus};')(...Object.values(deps));
  for (const queueId of [440, 420]) {
    const data = { ...response, queueId };
    assert.equal(functions.liveSnapshotComplete(data), true);
    const dom = new JSDOM(functions.renderLiveRefreshStatus(data) + functions.renderLiveInsights(data));
    try {
      const doc = dom.window.document;
      assert.equal(doc.querySelector('.live-team.is-blue h3').textContent, '我方');
      assert.equal(doc.querySelectorAll('.live-team.is-blue article').length, 5);
      const cards = [...doc.querySelectorAll('.live-team.is-red article')];
      assert.equal(cards.length, 5);
      const hidden = cards[2];
      assert.equal(hidden.querySelector('.live-player-name').textContent, '隐藏玩家');
      assert.equal(hidden.querySelector('.live-player-name').disabled, true);
      assert.equal(hidden.querySelector('[data-player-ref]'), null);
      assert.match(hidden.textContent, /中路/);
      assert.equal(hidden.querySelector('img').getAttribute('data-champion-id'), '1');
      assert.match(hidden.querySelector('img').getAttribute('src'), /id=1/);
      assert.doesNotMatch(doc.querySelector('.live-refresh-status').textContent, /数据尚未完整/);
    } finally { dom.window.close(); }
  }
  const incomplete = { ...response, players: response.players.filter((p) => !p.hidden) };
  assert.equal(functions.liveSnapshotComplete(incomplete), false);
  assert.match(functions.renderLiveRefreshStatus(incomplete), /数据尚未完整/);
});
