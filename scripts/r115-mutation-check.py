#!/usr/bin/env python3
"""R115 focused offline mutation checks. Go overlays never edit working files."""
from pathlib import Path
import json,os,subprocess,tempfile
root=Path(__file__).resolve().parent.parent
out=root/'docs/r115-validation/mutations';out.mkdir(parents=True,exist_ok=True)
env=dict(os.environ,GOCACHE=str(root/'.gocache'))
checks=[
 ('route-case','lcu_events.go',"c += 'a' - 'A'","c += 0",'TestR115_LCUEventRoutingParity'),
 ('route-session','lcu_events.go','case "/lol-champ-select/":','case "/removed-service/":','TestR115_LCUEventRoutingParity'),
 ('singleflight','asset_cache.go','if flight := a.assetFlights[key]; flight != nil {','if flight := a.assetFlights[key]; false && flight != nil {','TestR115_AssetSingleflight'),
 ('broadcast-default','watch_rules.go','watchBroadcastRule{Visibility: "self"}','watchBroadcastRule{Visibility: "self", TeamComposition: true}','TestR115_BroadcastDefaults'),
 ('broadcast-visibility','watch_rules.go','if rule.Visibility == "team" {','if rule.Visibility != "self" {','TestR115_BroadcastContents'),
 ('broadcast-safe-id','watch_rules.go','if !safeLCUChatIdentifier(conversationID) {','if false && !safeLCUChatIdentifier(conversationID) {','TestR115_BroadcastContents'),
 ('broadcast-pause','watch_rules.go','if r.customPaused() {\n\t\treturn finish("skipped_custom", "custom-paused")','if false && r.customPaused() {\n\t\treturn finish("skipped_custom", "custom-paused")','TestR115_BroadcastContents'),
 ('broadcast-retry','watch_rules.go','return finish("write_failed", "message-write-failed")','return finish("failed", "message-write-failed")','TestR115_BroadcastContents'),
 ('broadcast-double','watch_rules.go','if err := r.requestWatchJSON(ctx, client, http.MethodPost, path, body); err != nil {','_ = r.requestWatchJSON(ctx, client, http.MethodPost, path, body)\n if err := r.requestWatchJSON(ctx, client, http.MethodPost, path, body); err != nil {','TestR115_BroadcastContents'),
 ('lineup-self','watch_broadcast.go','if rule.Visibility != "team" {','if false {','TestR115_BroadcastContents'),
 ('load-dedup','web/section-loader.js','if (flights.has(name)) return flights.get(name);','if (false) return flights.get(name);','web'),
 ('load-failure','web/section-loader.js','notice.setAttribute("role", "alert");','notice.setAttribute("role", "status");','web'),
 ('load-stale','web/section-loader.js','if (revision !== generation) return;','if (false) return;','web'),
]
results=[]
def run(name,args,variables=env):
 result=subprocess.run(args,cwd=root,env=variables,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=90)
 (out/(name+'.log')).write_text(result.stdout)
 return result
base=run('baseline-go',['go','test','-run','TestR115','-count=1','.'])
web=run('baseline-web',['node','--test','web/r115.test.cjs'])
if base.returncode or web.returncode:raise SystemExit('baseline failed')
with tempfile.TemporaryDirectory(prefix='r115-mutations-') as tmp:
 for name,file,old,new,pattern in checks:
  source=(root/file).read_text()
  if old not in source:raise SystemExit('anchor absent: '+name)
  replacement=Path(tmp)/(name+Path(file).suffix);replacement.write_text(source.replace(old,new))
  if pattern=='web':result=run(name,['node','--test','web/r115.test.cjs'],dict(env,R115_LOADER_SOURCE=str(replacement)))
  else:
   overlay=Path(tmp)/(name+'.json');overlay.write_text(json.dumps({'Replace':{str(root/file):str(replacement)}}))
   result=run(name,['go','test','-overlay',str(overlay),'-vet=off','-run',pattern,'-timeout','8s','-count=1','.'])
  killed=result.returncode!=0 and ('--- FAIL:' in result.stdout or 'AssertionError' in result.stdout or '✖' in result.stdout) and '[build failed]' not in result.stdout
  results.append({'id':name,'file':file,'killed':killed,'exitCode':result.returncode})
  print(name,'KILLED' if killed else 'SURVIVED/ERROR',flush=True)
(out/'matrix.json').write_text(json.dumps({'baseline':True,'scope':'adopted R115 changes only; deferred sections are not claimed tested','mutations':results},ensure_ascii=False,indent=2)+'\n')
if not all(x['killed'] for x in results):raise SystemExit(1)
