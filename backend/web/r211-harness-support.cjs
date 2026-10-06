'use strict';
// Focused legacy harnesses compile the production dependencies introduced by R211.
// Respect injected dependencies; do not replace behavior with test-only stubs.
function expand(source, names, dependencies = {}) {
  const result = [...names];
  const helpers = ['overviewRenderCacheFields', 'resetOverviewRenderCache', 'reportOverviewCardReady', 'copySummonerText', 'masteryMarkProgress', 'masteryGradeBoxes', 'computeOverviewStreak', 'renderOverviewStreak', 'bindSummonerCopy', 'reportDirtyOverview', 'matchSubject', 'readMatchScores', 'championTableNumber', 'championTableSortedRows', 'renderChampionTable', 'bindChampionTable', 'renderMatchTags', 'renderMultiKillTag', 'matchKeywordMetadata', 'matchTimelineKey', 'hydrateVisibleMatchTags', 'matchTagsBackgroundAllowed', 'matchTierScrollRoot', 'matchTierNodeIsVisible'];
  const {extract} = require('./r188-harness.cjs');
  for (let changed = true; changed;) {
    changed = false;
    const bodies = result.map(name => extract(source, name)).join('\n');
    for (const helper of helpers) if (!result.includes(helper) && !dependencies[helper] && new RegExp(`\\b${helper}\\b`).test(bodies)) {
      result.push(helper); changed = true;
    }
  }
  return result;
}
module.exports = {expand};
