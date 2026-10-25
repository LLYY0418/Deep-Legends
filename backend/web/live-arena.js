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
    if (!['rift', 'aram', 'arena', 'none'].includes(kind) || player.hidden === true && !['rift','aram'].includes(kind)) return span(legacyCopy, false);
    if (kind === 'none') return '';
    if (kind === 'arena') {
      const fame = Number(player.arenaFame?.level) > 0 ? player.arenaFame : null;
      if (!fame) return '';
      const tier = tiers[String(fame.tier || '').toUpperCase()] || '';
      let tooltip=`斗魂等级 ${h.number(fame.level)} · 名望 ${h.number(fame.fame)}`;
      if(fame.source==='progression_event'){tooltip+='\n截至上一局结束';if(Number.isInteger(fame.lastDelta))tooltip+=`\n上一局 ${fame.lastDelta>=0?'+':''}${h.number(fame.lastDelta)}`;}
      return `<span class="arena-fame${tier}" data-tooltip="${h.escapeHTML(tooltip)}" data-tooltip-size="compact"><svg class="arena-fame-icon" viewBox="0 0 16 16" aria-hidden="true"><path d="M8 1.5 13.5 4.6v6.8L8 14.5 2.5 11.4V4.6Z"/><path class="arena-fame-blades" d="m5.4 5.4 5.2 5.2M10.6 5.4 5.4 10.6"/></svg><b>${h.number(fame.level)}级</b><span class="arena-fame-divider" aria-hidden="true">·</span><span>${h.number(fame.fame)} 名望</span></span>`;
    }
    const valid=value=>value?.tier && /^(IRON|BRONZE|SILVER|GOLD|PLATINUM|EMERALD|DIAMOND|MASTER|GRANDMASTER|CHALLENGER)$/i.test(value.tier);
    if(!player.hidden && ['rift','aram'].includes(kind)) {
      const rankHTML=(rank,label)=>{
        const tier=String(rank?.tier || '').toLowerCase(),known=valid(rank);
        const title=(label?label+' ':'')+(known?h.rankTitle(rank):'未定级');
        return {copy:title,html:(known?`<img class="live-rank-crest" src="/rank-crests/${tier}.png" alt="" decoding="async">`:'')+`<b class="live-rank-title">${h.escapeHTML(title)}</b>`};
      };
      const position=kind==='rift' && player.position?String(legacyCopy).split(' · ')[0]:'';
      const ranks=kind==='rift'?[rankHTML(player.soloRank,'单双'),rankHTML(player.flexRank,'组排')]:[rankHTML(player.soloRank,'')];
      const copy=[position,...ranks.map(r=>r.copy)].filter(Boolean).join(' · ');
      return `<span class="live-player-rank" data-tooltip="${h.escapeHTML(copy)}" data-tooltip-overflow="self" data-tooltip-size="compact">${position?h.escapeHTML(position)+' · ':''}${ranks.map(r=>r.html).join(' · ')}</span>`;
    }
    const rank=[player.soloRank,player.flexRank,player.rank].find(valid);
    if(player.hidden===true && !rank)return '';
    const tier=String(rank?.tier||'').toLowerCase();
    const flex=rank && (rank===player.flexRank || rank.queueType==='RANKED_FLEX_SR');
    const label=rank?`${flex?'灵活 ':''}${h.rankTitle(rank)}${['master','grandmaster','challenger'].includes(tier)?' '+h.number(rank.leaguePoints)+' LP':''}`:'未定级';
    const position=kind==='rift' && !player.hidden?String(legacyCopy).split(' · ')[0]:'';
    return `<span class="live-player-rank">${position?h.escapeHTML(position)+' · ':''}${rank?`<img class="live-rank-crest" src="/rank-crests/${tier}.png" alt="" decoding="async">`:''}<b class="live-rank-title">${h.escapeHTML(label)}</b></span>`;
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

  function aramKitCombos(data,source,dependencies) {
    const {liveAugmentRecommendationSource,liveRecommendationsFor,renderLiveAugmentIcon,escapeHTML,rate,number}=dependencies;
    return renderLiveARAMKitCombos(data,source);
function renderLiveARAMKitCombos(data,source) {
    if(source!=="hextech")return "";
    const rows=(liveRecommendationsFor(data)?.augmentCombinations||[]).filter(row=>row.assets?.length===2&&Number(row.games)>=200).slice(0,3);
    if(!rows.length)return "";
    return `<section class="live-aramkit-combos"><h4>推荐组合 <small>前 ${rows.length} 组</small></h4><div class="live-aramkit-combo-grid">${rows.map(row=>`<article class="live-aramkit-combo"><div class="live-combo-pair">${row.assets.map(asset=>renderLiveAugmentIcon({assets:[asset]},source)).join("")}</div><div class="live-combo-names"><strong>${escapeHTML(row.assets[0].name)}</strong><strong><span class="live-combo-plus">＋</span> ${escapeHTML(row.assets[1].name)}</strong></div><div class="live-combo-metrics"><b class="win-rate-value">${rate(row.winRate)}</b><small>${number(row.games)} 场</small></div></article>`).join("")}</div></section>`;
  }
  }
function renderLiveRecentPositions(player, queueID) {
    if (![420, 440].includes(Number(queueID))) return "";
    const labels = { top: "上路", jungle: "打野", middle: "中路", bottom: "下路", utility: "辅助" };
    const positions = [...new Set((Array.isArray(player.recentPositions) ? player.recentPositions : [])
      .filter((item) => item && Number(item.games) > 0 && Object.hasOwn(labels, item.position))
      .map((item) => labels[item.position]))];
    if (!positions.length) return "";
    return `<div class="live-recent-positions" aria-label="常用位置"><span class="live-position-label">常用位置：</span>${positions.map((label) => `<span class="live-position-sample">${label}</span>`).join("")}</div>`;
  }
function liveConfidenceText(notice) {
    const low = Number(notice?.confidenceLow);
    const high = Number(notice?.confidenceHigh);
    if (!Number.isFinite(low) || !Number.isFinite(high) || (low <= 0 && high <= 0)) return "";
    return `上游置信区间 ${(low * 100).toFixed(1)}%–${(high * 100).toFixed(1)}%`;
  }
  function liveEvidenceSuffix(notice) {
    const evidence = String(notice?.evidence || "").trim();
    if (!evidence) return "上游未标注证据等级";
    if (evidence !== "supported") return `上游证据等级：${evidence}（未达显著性）`;
    return "";
  }
  function createRoster(dependencies) {
    const {state,liveAugmentRecommendationSource,ensureMayhemHeroRankings,orderLivePlayers,clusterPremadePlayers,recordLiveRosterRendered,isSummonersRiftMatch,insightTeamLayout,renderLiveRosterNoticeBars,renderLiveTeamPortraitTags,isARAMRelatedMatch,escapeHTML,renderLiveRecentPositions,practicePlayerPosition,champSelectEnemyPlaceholder,renderInsightMatches,arenaLivePlayerGroups,iconFigure,liveDisplayedChampionId,maskedPlayerName,liveBraveryPortrait,liveHistoryStateOf,liveHistorySettled,rankTitle,positionLabel,liveTagVisible,renderLivePremadeTag,number,percent,kda,proBadgeAttributes,renderMayhemHeroBadge,renderProIdentityBadge}=dependencies;
function renderLivePlayer(player, index, arenaMode = false, currentChampionId = 0, premadePlayers = [], recentPositions = "", aramMode = false, hextechMode = false, phase = "", displayPosition = "", identityKind = "") {
    if (champSelectEnemyPlaceholder(player, phase, premadePlayers)) {
      return `<article class="live-player">${iconFigure("champion", liveDisplayedChampionId(player, currentChampionId), player.championName, "live")}<div class="live-player-copy live-player-placeholder">暂无玩家信息，进入游戏后显示</div></article>`;
    }
    const rank = player.rank;
    const games = Array.isArray(player.recentGames) ? player.recentGames.slice(0, 10) : [];
    const recordGames = games.length;
    const wins = games.filter(game => game.win === true).length;
    const totals = games.reduce((total, game) => ({kills: total.kills + Number(game.kills || 0), deaths: total.deaths + Number(game.deaths || 0), assists: total.assists + Number(game.assists || 0)}), {kills: 0, deaths: 0, assists: 0});
    const displayStats = {games: recordGames, wins, losses: recordGames - wins, winRate: recordGames ? wins * 100 / recordGames : 0, kda: (totals.kills + totals.assists) / Math.max(1, totals.deaths)};
    const displayName = maskedPlayerName(player, index);
    const championId = phase !== "ChampSelect" && Number(player.championId)>0 ? Number(player.championId) : liveDisplayedChampionId(player, currentChampionId);
    const portrait = arenaMode && liveBraveryPortrait(player, arenaMode, phase) ? `<span class="game-icon is-live" title="勇敢举动" data-tooltip="勇敢举动"><img src="/bravery.svg" alt="勇敢举动"></span>` : iconFigure("champion",championId,player.championName,"live");
	const historyState = liveHistoryStateOf(player);
	const historyPending = !liveHistorySettled(player);
    const rankCopy = rank?.tier ? rankTitle(rank) : "未定级";
    const contextCopy = player.hidden === true ? (player.position ? positionLabel(player.position) : "") : arenaMode || aramMode ? "" : `${positionLabel(displayPosition || player.position)} · ${rankCopy}`;
    const liveArena = identityKind ? globalThis.deepLegendsLiveArena : null;
    const rowTone = (player.isCurrent ? " is-self" : player.isAlly ? " is-ally" : "") + (player.hidden === true ? " is-hidden" : "");
    const hiddenChip = !liveTagVisible("hidden") ? "" : `${player.privateHistory === true ? '<span class="player-tab-hidden">隐藏战绩</span>' : ""}${player.hidden === true ? '<span class="player-tab-hidden">隐藏身份</span>' : player.identityUnresolved === true ? '<span class="player-tab-hidden">身份待公开</span>' : ""}`;
    const liveAutofillChip = liveTagVisible("autofill") && player.autofill === true ? '<span class="match-autofill-chip">补位</span>' : "";
    const premadeTag = !liveTagVisible("premade") ? "" : renderLivePremadeTag(player, premadePlayers, currentChampionId);
	const emptySummary = historyState === "unavailable" ? player.identityUnresolved === true ? "身份尚未公开" : "客户端未公开该玩家" : historyState === "failed" ? "读取失败" : "本模式暂无战绩";
	const historySummary = historyPending
	  ? '<dl class="is-history-pending" aria-label="战绩读取中"><div><span class="live-history-skeleton"></span></div><div><span class="live-history-skeleton"></span></div><div><span class="live-history-skeleton"></span></div></dl>'
	  : liveArena?.arenaSummary(player, identityKind, emptySummary) || `<dl><div><dt>${recordGames ? `近 ${number(recordGames)} 局` : "当前模式"}</dt><dd class="live-record-value">${displayStats.games ? `${number(displayStats.wins)}胜 ${number(displayStats.losses)}负` : emptySummary}</dd></div>${player.hidden === true && !displayStats.games ? "" : `<div><dt>胜率</dt><dd class="win-rate-value">${displayStats.games ? percent(displayStats.winRate) : "—"}</dd></div><div><dt>KDA</dt><dd class="live-kda-value">${displayStats.games ? `${kda(displayStats.kda)}:1` : "—"}</dd></div>`}</dl>`;
	return `<article class="live-player${rowTone}">${portrait}<div class="live-player-copy"><div class="live-player-identity"><button class="live-player-name" type="button" ${player.playerRef ? `data-player-ref="${escapeHTML(player.playerRef)}" ${proBadgeAttributes(player)}` : "disabled"} data-tooltip="${escapeHTML(displayName)}" data-tooltip-overflow="self" data-tooltip-size="compact">${escapeHTML(displayName)}</button>${liveTagVisible("self") && player.isCurrent ? '<span class="self-chip">自己</span>' : ""}${hiddenChip}${hextechMode?renderMayhemHeroBadge(player,championId,true,phase):""}${liveAutofillChip}${premadeTag}${liveTagVisible("squad") && player.mySquad && !(identityKind === "arena" && liveArena) ? '<span class="my-squad-chip">我的小队</span>' : ""}${liveTagVisible("pro") ? renderProIdentityBadge(player) : ""}</div>${liveArena ? liveArena.identityLine(player, identityKind, contextCopy) : contextCopy ? `<span>${phase!=='ChampSelect' || player.hidden || arenaMode || aramMode?escapeHTML(contextCopy):`${escapeHTML(positionLabel(displayPosition || player.position))} · <b class="live-rank-title">${escapeHTML(rankCopy)}</b>`}</span>` : ""}${liveTagVisible("positions") ? recentPositions : ""}</div>${historySummary}</article>`;
  }
function renderLiveInsights(data, payload) {
    if(liveAugmentRecommendationSource(data)==="hextech")ensureMayhemHeroRankings(data);
    const seenPlayerKeys = new Set();
    let duplicatesDropped = 0;
    const players = (data.players || []).filter((player, index) => {
      const key = String(player.playerRef || `${player.teamId}:${player.cellId ?? index}`);
      if (seenPlayerKeys.has(key)) { duplicatesDropped++; return false; }
      seenPlayerKeys.add(key); return true;
    });
    if (duplicatesDropped) {
      const diagnosticKey = `${data.gameId || 0}:${data.phase || ""}:${duplicatesDropped}`;
      state.liveDuplicateDiagnostics ||= new Set();
      if (!state.liveDuplicateDiagnostics.has(diagnosticKey)) { state.liveDuplicateDiagnostics.add(diagnosticKey); window.reportFlowDiagnostic?.("live_roster_duplicate_dropped", "deduplicated", { gameId: Number(data.gameId || 0), queueId: Number(data.queueId || 0), count: duplicatesDropped }); }
    }
    if (!players.length) return '<div class="recommendation-empty"><strong>暂无玩家数据</strong><p>进入英雄选择或对局后，这里会展示自己、队友与对手的队伍信息与最近战绩。</p></div>';
		const arenaMode = liveAugmentRecommendationSource(data) === "arena";
		const basePlayers = orderLivePlayers(players, !arenaMode);
		const orderedPlayers = arenaMode ? [
          ...basePlayers.filter(player => player.isCurrent),
          ...basePlayers.filter(player => !player.isCurrent && player.mySquad),
          ...clusterPremadePlayers(basePlayers.filter(player => !player.isCurrent && !player.mySquad)),
        ] : basePlayers;
	    recordLiveRosterRendered(data, orderedPlayers.filter((player) => player.teamId === 100).length, orderedPlayers.filter((player) => player.teamId === 200).length);
    const selfPlayer = orderedPlayers.find((player) => player.isCurrent && [100, 200].includes(Number(player.teamId)));
    const selfTeam = Number(selfPlayer?.teamId) || 100;
    const foeTeam = selfTeam === 200 ? 100 : 200;
    const relativeTeamsKnown = Boolean(selfPlayer);
    const positionAligned = !arenaMode && isSummonersRiftMatch(data);
    const alignment = insightTeamLayout(orderedPlayers, positionAligned);
    if (!arenaMode && !positionAligned) {
      for (const teamID of [100, 200]) alignment.teams.set(teamID, clusterPremadePlayers(alignment.teams.get(teamID)));
    }
    const headerNotice = !relativeTeamsKnown ? "无法确定你所在阵营，按蓝方/红方展示" : alignment.reason;
    const bars = renderLiveRosterNoticeBars(payload, players);
    const portraitTags = renderLiveTeamPortraitTags(payload);
    const supplement = bars ? `<div class="live-roster-supplement">${bars}</div>` : "";
    const liveGameMode = String(data.gameMode || "").toUpperCase();
    const identityKind = arenaMode ? "arena" : isARAMRelatedMatch(data) ? "aram" : isSummonersRiftMatch(data) && ["", "CLASSIC", "PRACTICETOOL"].includes(liveGameMode) ? "rift" : "none";
    const playerRows = (source) => source.map((player, index) => {
        const key = escapeHTML(String(player.playerRef || `${player.teamId}:${player.cellId ?? index}`));
        const card = renderLivePlayer(player, index, arenaMode, data.currentChampionId, orderedPlayers, renderLiveRecentPositions(player, data.queueId), isARAMRelatedMatch(data), liveAugmentRecommendationSource(data) === "hextech", data.phase, liveGameMode === "PRACTICETOOL" ? practicePlayerPosition(player, data) : "", identityKind)
          .replace('<article ', `<article data-live-player-row="card:${key}"${data.phase==='ChampSelect'?' data-champselect-insight':''} `);
        const history = champSelectEnemyPlaceholder(player, data.phase, orderedPlayers) ? '<div class="insight-match-row"></div>' : renderInsightMatches(player);
        return card + history.replace('<div ', `<div data-live-player-row="history:${key}" `);
      }).join("");
    const team = (teamID, label, tone, source = orderedPlayers, extra = "") => {
      const rows = playerRows(source.filter((player) => teamID === null || player.teamId === teamID));
      return `<section class="live-team is-${tone}"><header><h3>${escapeHTML(label)}</h3><span>${escapeHTML(headerNotice || "当前模式最新战绩")}</span>${extra}</header><div class="live-player-list is-insight">${rows || '<p class="section-empty">客户端暂未公开这一队的玩家</p>'}</div></section>`;
    };
		if (arenaMode) {
		  const groups = arenaLivePlayerGroups(data, orderedPlayers);
		  if (groups.length >= 2) {
		    return `<div class="live-teams is-insight is-arena is-grouped">${groups.map((group) => team(null, group.label, "arena", group.players)).join("")}</div>`;
		  }
		  const title = data.phase === "ChampSelect" ? "己方小队" : "全部玩家";
		  return globalThis.deepLegendsLiveArena?.squadsMarkup(orderedPlayers, data.arenaSquadSize, playerRows) || `<div class="live-teams is-insight is-arena">${team(null, title, "arena")}</div>`;
		}
    if (!relativeTeamsKnown) return `${supplement}<div class="live-teams is-insight">${team(100, "蓝方", "blue", alignment.teams.get(100))}${team(200, "红方", "red", alignment.teams.get(200))}</div>`;
    return `${supplement}<div class="live-teams is-insight">${team(selfTeam, "我方", "blue", alignment.teams.get(selfTeam), portraitTags)}${team(foeTeam, "对方", "red", alignment.teams.get(foeTeam))}</div>`;
  }
    return {renderLivePlayer,renderLiveInsights};
  }
  return Object.freeze({ createRoster,liveEvidenceSuffix,liveConfidenceText,aramKitCombos,renderLiveRecentPositions,identityLine, arenaSummary, chips, squadCards, squadsMarkup, openMatch });
});
