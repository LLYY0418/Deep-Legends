"""R258 P1/P2/P3/P4/P5 assertion mutations; production files remain untouched."""
import argparse, hashlib, json, pathlib, re, subprocess, tempfile
parser=argparse.ArgumentParser();parser.add_argument('--output',required=True);args=parser.parse_args()
root=pathlib.Path(__file__).resolve().parents[1];out=pathlib.Path(args.output);out.mkdir(parents=True,exist_ok=False)
mutations=[
 ('global-overview-clear','backend/overview_cache.go','a.invalidateOverviewPlayer(playerRef)','if a.overviewQueries != nil { a.overviewQueries.mu.Lock(); for key,element:=range a.overviewQueries.entries {a.overviewQueries.removeElementLocked(element);delete(a.overviewQueries.flights,key)}; a.overviewQueries.mu.Unlock() }','^TestR258OverviewInvalidationKeepsOtherAccount'),
 ('legacy-status-connection','backend/web/gameplay.js','function connected() { return state.status?.clientView?.state === "ready"; }','function connected() { return Boolean(state.status?.connected); }','R258_GAMEPLAY_SOURCE'),
 ('tencent-snapshot-disabled','backend/gameplay.go','if isCurrent && begIndex == 0 && normalizeGameplayMatchFilter(matchFilter) == "all" {','if isCurrent && clientRiotPlatform(client) != "" && begIndex == 0 && normalizeGameplayMatchFilter(matchFilter) == "all" {','^TestR258TencentOverviewFirstCard'),
 ('ranked-pages-before-first-card','backend/gameplay.go','if isCurrent {\n\t\t\t\t<-detailsReady','if isCurrent && clientRiotPlatform(client) != "" {\n\t\t\t\t<-detailsReady','^TestR258TencentOverviewFirstCard'),
 ('objective-on-connect','backend/connection_manager.go','a.scheduleConnectionWork(sessionCtx, client, "connection_manager.objective", true, func() { a.collectObjectiveDiagnostics(sessionCtx, client, "connected") })','a.goSafe("connection_manager.objective", func() { a.collectObjectiveDiagnostics(sessionCtx, client, "connected") })','^TestR258ConnectedSessionDoesNotProbeObjective'),
 ('exit-discovery-connected','backend/main.go','clientDiscovery = "exiting"','clientDiscovery = "connected"','^TestR258ShutdownStatus'),
 ('restore-without-recent-event','backend/client_shutdown.go',' || a.clientLastEventAt.IsZero() || now.Sub(a.clientLastEventAt) > 5*time.Second','','^TestR258ShutdownStatus'),
 ('restore-eight-second-probe','backend/connection_manager.go','client.probeContext(ctx, discoveryProbeTimeout)','client.probeContext(ctx, 8*time.Second)','^TestR258Unsignaled'),
 ('default-visible','backend/web/default/index.html','class="startup-loading" hidden','class="startup-loading"','R258_HTML_SOURCE'),
 ('show-starting-overlay','backend/web/app.js','el.startupLoading.hidden = true;','el.startupLoading.hidden = false;','R258_APP_SOURCE'),
]
before={p:hashlib.sha256((root/p).read_bytes()).hexdigest() for _,p,*_ in mutations};rows=[]
for name,target,old,new,check in mutations:
 source=(root/target).read_text();assert old in source,name
 with tempfile.TemporaryDirectory(prefix='r258-mutant-') as d:
  temp=pathlib.Path(d);replacement=temp/pathlib.Path(target).name;replacement.write_text(source.replace(old,new,1))
  if target.endswith('.go'):
   overlay=temp/'overlay.json';overlay.write_text(json.dumps({'Replace':{str(root/target):str(replacement)}}));command=['go','test','-overlay',str(overlay),'-count=1','-run',check,'./backend'];env=None
  else:
   import os
   command=['node','--test','backend/web/r258.test.cjs'];env={**os.environ,check:str(replacement)}
  result=subprocess.run(command,cwd=root,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
  (out/(name+'.log')).write_text(result.stdout)
  killed=result.returncode!=0 and (('--- FAIL: TestR258' in result.stdout) if target.endswith('.go') else ('AssertionError' in result.stdout)) and not re.search(r'build failed|setup failed|undefined:',result.stdout)
  row={'name':name,'assertion_killed':killed,'exit_code':result.returncode};rows.append(row);print(json.dumps(row),flush=True)
assert before=={p:hashlib.sha256((root/p).read_bytes()).hexdigest() for p in before}
(out/'results.json').write_text(json.dumps({'unchanged_source':True,'rows':rows,'all_assertion_killed':all(r['assertion_killed'] for r in rows)},indent=2)+'\n')
raise SystemExit(0 if all(r['assertion_killed'] for r in rows) else 1)
