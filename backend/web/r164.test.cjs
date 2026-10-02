'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const source = fs.readFileSync(path.join(__dirname, 'gameplay.js'), 'utf8');
const styles = fs.readFileSync(path.join(__dirname, 'gameplay.css'), 'utf8');

function extract(name) {
  const start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, name);
  const bodyStart = source.indexOf('{', source.indexOf(')', start));
  let depth = 0;
  let quote = '';
  let escaped = false;
  for (let index = bodyStart; index < source.length; index += 1) {
    const char = source[index];
    if (quote) {
      if (escaped) escaped = false;
      else if (char === '\\') escaped = true;
      else if (char === quote) quote = '';
      continue;
    }
    if (char === '"' || char === "'" || char === '`') { quote = char; continue; }
    if (char === '{') depth += 1;
    if (char === '}' && --depth === 0) return source.slice(start, index + 1);
  }
  assert.fail(`${name} is unbalanced`);
}

test('R164 non-Rift teammates stay adjacent by verified premade group while Rift keeps lane order', () => {
  const state = { settings: { liveOrder: 'team' } };
  const names = ['livePremadeRoster', 'clusterPremadePlayers', 'orderLivePlayers', 'insightTeamLayout', 'isSummonersRiftMatch', 'renderLiveInsights','champSelectEnemyPlaceholder'];
  const render = Function('state', 'SUMMONERS_RIFT_QUEUE_IDS', 'liveAugmentRecommendationSource', 'recordLiveRosterRendered', 'renderLiveRosterNoticeBars', 'renderLiveTeamPortraitTags', 'renderLivePlayer', 'renderInsightMatches', 'renderLiveRecentPositions', 'isARAMRelatedMatch', 'arenaLivePlayerGroups', 'escapeHTML', `${names.map(extract).join('\n')}; return renderLiveInsights;`)(
    state, [420, 440], () => 'hextech', () => {}, () => '', () => '',
    (player) => `<player id="${player.id}"${player.isCurrent ? ' current' : ''}></player>`, () => '', () => '', () => true, () => [], String,
  );
  const players = [
    { id: 'self', teamId: 100, isCurrent: true, position: 'bottom', premadeGroup: '1', premadeSize: 2 },
    { id: 'solo', teamId: 100, position: 'jungle' },
    { id: 'mate', teamId: 100, position: 'top', premadeGroup: '1', premadeSize: 2 },
    { id: 'foe-a', teamId: 200, position: 'utility', premadeGroup: '2', premadeSize: 2 },
    { id: 'foe-solo', teamId: 200, position: 'middle' },
    { id: 'foe-b', teamId: 200, position: 'top', premadeGroup: '2', premadeSize: 2 },
  ];
  const ids = (html) => [...html.matchAll(/<player id="([^"]+)"/g)].map((match) => match[1]);
  assert.deepEqual(ids(render({ available: true, gameMode: 'KIWI', mapId: 12, queueId: 2400, players })),
    ['self', 'mate', 'solo', 'foe-a', 'foe-b', 'foe-solo']);
  const rift = render({ available: true, gameMode: 'CLASSIC', mapId: 11, queueId: 420, players });
  assert.deepEqual(ids(rift), ['mate', 'solo', 'self', 'foe-b', 'foe-solo', 'foe-a']);
  assert.match(rift, /id="self" current/);
  assert.deepEqual(ids(render({ available: true, gameMode: 'CLASSIC', mapId: 0, queueId: 420, players })),
    ['mate', 'solo', 'self', 'foe-b', 'foe-solo', 'foe-a']);
});

test('R165 live chips distinguish private history, hidden identity, and direct autofill', () => {
  const names = ['liveHistoryStateOf', 'liveHistorySettled', 'renderLivePlayer','champSelectEnemyPlaceholder'];
  const render = Function('liveDisplayedChampionId', 'maskedPlayerName', 'renderLivePremadeTag', 'renderProIdentityBadge', 'proBadgeAttributes', 'iconFigure', 'escapeHTML', 'positionLabel', 'rankTitle', 'number', 'percent', 'kda', `${names.map(extract).join('\n')}; return renderLivePlayer;`)(
    () => 0, (player) => player.displayName, () => '', () => '', () => '', () => '<icon></icon>', String, () => '', () => '', String, String, String,
  );
  const hidden = render({ displayName: '隐藏玩家', hidden: true, historyState: 'unavailable' }, 0, false, 0, [], '', true);
  const privateHistory = render({ displayName: '公开身份', privateHistory: true, historyState: 'unavailable' }, 1, false, 0, [], '', true);
  const both = render({ displayName: '两种隐藏', hidden: true, privateHistory: true, autofill: true, historyState: 'unavailable' }, 2, false, 0, [], '', true);
  const ordinary = render({ displayName: '普通玩家', hidden: false, historyState: 'unavailable' }, 1, false, 0, [], '', true);
  assert.match(hidden, /<span class="player-tab-hidden">隐藏身份<\/span>/);
  assert.doesNotMatch(hidden, /隐藏战绩/);
  assert.match(privateHistory, /<span class="player-tab-hidden">隐藏战绩<\/span>/);
  assert.doesNotMatch(privateHistory, /隐藏身份/);
  assert.match(both, /隐藏战绩.*隐藏身份.*补位/);
  assert.match(hidden, /class="live-player-name" type="button" disabled/);
  assert.doesNotMatch(ordinary, /player-tab-hidden/);
  assert.match(styles, /\.player-tab-hidden\s*\{/);
});

test('R168 live identity copy separates unresolved champ select names from explicit privacy', () => {
  const names = ['playerLabel', 'maskedPlayerName', 'maskedListName', 'playerParticipantName', 'liveHistoryStateOf', 'liveHistorySettled', 'renderLivePlayer', 'renderInsightMatches','champSelectEnemyPlaceholder'];
  const helpers = Function('state', 'liveDisplayedChampionId', 'renderLivePremadeTag', 'renderProIdentityBadge', 'proBadgeAttributes', 'iconFigure', 'escapeHTML', 'positionLabel', 'rankTitle', 'number', 'percent', 'kda', `${names.map(extract).join('\n')}; return { playerLabel, maskedPlayerName, maskedListName, playerParticipantName, renderLivePlayer, renderInsightMatches };`)(
    { settings: { maskNames: false } }, () => 0, () => '', () => '', () => '', () => '<icon></icon>', String, () => '', () => '', String, String, String,
  );
  const unresolved = { displayName: '隐藏玩家', identityUnresolved: true, hidden: false, historyState: 'unavailable' };
  const hidden = { displayName: '隐藏玩家', identityUnresolved: false, hidden: true, historyState: 'unavailable' };
  const unresolvedHTML = helpers.renderLivePlayer(unresolved, 0, false, 0, [], '', true);
  const hiddenHTML = helpers.renderLivePlayer(hidden, 0, false, 0, [], '', true);
  assert.equal(helpers.playerLabel(unresolved), '身份待公开');
  assert.equal(helpers.maskedPlayerName(unresolved, 0), '身份待公开');
  assert.equal(helpers.maskedListName(unresolved, 0), '身份待公开');
  assert.equal(helpers.playerParticipantName(unresolved, 0), '身份待公开');
  assert.match(unresolvedHTML, /身份待公开.*身份待公开/);
  assert.match(unresolvedHTML, /身份尚未公开/);
  assert.doesNotMatch(unresolvedHTML, /隐藏身份|客户端未公开该玩家/);
  assert.match(helpers.renderInsightMatches(unresolved), /身份尚未公开/);
  assert.match(hiddenHTML, /隐藏身份/);
  assert.match(hiddenHTML, /客户端未公开该玩家/);
  assert.doesNotMatch(hiddenHTML, /身份待公开/);
  assert.match(helpers.renderInsightMatches(hidden), /客户端未公开该玩家/);
  assert.equal(helpers.playerLabel({ gameName: 'private-name', hidden: true }), '隐藏玩家');
  assert.equal(helpers.maskedListName({ displayName: 'private-name', hidden: true }, 0), '隐藏玩家');
  assert.equal(helpers.playerParticipantName({ gameName: 'private-name', hidden: true }, 0), '隐藏玩家');
});
