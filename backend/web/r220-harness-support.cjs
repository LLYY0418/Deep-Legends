'use strict';
// Focused legacy harnesses need the actual platform helpers introduced by R220.
const {extract} = require('./r188-harness.cjs');
function prelude(source, dependencies = {}) {
  const constants = ['RIOT_REGIONS','RIOT_REGION_LABELS'].map(name => source.match(new RegExp(`const ${name} = [^\\n]+`))?.[0] || '').join('\n');
  const helpers = ['scheduleQuotaRetry','historyRecoveryActive','cancelHistoryRecovery','serviceOutageIcon','historyServiceState','renderHistoryServiceStatus','historyStatsPending','riotTab','savePlayerScroll','relaySlowState','renderRelaySlowNotice','retryCollectionAfterConnection','scheduleHistoryServerRetry','visiblePlayerTabs','scheduleCatalogViews','resetPlayerHistoryConditions','cancelAdvancedMatchSearch','overviewDetailArrow','maskedPlayerName','playerLabel','recordBuildPlayerSelection','resetBuildPlayerSelection','disposeMatchRowHeight','observeMatchRowHeight','selfTabReady','updateClientView','applyClientViewStatus','acceptClientView','renderClientConnection','riotRegion','clientRegion','isRiotSearchRegion','practicePlayerPosition','livePositionValue','livePositionDisplay'].filter(name => !dependencies[name] && source.includes(`function ${name}(`)).map(name => extract(source,name)).join('\n');
  return constants+'\nlet multikillGradientSeq = 0; let searchClientPlatform = "";\n'+helpers+'\n';
}
module.exports={prelude};
