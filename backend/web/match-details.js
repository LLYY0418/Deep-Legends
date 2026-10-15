/* R277: expanded match tables, team analysis and rune-effect rendering only.
 * Loaded through the same-origin section loader; no eval, inline scripts or CSS. */
(() => {
  'use strict';
  function create(dependencies) {
    const {state,readMatchScores,escapeHTML,number,kda,plainInteger,compactNumber,plainText,iconFigure,playerParticipantName,proBadgeAttributes,autofillChip,renderMatchScoreCell,renderItemIcons,matchPlayerGroups,isCurrentMatchParticipant,arenaTeamMeta,arenaPlacementResult,augmentIconFigure,arenaGroupLabel,matchModeKind,positionLabel,itemIconFigure,renderRuneStyleIcon,perkIconFigure,isSummonersRiftMatch,TEAM_ANALYSIS_METRICS,TEAM_ANALYSIS_POSITIONS,SKILL_SLOT_LETTERS,PERK_EFFECT_OVERRIDES} = dependencies;
function matchTableRows(players, scores, match) {
    return players.map((item, index) => {
      const record = scores.get(Number(item.participantId));
      const name = playerParticipantName(item, index);
      return `<tr><td><button class="participant-link" type="button" ${item.playerRef ? `data-player-ref="${escapeHTML(item.playerRef)}" ${proBadgeAttributes(item)}` : "disabled"} data-tooltip="${escapeHTML(name)}" data-tooltip-overflow=".participant-name" data-tooltip-size="compact">${iconFigure("champion", item.championId, item.championName, "small", false)}<span class="participant-name${isCurrentMatchParticipant(item, match) ? " is-current-player" : ""}">${escapeHTML(name)}</span>${autofillChip(item)}</button></td><td>${renderMatchScoreCell(record)}</td><td>${number(item.kills)} / ${number(item.deaths)} / ${number(item.assists)}<small>${kda(item.kda)}:1</small></td><td>${number(item.damage)}<small>承伤 ${number(item.damageTaken)}</small></td><td>${number(item.wardsPlaced)} / ${number(item.wardsKilled)}</td><td>${number(item.cs)}<small>${number(item.csPerMinute)}/分钟</small></td><td><div class="table-items">${renderItemIcons(item.itemIds || [], "small")}</div></td></tr>`;
    }).join("");
  }

function matchTableShell(rows) {
    return `<div class="table-scroll"><table class="match-table"><colgroup><col class="match-col-player"><col class="match-col-score"><col class="match-col-kda"><col class="match-col-damage"><col class="match-col-wards"><col class="match-col-cs"><col class="match-col-items"></colgroup><thead><tr><th>玩家</th><th>评分</th><th>KDA</th><th>伤害</th><th>守卫</th><th>CS</th><th>装备</th></tr></thead><tbody>${rows}</tbody></table></div>`;
  }

function renderMatchOverview(match) {
    const scores = readMatchScores(match);
    const grouping = matchPlayerGroups(match);
    // 斗魂竞技场：一队一个分组，按名次排序完整展示全部玩家。
    if (grouping.arena) {
      return renderArenaMatchOverview(match);
    }
    const teams = grouping.groups.map((group) => {
      const teamID = group.teamId;
      const team = (match.teams || []).find((item) => item.teamId === teamID) || {};
      return `<section class="team-overview is-${teamID === 100 ? "blue" : "red"}"><header><strong>${teamID === 100 ? "蓝方" : "红方"}${team.win ? " · 胜利" : ""}</strong><span>${number(team.kills)} 击杀 · ${number(team.gold)} 金币</span></header>${matchTableShell(matchTableRows(group.players, scores, match))}</section>`;
    }).join("");
    return `<div class="match-overview-teams">${teams}</div>`;
  }

function renderArenaMatchOverview(match) {
    const scores = readMatchScores(match);
    const grouping = matchPlayerGroups(match);
    const sections = grouping.groups.map((group) => {
      const team = arenaTeamMeta(group);
      const kills = group.players.reduce((sum, item) => sum + (Number(item.kills) || 0), 0);
      const damage = group.players.reduce((sum, item) => sum + (Number(item.damage) || 0), 0);
      const tone = group.placement === 1 ? " is-first" : arenaPlacementResult(match, group.placement) === "win" ? " is-top" : "";
      const emblem = team.iconPath ? `<span class="game-icon is-small"><img data-queued-src="${escapeHTML(team.iconPath)}" alt="" loading="lazy" decoding="async" data-game-image></span>` : "";
      const players = group.players.map((item, index) => {
        const record = scores.get(Number(item.participantId));
        const name = playerParticipantName(item, index);
		return `<div class="arena-detail-player"><button class="participant-link" type="button" ${item.playerRef ? `data-player-ref="${escapeHTML(item.playerRef)}" ${proBadgeAttributes(item)}` : "disabled"} data-tooltip="${escapeHTML(name)}" data-tooltip-overflow=".participant-name" data-tooltip-size="compact">${iconFigure("champion", item.championId, item.championName, "small", false)}<span class="participant-name${isCurrentMatchParticipant(item, match) ? " is-current-player" : ""}">${escapeHTML(name)}</span></button><span class="arena-detail-augments">${(item.augmentIds || []).slice(0, 6).map((id) => augmentIconFigure(id, "small")).join("")}</span>${renderMatchScoreCell(record)}<span class="arena-detail-kda"><b>${number(item.kills)} / ${number(item.deaths)} / ${number(item.assists)}</b><small>${kda(item.kda)}:1 KDA</small></span><span class="arena-detail-damage" aria-label="${escapeHTML(`伤害 ${plainInteger(item.damage)}，承伤 ${plainInteger(item.damageTaken)}`)}" data-tooltip="${escapeHTML(`伤害 ${number(item.damage)} · 承伤 ${number(item.damageTaken)}`)}" data-tooltip-size="compact"><b class="match-damage-value">${plainInteger(item.damage)}</b><i aria-hidden="true">/</i><b class="match-taken-value">${plainInteger(item.damageTaken)}</b></span><span class="arena-detail-items">${renderItemIcons(item.itemIds || [], "small")}</span></div>`;
      }).join("");
      return `<section class="arena-detail-team${tone}"><header><b>#${number(group.placement || "—")}</b>${emblem}<strong>${escapeHTML(team.name)}</strong><span>${number(kills)} 击杀 · ${compactNumber(damage)} 伤害</span></header><div class="arena-detail-player-list">${players}</div></section>`;
    }).join("");
	  return `<div class="arena-match-detail"><header><strong>最终排名</strong><span>按小队名次展示全部玩家、海克斯与装备</span></header><div class="arena-detail-columns" role="row"><span>玩家</span><span class="arena-column-augments">海克斯</span><span class="arena-column-score">评分</span><span>KDA</span><span class="arena-column-damage">伤害 / 承伤</span><span class="arena-column-items">装备</span></div>${sections}</div>`;
	}

function isARAMRelatedMatch(match) {
	  return ["aram", "mayhem", "mayhem-classic"].includes(matchModeKind(match));
	}

function teamAnalysisMetricsFor(match) {
	  const summonersRift = isSummonersRiftMatch(match);
	  const aram = isARAMRelatedMatch(match);
	  return TEAM_ANALYSIS_METRICS.filter((metric) => !(metric.summonersRiftOnly && !summonersRift) && !(metric.unavailableInARAM && aram));
	}

function teamAnalysisMembers(players) {
	  return [...players].sort((left, right) => {
		const leftPosition = TEAM_ANALYSIS_POSITIONS.indexOf(String(left.position || "").toLowerCase());
		const rightPosition = TEAM_ANALYSIS_POSITIONS.indexOf(String(right.position || "").toLowerCase());
		const leftOrder = leftPosition < 0 ? TEAM_ANALYSIS_POSITIONS.length : leftPosition;
		const rightOrder = rightPosition < 0 ? TEAM_ANALYSIS_POSITIONS.length : rightPosition;
		return leftOrder - rightOrder || Number(left.participantId) - Number(right.participantId);
	  });
	}

function teamAnalysisMetricValue(item, metric, teamKills) {
	  if (!item) return null;
	  if (metric.key === "killParticipation") {
		if (!(teamKills > 0)) return null;
		return Math.min(100, Math.round(((Number(item.kills) || 0) + (Number(item.assists) || 0)) * 1000 / teamKills) / 10);
	  }
	  if (metric.nullable && (item[metric.key] === null || item[metric.key] === undefined)) return null;
	  const value = Number(item[metric.key]);
	  return Number.isFinite(value) ? Math.max(0, value) : null;
	}

function teamAnalysisMetricLabel(value, metric) {
	  if (value === null) return "—";
	  return metric.percent ? `${number(value)}%` : number(value);
	}

function renderTeamAnalysis(match, tab) {
    const participants = match.participants || [];
    const sortKey = (team = "") => `${String(match.gameId)}:${String(team)}`;
    const metricFor = (team = "") => tab?.damageSorts?.get(sortKey(team)) === "damageTaken" ? "damageTaken" : "damage";
    const sortButton = (team = "", currentMetric = metricFor(team)) => {
      const label = currentMetric === "damageTaken" ? "承伤" : "伤害";
      const next = currentMetric === "damageTaken" ? "伤害" : "承伤";
      return `<button class="damage-sort-toggle" type="button" data-damage-sort="${currentMetric}" data-game-id="${match.gameId}" data-team="${escapeHTML(team)}" aria-label="当前按${label}排序，切换为按${next}排序" data-tooltip="当前按${label}排序\n点击切换为按${next}排序"><svg class="function-icon" aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" shape-rendering="geometricPrecision"><use href="/icons.svg#dl-icon-shape-aacd87806b"/></svg><span>${label}</span></button>`;
    };
    const damageRow = (item, index, tone, rowMetric, peak) => {
      const value = Math.max(0, Number(item[rowMetric]) || 0);
      const share = Math.round(value * 1000 / peak) / 10;
      const rowLabel = rowMetric === "damageTaken" ? "承伤" : "伤害";
      return `<div class="damage-row">${iconFigure("champion", item.championId, item.championName, "tiny", false)}<span class="damage-name" data-tooltip="${escapeHTML(playerParticipantName(item, index))}" data-tooltip-overflow="self" data-tooltip-size="compact">${escapeHTML(playerParticipantName(item, index))}</span><span class="damage-bar-line"><span class="damage-bar is-${tone}" role="img" aria-label="${escapeHTML(`${rowLabel} ${number(value)}，相对本组最高值 ${share}%`)}"><i data-bar-width="${value > 0 ? Math.max(1, share) : 0}"></i></span><b>${number(value)}</b></span></div>`;
    };
    const grouping = matchPlayerGroups(match);
    // 斗魂竞技场：红蓝双方总量对比无意义，改为按名次逐小队展示伤害与承伤。
    if (grouping.arena) {
      const sections = grouping.groups.map((group) => {
        const team = String(group.subteamId || group.placement || "arena");
        const groupMetric = metricFor(team);
        const groupLabel = groupMetric === "damageTaken" ? "承伤" : "伤害";
        const groupPeak = Math.max(1, ...group.players.map((item) => Number(item[groupMetric]) || 0));
        const tone = group.placement === 1 ? "blue" : "red";
        return `<section class="damage-team is-arena"><div class="damage-team-head"><h4>${escapeHTML(arenaGroupLabel(group))} · ${groupLabel}</h4>${sortButton(team, groupMetric)}</div>${[...group.players].sort((left, right) => (Number(right[groupMetric]) || 0) - (Number(left[groupMetric]) || 0) || Number(left.participantId) - Number(right.participantId)).map((item, index) => damageRow(item, index, tone, groupMetric, groupPeak)).join("")}</section>`;
      }).join("");
      return `<div class="damage-compare is-arena">${sections}</div>`;
    }
    const metrics = [["kills", "英雄击杀"], ["gold", "金币"], ["damage", "伤害"], ["visionScore", "视野分数"], ["damageTaken", "承伤"], ["cs", "小兵分数"]];
    const blue = (match.teams || []).find((team) => team.teamId === 100) || {};
    const red = (match.teams || []).find((team) => team.teamId === 200) || {};
    // 红蓝双色对峙条：左段蓝方、右段红方，长度即各自占比。
    const totals = `<div class="team-analysis-grid">${metrics.map(([key, label]) => {
      const left = Number(blue[key] || 0); const right = Number(red[key] || 0); const total = Math.max(1, left + right); const share = Math.round(left * 100 / total);
      return `<section class="team-metric"><div class="team-metric-head"><b class="team-metric-value is-blue">${number(left)}</b><h4>${label}</h4><b class="team-metric-value is-red">${number(right)}</b></div><div class="team-metric-track" role="img" aria-label="${label}：蓝方 ${share}%，红方 ${100 - share}%" data-tooltip="蓝方 ${share}% · 红方 ${100 - share}%" data-tooltip-size="compact"><i data-blue-share="${share}"></i></div></section>`;
    }).join("")}</div>`;
	  const gameID = String(match.gameId);
	  const availableMetrics = teamAnalysisMetricsFor(match);
	  const selectedKey = tab?.teamAnalysisMetrics?.get(gameID) || availableMetrics[0].key;
	  const selectedMetric = availableMetrics.find((metric) => metric.key === selectedKey) || availableMetrics[0];
	  const blueMembers = teamAnalysisMembers(participants.filter((item) => Number(item.teamId) === 100));
	  const redMembers = teamAnalysisMembers(participants.filter((item) => Number(item.teamId) === 200));
	  const teamKills = new Map([100, 200].map((teamID) => [teamID, participants.filter((item) => Number(item.teamId) === teamID).reduce((sum, item) => sum + (Number(item.kills) || 0), 0)]));
	  const values = participants.map((item) => teamAnalysisMetricValue(item, selectedMetric, teamKills.get(Number(item.teamId))));
	  const peak = Math.max(1, ...values.filter((value) => value !== null));
	  const metricTabs = `<div class="team-analysis-metrics" role="tablist" aria-label="队伍分析指标">${availableMetrics.map((metric) => {
		const active = metric.key === selectedMetric.key;
		return `<button id="team-analysis-metric-${metric.key}-${gameID}" type="button" role="tab" aria-selected="${active}" aria-controls="team-analysis-chart-${gameID}" tabindex="${active ? 0 : -1}" data-team-analysis-metric="${metric.key}" data-game-id="${gameID}">${metric.label}</button>`;
	  }).join("")}</div>`;
	  const playerCell = (item, index, tone) => item
		? `<span class="team-duel-player is-${tone}" data-tooltip="${escapeHTML(`${playerParticipantName(item, index)} · ${item.championName || "未知英雄"} · ${positionLabel(item.position)}`)}" data-tooltip-size="compact">${iconFigure("champion", item.championId, item.championName, "small", false)}</span>`
		: '<span class="team-duel-player is-empty" aria-hidden="true"></span>';
	  const valueCell = (item, tone) => {
		const value = teamAnalysisMetricValue(item, selectedMetric, teamKills.get(Number(item?.teamId)));
		return `<b class="team-duel-value is-${tone}">${teamAnalysisMetricLabel(value, selectedMetric)}</b>`;
	  };
	  const bar = (item, tone) => {
		const value = teamAnalysisMetricValue(item, selectedMetric, teamKills.get(Number(item?.teamId)));
		const share = value === null ? 0 : Math.round(value * 1000 / peak) / 10;
		const label = item ? `${item.championName || "未知英雄"}${selectedMetric.label} ${teamAnalysisMetricLabel(value, selectedMetric)}` : `${tone === "blue" ? "蓝方" : "红方"}该位置无数据`;
		return `<span class="team-duel-half is-${tone}" role="img" aria-label="${escapeHTML(label)}"><i data-bar-width="${value > 0 ? Math.max(1, share) : 0}"></i></span>`;
	  };
	  const rowCount = Math.max(blueMembers.length, redMembers.length);
	  const rows = Array.from({ length: rowCount }, (_, index) => {
		const bluePlayer = blueMembers[index]; const redPlayer = redMembers[index];
		return `<div class="team-duel-row">${playerCell(bluePlayer, index, "blue")}${valueCell(bluePlayer, "blue")}<span class="team-duel-bars">${bar(bluePlayer, "blue")}<i aria-hidden="true"></i>${bar(redPlayer, "red")}</span>${valueCell(redPlayer, "red")}${playerCell(redPlayer, index, "red")}</div>`;
	  }).join("");
	  const contribution = `<section class="team-contribution">${metricTabs}<div id="team-analysis-chart-${gameID}" class="team-duel-chart" role="tabpanel" aria-labelledby="team-analysis-metric-${selectedMetric.key}-${gameID}"><header class="team-duel-head"><b class="is-blue">蓝方</b><strong>${selectedMetric.label}</strong><b class="is-red">红方</b></header>${rows}</div></section>`;
	  return totals + contribution;
	}

function renderItemRoute(timeline) {
    const groups = timeline?.itemGroups || [];
    if (!groups.length) return "";
    return `<div class="timeline-route">${groups.map((group) => `<div class="timeline-group"><div class="timeline-group-items">${(group.events || []).map((event) => `<span class="timeline-item${event.sold ? " is-sold" : ""}"${event.sold ? ' data-tooltip="已出售" data-tooltip-size="compact"' : ""}>${itemIconFigure(event.itemId, "slot")}</span>`).join("")}</div><small>${number(group.minute)}分</small></div>`).join('<span class="timeline-arrow" aria-hidden="true"><svg class="function-icon" viewBox="0 0 24 24" aria-hidden="true"><use href="/icons.svg#dl-icon-chevron-right"/></svg></span>')}</div>`;
  }

function skillPrioritySummary(order) {
    const counts = { 1: 0, 2: 0, 3: 0 };
    const maxedAt = {};
    order.forEach((up, index) => {
      if (up.slot < 1 || up.slot > 3) return;
      counts[up.slot] += 1;
      if (counts[up.slot] === 5 && maxedAt[up.slot] === undefined) maxedAt[up.slot] = index;
    });
    const ranked = [1, 2, 3].filter((slot) => counts[slot] > 0)
      .sort((left, right) => (maxedAt[left] ?? 99) - (maxedAt[right] ?? 99) || counts[right] - counts[left]);
    if (ranked.length < 2) return "";
    return `<span class="match-skill-priority">主升 <b class="is-slot-${ranked[0]}">${SKILL_SLOT_LETTERS[ranked[0]]}</b> · 副升 <b class="is-slot-${ranked[1]}">${SKILL_SLOT_LETTERS[ranked[1]]}</b></span>`;
  }

function renderSkillOrder(timeline) {
    const order = timeline?.skillOrder || [];
    if (!order.length) return "";
    const cells = order.map((up) => `<span class="match-skill-cell is-slot-${up.slot}" data-tooltip="第 ${up.level} 级：升级 ${SKILL_SLOT_LETTERS[up.slot] || "?"}" data-tooltip-size="compact"><b>${SKILL_SLOT_LETTERS[up.slot] || "?"}</b><small>${up.level}</small></span>`).join("");
    return `<div class="match-skill-seq">${cells}</div>`;
  }

function perkEffectLines(perk, vars) {
    if (!Array.isArray(vars) || vars.length < 3) return [];
    const templates = Object.hasOwn(PERK_EFFECT_OVERRIDES, Number(perk?.id)) ? PERK_EFFECT_OVERRIDES[Number(perk.id)] : perk?.eogDescs || [];
    const lines = [];
    for (const template of templates) for (const raw of String(template).split(/<br\s*\/?>/gi)) {
      const match = raw.trim().match(/^([^：:]+)[：:]\s*(.+)$/);
      if (!match) continue;
      const label = plainText(match[1]).replace(/^提供的/, "").replace(/(?:的总和|总和|总计)$/, "").trim();
      const valueTemplate = match[2].trim();
      const integer = valueTemplate.match(/^@eogvar([123])@$/i);
      const time = /^@eogvar1@:@eogvar2@$/i.test(valueTemplate);
      const digitTime = /^@eogvar1@:@eogvar2@@eogvar3@$/i.test(valueTemplate);
      let value;
      if (integer) value = Number(vars[Number(integer[1]) - 1]);
      else if (time && Number.isSafeInteger(Number(vars[0])) && Number.isSafeInteger(Number(vars[1])) && vars[0] >= 0 && vars[1] >= 0 && vars[1] < 60) value = `${Number(vars[0])}:${String(Number(vars[1])).padStart(2, "0")}`;
      else if (digitTime && vars.slice(0, 3).every(value => Number.isSafeInteger(value)) && vars[0] >= 0 && vars[1] >= 0 && vars[1] <= 9 && vars[2] >= 0 && vars[2] <= 9) value = `${vars[0]}:${vars[1]}${vars[2]}`;
      else continue;
      if (!label || (typeof value === "number" && !Number.isSafeInteger(value))) continue;
      const kind = time || digitTime ? "other" : /伤害/.test(label) ? "damage" : /治疗|回复|护盾/.test(label) ? "heal" : /金币/.test(label) ? "gold" : "other";
      lines.push({ label, value, kind });
    }
    return lines;
  }

function selectedRuneEffects(subject) {
    if (!state.perks) return null;
    const selected = new Set((subject.perkIds || []).map(Number));
    const stats = new Map((subject.perkStats || []).map(item => [Number(item.perkId), item.vars]));
    const catalog = new Map((state.perks.perks || []).map(item => [Number(item.id), item]));
    const styles = state.perks.styles || [];
    const rows = [];
    for (const [styleID, secondary] of [[subject.primaryStyleId, false], [subject.subStyleId, true]]) {
      const style = styles.find(item => Number(item.id) === Number(styleID));
      const slots = secondary ? (style?.slots || []).slice(1) : style?.slots || [];
      slots.forEach((slot, index) => {
        for (const entry of slot.perks || []) if (selected.has(Number(entry.id))) {
          const perk = { ...entry, ...(catalog.get(Number(entry.id)) || {}) };
          rows.push({ perk, style, secondary, keystone: !secondary && index === 0, lines: perkEffectLines(perk, stats.get(Number(perk.id))) });
        }
      });
    }
    return rows;
  }

function runeEffectTotals(rows) {
    const totals = { damage: 0, heal: 0, gold: 0 };
    for (const row of rows || []) for (const kind of Object.keys(totals)) {
      totals[kind] += Math.max(0, ...row.lines.filter(line => line.kind === kind && typeof line.value === "number").map(line => line.value));
    }
    return totals;
  }

function renderRuneYield(subject) {
    const totals = runeEffectTotals(selectedRuneEffects(subject));
    const chips = [["damage", "伤害"], ["heal", "治疗"], ["gold", "金币"]].filter(([kind]) => totals[kind] > 0).map(([kind, label]) => `<span class="is-${kind}">${label}<b>${totals[kind].toLocaleString("en-US")}</b></span>`).join("");
    return chips ? `<div class="rune-yield">${chips}</div>` : "";
  }

function renderRuneEffects(subject) {
    const rows = selectedRuneEffects(subject);
    if (!rows) return "";
    let secondaryStarted = false;
    const content = rows.map(row => {
      let divider = "";
      if (row.secondary && !secondaryStarted) { secondaryStarted = true; divider = `<div class="eff-divider">${renderRuneStyleIcon(row.style)}<span>${escapeHTML(row.style.name)}</span></div>`; }
      const description = !row.lines.length ? plainText(row.perk.shortDesc).replace(/\s+/g, " ") : "";
      const stats = row.lines.map(line => `<div class="stat"><b>${escapeHTML(typeof line.value === "number" ? line.value.toLocaleString("en-US") : line.value)}</b><span>${escapeHTML(line.label)}</span></div>`).join("");
      const subtitle = [row.keystone ? row.style.name : "", description].filter(Boolean).join(" · ");
      return `${divider}<div class="eff${row.keystone ? " is-keystone" : ""}" data-perk-id="${Number(row.perk.id)}" tabindex="0">${perkIconFigure(row.perk.id, "rune", false)}<div class="name"><strong>${escapeHTML(row.perk.name)}${row.keystone ? '<span class="kchip">基石</span>' : ""}</strong>${subtitle ? `<small>${escapeHTML(subtitle)}</small>` : ""}</div>${stats ? `<div class="stats">${stats}</div>` : ""}</div>`;
    }).join("");
    return `<div class="rune-effects" aria-label="符文效果">${content}</div>`;
  }
    return Object.freeze({renderMatchOverview,renderArenaMatchOverview,renderTeamAnalysis,renderItemRoute,skillPrioritySummary,renderSkillOrder,renderRuneYield,renderRuneEffects});
  }
  window.deepLegendsMatchDetails=Object.freeze({create});
  window.deepLegendsSections.register('match-details',()=>{});
})();
