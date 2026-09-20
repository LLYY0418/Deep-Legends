"""R99 negative controls using temporary Go overlays / JS sources."""
import json, os, subprocess, tempfile
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
OUT=ROOT/'docs/r99-validation/mutations';OUT.mkdir(parents=True,exist_ok=True)
ENV=dict(os.environ,GOCACHE='/tmp/deep-legends-go-cache')
START=os.environ.get('R99_MUTATION_FROM','')
started=not START
rows=[]
if START and (OUT/'result.json').exists():
 for row in json.loads((OUT/'result.json').read_text()):
  if row['name']==START:break
  rows.append(row)
def probe(name,file,old,new,test,kind='go'):
 global started
 if not started and name!=START:return
 started=True
 source=(ROOT/file).read_text();assert source.count(old)==1,(name,source.count(old))
 cmd=['go','test','-count=1','-timeout=30s','-run',test,'.'] if kind=='go' else ['node','--test','--test-name-pattern=R99','web/suite.test.cjs']
 if kind=='browser':cmd=['node','desktop/r99-facade-picker-layout.cjs']
 baseline=subprocess.run(cmd,cwd=ROOT,env=ENV,capture_output=True,text=True,timeout=150)
 (OUT/(name+'-baseline.txt')).write_text(baseline.stdout+baseline.stderr)
 assert baseline.returncode==0,(name,'baseline',baseline.stdout[-1500:],baseline.stderr[-1500:])
 with tempfile.TemporaryDirectory(prefix='r99-mutant-') as directory:
  modified=Path(directory)/Path(file).name;modified.write_text(source.replace(old,new))
  env=ENV.copy()
  if kind=='go':
   overlay=Path(directory)/'overlay.json';overlay.write_text(json.dumps({'Replace':{str(ROOT/file):str(modified)}}));cmd=cmd[:2]+['-overlay',str(overlay)]+cmd[2:]
   if file=='pro_seed_accounts.go':env['R99_SEED_SOURCE']=str(modified)
  elif kind=='browser':env['R99_CSS_SOURCE' if file.endswith('.css') else 'R99_SUITE_SOURCE']=str(modified);env['R99_BROWSER_OUTPUT']=str(OUT/name)
  else:env['R99_SUITE_SOURCE']=str(modified)
  changed=subprocess.run(cmd,cwd=ROOT,env=env,capture_output=True,text=True,timeout=150)
 log=changed.stdout+changed.stderr;(OUT/(name+'.txt')).write_text(log)
 killed=changed.returncode!=0 and ('FAIL: Test' in log or 'AssertionError' in log) and 'build failed' not in log and 'test timed out' not in log
 rows.append({'name':name,'file':file,'test':test,'baseline':baseline.returncode,'exit':changed.returncode,'killed':killed})
 (OUT/'result.json').write_text(json.dumps(rows,indent=2)+'\n');print(name,'KILLED' if killed else 'INVALID/SURVIVED',flush=True)
 assert killed,log[-2400:]
probe('missing-inventory-quadrant','r99_probe.go','\t"/lol-regalia/v3/inventory/REGALIA_CREST",\n','','^TestR99ProbeReadMatrixAndPrivacy$')
probe('automatic-write-probe','r99_probe.go','func (a *app) recordR99SurfaceShape(ctx context.Context, client *LCUClient) {','func (a *app) recordR99SurfaceShape(ctx context.Context, client *LCUClient) { a.runR99WriteProbe(ctx,client)','^TestR99ConnectionNeverWritesAndProbesOnce$')
probe('raw-chat-diagnostic','r99_probe.go','"original_read": true, "status": status.Load()', '"raw": chat, "original_read": true, "status": status.Load()','^TestR99WriteProbeFreshIconAndAccentRestore$')
probe('wrong-accent-restore','r99_probe.go','map[string]any{"bannerAccent": original}','map[string]any{"bannerAccent": strconv.Itoa(next)}','^TestR99WriteProbeFreshIconAndAccentRestore$')
probe('stale-icon-probe','r99_probe.go','if icon, ok := current["profileIconId"].(float64);','current = map[string]any{"profileIconId":float64(a.summoner.ProfileIconID)}\n if icon, ok := current["profileIconId"].(float64);','^TestR99WriteProbeFreshIconAndAccentRestore$')
probe('extra-icon-body-field','facade_icons.go','map[string]any{"profileIconId": id}','map[string]any{"profileIconId": id,"inventoryToken":""}','^TestR99IconWriteAccepts201AndRefreshesSnapshot$')
probe('unknown-icon-write','facade_icons.go','id <= 0 || !a.knownFacadeIcon(ctx, client, id)','id <= 0','^TestR99IconRejectsUnknownOrDisabledWithoutWrite$')
probe('missing-icon-reread','facade_icons.go','_, err = a.applySummonerIdentity(client, next, time.Now())','_ = next // mutation: omit snapshot publication','^TestR99IconWriteAccepts201AndRefreshesSnapshot$')
probe('swallowed-identity-event','connection_manager.go','return hasLCUEventPrefix(uri, "/lol-challenges/v1/summary-player-data") ||','return hasLCUEventPrefix(uri,"/lol-summoner/v1/current-summoner") || hasLCUEventPrefix(uri, "/lol-challenges/v1/summary-player-data") ||','^TestR99SummonerEventStillUpdatesIdentity$')
probe('private-icon-projection','facade_icons.go','type facadeIcon struct {','type facadeIcon struct { OwnershipType string `json:"ownershipType"`','^TestR99IconCatalogProjectionCachingAndClientSwitch$')
probe('client-cache-crossing','facade_icons.go','if c.client != client {','if c.client == nil {','^TestR99IconCatalogProjectionCachingAndClientSwitch$')
probe('rank-crest-overwrite','facade_icons.go','"preferredCrestType": current["preferredCrestType"]','"preferredCrestType": "prestige"','^TestR99RankBannerPreservesCrestAndRejectsUnknown$')
probe('seed-zero-ttl','pro_seed_accounts.go','30*24*time.Hour, &anchor','0, &anchor','^TestR99SeedRenameSurvivesRestartAndTTL$')
probe('seed-old-name-lookup','pro_seed_accounts.go','return p.fetchAccountByPUUID(ctx, anchor.PUUID)','return p.accountByRiotID(ctx,seed.GameName,seed.TagLine)','^TestR99SeedRenameSurvivesRestartAndTTL$')
probe('seed-no-budget','pro_seed_accounts.go','ctx = withRiotQueueLimit(ctx, 100*time.Millisecond)','// mutation: omit queue budget','^TestR99SeedRespectsQuotaBudget$')
probe('seed-missing-partial','pro_players.go','snapshot := withProSeed(append(cloneProTeams(base), retainProSupplements(partial, previous, time.Now())...), seeds)','snapshot := append(cloneProTeams(base), retainProSupplements(partial, previous, time.Now())...)','^TestR99SeedPresentInEveryPipelinePublication$')
probe('seed-copied-id','pro_seed_accounts.go','GameName: "모든일은같이"','GameName: "'+('A'*78)+'"','^TestR99SeedSourceHasNoCopiedStableID$')
probe('seed-lru-evicted','binary_disk_budget.go','if strings.HasPrefix(filepath.Base(key), "proseed-") {','if false && strings.HasPrefix(filepath.Base(key), "proseed-") {','^TestR99SeedAnchorSurvivesLRUAndHasOnlyStableField$')
probe('modal-theme-not-restored','web/suite.js','      window.desktopTheme?.setModalOpen?.(false);','      // mutation: no modal reset','R99','js')
probe('dialog-in-root','web/suite.js','    document.body.append(dialog);','    roots.facade.append(dialog);','R99','js')
probe('unsliced-grid','web/suite.js','added < 72 && performance.now() - started < 7','true','R99','js')
probe('inline-style','web/suite.js','<div class="facade-picker-sheet"><header','<div class="facade-picker-sheet" style="color:red"><header','R99','js')
probe('no-aspect-ratio','web/suite.css','width: 100%; aspect-ratio: 1;','width: 100%;','R99','browser')
probe('no-block-image','web/suite.css','.facade-icon-picture img { display: block;','.facade-icon-picture img {','R99','browser')
probe('quota-identity-path-leak','riot_api.go','"event": "riot_rate_limited", "host": host, "path": riotDiagnosticPath(requestPath),','"event": "riot_rate_limited", "host": host, "path": requestPath,','^TestR99SeedQuotaDiagnosticsNeverContainIdentity$')
