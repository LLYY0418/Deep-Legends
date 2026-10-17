(() => {
  "use strict";
  const ranked = id => [420, 440].includes(Number(id));
  const mode = match => [2300, 2400, 3270].includes(Number(match.queueId)) ? "2300" : String(match.queueId);
  function select(data, tab) {
    let rows = data.matches || [];
    const cached = tab.overviewMatchSnapshot?.matches || [];
    const conflict = [...rows, ...cached].some(m => ranked(m.queueId)) || ["420", "440"].some(key => Number(data.rankedQueues?.[key]?.seasonGames) > 0 || Number(data.rankedQueues?.[key]?.recentRanked?.games) > 0 || Number(data.seasonQueueStats?.[key]?.games) > 0);
    if (conflict) { tab.rankedHistorySeen=true; delete tab.nonRankedConfirmed; return null; }
    if(tab.rankedHistorySeen)return null;
    const ranks = data.ranks;
    const ready = !tab.loading && !tab.initialPagePending && !tab.initialPageError && !tab.error && !data.capabilities?.some(c=>c.name==="match-history" && c.state==="failed") && data.capabilities?.some(c => c.name === "ranked-stats" && c.state === "available") && Array.isArray(ranks) && ranks.filter(r => ["RANKED_SOLO_5X5", "RANKED_FLEX_SR"].includes(String(r.queueType).toUpperCase())).every(r => r.wins === 0 && r.losses === 0 && Number(r.winRate) >= 0);
    if (!ready) return null;
    tab.nonRankedConfirmed = true;
    const seen=new Set();
    rows=[...rows,...(tab.casualHistory || [])].filter(m=>{const id=String(m.gameId);if(seen.has(id))return false;seen.add(id);return true;}).sort((a,b)=>Number(b.createdAt || 0)-Number(a.createdAt || 0)).slice(0,100);
    tab.casualHistory=rows;
    const groups = new Map();
    for (const match of rows) {
      if (ranked(match.queueId) || !(Number(match.queueId) > 0) || match.gameType === "CUSTOM_GAME" || match.gameMode === "PRACTICETOOL") continue;
      const key = mode(match);
      if (!groups.has(key)) groups.set(key, { key, label: key === "2300" ? "海斗" : key === "450" ? "极地大乱斗" : ["1700", "1710"].includes(key) ? "斗魂竞技场" : match.queueLabel || "其他模式", matches: [] });
      groups.get(key).matches.push(match);
    }
    const modes = [...groups.values()].sort((a, b) => b.matches.length - a.matches.length || Number(a.key) - Number(b.key));
    const selected = modes.find(m => m.key === tab.casualMode) || modes[0];
    if (!selected) return { modes, selected: null, champions: [], overall: {}, stats: {} };
    tab.casualMode = selected.key;
    const samples = selected.matches.map(match => ({ match, player: match.participants?.find(p => Number(p.participantId) === Number(match.subjectParticipantId)) || match.participants?.find(p => p.playerRef && p.playerRef === data.player?.playerRef) })).filter(s => s.player);
    function aggregate(items) {
      const games = items.length, sum = key => items.reduce((n, s) => n + Number(s.player[key] || 0), 0), kills = sum("kills"), deaths = sum("deaths"), assists = sum("assists");
      const wins = items.filter(s => s.match.result === "win").length;
      const minutes = items.reduce((n, s) => n + Number(s.match.duration || 0) / 60, 0);
      return { games, wins, losses: games - wins, winRate: games ? wins / games * 100 : 0, kills: kills / (games || 1), deaths: deaths / (games || 1), assists: assists / (games || 1), kda: (kills + assists) / Math.max(1, deaths), cs: sum("cs") / (games || 1), csPerMinute: minutes ? sum("cs") / minutes : 0 };
    }
    const champions = new Map();
    for (const sample of samples) {
      const id = sample.player.championId;
      if (!champions.has(id)) champions.set(id, []);
      champions.get(id).push(sample);
    }
    const championRows = [...champions.values()].map(items => ({ ...aggregate(items), championId: items[0].player.championId, championName: items[0].player.championName })).sort((a, b) => b.games - a.games || a.championId - b.championId);
    return { modes, selected, stats: aggregate(samples.slice(0, 20)), overall: aggregate(samples), champions: championRows };
  }
  window.deepLegendsOverviewCasual = { select };
  window.deepLegendsSections?.register("overview-casual",()=>{});
})();
