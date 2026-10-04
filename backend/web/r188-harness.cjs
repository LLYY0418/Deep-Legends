'use strict';
const fs = require('node:fs');
const path = require('node:path');
const web = __dirname;
function read(name, directory = web) { return fs.readFileSync(path.join(directory, name), 'utf8'); }
function extract(source, name) {
  let start = source.indexOf(`function ${name}(`);
  if (start < 0) throw Error(`Missing function ${name}`);
  if (source.slice(start - 6, start) === 'async ') start -= 6;
  const body = source.indexOf(') {', start) + 2;
  let depth = 0, quote = '', escaped = false;
  for (let i = body; i < source.length; i++) {
    const c = source[i];
    if (quote) { if (escaped) escaped = false; else if (c === '\\') escaped = true; else if (c === quote) quote = ''; continue; }
    if (c === '"' || c === "'" || c === '`') { quote = c; continue; }
    if (c === '{') depth++;
    if (c === '}' && --depth === 0) return source.slice(start, i + 1);
  }
  throw Error(`Unbalanced function ${name}`);
}
function compile(source, names, dependencies = {}) {
  dependencies = { recordLiveRecommendationRender: () => {}, ensureLiveRecommendationForRender: () => {}, ...dependencies };
  return Function(...Object.keys(dependencies), names.map(name => extract(source, name)).join('\n') + `\nreturn {${names.join(',')}};`)(...Object.values(dependencies));
}
const escapeHTML = value => String(value ?? '').replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;').replaceAll('>', '&gt;');
function fixture({directory = web, gameplaySource, active = 'runes', locked = false, pending = false, notice = ''} = {}) {
  const source = gameplaySource || read('gameplay.js', directory);
  const data = {phase: 'ChampSelect', available: true, queueId: 440, gameId: 188, champSelectNotice: notice, players: [
    {isCurrent: true, teamId: 100, position: 'mid', championId: locked ? 69 : 0, championPickIntent: 69, championLocked: locked},
    {teamId: 200, position: 'mid', championId: 103, championName: '阿狸', championLocked: true},
  ]};
  const state = {settings: {maskNames: false}, recommendationTab: active, laneMatchupCandidates: new Map(), laneMatchupPairs: new Map()};
  const events = [];
  const names = ['laneMatchupEnemies', 'laneMatchupInference', 'laneMatchupContext', 'laneMatchupOwnChampionId', 'laneMatchupOwnLocked', 'laneMatchupTier', 'laneMatchupCandidateKey', 'laneMatchupPairKey', 'recordLaneMatchupCardDiagnostic', 'renderLaneMatchupCard', 'renderRecommendationArea', 'livePremadeRoster', 'renderLivePremadeTag', 'liveDisplayedChampionId', 'maskedPlayerName'];
  const iconFigure = (_kind, id, name) => `<span class="game-icon"><img width="22" height="22" src="/fixture-icon/${id}" alt="${escapeHTML(name)}"></span>`;
  const deps = {state, escapeHTML, iconFigure, rate: value => `${Number(value).toFixed(1)}%`, LIVE_PREMADE_MIN_SHARED_GAMES: 5,
    liveRecommendationTier: () => 'emerald_plus', livePositionValue: value => value || '',
    playerLabel: player => player.gameName + (player.tagLine ? '#' + player.tagLine : ''),
    proxyAsset: value => `/api/image?path=${encodeURIComponent(value)}`, assetPath: (kind, id) => `${kind}/${id}`,
    recordItemSetClientDiagnostic: (event, reason, fields) => events.push({event, reason, ...fields}),
    liveRecommendationTarget: () => ({championId: 69, key: 'self'}), liveRecommendationsFor: () => ({}), liveAugmentRecommendationSource: () => '',
    recommendationCapabilities: () => ({hasRunes: true, hasAugments: false}), recommendationTabSpecs: () => [['runes', '符文'], ['build', '出装'], ['insight', '详情']],
    recommendationActiveTab: () => state.recommendationTab, selectedRuneRecommendation: () => null, recommendationPanelBusy: () => false,
    renderLiveInsights: () => '<div class="live-player-list"></div>', renderChampionRecommendationHeader: () => '', renderRuneRecommendations: () => '',
    renderBuildRecommendation: () => '', renderRecommendationDataNotices: () => '', recommendationEmptyPanel: () => '',
  };
  const functions = compile(source, names, deps);
  const context = functions.laneMatchupContext(data);
  const pair = {status: pending ? 'pending' : 'succeeded', data: pending ? null : {winRate: 47.6}};
  const candidates = {rows: [
    {championId: 61, name: '奥莉安娜', winRate: 47.9}, {championId: 7, name: '乐芙兰', winRate: 46.7},
    {championId: 112, name: '维克托', winRate: 48.2}, {championId: 134, name: '辛德拉', winRate: 48.9},
  ]};
  state.laneMatchupPairs.set(functions.laneMatchupPairKey(data, context), pair);
  state.laneMatchupCandidates.set(functions.laneMatchupCandidateKey(context), pending ? {rows: []} : candidates);
  return {source, data, state, pair, candidates, events, ...functions};
}
module.exports = {read, extract, compile, escapeHTML, fixture};
