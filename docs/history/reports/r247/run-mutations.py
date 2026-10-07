from pathlib import Path
import tempfile,subprocess,datetime,time,json,os,shutil,sys
root=Path(__file__).resolve().parents[4];out=Path(__file__).parent;source=(root/'desktop/refresh-orchestration.test.cjs').read_text();rows=[]
tmp=Path(tempfile.mkdtemp(prefix='r247-status-commit-',dir='/private/tmp'))
try:
 for p in (root/'desktop').glob('*.cjs'):
  if p.name!='refresh-orchestration.test.cjs':(tmp/p.name).symlink_to(p)
 (tmp/'node_modules').symlink_to(root/'desktop/node_modules',target_is_directory=True)
 delayed=source.replace('        completedStatusRequests += 1;','        completedStatusRequests += 1;\n        if (completedStatusRequests === 1) w.document.dispatchEvent(new w.Event("visibilitychange"));\n        await new Promise(resolve => setTimeout(resolve, 50));')
 assert delayed!=source
 mutant=delayed
 conditions=[('w.__collectionStateForTest().dirty === true && !w.__collectionStateForTest().inFlight, "dirty status did not reach the shell"','completedStatusRequests >= 1, "dirty status did not reach the shell"'),('w.__collectionStateForTest().dirty === false && !w.__collectionStateForTest().inFlight, "clean status did not release the local rescan gate"','completedStatusRequests >= 2, "clean status did not release the local rescan gate"'),('w.__collectionStateForTest().dirty === true && !w.__collectionStateForTest().inFlight, "second dirty status did not reach the shell"','completedStatusRequests >= 3, "second dirty status did not reach the shell"')]
 for old,new in conditions:
  assert mutant.count(old)==1;mutant=mutant.replace(old,new)
 if len(sys.argv)>1 and sys.argv[1]=='debug':
  mutant=mutant.replace('        completedStatusRequests += 1;', '        completedStatusRequests += 1;\n        console.log("status_count", completedStatusRequests, payload.collectionDirty, w.__collectionStateForTest());')
  mutant=mutant.replace('    collectionDirty = false;', '    console.log("before_clean", completedStatusRequests, w.__collectionStateForTest());\n    collectionDirty = false;')
  mutant=mutant.replace('    w.Date.now = () => originalNow() + 60001;', '    console.log("before_advance", completedStatusRequests, w.__collectionStateForTest());\n    w.Date.now = () => originalNow() + 60001;')
 variants=[('old-counter',mutant,5),('committed-state-control',delayed,5)]
 if len(sys.argv)>1:variants=variants[:1];variants=[('probe',mutant,1)]
 for label,text,count in variants:
  (tmp/'refresh-orchestration.test.cjs').write_text(text)
  for i in range(1,count+1):
   started=datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)));at=time.monotonic();cmd=['node','--test','--test-name-pattern=dirty collection rescans',str(tmp/'refresh-orchestration.test.cjs')];p=subprocess.run(cmd,cwd=root,env={**os.environ,'DEEP_LEGENDS_WEB_ROOT':str(root/'backend/web')},stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=30);(out/f'mutation-{label}-{i}.log').write_text(p.stdout);killed=p.returncode==1 and 'AssertionError [ERR_ASSERTION]' in p.stdout and not any(s in p.stdout for s in ['SyntaxError','MODULE_NOT_FOUND']);row={'variant':label,'run':i,'started':started.isoformat(),'finished':datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).isoformat(),'duration_s':round(time.monotonic()-at,3),'exit_code':p.returncode,'assertion_killed':killed,'last5':p.stdout.rstrip().splitlines()[-5:]};rows.append(row);print(row,flush=True)
   (out/('mutation-probe.json' if len(sys.argv)>1 else 'mutations.json')).write_text(json.dumps({'fake_fetch_delay_ms':50,'additional_status_trigger':'one visible visibilitychange while first response is pending, reproducing overlapping status reads','original_assertions_and_budgets_preserved':True,'runs':rows,'temporary_directory_deleted':True},ensure_ascii=False,indent=2)+'\n')
   if (label=='old-counter' and not killed) or (label=='committed-state-control' and p.returncode):sys.exit(2)
finally:shutil.rmtree(tmp)
