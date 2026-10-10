"""R269 negative assertions execute only in temporary source copies."""
import argparse,json,os,shutil,subprocess,tempfile,time
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--out',type=Path,required=True);p.add_argument('--browser',action='store_true');ARGS=p.parse_args();OUT=ARGS.out;OUT.mkdir(parents=True,exist_ok=True);results=[]
def run(name,cmd,cwd=ROOT,extra=None,negative=False):
 start=time.monotonic();r=subprocess.run(cmd,cwd=cwd,env={**os.environ,'RIOT_API_KEY':'RGAPI-00000000-0000-0000-0000-000000000000',**(extra or {})},capture_output=True,text=True,timeout=180);log=r.stdout+r.stderr;(OUT/(name+'.log')).write_text(log);ok=r.returncode!=0 if negative else r.returncode==0
 if negative:ok=ok and any(s in log for s in ['FAIL','AssertionError','✖','not ok']) and not any(s in log for s in ['build failed','SyntaxError:','Cannot find module','no tests to run'])
 results.append(dict(name=name,exitCode=r.returncode,negative=negative,assertionVerified=ok,seconds=round(time.monotonic()-start,3)));(OUT/'result.json').write_text(json.dumps(results,indent=2));print(json.dumps(results[-1]),flush=True);assert ok,name
with tempfile.TemporaryDirectory(prefix='r269-mutants-') as temp:
 temp=Path(temp);source=(ROOT/'backend/web/history-filters.js').read_text();node=[
 ('p2-detached-dialog',[('doc.body.append(menu)','container.append(menu)')]),
 ('p2-nonmodal-not-repaired',[("if(!menu.matches(':modal'))","if(!menu.open)")]),
 ('p2-inactive-steals-dialog',[("container._afDialog=menu;return menu;","if(menu._afOwner!==container && menu.open)menu._afOwner?._afClose?.();menu._afOwner=container;container._afDialog=menu;return menu;")]),
 ('p3-hide-zero-choices',[('for(const value of fixed)map.set(value,{value,label:labels[value],count:0,wins:0,recent:0});','')]),
 ('p4-resurrect-stale-edit',[("if(!m.open)m.conditions=clone(conditions(t))","if(!m.open && !m.conditions)m.conditions=clone(conditions(t))"),("delete m.conditions;","")]),
 ('p5-stop-keeps-request',[("abort?.();",";")]),
 ('p5-foreign-scan-300',[("ctx.foreign?.()?100:300","ctx.foreign?.()?300:300")])]
 run('node-original',['node','--test','backend/web/r269.test.cjs'])
 for name,changes in node:
  mutant=source
  for a,b in changes:assert a in mutant,(name,a);mutant=mutant.replace(a,b)
  file=temp/(name+'.cjs');file.write_text(mutant);run(name,['node','--test','backend/web/r269.test.cjs'],extra={'R269_FILTER_SOURCE':str(file)},negative=True)
 go=temp/'go';shutil.copytree(ROOT/'backend',go/'backend');shutil.copy2(ROOT/'go.mod',go/'go.mod');shutil.copy2(ROOT/'go.sum',go/'go.sum')
 cases=[
 ('p1-no-recovery','riot_puuid_scope.go','fresh, scope, lookupErr := p.recoverPUUID(ctx, id)','return err\n fresh, scope, lookupErr := p.recoverPUUID(ctx, id)','TestR269CredentialScopesAndPUUIDRecovery'),
 ('p1-no-credential-partition','riot_puuid_scope.go',' + "|credential:" + riotCredentialScope(ctx)', '','TestR269CredentialScopesAndPUUIDRecovery'),
 ('p1-inflight-key-change','riot_routing.go','if credential, ok := ctx.Value(riotPinnedCredentialKey{}).(riotPinnedCredential); ok {','if credential, ok := ctx.Value(riotPinnedCredentialKey{}).(riotPinnedCredential); false && ok {','TestR269IdentityPinsKeyDuringLookup'),
 ('p5-repeat-first-screen','gameplay.go','respondJSON(w, riotOverviewPage{response.Player, response.Matches, response.Pagination, response.Capabilities})','respondJSON(w, response)','TestR269PaginationIsDeltaAndTimeIsPushedDown'),
 ('p8-fast-retry','sgp_api.go','1500 * time.Millisecond, 3 * time.Second','250 * time.Millisecond, 750 * time.Millisecond','TestR269SGPBackoffAndEarlyProbe'),
 ('p10-short-success-ttl','aramkit_rating.go','aramkitRatingSuccessTTL    = 6 * time.Hour','aramkitRatingSuccessTTL    = 10 * time.Minute','TestR269MayhemDiskTTLAndNoRecordCache'),
 ('p10-no-disk-cache','aramkit_rating.go','if !present && c.disk != nil {','if false && !present && c.disk != nil {','TestR269MayhemDiskTTLAndNoRecordCache'),
 ('p11-poll-spam','r269_diagnostics.go','if same && (stateOnly || now.Sub(prior.at) < time.Minute) {','if false && same && (stateOnly || now.Sub(prior.at) < time.Minute) {','TestR269RequestCommonFieldsAndWindowCompaction'),
 ('p12-unsafe-message','r269_diagnostics.go','message = errorUserPath.ReplaceAllString(message, "${1}[redacted]")','message = message','TestR269DiagnosticMessageAndUserScope')]
 run('go-original',['go','test','./backend','-count=1','-run','^TestR269'],cwd=go)
 for name,file,a,b,test in cases:
  target=go/'backend'/file;original=target.read_text();assert a in original,(name,a);target.write_text(original.replace(a,b))
  try:run(name,['go','test','./backend','-count=1','-run','^'+test+'$'],cwd=go,negative=True)
  finally:target.write_text(original)
 run('go-restored',['go','test','./backend','-count=1','-run','^TestR269'],cwd=go)

if ARGS.browser:
 with tempfile.TemporaryDirectory(prefix='r269-browser-mutants-') as temp:
  temp=Path(temp);css=temp/'css';css.mkdir();(css/'gameplay.css').write_bytes(subprocess.check_output(['git','show','2949963f:backend/web/gameplay.css'],cwd=ROOT))
  run('p6-restore-broad-layout',['node','scripts/r269-geometry.cjs'],extra={'R266_WEB_OVERRIDE':str(css),'R269_GEOMETRY_OUT':str(OUT/'p6-geometry')},negative=True)
  web=temp/'career';web.mkdir();source=(ROOT/'backend/web/gameplay.js').read_text();old='const width = container.querySelector(".overview-layout")?.clientWidth || container.clientWidth;';assert old in source;source=source.replace(old,'const width = container.querySelector(".overview-layout")?.getBoundingClientRect().width || container.getBoundingClientRect().width;');old='nodes.careerDialog.open && !container.querySelector("[data-open-career-dialog]")?.offsetParent';assert old in source;source=source.replace(old,'nodes.careerDialog.open && width > 1020');(web/'gameplay.js').write_text(source)
  run('p7-scaled-width-closes-career',['node','scripts/r269-browser.cjs'],extra={'R266_WEB_OVERRIDE':str(web),'R269_BROWSER_OUT':str(OUT/'p7-browser')},negative=True)
  run('browser-restored',['node','scripts/r269-browser.cjs'],extra={'R269_BROWSER_OUT':str(OUT/'browser-restored')})
