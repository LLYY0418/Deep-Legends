'use strict';
// R263：对局页深度优化——名称下方按模式（峡谷单双 + 灵活、斗魂等级名望、娱乐模式无段位）、
// 斗魂 30 局胜率 / 吃鸡率与名次小卡、斗魂按预组队出卡片、战绩小卡点击弹窗。
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { JSDOM } = require('../../desktop/node_modules/jsdom');
const { compile, escapeHTML } = require('./r188-harness.cjs');

const js = fs.readFileSync(path.join(__dirname, 'gameplay.js'), 'utf8');
const gameplayCSS = fs.readFileSync(path.join(__dirname, 'gameplay.css'), 'utf8');
// R263 的新渲染与样式在按需模块里（首屏体积预算），首次进入对局页时由 section-loader 加载。
const moduleSource = fs.readFileSync(path.join(__dirname, 'live-arena.js'), 'utf8');
const css = fs.readFileSync(path.join(__dirname, 'live-arena.css'), 'utf8');
const liveArena = require('./live-arena.js');

// 模块从 window.deepLegendsMatchCards.live 取 gameplay.js 的工具；node 下 root 是 globalThis。
function installModule(live = {}) {
  globalThis.deepLegendsLiveArena = liveArena;
  globalThis.deepLegendsMatchCards = { live };
}
function removeModule() {
  delete globalThis.deepLegendsLiveArena;
  delete globalThis.deepLegendsMatchCards;
}

function playerRenderer() {
  const deps = {
    state: { settings: { maskNames: false } }, escapeHTML,
    champSelectEnemyPlaceholder: () => false, liveDisplayedChampionId: (player) => player.championId || 0,
    maskedPlayerName: (player) => player.name || '玩家', renderLivePremadeTag: () => '', proBadgeAttributes: () => '', renderProIdentityBadge: () => '',
    iconFigure: (_kind, id) => `<span class="game-icon" data-id="${id}"></span>`,
    positionLabel: (value) => ({ top: '上路', jungle: '打野', middle: '中路' })[value] || '位置未知',
    numberFormatter: new Intl.NumberFormat('zh-CN'),
  };
  const f = compile(js, ['number', 'percent', 'kda', 'rankTitle', 'liveHistoryStateOf', 'liveHistorySettled', 'insightScore', 'renderLivePlayer', 'renderInsightMatches'], deps);
  installModule({ rankTitle: f.rankTitle, insightScore: f.insightScore, iconFigure: deps.iconFigure });
  return f;
}

const solo = { queueType: 'RANKED_SOLO_5x5', tier: 'EMERALD', division: 'IV', leaguePoints: 48 };
const flex = { queueType: 'RANKED_FLEX_SR', tier: 'PLATINUM', division: 'I', leaguePoints: 59 };

function identity(html) {
  const dom = new JSDOM(html);
  const copy = dom.window.document.querySelector('.live-player-copy');
  return { copy, line: [...copy.children].find((node) => node !== copy.querySelector('.live-player-identity')) };
}

test('R263 P3 Summoner\'s Rift shows position plus solo and flex ranks, current queue first', () => {
  const f = playerRenderer();
  const base = { name: '甲', playerRef: 'p1', position: 'jungle', historyState: 'ok', recentGames: [], soloRank: solo, flexRank: flex };
  const soloQueue = identity(f.renderLivePlayer({ ...base, rank: solo }, 0, false, 0, [], '', false, false, 'InProgress', '', 'rift'));
  assert.equal(soloQueue.line.textContent, '打野 · 单双 翡翠 IV 48 LP · 灵活 铂金 I 59 LP');
  const flexQueue = identity(f.renderLivePlayer({ ...base, rank: flex }, 0, false, 0, [], '', false, false, 'InProgress', '', 'rift'));
  assert.equal(flexQueue.line.textContent, '打野 · 灵活 铂金 I 59 LP · 单双 翡翠 IV 48 LP');
  const unranked = identity(f.renderLivePlayer({ ...base, flexRank: null, rank: solo }, 0, false, 0, [], '', false, false, 'InProgress', '', 'rift'));
  assert.equal(unranked.line.textContent, '打野 · 单双 翡翠 IV 48 LP · 灵活 未定级');
  // Callers that do not pass a kind keep the previous single-rank line.
  const legacy = identity(f.renderLivePlayer({ ...base, rank: solo }, 0, false, 0, [], '', false, false, 'InProgress', ''));
  assert.equal(legacy.line.textContent, '打野 · 翡翠 IV');
});

test('R263 before the lazy module loads the live page renders exactly as before', () => {
  const f = playerRenderer();
  removeModule();
  try {
    const base = { name: '甲', playerRef: 'p1', position: 'jungle', historyState: 'ok', rank: solo, soloRank: solo, flexRank: flex, mySquad: true,
      arenaFame: { level: 12, fame: 87510, tier: 'GLADIATOR' }, arenaRecord: { games: 30, topHalf: 17, top1: 6 },
      recentGames: [{ gameId: 1, placement: 1, win: true, kills: 5, deaths: 2, assists: 7 }] };
    assert.equal(identity(f.renderLivePlayer(base, 0, false, 0, [], '', false, false, 'InProgress', '', 'rift')).line.textContent, '打野 · 翡翠 IV');
    const arena = new JSDOM(f.renderLivePlayer(base, 0, true, 0, [], '', false, false, 'InProgress', '', 'arena')).window.document;
    assert.equal(arena.querySelector('.arena-fame'), null);
    assert.ok(arena.querySelector('.live-kda-value'));
    assert.ok(arena.querySelector('.my-squad-chip'));
    const chips = new JSDOM(f.renderInsightMatches(base)).window.document;
    assert.ok(chips.querySelector('span.insight-match[data-tooltip]'));
    assert.equal(chips.querySelector('[data-live-match]'), null);
  } finally {
    installModule();
  }
});

test('R263 P3 entertainment modes show no rank under the name', () => {
  const f = playerRenderer();
  const html = f.renderLivePlayer({ name: '乙', playerRef: 'p2', position: 'middle', historyState: 'ok', recentGames: [], rank: solo, soloRank: solo }, 0, false, 0, [], '', true, true, 'InProgress', '', 'none');
  const { copy, line } = identity(html);
  assert.equal(line, undefined);
  assert.doesNotMatch(copy.textContent, /翡翠|LP|未定级/);
});

test('R263 P2 Arena shows level and fame instead of rank, and 30-game win and top-1 rates instead of KDA', () => {
  const f = playerRenderer();
  const html = f.renderLivePlayer({ name: '丙', playerRef: 'p3', historyState: 'ok', mySquad: true, rank: solo, soloRank: solo,
    arenaFame: { level: 12, fame: 87510, tier: 'GLADIATOR' }, arenaRecord: { games: 30, topHalf: 17, top1: 6 },
    recentGames: [{ gameId: 1, placement: 1, win: true, kills: 5, deaths: 2, assists: 7 }] }, 0, true, 0, [], '', false, false, 'InProgress', '', 'arena');
  const dom = new JSDOM(html);
  const document = dom.window.document;
  const fame = document.querySelector('.arena-fame');
  assert.ok(fame.classList.contains('is-gladiator'));
  assert.equal(fame.textContent, '12级·87,510 名望');
  assert.equal(fame.dataset.tooltip, '斗魂等级 12 · 名望 87,510');
  assert.ok(fame.querySelector('svg.arena-fame-icon'));
  assert.doesNotMatch(document.querySelector('.live-player-copy').textContent, /翡翠|LP/);
  assert.equal(document.querySelector('.my-squad-chip'), null, 'the squad card title already says 我的小队');
  const labels = [...document.querySelectorAll('dl.is-arena-stats dt')].map((node) => node.textContent);
  assert.deepEqual(labels, ['近 30 局', '胜率', '吃鸡率']);
  const values = [...document.querySelectorAll('dl.is-arena-stats dd')].map((node) => node.textContent);
  assert.deepEqual(values, ['17胜 13负', '57%', '20%']);
  assert.equal(document.querySelector('.live-kda-value'), null);
});

test('R263 P2 Arena without fame data renders nothing under the name and no placeholder text', () => {
  const f = playerRenderer();
  const html = f.renderLivePlayer({ name: '丁', playerRef: 'p4', historyState: 'ok', recentGames: [] }, 0, true, 0, [], '', false, false, 'InProgress', '', 'arena');
  const { line } = identity(html);
  assert.equal(line, undefined);
  assert.doesNotMatch(html, /未提供|暂无名望/);
  assert.match(html, /<dt>吃鸡率<\/dt>/);
});

test('R263 P2.3/P4 Arena chips show placement; every chip is a button that opens the match', () => {
  const f = playerRenderer();
  const games = [
    { gameId: 11, placement: 1, win: true, kills: 9, deaths: 1, assists: 3, championName: '贾克斯' },
    { gameId: 12, placement: 3, win: true, kills: 4, deaths: 3, assists: 8 },
    { gameId: 13, placement: 5, win: false, kills: 1, deaths: 6, assists: 2 },
    { gameId: 14, placement: 0, win: false, kills: 0, deaths: 0, assists: 0 },
  ];
  const dom = new JSDOM(f.renderInsightMatches({ playerRef: 'player_ref_x', historyState: 'ok', recentGames: games }));
  const chips = [...dom.window.document.querySelectorAll('button.insight-match')];
  assert.equal(chips.length, 4);
  assert.deepEqual(chips.map((chip) => chip.querySelector('.insight-placement').textContent), ['第1名', '第3名', '第5名', '—']);
  assert.deepEqual(chips.map((chip) => chip.querySelector('.insight-placement').className.split(' ')[1]), ['is-first', 'is-top', 'is-bottom', 'is-unknown']);
  assert.equal(dom.window.document.querySelector('.insight-score'), null, 'Arena chips do not show the KDA ratio');
  assert.equal(chips[0].dataset.liveMatch, '11');
  assert.equal(chips[0].dataset.liveMatchPlayer, 'player_ref_x');
  assert.equal(chips[0].getAttribute('type'), 'button');
  assert.equal(chips[0].hasAttribute('data-tooltip'), false);
  assert.match(chips[0].getAttribute('aria-label'), /贾克斯 · 9\/1\/3 · 第1名 · 查看对局详情/);
  assert.equal(chips[0].hasAttribute('data-player-ref'), false, 'chips must not trigger the name overlay handler');
});

test('R263 P4 Summoner\'s Rift chips keep the KDA ratio; chips without a game id are disabled', () => {
  const f = playerRenderer();
  const dom = new JSDOM(f.renderInsightMatches({ playerRef: 'player_ref_y', historyState: 'ok', recentGames: [
    { gameId: 21, win: true, kills: 6, deaths: 2, assists: 6 }, { win: false, kills: 1, deaths: 5, assists: 2 },
  ] }));
  const chips = [...dom.window.document.querySelectorAll('button.insight-match')];
  assert.equal(chips[0].querySelector('.insight-score').textContent, '6.0');
  assert.equal(dom.window.document.querySelector('.insight-placement'), null);
  assert.equal(chips[1].disabled, true);
  assert.equal(chips[1].hasAttribute('data-live-match'), false);
});

function insightsRenderer() {
  const deps = {
    state: { settings: { liveOrder: 'team' } }, window: {}, escapeHTML,
    liveAugmentRecommendationSource: (data) => (data.gameMode === 'CHERRY' ? 'arena' : ''), recordLiveRosterRendered: () => {},
    isSummonersRiftMatch: (data) => Number(data.mapId) === 11,
    renderLiveRosterNoticeBars: () => '', renderLiveTeamPortraitTags: () => '', renderLiveRecentPositions: () => '',
    renderLivePlayer: (player, _index, _arena, _champion, _all, _positions, _aram, _hextech, _phase, _display, kind) => `<article class="live-player" data-kind="${kind}">${player.playerRef}</article>`,
    renderInsightMatches: () => '<div class="insight-match-row"></div>', isARAMRelatedMatch: () => false, champSelectEnemyPlaceholder: () => false, arenaTeamMeta: () => null,
  };
  installModule();
  return compile(js, ['orderLivePlayers', 'clusterPremadePlayers', 'livePremadeRoster', 'arenaLivePlayerGroups', 'insightTeamLayout', 'renderLiveInsights'], deps);
}

function arenaPlayers() {
  const keys = ['mine', 'mine', 'mine', 'premade:B', 'premade:A', 'premade:A', 'premade:B', ...Array(11).fill('')];
  return keys.map((key, index) => ({ playerRef: `p${index}`, teamId: 100, isCurrent: index === 0, mySquad: index < 3, arenaSquadKey: key, modeStats: {} }));
}

test('R263 P1 Arena renders 我的小队, premade 小队 A/B and 未知小队 cards with fixed slots', () => {
  const f = insightsRenderer();
  const html = f.renderLiveInsights({ phase: 'InProgress', gameMode: 'CHERRY', queueId: 1750, arenaSquadSize: 3, players: arenaPlayers() });
  const document = new JSDOM(html).window.document;
  const grid = document.querySelector('.live-teams');
  assert.ok(grid.classList.contains('is-squads') && grid.classList.contains('is-squad-size-3'));
  assert.equal(grid.hasAttribute('style'), false, 'CSP blocks style attributes');
  const cards = [...grid.querySelectorAll(':scope > section.live-team.is-squad')];
  assert.deepEqual(cards.map((card) => card.getAttribute('aria-label')), ['我的小队', '小队 A', '小队 B', '未知小队', '未知小队', '未知小队', '未知小队']);
  assert.deepEqual(cards.map((card) => card.querySelectorAll('article.live-player').length), [3, 2, 2, 3, 3, 3, 2]);
  assert.deepEqual(cards.map((card) => card.querySelectorAll('.live-squad-slot.is-empty').length), [0, 1, 1, 0, 0, 0, 1]);
  assert.deepEqual([...cards[1].querySelectorAll('article')].map((node) => node.textContent), ['p4', 'p5']);
  assert.ok(cards[1].querySelector('.premade-team-tag.is-color-0.live-squad-tag'));
  assert.ok(cards[2].querySelector('.premade-team-tag.is-color-1.live-squad-tag'));
  assert.equal([...grid.querySelectorAll('article')][0].dataset.kind, 'arena');
  assert.equal(document.querySelector('.my-squad-chip'), null);
});

test('R263 P1 Arena without squad keys (older backend) keeps the previous roster', () => {
  const f = insightsRenderer();
  const players = arenaPlayers().map(({ arenaSquadKey, ...player }) => player);
  const html = f.renderLiveInsights({ phase: 'InProgress', gameMode: 'CHERRY', queueId: 1750, players });
  assert.match(html, /live-teams is-insight is-arena">/);
  assert.doesNotMatch(html, /is-squads/);
});

test('R263 P3 identity kind follows the mode: Rift CLASSIC, URF and ARAM', () => {
  const f = insightsRenderer();
  const players = [{ playerRef: 'a', teamId: 100, isCurrent: true }, { playerRef: 'b', teamId: 200 }];
  const kind = (data) => new JSDOM(f.renderLiveInsights({ phase: 'InProgress', players, ...data })).window.document.querySelector('article').dataset.kind;
  assert.equal(kind({ gameMode: 'CLASSIC', mapId: 11, queueId: 420 }), 'rift');
  assert.equal(kind({ gameMode: 'URF', mapId: 11, queueId: 900 }), 'none', 'URF is on the Rift map but is an entertainment mode');
  assert.equal(kind({ gameMode: 'ARAM', mapId: 12, queueId: 450 }), 'none');
  assert.equal(kind({ gameMode: 'KIWI', mapId: 12, queueId: 2400 }), 'none');
});

test('R263 P1 CSS: two squads per row with subgrid alignment, single column only when narrow', () => {
  assert.match(css, /\.live-teams\.is-insight\.is-arena\.is-squads\s*\{[^}]*grid-template-columns:\s*repeat\(2,minmax\(0,1fr\)\)/);
  assert.match(css, /\.live-teams\.is-arena\.is-squads > \.live-team\.is-squad\s*\{[^}]*grid-row:\s*span 7;[^}]*grid-template-rows:\s*subgrid/);
  assert.match(css, /\.is-squad-size-2 > \.live-team\.is-squad\s*\{[^}]*span 5/);
  assert.match(css, /\.live-squad-slot\.is-empty\s*\{[^}]*grid-row:\s*span 2[^}]*border:\s*1px dashed/);
  const narrow = css.slice(css.indexOf('@container recommendation-area (max-width: 840px)'));
  assert.match(narrow.slice(0, 900), /\.live-teams\.is-insight\.is-arena\.is-squads\s*\{\s*grid-template-columns:\s*minmax\(0,1fr\)/);
  // The 1080px rule that collapses older Arena layouts must not win over the squads grid.
  const wide = gameplayCSS.slice(gameplayCSS.indexOf('@container recommendation-area (max-width: 1080px)'));
  assert.doesNotMatch(wide.slice(0, 400), /is-squads/);
});

test('R263 P2 fame tokens: light and dark values defined once, tier classes are literal', () => {
  for (const tier of ['wood', 'bronze', 'silver', 'gold', 'gladiator']) {
    assert.match(css, new RegExp(`--arena-tier-${tier}: #[0-9a-f]{6};`));
    assert.match(css, new RegExp(`--arena-tier-${tier}-dark: #[0-9a-f]{6};`));
    assert.match(css, new RegExp(`\\.arena-fame\\.is-${tier}\\s*\\{\\s*--arena-tier: var\\(--arena-tier-${tier}\\)`));
    assert.match(moduleSource, new RegExp(`' is-${tier}'`));
  }
});

function dialogHarness({ api }) {
  const dom = new JSDOM('<!doctype html><body><div data-live-player-row="card:k"><button class="live-player-name">玩家甲</button></div><div data-live-player-row="history:k"><button class="insight-match" data-live-match="42" data-live-match-player="player_ref_z">x</button></div></body>', { pretendToBeVisual: true, runScripts: 'outside-only' });
  const { window } = dom;
  window.HTMLDialogElement.prototype.showModal = function showModal() { this.setAttribute('open', ''); };
  window.HTMLDialogElement.prototype.close = function close() { if (!this.hasAttribute('open')) return; this.removeAttribute('open'); this.dispatchEvent(new window.Event('close')); };
  Object.defineProperty(window.HTMLDialogElement.prototype, 'open', { get() { return this.hasAttribute('open'); }, configurable: true });
  window.requestAnimationFrame = (callback) => callback();
  const mounts = [];
  const modal = [];
  const renders = [];
  const registered = new Map();
  const state = { status: { serverId: 'HN10' }, controllers: new Map() };
  window.desktopTheme = { setModalOpen: (value) => modal.push(value) };
  window.deepLegendsSections = { register: (name, callback) => registered.set(name, callback) };
  window.deepLegendsMatchCards = {
    mount: (container, options) => { mounts.push({ container, options }); container.innerHTML = '<article class="match-entry"></article>'; return { destroy: () => mounts.push('destroyed') }; },
    live: { state, api, clientRegion: () => '', render: () => renders.push('render') },
  };
  window.eval(moduleSource);
  const button = window.document.querySelector('[data-live-match]');
  button.getClientRects = () => [{ width: 1, height: 1 }]; // jsdom has no layout
  return { window, module: window.deepLegendsLiveArena, mounts, modal, renders, registered, button };
}

test('R263 P4 clicking a chip opens a modal with the overview match card expanded', async () => {
  const requests = [];
  const createdAt = '2026-10-09T13:00:00Z';
  const match = { gameId: 42, queueLabel: '斗魂竞技场', createdAt, subjectParticipantId: 2, participants: [{ participantId: 1, playerRef: 'a' }, { participantId: 2, playerRef: 'player_public_subject' }] };
  const h = dialogHarness({ api: async (url, _options, key) => { requests.push([url, key]); return match; } });
  await h.module.openMatch(h.button);
  assert.deepEqual(requests, [['/api/gameplay/live/match?gameId=42&player=player_ref_z', 'live-match-dialog']]);
  const dialog = h.window.document.querySelector('dialog.live-match-dialog');
  assert.ok(dialog.open);
  assert.deepEqual(h.modal, [true]);
  assert.equal(dialog.querySelector('#live-match-dialog-title').textContent, '斗魂竞技场');
  const when = new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(createdAt));
  assert.equal(dialog.querySelector('#live-match-dialog-context').textContent, `玩家甲 · ${when}`);
  assert.equal(h.mounts.length, 1);
  assert.equal(h.mounts[0].options.openMatchId, '42', 'the match must be expanded by default');
  assert.equal(h.mounts[0].options.playerRef, 'player_public_subject');
  assert.equal(h.mounts[0].options.matches.length, 1);
  assert.equal(h.mounts[0].options.matches[0], match);
  assert.ok(h.mounts[0].container.classList.contains('match-list'));
  assert.equal(dialog.parentElement, h.window.document.body, 'the dialog lives outside the live page re-render scope');
  dialog.close();
  assert.equal(dialog.open, false);
  assert.deepEqual(h.modal, [true, false]);
  assert.equal(h.mounts.at(-1), 'destroyed');
  assert.equal(h.window.document.activeElement, h.button, 'focus returns to the chip');
});

test('R263 P4 failures stay inside the dialog with a retry button', async () => {
  let attempts = 0;
  const h = dialogHarness({ api: async () => { attempts += 1; if (attempts === 1) throw new Error('这场对局的详情暂不可用'); return { gameId: 42, participants: [] }; } });
  await h.module.openMatch(h.button);
  const dialog = h.window.document.querySelector('dialog.live-match-dialog');
  assert.equal(dialog.querySelector('[role="alert"] span').textContent, '这场对局的详情暂不可用');
  dialog.querySelector('[data-retry-live-match]').click();
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(attempts, 2);
  assert.equal(h.mounts.length, 1);
});

test('R263 P4 wiring: lazy module loads with the live section, re-renders once, and chip clicks are delegated', async () => {
  const loader = fs.readFileSync(path.join(__dirname, 'section-loader.js'), 'utf8');
  assert.match(loader, /const modules = \{[^}]*\blive: \["live-arena"\]/);
  assert.match(loader, /const styles = \{[^}]*"live-arena": \["live-arena"\]/);
  const index = fs.readFileSync(path.join(__dirname, 'index.html'), 'utf8');
  assert.doesNotMatch(index, /live-arena/, 'not part of the initial bundle');
  assert.match(js, /window\.deepLegendsMatchCards = Object\.freeze\(\{[\s\S]{0,200}live: \{ state, api, clientRegion, rankTitle, iconFigure, insightScore, render: renderLive \}/);
  assert.match(js, /if \(options\.openMatchId\) tab\.openMatches\.add\(options\.openMatchId\)/);
  assert.match(css, /\.live-match-dialog-content\s*\{[^}]*container:\s*matches-column \/ inline-size/);
  let calls = 0;
  const h = dialogHarness({ api: async () => { calls += 1; return { gameId: 42, participants: [] }; } });
  assert.ok(h.registered.has('live-arena'), 'section-loader requires the module to register');
  h.registered.get('live-arena')();
  assert.deepEqual(h.renders, ['render']);
  h.button.click();
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(calls, 1);
  assert.ok(h.window.document.querySelector('dialog.live-match-dialog').open);
});
