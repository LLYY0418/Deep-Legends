"""R273 first-card guarantees and badge metadata regressions; real DOM assertions."""
import argparse,json,os,subprocess,tempfile,time
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
import runpy
_proof=runpy.run_path(str(ROOT/'scripts/mutation-source-proof.py'));_snapshot=_proof['snapshot'](ROOT)
p=argparse.ArgumentParser();p.add_argument('--out',type=Path,required=True);args=p.parse_args();args.out=args.out.resolve();args.out.mkdir(parents=True,exist_ok=True);rows=[]
js=['node','--test','backend/web/r273-first-chunk.test.cjs','backend/web/r272-score-tags.test.cjs']
def run(name,env=None,negative=False,command=None):
 start=time.monotonic();r=subprocess.run(command or js,cwd=ROOT,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=90)
 log=r.stdout+r.stderr;(args.out/(name+'.log')).write_text(log)
 valid=(r.returncode!=0 and any(x in log for x in ['AssertionError','--- FAIL:']) and not any(s in log for s in ['SyntaxError','ReferenceError','TypeError','ENOENT','build failed'])) if negative else r.returncode==0
 rows.append(dict(name=name,negative=negative,exitCode=r.returncode,assertionVerified=valid,seconds=round(time.monotonic()-start,3)))
 (args.out/'result.json').write_text(json.dumps(rows,indent=2)+'\n');print(json.dumps(rows[-1]),flush=True);assert valid,name
run('baseline-js')
with tempfile.TemporaryDirectory(prefix='r273-mutants-') as tmp:
 def mutate(name,relative,before,after,key):
  source=(ROOT/relative).read_text();assert source.count(before)==1,(name,source.count(before));target=Path(tmp)/(name+'.js');target.write_text(source.replace(before,after,1));run(name,{key:str(target)},True)
 mutate('existing-skin-first-clock-cutoff','backend/web/app.js','index<end && (first || performance.now()-start<8)','index<end && performance.now()-start<8','R273_APP_SOURCE')
 mutate('empty-skin-first-clock-cutoff','backend/web/app.js','index<end && (first || performance.now()-started<8)','index<end && performance.now()-started<8','R273_APP_SOURCE')
 mutate('grouped-first-card-guarantee-lost','backend/web/app.js','const first = index === 0 && firstCards > 0;','const first = false;','R273_APP_SOURCE')
 mutate('chroma-counts-structure-as-card','backend/web/app.js','cards += Number(tasks[index++]()) || 0;','tasks[index++](); cards++;','R273_APP_SOURCE')
 mutate('loot-first-clock-cutoff','backend/web/app.js','first ? completed<40 : completed<1 || performance.now()-started<7','completed<1 || performance.now()-started<7','R273_APP_SOURCE')
 mutate('facade-first-clock-cutoff','backend/web/favorites-facade.js','first ? completed < 40 : completed < 1 || performance.now() - started < 7','completed < 1 || performance.now() - started < 7','R273_FACADE_SOURCE')
 badge='    const badge = scoreBadgeChip(record);\n    if (badge) return badge;\n'
 invalid='    if(card && (!Number.isInteger(Number(record?.rank)) || Number(record?.rank)<1 || !Number.isInteger(Number(record?.total)) || Number(record?.total)<Number(record.rank)))return "";\n'
 mutate('badge-requires-rank-metadata','backend/web/gameplay.js',badge+invalid,invalid+badge,'R272_GAMEPLAY_SOURCE')
with tempfile.TemporaryDirectory(prefix='r273-go-mutants-') as tmp:
 tmp=Path(tmp)
 pattern='^Test(R100SuccessfulDetailSurvivesCallerCancellation|R273(ForegroundOvertakesBackgroundResumeFlight|FailureTimingSchemaAndPrivacy))$'
 command=['go','test','./backend','-run',pattern,'-count=1','-v']
 run('baseline-go',command=command)
 source=(ROOT/'backend/riot_flight_priority.go').read_text()
 before='if priority == nil || priority.promoted || isRiotBackground(ctx) || ctx.Err() != nil {'
 assert source.count(before)==1
 target=tmp/'priority.go';target.write_text(source.replace(before,'if true {',1))
 overlay=tmp/'overlay.json';overlay.write_text(json.dumps({'Replace':{str(ROOT/'backend/riot_flight_priority.go'):str(target)}}))
 run('foreground-waits-background-flight',negative=True,command=['go','test','-overlay='+str(overlay),'./backend','-run',pattern,'-count=1','-v'])
 failure=(args.out/'foreground-waits-background-flight.log').read_text()
 for kind in ['account','summoner','ids','detail','client-identity']:
  assert '--- FAIL: TestR273ForegroundOvertakesBackgroundResumeFlight/'+kind in failure,kind
 assert failure.count('foreground joined background resume-window flight')==4
 assert failure.count('foreground client identity waited for background gate')==1
 records=[json.loads(line.split('R273_FAILURE_TIMING ',1)[1]) for line in failure.splitlines() if 'R273_FAILURE_TIMING ' in line]
 assert len(records)==10,len(records)
 spans=['detail_worker','detail_cache','detail_cache_lock','detail_store_lock','detail_disk_write','limiter_admission','limiter_queue','rate_wait','foreground_wait','profile_wait']
 for row in records:
  assert all(isinstance(row[k],dict) for k in ['state_initial','state_before','state_after','phases','spans_us'])
  assert all(k in row['phases'] for k in ['account','summoner','matchIDs','details'])
  assert all(isinstance(row['spans_us'].get(k),list) for k in spans)
  assert all(len(span)==2 and span[1]>=span[0] for entries in row['spans_us'].values() for span in entries)
  assert not any(secret in json.dumps(row) for secret in ['r98-subject','KR_1','Fixture','anonymous-lcu','RGAPI-'])
 assert any(row['spans_us']['foreground_wait'] for row in records if row['stage']=='controlled-background')
 assert any(row['spans_us']['detail_worker'] for row in records if row['stage']=='controlled-overlap')
 run('restored-go',command=command)
run('restored-js')

_proof['unchanged'](ROOT,_snapshot)
