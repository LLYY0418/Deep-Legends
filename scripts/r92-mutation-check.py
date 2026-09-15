"""Behavioral mutations in Go overlays / JS copies; production files untouched."""
import json,os,pathlib,subprocess,tempfile
root=pathlib.Path(__file__).resolve().parent.parent
out=pathlib.Path(os.environ.get('R92_MUTATION_OUTPUT','/tmp/deep-legends-r92/mutations'));out.mkdir(parents=True,exist_ok=True)
env=dict(os.environ,GOCACHE='/tmp/deep-legends-r89/go-cache')
cases=[]
def go(name,test,*changes):cases.append((name,'go',test,changes))
def js(name,test,*changes):cases.append((name,'js',test,changes))
go('G1-default-four','TestR92DefaultPersonalConcurrency',('riot_match_cache.go','return 8','return 4',2))
go('G1-effective-four','TestR89ActualConcurrencyPhasesAndQueue',('riot_api.go','semaphore := make(chan struct{}, provider.detailConcurrency())','semaphore := make(chan struct{}, 4)',1))
go('G2-no-match-persistence','TestR89RiotMatchDiskAndImmutableData',('riot_api.go','p.persistRiotMatch(key, &match)','// no match persistence',1))
go('G3-widen-both','TestR92RiotMatchConcreteDiskBudget',('riot_match_cache.go','riotMatchCacheMax, 128<<20','6000000, 128<<30',2))
go('G3-widen-bytes-only','TestR92RiotMatchConcreteDiskBudget',('riot_match_cache.go','riotMatchCacheMax, 128<<20','riotMatchCacheMax, 128<<30',2))
go('G3-remove-eviction','TestR92RiotMatchConcreteDiskBudget',('binary_disk_budget.go','for len(c.strictEntries) > c.diskMaxEntries || c.strictBytes > c.diskMaxBytes {','for false {',1))
go('I1-serial','TestR92ProLadderStartsBeforeSupplementsFinish',('pro_players.go','ladder.submit(teams)','// directory jobs delayed',1),('pro_players.go','ladder.submit(snapshot)','// partial jobs delayed',1))
go('I1-dropped-new','TestR92ProLadderNewAccountsAndDedup',('pro_players_ladder.go','if !p.seen[key] {','if !p.seen[key] && len(p.seen) == 0 {',1))
go('I2-persist-off','TestR92CommunityImageIndependentDiskRestart',('community_images.go','30*24*time.Hour, true, loader','30*24*time.Hour, false, loader',1))
js('G-first-screen-blocked','parallel supplements',('web/gameplay.js','void loadOPGGSeasonSummary(tab, force && !quiet);','// serial season',1),('web/gameplay.js','void loadOverviewCurrentGame(tab, force && !quiet);','// serial current',1))
go('G3-byte-eviction-off','TestR92RiotMatchByteBudgetAcrossRestart',('binary_disk_budget.go','for len(c.strictEntries) > c.diskMaxEntries || c.strictBytes > c.diskMaxBytes {','for len(c.strictEntries) > c.diskMaxEntries {',1))
go('G3-restart-byte-accounting-off','TestR92RiotMatchByteBudgetAcrossRestart',('binary_disk_budget.go','c.strictBytes += info.Size()','// forgot existing bytes',1))
selected=os.environ.get('R92_MUTATION_FILTER')
if selected:cases=[c for c in cases if c[0] in selected.split(',')]
results=[]
for kind,cmd in [('go',['go','test','-run','^(TestR92(Riot|Pro|Community|Default)|TestR89(KeyConcurrency|ActualConcurrency|RiotMatchDisk))','-count=1']),('js',['node','--test','web/r89.test.cjs'])]:
 if not any(c[1]==kind for c in cases):continue
 p=subprocess.run(cmd,cwd=root,env=env,capture_output=True,text=True);(out/(kind+'-baseline.log')).write_text(p.stdout+p.stderr)
 if p.returncode:raise SystemExit(kind+' baseline red; stop')
for name,kind,test,changes in cases:
 with tempfile.TemporaryDirectory(prefix='r92-mutant-') as tmp:
  replacements={}
  for file,old,new,count in changes:
   data=replacements.get(file,(root/file).read_text())
   if data.count(old)!=count:raise SystemExit(f'{name}: anchor {data.count(old)} != {count}')
   replacements[file]=data.replace(old,new)
  overlay={}
  for file,data in replacements.items():
   target=pathlib.Path(tmp)/pathlib.Path(file).name;target.write_text(data);overlay[str(root/file)]=str(target)
  runenv=env.copy()
  if kind=='go':
   config=pathlib.Path(tmp)/'overlay.json';config.write_text(json.dumps({'Replace':overlay}));cmd=['go','test','-overlay',str(config),'-run','^'+test+'$','-count=1','-timeout','60s']
  else:
   runenv['R89_GAMEPLAY_SOURCE']=overlay[str(root/'web/gameplay.js')];cmd=['node','--test','--test-name-pattern',test,'web/r89.test.cjs']
  p=subprocess.run(cmd,cwd=root,env=runenv,capture_output=True,text=True,timeout=90);log=p.stdout+p.stderr;(out/(name+'.log')).write_text(log)
  killed=p.returncode!=0 and '[build failed]' not in log and 'SyntaxError' not in log and 'no tests to run' not in log
  results.append({'id':name,'killed':killed,'exit':p.returncode});print(name,'KILLED' if killed else 'SURVIVED/INVALID',flush=True);(out/'results.json').write_text(json.dumps(results,indent=2)+'\n')
  if not killed:raise SystemExit('invalid mutation '+name)
print('All',len(results),'mutations killed')
