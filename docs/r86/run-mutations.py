import os, json, subprocess, tempfile
from pathlib import Path
root=Path(__file__).resolve().parents[2]
environment={**os.environ,'GOCACHE':os.environ.get('GOCACHE',str(root/'.gocache')),'GOTMPDIR':os.environ.get('GOTMPDIR',tempfile.gettempdir())}
cases=[
 ('P1-3 pool bypass','champions.go','\treturn &http.Client{\n\t\tTimeout:   12 * time.Second,','\ttransport.MaxIdleConnsPerHost = 2\n\treturn &http.Client{\n\t\tTimeout:   12 * time.Second,','TestProviderTransportPoolsReuseEightConnections'),
 ('P1-3 h2 bypass','champions.go','\treturn &http.Client{\n\t\tTimeout:   12 * time.Second,','\ttransport.ForceAttemptHTTP2 = false\n\treturn &http.Client{\n\t\tTimeout:   12 * time.Second,','TestChampionTransportNegotiatesHTTP2'),
 ('P1-5 accounting bypass','sgp_api.go','\tp.historyBytes += entry.bytes','\tp.historyBytes += entry.bytes\n\tp.historyBytes = 0','TestMatchHistoryCacheEvictsByBytes'),
 ('P1-6 independent detail route','riot_api.go','func (p *riotProvider) matchByIDWithCache(ctx context.Context, matchID string) (result *riotMatch, status string, resultErr error) {','func (p *riotProvider) matchByIDWithCache(ctx context.Context, matchID string) (result *riotMatch, status string, resultErr error) {\n if strings.HasPrefix(matchID, "KR_") { var direct riotMatch; err := p.get(ctx, riotClusterHost, "/lol/match/v5/matches/"+matchID,nil,&direct); return &direct,"miss",err }','TestR86RiotMatchSingleflightAndCanceledWaiter'),
 ('P1-7 late deadline bypass','watch_rules.go','func (r *watchRunner) schedule(client *LCUClient, action string, delayMS int, method, path string, body any) bool {','func (r *watchRunner) schedule(client *LCUClient, action string, delayMS int, method, path string, body any) bool {\n if action == "play-again" {delayMS=10000}','TestR86PlayAgainEarlierPhaseRearmsWithoutDuplicateOrPostponement'),
 ('P1-14 negative cache bypass','riot_api.go','func (p *championProvider) championNamesZH(ctx context.Context) map[int64]string {','func (p *championProvider) championNamesZH(ctx context.Context) map[int64]string {\n p.mu.Lock();p.catalogNamesFailUntil=time.Time{};p.mu.Unlock()','TestR86ChampionNamesNegativeCacheAndExpiry'),
 ('P1-17 independent page cap','sgp_api.go','\t\tquery := url.Values{\n\t\t\t"startIndex":','\t\tif fetched > 0 { pageSize = min(pageSize, sgpFallbackPageSize) }\n\t\tquery := url.Values{\n\t\t\t"startIndex":','TestMatchHistoryPhysicalPageCacheReusesTwentyBeforeFifty'),
 ('P1-18 independent global lock','rank_insights.go','func (a *app) playerRankScoreWithCacheStatus(ctx context.Context, client *LCUClient, playerRef string, isCurrent bool, serverID, privacy string) (rankScoreEntry, bool) {','func (a *app) playerRankScoreWithCacheStatus(ctx context.Context, client *LCUClient, playerRef string, isCurrent bool, serverID, privacy string) (rankScoreEntry, bool) {\n a.mu.Lock();a.mu.Unlock()','TestR86RankCacheDoesNotAcquireApplicationLock'),
 ('P1-19 canceled reservation bypass','riot_api.go','\t\tp.limitMu.Lock()\n\t\tif err := ctx.Err(); err != nil {','\t\tp.limitMu.Lock()\n if ctx.Err()!=nil { p.longWindow=append(p.longWindow,time.Now());p.limitMu.Unlock();return ctx.Err() }\n\t\tif err := ctx.Err(); err != nil {','TestR86CanceledQuotaWaitDoesNotReserveTokens'),
 ('P2-12 snapshot prune bypass','storage.go','func (s *localStore) pruneSnapshots() {','func (s *localStore) pruneSnapshots() {\n return','TestR86SnapshotsPruneByBytesAndCount'),
]
cases.extend([
 ('P1-1 independent stat route','storage.go','func (s *localStore) appendDiagnosticLocked(record map[string]any) (bool, error) {','func (s *localStore) appendDiagnosticLocked(record map[string]any) (bool, error) {\n _, _ = s.statDiagnostic(filepath.Join(s.root, "logs", "diagnostics.jsonl"))','TestR86DiagnosticBufferBudgetAndImmediateRead'),
 ('P1-2 independent payload scan','champion_cache.go','func (c *championDataCache) writeDisk(entry championCacheEnvelope) error {','func (c *championDataCache) writeDisk(entry championCacheEnvelope) error {\n entries,_:=os.ReadDir(c.dir);for _,item:=range entries {_,_=c.readCacheFile(filepath.Join(c.dir,item.Name()))}','TestR86ChampionDiskMigrationPruneAndThrottle'),
 ('P1-2 recovery data deletion bypass','champion_cache.go','func (c *championDataCache) pruneDisk() error {','func (c *championDataCache) pruneDisk() error {\n files,_:=os.ReadDir(c.dir);for _,file:=range files {if strings.HasPrefix(file.Name(),"hexdata-") {_=os.Remove(filepath.Join(c.dir,file.Name()))}}','TestR86ChampionDiskMigrationPruneAndThrottle'),
 ('P1-4 independent context bypass','lcu_api.go','data, err := api.client.GetBytesContext(ctx, path)','data, err := api.client.GetBytesContext(context.Background(), path)','TestR86OverviewDeadlineCancelsQueuesAndMastery'),
 ('P1-5 independent backfill cache route','season_stats.go','scan, seasonScanBackgroundPages, false)','scan, seasonScanBackgroundPages, true)','TestR86TenPageSeasonBackfillDoesNotPopulateHistoryCache'),
 ('P2-8 independent empty-process scan','lcu.go','func discoverLCUFromProcesses(query processQueryResult, commandErr error, candidates func([]string) []string) (*LCUClient, LCUDiscoveryStatus, error) {','func discoverLCUFromProcesses(query processQueryResult, commandErr error, candidates func([]string) []string) (*LCUClient, LCUDiscoveryStatus, error) {\n if query.ProcessCount==0 {candidates(query.CommandLines)}','TestR86EmptyProcessDiscoverySkipsDiskButErrorsRetainFallback'),
 ('P2-9 independent backoff override','connection_manager.go','if !ops.wait(ctx, backoff) {','backoff=maximumDiscoveryBackoff\n if !ops.wait(ctx, backoff) {','TestR86ConnectionBackoffResetsAfterSuccessfulDiscovery'),
])
results=[]
with tempfile.TemporaryDirectory(prefix='r86-mutations-') as tmp:
 for name,file,old,new,test in cases:
  source=(root/file).read_text(encoding='utf-8')
  assert old in source, name
  target=Path(tmp)/file;target.write_text(source.replace(old,new,1),encoding='utf-8')
  overlay=Path(tmp)/'overlay.json';overlay.write_text(json.dumps({'Replace':{str(root/file):str(target)}}),encoding='utf-8')
  run=subprocess.run(['go','test','-overlay',str(overlay),'.','-run','^'+test+'$','-count=1','-timeout=25s'],cwd=root,env=environment,encoding='utf-8',errors='replace',stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=120)
  killed=run.returncode!=0 and ('--- FAIL: '+test) in run.stdout and '[build failed]' not in run.stdout
  results.append({'case':name,'test':test,'killed':killed,'exit':run.returncode,'output':run.stdout[-2000:]})
  print(name, 'KILLED' if killed else 'NOT PROVEN',flush=True)
(root/'docs/r86/mutation-results.json').write_text(json.dumps(results,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
raise SystemExit(0 if all(x['killed'] for x in results) else 1)
