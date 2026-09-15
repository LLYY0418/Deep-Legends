"""Isolated Go overlays / JS copies. Never edits the active working tree."""
import json, os, pathlib, subprocess, tempfile
root=pathlib.Path(__file__).resolve().parent.parent
out=pathlib.Path(os.environ.get('R89_MUTATION_OUTPUT','/tmp/deep-legends-r89/mutations'));out.mkdir(parents=True,exist_ok=True)
env=dict(os.environ,GOCACHE='/tmp/deep-legends-r89/go-cache')
cases=[]
def go(name,test,*changes):cases.append((name,'go',test,changes))
def js(name,test,*changes):cases.append((name,'js',test,changes))
go('A1-image-policy','ImagePolicy',('champion_cache.go','return 7 * 24 * time.Hour, 30 * 24 * time.Hour, true','return 0, 0, false'))
go('A3-persistence','ImagePolicy',('champion_cache.go','return 7 * 24 * time.Hour, 30 * 24 * time.Hour, true','return 7 * 24 * time.Hour, 30 * 24 * time.Hour, false'))
go('A4-singleflight','ImagePolicy',('asset_cache.go','if flight := a.assetFlights[key]; flight != nil {','if flight := a.assetFlights[key]; false {'),('champion_cache.go','if flight := c.flights[key]; flight != nil {','if flight := c.flights[key]; false {'),('champion_cache.go','if existing := c.flights[key]; existing != nil {','if existing := c.flights[key]; false {'))
go('A5-disk-budget','DiskBudget',('binary_disk_budget.go','for len(c.strictEntries) > c.diskMaxEntries || c.strictBytes > c.diskMaxBytes {','for false {'))
go('A-negative-cache','ImageNegative',('champion_images.go','championImageMax, time.Minute, func','championImageMax, 0, func'))
go('A-warm-final-resources','WarmPaths',('champion_images.go','ids[:min(120, len(ids))]','ids[:min(100, len(ids))]'))
go('A-warm-preemption','ForegroundCancels',('champion_images.go','w.cancel()','// mutation: cancel omitted'))
go('B2-raw-identity','SupplementIdentity',('overview_supplement_request.go','if !strings.EqualFold(strings.TrimSpace(region), riotRegionKR)','if true || !strings.EqualFold(strings.TrimSpace(region), riotRegionKR)'))
go('B3-ref-priority','SupplementIdentity',('overview_supplement_request.go','if strings.TrimSpace(playerRef) != "" {','if false {'))
go('C1-personal-guard','KeyConcurrency',('riot_match_cache.go','limit := 8','limit := 20'))
go('C1-actual-concurrency','ActualConcurrency',('riot_api.go','semaphore := make(chan struct{}, provider.detailConcurrency())','semaphore := make(chan struct{}, 4)'))
go('C2-match-disk','RiotMatchDisk',('riot_api.go','p.persistRiotMatch(key, &match)','// mutation: persistence omitted'))
go('C2-identity-disk','IdentityRestart',('riot_identity_cache.go','key, ttl, 0, true,','key, ttl, 0, false,'))
go('C3-short-limit','Limiter200',('riot_api.go','shortLimit  = 15','shortLimit  = 200'))
go('C3-long-limit','Limiter200',('riot_api.go','longLimit   = 90','longLimit   = 200'))
go('E1-details-phase','ActualConcurrency',('riot_api.go','phases.markSpan("details", detailsStarted, time.Now())','_ = detailsStarted'))
go('E2-asset-disk-event','DiagnosticEvents',('champion_images.go','"event": "asset_fetch"','"event": "omitted_asset_fetch"'))
go('E3-queue-duration','ActualConcurrency',('riot_api.go','tracker.queueWait += time.Since(queuedAt)','_ = queuedAt'))
go('E4-catalog-disk-event','DiagnosticEvents',('catalog_diagnostics.go','"event": "catalog_load"','"event": "omitted_catalog_load"'))
go('E4-participant-items','DiagnosticEvents',('catalog_diagnostics.go','"length": len(participant.ItemIDs)','"length": 0'))
js('B1-parallel','parallel supplements',('web/gameplay.js','void loadOPGGSeasonSummary(tab, force && !quiet);','// mutation: serial seasonal'),('web/gameplay.js','void loadOverviewCurrentGame(tab, force && !quiet);','// mutation: serial current'))
js('B3-dedup','parallel supplements',('web/gameplay.js','syncOverviewSupplementRefs(tab);','// mutation: no ref adoption'))
js('C-first-five','parallel supplements',('web/gameplay.js','initialKRPage ? 5 : state.settings.matchCount','initialKRPage ? 20 : state.settings.matchCount'))
js('A-catalog-retry','catalog failure retries',('web/gameplay.js','setTimeout(() => { void ensureItems(); }, 30_000)','setTimeout(() => {}, 30_000)'))
js('E4-client-failure','catalog failure retries',('web/gameplay.js','recordItemSetClientDiagnostic("catalog_client", "failed",','void ("catalog_client", "failed",'))
js('E4-missing-id','catalog failure retries',('web/gameplay.js','recordItemSetClientDiagnostic("item_id_not_in_catalog",','void ("item_id_not_in_catalog",'))
js('E4-retained-dom','catalog arrival invalidates',('web/gameplay.js','tab.matchViewRevision = Number(tab.matchViewRevision || 0) + 1;','tab.matchViewRevision = Number(tab.matchViewRevision || 0);'))
js('C-completion-retry','completion failure',('web/gameplay.js','tab.initialPageError = error.message || "请求失败"','tab.initialPageError = ""'))
js('D-overlay-close','closing an overlay',('web/gameplay.js','if (closed) {\n      closed.closed = true;','if (closed) {\n      closed.closed = false;'))
selected=os.environ.get('R89_MUTATION_FILTER','')
if selected:cases=[case for case in cases if case[0] in selected.split(",")]
results=[]
# The exact selected suite must be green before any mutation runs.
for kind,cmd in [('go',['go','test','-run','^TestR89','-count=1']),('js',['node','--test','web/r89.test.cjs'])]:
 if not any(case[1]==kind for case in cases):continue
 p=subprocess.run(cmd,cwd=root,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
 (out/(kind+'-baseline.log')).write_text(p.stdout)
 if p.returncode:raise SystemExit(kind+' baseline failed; no valid mutation result')
for name,kind,test,changes in cases:
 with tempfile.TemporaryDirectory(prefix='r89-mutant-') as tmp:
  replacements={}
  for file,old,new in changes:
   text=replacements.get(file,(root/file).read_text())
   if text.count(old)!=1:raise SystemExit(f'{name}: anchor count {text.count(old)} for {old}')
   replacements[file]=text.replace(old,new)
  overlay={}
  for file,text in replacements.items():
   target=pathlib.Path(tmp)/pathlib.Path(file).name;target.write_text(text);overlay[str(root/file)]=str(target)
  runenv=env.copy()
  if kind=='go':
   config=pathlib.Path(tmp)/'overlay.json';config.write_text(json.dumps({'Replace':overlay}))
   cmd=['go','test','-overlay',str(config),'-run','^TestR89'+test,'-count=1','-timeout','60s']
  else:
   runenv['R89_GAMEPLAY_SOURCE']=overlay[str(root/'web/gameplay.js')]
   cmd=['node','--test','--test-name-pattern',test,'web/r89.test.cjs']
  p=subprocess.run(cmd,cwd=root,env=runenv,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=90)
  (out/(name+'.log')).write_text(p.stdout)
  killed=p.returncode!=0 and '[build failed]' not in p.stdout and 'SyntaxError' not in p.stdout
  results.append({'id':name,'killed':killed,'exit':p.returncode})
  print(name,'KILLED' if killed else 'SURVIVED/INVALID',flush=True)
  (out/'results.json').write_text(json.dumps(results,indent=2)+'\n')
  if not killed:raise SystemExit('mutation did not produce a behavioral failure: '+name)
print('All',len(results),'independent mutations killed')
