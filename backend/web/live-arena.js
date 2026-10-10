/* R263：对局页按需模块——名称下方按模式（峡谷单双 + 灵活、斗魂等级名望）、斗魂右侧
   30 局胜率 / 吃鸡率、斗魂按预组队出卡片、名次小卡与战绩详情弹窗。首次进入对局页时由
   section-loader 连同 live-arena.css 一起加载（不计入首屏体积预算），加载完成后让
   gameplay.js 重绘一次；加载前对局页按原来的样子渲染。gameplay.js 闭包里的工具经
   window.deepLegendsMatchCards.live 取得（首屏体积预算只给 gameplay.js 留了很少余量，
   数字 / 百分比 / 时间格式在这里各自实现，和 gameplay.js 同一格式）。 */
(function (root, factory) {
  const api = factory(root);
  if (typeof module === 'object' && module.exports) module.exports = api;
  else {
    root.deepLegendsLiveArena = api;
    root.deepLegendsSections?.register('live-arena', () => root.deepLegendsMatchCards?.live?.render?.());
    // 小卡点击用委托：对局页每次重绘都换新节点，不必逐个绑定。
    root.document?.addEventListener('click', (event) => {
      const button = event.target?.closest?.('[data-live-match]');
      if (button && !button.disabled) api.openMatch(button);
    });
  }
})(typeof window === 'undefined' ? globalThis : window, (root) => {
  'use strict';
  const numberFormat = new Intl.NumberFormat('zh-CN');
  const helpers = () => {
    const host = root.deepLegendsMatchCards || {};
    return {
      ...host.live,
      mount: host.mount,
      escapeHTML: root.deepLegendsRuntime?.escapeHTML || ((value) => String(value ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c])),
      number: (value) => Number.isFinite(Number(value)) ? numberFormat.format(Number(value)) : '—',
      percent: (value) => Number.isFinite(Number(value)) ? `${Math.round(Number(value))}%` : '—',
    };
  };
  const exactTime = (value) => {
    const numeric = Number(value);
    const date = value == null || value === '' || value === 0 ? null : Number.isFinite(numeric) && numeric > 0 ? new Date(numeric) : new Date(String(value));
    return date && !Number.isNaN(date.valueOf()) ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(date) : '';
  };
  const tiers = { WOOD: ' is-wood', BRONZE: ' is-bronze', SILVER: ' is-silver', GOLD: ' is-gold', GLADIATOR: ' is-gladiator' };

  // 名称下方那一行。kind："rift" = 召唤师峡谷（位置 + 单双 + 灵活段位，本局队列在前，
  // 对齐 LeagueAkari）；"arena" = 斗魂等级 + 名望；"none" = 其他娱乐模式不显示段位。
  // 隐藏玩家与未知 kind 沿用 gameplay.js 算好的 legacy 文案；峡谷的 legacy 文案是
  // 「位置 · 段位」，位置取它的第一段。
  function identityLine(player, kind, legacyCopy) {
    const h = helpers();
    const span = (copy, tooltip) => copy ? `<span${tooltip ? ` data-tooltip="${h.escapeHTML(copy)}" data-tooltip-overflow="self" data-tooltip-size="compact"` : ''}>${h.escapeHTML(copy)}</span>` : '';
    if (player.hidden === true || !['rift', 'arena', 'none'].includes(kind)) return span(legacyCopy, false);
    if (kind === 'none') return '';
    if (kind === 'arena') {
      const fame = Number(player.arenaFame?.level) > 0 ? player.arenaFame : null;
      if (!fame) return '';
      const tier = tiers[String(fame.tier || '').toUpperCase()] || '';
      return `<span class="arena-fame${tier}" data-tooltip="${h.escapeHTML(`斗魂等级 ${h.number(fame.level)} · 名望 ${h.number(fame.fame)}`)}" data-tooltip-size="compact"><svg class="arena-fame-icon" viewBox="0 0 16 16" aria-hidden="true"><path d="M8 1.5 13.5 4.6v6.8L8 14.5 2.5 11.4V4.6Z"/><path class="arena-fame-blades" d="m5.4 5.4 5.2 5.2M10.6 5.4 5.4 10.6"/></svg><b>${h.number(fame.level)}级</b><span class="arena-fame-divider" aria-hidden="true">·</span><span>${h.number(fame.fame)} 名望</span></span>`;
    }
    if (!player.soloRank && !player.flexRank) return span(legacyCopy, true);
    const queue = (label, value) => `${label} ${value?.tier ? `${h.rankTitle(value)} ${h.number(value.leaguePoints)} LP` : '未定级'}`;
    const ranks = [queue('单双', player.soloRank), queue('灵活', player.flexRank)];
    if (player.rank?.queueType === 'RANKED_FLEX_SR') ranks.reverse();
    return span([String(legacyCopy).split(' · ')[0], ...ranks].join(' · '), true);
  }

  // 斗魂右侧三格：近 N 局「X胜 Y负」（胜 = 名次在前一半，对齐 Akari isCherryPlacementWin）、
  // 胜率、吃鸡率（第 1 名 ÷ 局数）。返回空串时由 gameplay.js 画原来的胜率 / KDA。
  // 战绩读取中的骨架由 gameplay.js 先判断，这里不会被调用。
  function arenaSummary(player, kind, emptySummary) {
    if (kind !== 'arena') return '';
    const recordGames = Array.isArray(player.recentGames) ? player.recentGames.length : 0;
    const h = helpers();
    const record = Number(player.arenaRecord?.games) > 0 ? player.arenaRecord : null;
    const cell = (label, value, cls) => `<div><dt>${label}</dt><dd class="${cls}">${value}</dd></div>`;
    if (record) return `<dl class="is-arena-stats">${cell(`近 ${h.number(record.games)} 局`, `${h.number(record.topHalf)}胜 ${h.number(record.games - record.topHalf)}负`, 'live-record-value')}${cell('胜率', h.percent(record.topHalf * 100 / record.games), 'win-rate-value')}${cell('吃鸡率', h.percent(record.top1 * 100 / record.games), 'arena-top1-value')}</dl>`;
    if (player.hidden !== true && !recordGames) return `<dl class="is-arena-stats">${cell('当前模式', emptySummary, 'live-record-value')}${cell('胜率', '—', 'win-rate-value')}${cell('吃鸡率', '—', 'arena-top1-value')}</dl>`;
    return '';
  }

  // 斗魂卡片分组。后端 arenaSquadKey 给出「mine / premade:A… / 空」，这里只负责排序与切块：
  // 我的小队第一，其后按小队字母，剩余玩家按名单顺序每 squadSize 人一张「未知小队」。
  // 没有任何归属信息（老后端）时返回空数组，调用方回落到原来的单列表。
  function squadCards(players, squadSize) {
    const list = Array.isArray(players) ? players : [];
    if (!list.some((player) => player?.arenaSquadKey)) return [];
    const size = Math.min(4, Math.max(2, Number(squadSize) || 3));
    const mine = list.filter((player) => player.arenaSquadKey === 'mine' || (!player.arenaSquadKey && player.isCurrent));
    const premade = new Map();
    for (const player of list) {
      const key = String(player.arenaSquadKey || '');
      if (!key.startsWith('premade:')) continue;
      if (!premade.has(key)) premade.set(key, []);
      premade.get(key).push(player);
    }
    const unknown = list.filter((player) => !player.arenaSquadKey && !player.isCurrent);
    const squads = [];
    if (mine.length) squads.push({ kind: 'mine', label: '我的小队', players: mine.slice(0, size), size });
    const letters = [...premade.keys()].sort((left, right) => left.localeCompare(right, 'en', { numeric: true }));
    for (const key of letters) {
      const letter = key.slice('premade:'.length);
      const code = letter.length === 1 ? letter.charCodeAt(0) - 65 : Number(letter) - 1;
      squads.push({ kind: 'premade', label: `小队 ${letter}`, players: premade.get(key).slice(0, size), size, colorIndex: ((Number.isFinite(code) ? code : 0) % 12 + 12) % 12 });
    }
    for (let index = 0; index < unknown.length; index += size) {
      squads.push({ kind: 'unknown', label: '未知小队', players: unknown.slice(index, index + size), size });
    }
    return squads;
  }

  // 每张卡固定 squadSize 个槽，两张一行；不满的槽画同高空位，保证同一行两张卡的玩家行
  // 对齐（subgrid）。CSP 禁止 style 属性，槽数用 is-squad-size-N 类名。
  function squadsMarkup(players, squadSize, playerRows) {
    const { escapeHTML } = helpers();
    const squads = squadCards(players, squadSize);
    if (!squads.length) return '';
    const size = squads[0].size;
    const cards = squads.map((squad) => {
      const empty = '<div class="live-squad-slot is-empty" aria-hidden="true"></div>'.repeat(Math.max(0, size - squad.players.length));
      const title = squad.kind === 'premade' ? `<span class="premade-team-tag is-color-${squad.colorIndex} live-squad-tag">${escapeHTML(squad.label)}</span>` : `<h3>${escapeHTML(squad.label)}</h3>`;
      return `<section class="live-team is-arena is-squad is-squad-${squad.kind}" aria-label="${escapeHTML(squad.label)}"><header>${title}</header><div class="live-player-list is-insight">${playerRows(squad.players)}${empty}</div></section>`;
    }).join('');
    return `<div class="live-teams is-insight is-arena is-squads is-squad-size-${size}">${cards}</div>`;
  }

  // 战绩小卡：每张都是按钮，点击打开这场对局的详情弹窗（没有 gameId 的禁用）；斗魂小卡
  // 右侧显示名次（对齐 Akari 的序数名次，未知时 —），其余模式保留 KDA 比值。
  function chips(games, player) {
    const h = helpers();
    const arena = games.some((game) => Number(game.placement) > 0);
    return games.map((game) => {
      const placement = Number(game.placement) || 0;
      const tone = placement === 1 ? 'is-first' : game.win ? 'is-top' : 'is-bottom';
      const tail = arena ? `<i class="insight-placement ${placement > 0 ? tone : 'is-unknown'}">${placement > 0 ? `第${h.number(placement)}名` : '—'}</i>` : h.insightScore(game);
      const gameID = Number(game.gameId) || 0;
      const openable = gameID > 0 && Boolean(player.playerRef);
      const outcome = arena ? (placement > 0 ? `第${placement}名` : '名次未知') : game.win ? '胜利' : '失败';
      const kda = `${h.number(game.kills)}/${h.number(game.deaths)}/${h.number(game.assists)}`;
      const label = `${game.championName || '英雄'} · ${kda} · ${outcome}${openable ? ' · 查看对局详情' : ''}`;
      return `<button class="insight-match is-${game.win ? 'win' : 'loss'}" type="button"${openable ? ` data-live-match="${gameID}" data-live-match-player="${h.escapeHTML(player.playerRef)}"` : ' disabled'} aria-label="${h.escapeHTML(label)}">${h.iconFigure('champion', game.championId, game.championName, 'tiny')}<b>${kda}</b>${tail}</button>`;
    }).join('');
  }

  // 战绩小卡弹窗（对齐 LeagueAkari 的 MatchPreviewer）。弹窗挂在 document.body 上，不在
  // 对局页的重绘范围里，自动刷新不会关掉或重画它；内容复用总览的战绩卡，默认展开详情。
  function ensureDialog(h) {
    const { state } = h;
    if (state.liveMatchDialog?.dialog?.isConnected) return state.liveMatchDialog;
    const dialog = document.createElement('dialog');
    dialog.className = 'career-dialog live-match-dialog';
    dialog.setAttribute('aria-labelledby', 'live-match-dialog-title');
    dialog.setAttribute('aria-describedby', 'live-match-dialog-context');
    dialog.innerHTML = '<section class="career-dialog-sheet"><header class="career-dialog-head"><div><h2 id="live-match-dialog-title">对局详情</h2><p id="live-match-dialog-context"></p></div><button class="dialog-close-button control-icon-button" type="button" aria-label="关闭对局详情" data-tooltip="关闭" data-close-live-match><svg class="control-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18"/></svg></button></header><div class="live-match-dialog-content"></div></section>';
    document.body.append(dialog);
    const entry = { dialog, title: dialog.querySelector('#live-match-dialog-title'), context: dialog.querySelector('#live-match-dialog-context'), content: dialog.querySelector('.live-match-dialog-content'), opener: null, view: null, token: 0 };
    dialog.querySelector('[data-close-live-match]').addEventListener('click', () => dialog.close());
    dialog.addEventListener('click', (event) => {
      if (event.target === dialog) { dialog.close(); return; }
      // 点参与者名字会打开玩家总览覆盖层：先关弹窗，覆盖层才不会被模态层挡住。
      if (event.target.closest?.('[data-player-ref]')) dialog.close();
    }, true);
    dialog.addEventListener('close', () => {
      entry.token++;
      state.controllers.get('live-match-dialog')?.abort();
      entry.view?.destroy?.();
      entry.view = null;
      entry.content.replaceChildren();
      window.desktopTheme?.setModalOpen?.(false);
      const opener = entry.opener;
      entry.opener = null;
      requestAnimationFrame(() => { if (opener?.isConnected && opener.getClientRects().length) opener.focus({ preventScroll: true }); });
    });
    state.liveMatchDialog = entry;
    return entry;
  }

  async function openMatch(button) {
    const h = helpers();
    const gameID = Number(button?.dataset?.liveMatch) || 0;
    const playerRef = String(button?.dataset?.liveMatchPlayer || '');
    if (!gameID || !playerRef) return;
    const entry = ensureDialog(h);
    const token = ++entry.token;
    entry.opener = button;
    entry.view?.destroy?.();
    entry.view = null;
    const row = button.closest('[data-live-player-row]');
    const card = row?.previousElementSibling?.matches?.('[data-live-player-row]') ? row.previousElementSibling : null;
    const name = card?.querySelector('.live-player-name')?.textContent?.trim() || '';
    entry.title.textContent = '对局详情';
    entry.context.textContent = name;
    entry.content.innerHTML = '<div class="live-match-dialog-loading" aria-label="正在读取这场对局"><span class="live-history-skeleton"></span><span class="live-history-skeleton"></span><span class="live-history-skeleton"></span></div>';
    if (!entry.dialog.open) {
      entry.dialog.showModal();
      window.desktopTheme?.setModalOpen?.(true);
    }
    try {
      const match = await h.api(`/api/gameplay/live/match?gameId=${encodeURIComponent(gameID)}&player=${encodeURIComponent(playerRef)}`, {}, 'live-match-dialog', 15000);
      if (token !== entry.token || !entry.dialog.open) return;
      const subject = (match?.participants || []).find((item) => Number(item.participantId) === Number(match.subjectParticipantId));
      entry.title.textContent = String(match?.queueLabel || '对局详情');
      entry.context.textContent = [name, exactTime(match?.createdAt)].filter(Boolean).join(' · ');
      entry.content.innerHTML = '<div class="match-list live-match-dialog-list"></div>';
      const region = h.clientRegion();
      entry.view = h.mount(entry.content.firstElementChild, {
        key: `live-match:${gameID}`, matches: [match], playerRef: subject?.playerRef || '',
        region, serverId: region ? '' : h.state.status?.serverId || '',
        openMatchId: String(gameID),
      });
    } catch (error) {
      if (token !== entry.token || !entry.dialog.open || error?.name === 'AbortError') return;
      const message = String(error?.message || '这场对局的详情读取失败').trim().split('\n')[0].slice(0, 120);
      entry.content.innerHTML = `<div class="live-match-dialog-error" role="alert"><span>${h.escapeHTML(message)}</span><button class="text-button" type="button" data-retry-live-match>重试</button></div>`;
      entry.content.querySelector('[data-retry-live-match]')?.addEventListener('click', () => openMatch(button));
    }
  }

  return Object.freeze({ identityLine, arenaSummary, chips, squadCards, squadsMarkup, openMatch });
});
