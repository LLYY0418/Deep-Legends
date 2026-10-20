'use strict';
// Focused legacy harnesses compile the production dependencies introduced by R211.
// Respect injected dependencies; do not replace behavior with test-only stubs.
function expand(source, names, dependencies = {}) {
  const result = [...names];
  const helpers = ['championDisplayName','mergeChampSelectFields','ensureFastClientRecommendation','noteChampSelectReady','noteChampSelectLatency','ensureHistoryFilterSearch','renderRuneSpellIcons','renderProSkillOrder','liveClientPositionsPending','champSelectEnemyPlaceholder','readSetting','applySidebar','preference','savePreference','liveTagVisible','privacyName','renderCollectionCounts','bindOverviewSubpageLinks','reportMatchFilterView','mergeMatchLP','matchContentVersion','renderLPChip','patchLPChip','retainFallbackFilterMatches','runPrestigeTasks','buildSubject','casualOverview','updateCareerCards','renderCollectionSkeleton','retainArenaBravery','liveBraveryPortrait','setRefreshBusy','resetCareerState','sidebarConstraint','cancelOverviewProgress','renderPendingMatchFilters','tabServerTitle','tabServerLabel','tabServerID','renderSummonerStrip','renderOverviewStreak','summonerRegionChip','summonerProChip','renderSummonerHighlights','highestCurrentRank','iconFigure','maskedProfileIcon','renderCareerSections','careerSectionEntries','rankedQueueData','rankedQueueSwitcher','rankedQueueLabel','rankedQueueNoun','isMayhemQueueId','finishOverviewLoad','renderCareerPendingMarkup','ensureScoreDetails','bindScoreDetails','scoreChip','readMatchScores','resolveOverviewFrame','reportClientView','boundMatchDOM','scheduleQuotaCountdown','scorePlacementChip','scoreRankChip','ordinalLabel','renderOverviewPendingPage','requestOverviewWithReferenceRetry','lootPendingAccount','recordTimelineClient','matchesPending','renderPendingMatches','reportDeferredMatchesCard','syncCollectionSession','scheduleQuotaRetry','historyRecoveryActive','cancelHistoryRecovery','historyServiceState','renderHistoryServiceStatus','historyStatsPending','serviceOutageIcon','riotTab','riotRegion','visiblePlayerTabs','savePlayerScroll','relaySlowState','renderRelaySlowNotice','retryCollectionAfterConnection','scheduleHistoryServerRetry','applyRenderedMetricStyles','scheduleCatalogViews','resetPlayerHistoryConditions','cancelAdvancedMatchSearch','disposeMatchRowHeight','observeMatchRowHeight','updateClientView','applyClientViewStatus','acceptClientView','renderClientConnection','selfTabReady','renderSeasonDataNote','renderRankWindowNote','bindSeasonDataNotes','syncOverviewStreak','bindMayhemRatingControls','mergeCurrentHistoryHead','updateMayhemRatingPopover','mayhemRatingStatus','renderRankMMRPopover','arenaPlacementResult','matchDataTags','fitMatchTags','stashMatchFilterDOM','restoreMatchFilterDOM','refreshMatchFilterHead','overviewDetailArrow','championTableQueueTabs','maskedPlayerName','playerLabel','recordBuildPlayerSelection','resetBuildPlayerSelection','overviewRenderCacheFields', 'resetOverviewRenderCache', 'reportOverviewCardReady', 'copySummonerText', 'masteryMarkProgress', 'masteryGradeBoxes', 'computeOverviewStreak', 'renderOverviewStreak', 'bindSummonerCopy', 'reportDirtyOverview', 'matchSubject', 'readMatchScores', 'championTableNumber', 'championTableSortedRows', 'renderChampionTable', 'bindChampionTable', 'renderMatchTags', 'renderMultiKillTag', 'matchKeywordMetadata', 'matchTimelineKey', 'hydrateVisibleMatchTags', 'matchTagsBackgroundAllowed', 'matchTierScrollRoot', 'matchTierNodeIsVisible'];
  const {extract} = require('./r188-harness.cjs');
  for (let changed = true; changed;) {
    changed = false;
    const bodies = result.map(name => extract(source, name)).join('\n');
    for (const helper of helpers) if (!result.includes(helper) && !dependencies[helper] && source.includes(`function ${helper}(`) && new RegExp(`\\b${helper}\\b`).test(bodies)) {
      result.push(helper); changed = true;
    }
  }
  return result;
}
function environment(dependencies={}) {
 const window=dependencies.window || dependencies.document?.defaultView || new EventTarget();
 const events=new EventTarget();
 for(const name of ['addEventListener','removeEventListener','dispatchEvent']) if(typeof window[name]!=='function')window[name]=events[name].bind(events);
 window.deepLegendsPrivacy ||= require('./privacy.js');
 window.Event ||= Event;
 return {CustomEvent:window.CustomEvent || CustomEvent,...dependencies,window};
}
module.exports = {expand,environment};
