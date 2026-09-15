"""Mutation probes on real binaries and real Chromium, without workspace edits."""
import json,os,pathlib,subprocess,tempfile
root=pathlib.Path(__file__).resolve().parent.parent
out=pathlib.Path('/tmp/deep-legends-r89/live-mutations');out.mkdir(parents=True,exist_ok=True)
env=dict(os.environ,GOCACHE='/tmp/deep-legends-r89/go-cache',DEEP_LEGENDS_ITEM_PREWARM='0')
results=[]
def binary_probe(name,changes):
 with tempfile.TemporaryDirectory(prefix='r89-live-mutant-') as tmp:
  tmp=pathlib.Path(tmp);mapping={}
  for file,old,new in changes:
   source=(root/file).read_text();assert source.count(old)==1,(name,file,old)
   replacement=tmp/pathlib.Path(file).name;replacement.write_text(source.replace(old,new));mapping[str(root/file)]=str(replacement)
  overlay=tmp/'overlay.json';overlay.write_text(json.dumps({'Replace':mapping}))
  binary=tmp/'service';subprocess.run(['go','build','-overlay',str(overlay),'-o',str(binary),'.'],cwd=root,env=env,check=True)
  measurements=[]
  for round in range(2 if name=='persistence' else 1):
   data=out/(name+'-data');runenv=dict(env,LOL_LOOT_DATA_DIR=str(data))
   with (out/f'{name}-{round}-service.log').open('w') as log:
    proc=subprocess.Popen([str(binary),'-desktop','-listen','127.0.0.1:0'],env=runenv,stdout=subprocess.PIPE,stderr=log,text=True)
    try:
     ready=json.loads(proc.stdout.readline().removeprefix('LOOT_READY '))
     target=out/f'{name}-{round}.json'
     with (out/f'{name}-{round}-probe.log').open('w') as output:
      subprocess.run(['python3',str(root/'scripts/r89-asset-probe.py'),ready['baseUrl'],str(data),str(target)],env=env,stdout=output,stderr=subprocess.STDOUT,check=True)
     measurements.append(json.loads(target.read_text()))
    finally:proc.terminate();proc.wait(timeout=5)
  killed=(measurements[-1]['rounds'][0]['duration_ms']>=1500 if name=='persistence' else measurements[0]['same'][1]['duration_ms']>=5 and measurements[0]['rounds'][1]['duration_ms']>=500)
  results.append({'id':'binary-'+name,'killed':killed,'measurements':measurements});assert killed,name
  print('binary',name,'KILLED',flush=True)
# Unchanged binary has already passed these exact live thresholds (saved asset probes).
binary_probe('memory-and-policy',[
 ('champion_cache.go','return 7 * 24 * time.Hour, 30 * 24 * time.Hour, true','return 0, 0, false'),
 ('asset_cache.go','if data, ok := a.assetCache[key]; ok {','if data, ok := a.assetCache[key]; ok && false {')])
binary_probe('persistence', [('champion_cache.go','return 7 * 24 * time.Hour, 30 * 24 * time.Hour, true','return 7 * 24 * time.Hour, 30 * 24 * time.Hour, false')])
for name,changes in [
 ('parallel',[('void loadOPGGSeasonSummary(tab, force && !quiet);','// serial seasonal'),('void loadOverviewCurrentGame(tab, force && !quiet);','// serial current')]),
 ('catalog-redraw',[('tab.matchViewRevision = Number(tab.matchViewRevision || 0) + 1;','tab.matchViewRevision = Number(tab.matchViewRevision || 0);')])]:
 with tempfile.TemporaryDirectory(prefix='r89-browser-mutant-') as tmp:
  s=(root/'web/gameplay.js').read_text()
  for old,new in changes:assert s.count(old)==1;s=s.replace(old,new)
  file=pathlib.Path(tmp)/'gameplay.js';file.write_text(s)
  runenv=dict(env,R89_GAMEPLAY_SOURCE=str(file),R89_OUTPUT=str(out/name))
  p=subprocess.run(['node','desktop/r89-browser.cjs'],cwd=root,env=runenv,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=90)
  (out/(name+'.log')).write_text(p.stdout)
  killed=p.returncode!=0 and 'AssertionError' in p.stdout
  results.append({'id':'chromium-'+name,'killed':killed});assert killed,(name,p.stdout)
  print('chromium',name,'KILLED',flush=True)
(out/'results.json').write_text(json.dumps(results,indent=2)+'\n')
