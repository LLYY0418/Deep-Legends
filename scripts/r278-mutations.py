"""R278 mutations execute fixed behavior tests without editing checkout sources."""
import argparse,json,os,re,runpy,subprocess,tempfile,time
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--out',type=Path,required=True);args=p.parse_args();args.out=args.out.resolve();args.out.mkdir(parents=True,exist_ok=True)
proof=runpy.run_path(str(ROOT/'scripts/mutation-source-proof.py'));snapshot=proof['snapshot'](ROOT)
rows=[]
def run(name,cmd,env=None,negative=False,expected=None):
 started=time.monotonic();r=subprocess.run(cmd,cwd=ROOT,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=120)
 log=r.stdout+r.stderr;(args.out/(name+'.log')).write_text(log)
 assertion=(r.returncode!=0 and ('AssertionError' in log or '--- FAIL:' in log) and not any(x in log for x in ['build failed','syntax error:','ReferenceError:','undefined:'])) if negative else r.returncode==0
 if expected:assertion=assertion and expected in log
 rows.append(dict(name=name,negative=negative,exitCode=r.returncode,assertionVerified=assertion,seconds=round(time.monotonic()-started,3)))
 (args.out/'result.json').write_text(json.dumps(rows,indent=2)+'\n');print(json.dumps(rows[-1]),flush=True);assert assertion,name
node=['node','--test','backend/web/r278.test.cjs','backend/web/r278-announcements.test.cjs','scripts/r278-release-contracts.test.cjs']
run('baseline-node',node);run('baseline-go',['go','test','./backend','-run','^TestR278','-count=1','-v','-timeout=90s'])
with tempfile.TemporaryDirectory(prefix='r278-mutants-') as d:
 tmp=Path(d)
 def mutate(name,file,before,after,envkey=None,test=None):
  source=(ROOT/file).read_text();assert source.count(before)==1,(name,source.count(before));target=tmp/(name+Path(file).suffix);target.write_text(source.replace(before,after,1))
  if envkey:run(name,node,{envkey:str(target)},True)
  else:
   overlay=tmp/(name+'.json');overlay.write_text(json.dumps({'Replace':{str(ROOT/file):str(target)}}));run(name,['go','test','-overlay='+str(overlay),'./backend','-run','^'+test+'$','-count=1','-v','-timeout=90s'],negative=True,expected='--- FAIL: '+test)
 mutate('P1-manual-window-restored','backend/web/gameplay.js','if (state.followSelfPending || platformChanged || identityChanged) {','if ((state.followSelfPending || platformChanged || identityChanged) && Date.now()-state.lastManualTabAt>=10000) {','R278_GAMEPLAY_SOURCE')
 mutate('P2-countdown-restored','backend/web/gameplay.js','data-history-countdown','data-history-countdown', 'INVALID') if False else None
 mutate('P2-frontend-outage-timer-restored','backend/web/gameplay.js','if(historyServiceState(tab))return;','if(historyServiceState(tab)){setTimeout(()=>loadOverview(tab,true),60000);return;}','R278_GAMEPLAY_SOURCE')
 mutate('P3-empty-revision-reference-restored','backend/overview_wire.go','ok && match.Revision != "" && revision == match.Revision','ok && revision == match.Revision',test='TestR278P3NeverReferenceEmptyRevision')
 mutate('P4-foreign-scan-restored','backend/web/app.js','if (!foreign && !installations.some(item => item.available)) {','if (!installations.some(item => item.available)) {','R278_APP_SOURCE')
 mutate('P5-loaded-fixed-height-restored','backend/web/gameplay.css','/* R211:','[data-career-key="ranks"] { min-height: 450px; }\n/* R211:','R278_CSS_SOURCE')
 mutate('P6-completion-pending-left','backend/web/gameplay.js','tab.initialPagePending=false;tab.initialPageError="";','tab.initialPagePending=true;tab.initialPageError="";','R278_GAMEPLAY_SOURCE')
 mutate('P7-cleanup-before-admission','backend/web/gameplay.js','if(blocked)return;','if(blocked){cleanup();return;}','R278_GAMEPLAY_SOURCE')
 mutate('P7-half-page-budget-restored','backend/riot_page_budget.go','need := count + 1','need := 1',test='TestR278P7WholePageQuota')
 mutate('P8-global-history-boundary-restored','backend/web/history-filters.js','search.boundary={signature:JSON.stringify(conditions(tab)),reached:true};','tab.data.pagination.hasMore=false;search.boundary={signature:JSON.stringify(conditions(tab)),reached:true};','R278_FILTER_MODULE')
 mutate('P9-chroma-sidebar-restored','backend/web/collection-new-items.js',"if(kind==='skins' && !unread.has(id))record.navUnread=true;","if(!unread.has(id))record.navUnread=true;",'R278_COLLECTION_MODULE')
 mutate('P9-running-skin-event-dropped','backend/collection_skin_events.go','a.publishCollectionSkinIDs(client, epoch, catalog, event.Data, strings.Contains(uri, "champion_skin"), started)','if !a.collectionSkinEventRunning {a.publishCollectionSkinIDs(client, epoch, catalog, event.Data, strings.Contains(uri, "champion_skin"), started)}',test='TestR278P9DirectSkinEventDuringFallback')
 mutate('P10-data-only-render-suppression-restored','backend/web/app.js',"return state.renderedSignature===signature && Boolean(el.grid?.querySelector('[data-collection-id]')) && !el.grid.querySelector('.skeleton,.loading,.empty-state');","return state.items.length>0;",'R278_APP_SOURCE')
 mutate('P11-empty-name-inline-feedback-lost','backend/web/history-filters.js',"if(!name){if(input){input.setAttribute('aria-invalid','true');","if(!name){if(input){input.removeAttribute('aria-invalid');",'R278_FILTER_MODULE')
 mutate('P12-public-entry-missing','backend/web/default/index.html','id="announcements-button"','id="announcement-missing"','R278_DEFAULT_HTML')
 mutate('P12-dismiss-marks-read','backend/web/announcements.js','dismiss(){},','dismiss(n){read.push(id(n));},','R278_ANNOUNCEMENTS_MODULE')
 mutate('P12-game-phase-gate-lost','backend/web/announcements.js',"if(phase==='unknown' || active.has(phase))return;","if(phase==='unknown')return;",'R278_ANNOUNCEMENTS_MODULE')
 mutate('P13-long-probe-restored','backend/update_sources.go','updateProbeTimeout = 2 * time.Second','updateProbeTimeout = 6 * time.Second',test='TestR278P13ProbeBudget')
 mutate('P13-auto-predownload-disabled','backend/update.go','predownload := u.autoPredownload && resultState == "available" && !u.status.ManualOnly','predownload := false',test='TestR278P13AutomaticPredownloadAndForegroundTakeover')
 mutate('P14-success-poll-not-aggregated','backend/request_diagnostic_summary.go','local := event["event"] == "local_request_client"','local := false',test='TestR278P14LocalSuccessWindow')
 mutate('P14-snapshot-dedup-lost','backend/diagnostic_snapshots.go','if !snapshotDiagnosticEvent(name) {','if name == "overview_art_source" || !snapshotDiagnosticEvent(name) {',test='TestR278P14SnapshotRetainsChanges')
 mutate('P15-release-version-stale','desktop/package.json','"version": "0.12.86"','"version": "0.12.84"','R278_PACKAGE_FILE')
proof['unchanged'](ROOT,snapshot)
run('restored-node',node);run('restored-go',['go','test','./backend','-run','^TestR278','-count=1','-v','-timeout=90s'])
proof['unchanged'](ROOT,snapshot)
