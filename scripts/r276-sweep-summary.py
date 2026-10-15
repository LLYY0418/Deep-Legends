"""Require exact coverage and intact evidence before declaring a clean sweep."""
import argparse, collections, json, math, statistics, subprocess
from pathlib import Path, PureWindowsPath
ROOT=Path(__file__).resolve().parents[1]
HEXDATA_TESTS=['TestHexdataSameHeroTenConcurrentLoadsUseOneRequestAndRestartUsesNone','TestHexdataColdRankingBudgetAndDiskRestart','TestR149MayhemCatalogsWarmConcurrentlyAndRestartFromDisk','TestR276HexdataRestartWaitsForStateAndUsesDisk','TestAdoptingNewBuildIDPrunesStaleHexdataFilesAsynchronously','TestPruneStaleHexdataBuildsRemovesExpiredBuildsOnly','TestPruneStaleHexdataBuildsMakesNoRequests','TestHexdataCircuitStatePersistsAcrossRestart','TestHexdataRestartClearsLongLivedPersistedCircuit','TestHexdataHTTP429OpensPersistentCircuitBeforeNextRequest']
PLAN=[(kind,index,iterations) for kind,count,iterations in [('linux-race',8,5),('linux-renderers',5,2),('browser-main',4,5),('browser-r265',4,5),('windows-go',4,3),('windows-renderers',4,3),('windows-cold',1,1),('linux-hexdata',1,1),('windows-hexdata',1,1)] for index in range(count)]
def upgrade_retry_evidence(folder,sha):
    errors=[];runs=[]
    if folder is None:return dict(total_retries=0,scope='no acceptance context supplied',runs=[]),[]
    for meta_file in sorted(folder.glob('*/acceptance-run.json')):
        try:
            meta=json.loads(meta_file.read_text());directory=meta_file.parent/'upgrade'
            data=json.loads((directory/'r276-legacy-installer-retries.json').read_text(encoding='utf-8-sig'))
            records=data['records'];assert isinstance(records,list)
            assert meta['head_sha']==sha and meta['conclusion']=='success' and meta['run_attempt']==1
            assert data['total_retries']==len(records)
            assert len(list(directory.glob('*-retry.json')))==len(records)
            def local(original):return directory/(PureWindowsPath(original).name if '\\' in original else Path(original).name)
            for record in records:
                assert record['phase']=='published-legacy-setup' and record['version'] in ['0.12.65','0.12.68','0.12.76','0.12.80','0.12.81']
                assert record['retry_count']==1 and record['attempt_count']==2 and record['completed'] is True
                assert record['first_exit_code']!=0 and record['second_exit_code']==0
                assert record['signature']=='invalid pointer found on stack + main.shellWndProc'
                first=local(record['first_stderr']);second=local(record['second_stderr']);assert first!=second and second.is_file()
                text=first.read_text(errors='replace');assert 'invalid pointer found on stack' in text and 'main.shellWndProc' in text
                assert json.loads(local(record['evidence_json']).read_text(encoding='utf-8-sig'))==record
                for stderr,exit_code in [(first,record['first_exit_code']),(second,record['second_exit_code'])]:
                    result=json.loads(stderr.with_name(stderr.name.replace('-stderr.log','-result.json')).read_text(encoding='utf-8-sig'))
                    assert result['version']==record['version'] and result['exit_code']==exit_code and result['stderr_bytes']==stderr.stat().st_size
            runs.append(dict(run=meta['id'],event=meta['event'],sha=meta['head_sha'],link=meta['html_url'],total_retries=len(records),records=records))
        except (OSError,ValueError,KeyError,AssertionError,TypeError) as e:errors.append('invalid acceptance legacy retry evidence '+str(meta_file)+': '+type(e).__name__)
    if len(runs)!=2 or {r['event'] for r in runs}!={'push','workflow_dispatch'}:errors.append('missing push/dispatch legacy retry acceptance evidence')
    return dict(total_retries=sum(r['total_retries'] for r in runs),scope='same-SHA acceptance CI; sweep does not execute installers',runs=runs),errors

def summarize(folder,jobs,repo,run,inventory=None,catalog=None,plan=PLAN,upgrade_folder=None):
    errors=[];samples=[];hexdata_stress=[];metrics=collections.defaultdict(list);failures=collections.defaultdict(list)
    jobmap={j['name']:j for page in jobs for j in page.get('jobs',[])}
    if inventory is None:
        inventory={'linux':sorted(str(p.relative_to(ROOT)).replace('\\','/') for d in ['backend/web','desktop','scripts'] for p in (ROOT/d).glob('*.test.cjs')),'win32':sorted(str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'desktop').glob('*.test.cjs'))}
    if catalog is None:catalog=json.loads(subprocess.check_output(['node','-e',"console.log(JSON.stringify(require('./scripts/r276-browser-catalog.cjs').guards.map(r=>r[0])))"],cwd=ROOT))
    seen_jobs={}
    def metric(name,value):
        if not isinstance(value,(float,int)) or not math.isfinite(value) or value<=0:errors.append('invalid metric '+name);return
        metrics[name].append(value)
    def read(file):
        try:return json.loads(file.read_text())
        except (OSError,ValueError) as e:errors.append(str(file.relative_to(folder))+': '+type(e).__name__);return None
    for file in folder.rglob('samples.json'):
        rows=read(file)
        if not isinstance(rows,list) or not rows:errors.append('empty samples '+str(file));continue
        key=(rows[0].get('kind'),rows[0].get('index'))
        if key in seen_jobs:errors.append('duplicate job '+str(key));continue
        seen_jobs[key]=(file.parent,rows)
    for kind,index,iterations in plan:
        key=(kind,index);name=kind+'-'+str(index);job=jobmap.get(name)
        if job is None or not job.get('html_url') or job.get('conclusion')!='success':errors.append('missing/failed job '+name)
        if key not in seen_jobs:errors.append('missing samples '+name);continue
        directory,rows=seen_jobs[key]
        labels=['hexdata'] if kind in ['linux-hexdata','windows-hexdata'] else ['full-go'] if kind in ['linux-race','windows-go'] else ['browser'] if kind.startswith('browser-') else ['calibration-before','renderers','calibration-after','budget'] if kind=='linux-renderers' else ['renderers'] if kind=='windows-renderers' else ['compile-cold','cold']
        expected={(iteration,label) for iteration in range(iterations) for label in labels};actual=[(s.get('iteration'),s.get('label')) for s in rows]
        if set(actual)!=expected or len(actual)!=len(expected):errors.append('incomplete/duplicate iterations '+name)
        for s in rows:
            iteration=s.get('iteration');s['jobLink']=job.get('html_url') if job else None
            if s.get('job')!=name or s.get('kind')!=kind or s.get('index')!=index or str(s.get('run'))!=str(run):errors.append('wrong sample identity '+name)
            if kind in ['linux-race','windows-go']:
                if not isinstance(iteration,int) or s.get('seed')!=int(run)*10000+index*100+iteration or s.get('cpu')!=[1,2,4][(index+iteration)%3]:errors.append('wrong scheduling seed/cpu '+name)
            metric(kind+'/'+str(s.get('label')),s.get('duration_ms'))
            if s.get('exitCode')!=0:
                for signature in s.get('signatures') or ['unclassified failure']:failures[signature].append(s)
            samples.append(s)
        for iteration in range(iterations):
            base=directory/str(iteration)
            if kind in ['linux-hexdata','windows-hexdata']:
                stress=read(base/'hexdata/stress-summary.json');platform='linux' if kind=='linux-hexdata' else 'win32'
                if not isinstance(stress,dict):continue
                stress_rows=stress.get('rows',[])
                if stress.get('platform')!=platform or stress.get('sha')!=rows[0].get('sha') or stress.get('count')!=200 or stress.get('cpu')!=[1,2,4] or stress.get('strategy')!='per-test-default-timeout' or stress.get('exitCode')!=0:errors.append('wrong Hexdata stress command/identity '+name)
                if sorted(r.get('test','') for r in stress_rows)!=sorted(HEXDATA_TESTS):errors.append('incomplete Hexdata test inventory '+name)
                try:events=[json.loads(line) for line in (base/'hexdata/stress.jsonl').read_text().splitlines() if line]
                except (OSError,ValueError):events=[];errors.append('missing/invalid Hexdata raw events '+name)
                if any(e.get('Action')=='fail' for e in events) or any('WARNING: DATA RACE' in e.get('Output','') for e in events):errors.append('failed/racing Hexdata raw events '+name)
                invocations=stress.get('invocations',[]);combined=[]
                if [i.get('test') for i in invocations]!=HEXDATA_TESTS:errors.append('incomplete/duplicate Hexdata invocation inventory '+name)
                for index,invocation in enumerate(invocations):
                    test=invocation.get('test');raw='invocation-'+str(index)+'.jsonl'
                    command=['go','test','-race','-json','-count=200','-cpu=1,2,4','-run','^('+str(test)+')$','./backend']
                    if invocation.get('command')!=command or invocation.get('raw')!=raw or invocation.get('exitCode')!=0 or invocation.get('error') is not None:errors.append('invalid Hexdata invocation command/status '+name+'/'+str(test))
                    try:part=[json.loads(line) for line in (base/'hexdata'/raw).read_text().splitlines() if line]
                    except (OSError,ValueError):part=[];errors.append('missing/invalid Hexdata invocation raw '+name+'/'+str(test))
                    combined.extend(part)
                    if sum(e.get('Action')=='pass' and e.get('Package')=='lol-loot-assistant/backend' and e.get('Test')==test for e in part)!=600 or sum(e.get('Action')=='pass' and e.get('Package')=='lol-loot-assistant/backend' and not e.get('Test') for e in part)!=1 or any(e.get('Action')=='fail' or 'WARNING: DATA RACE' in e.get('Output','') for e in part):errors.append('failed/partial Hexdata invocation '+name+'/'+str(test))
                if combined!=events:errors.append('Hexdata combined raw differs from invocation evidence '+name)

                for test in HEXDATA_TESTS:
                    evidence=[r for r in stress_rows if r.get('test')==test]
                    passes=[e for e in events if e.get('Action')=='pass' and e.get('Test')==test]
                    if len(evidence)!=1 or evidence[0].get('samples')!=600 or evidence[0].get('passed')!=600 or evidence[0].get('failed')!=0 or len(passes)!=600:errors.append('incomplete/failed Hexdata repetitions '+name+'/'+test)
                    durations=[e.get('Elapsed') for e in passes]
                    if not durations or any(not isinstance(v,(int,float)) or not math.isfinite(v) or v<0 for v in durations):errors.append('invalid Hexdata elapsed '+name+'/'+test)
                    else:metrics[kind+'/'+test].extend(v*1000 for v in durations)
                hexdata_stress.append(dict(kind=kind,jobLink=job.get('html_url') if job else None,**stress))
            elif kind in ['linux-renderers','windows-renderers']:

                timing=read(base/'renderer-timings.json');platform='linux' if kind=='linux-renderers' else 'win32'
                if not isinstance(timing,dict):continue
                files=timing.get('files',[])
                if timing.get('success') is not True or timing.get('platform')!=platform or sorted(row.get('file','') for row in files)!=inventory[platform]:errors.append('incomplete renderer inventory '+name)
                duration=timing.get('summary',{}).get('duration_ms');metric(platform+'/suite',duration)
                if isinstance(duration,(int,float)) and duration>(270000 if platform=='linux' else 240000):errors.append('renderer suite budget '+name)
                for item in files:
                    value=item.get('duration_ms');metric(platform+'/'+item.get('file',''),value)
                    if isinstance(value,(int,float)) and value>(100000 if platform=='linux' else 120000):errors.append('renderer file budget '+name)
                if platform=='linux':
                    budget=read(base/'budget.json')
                    if not isinstance(budget,dict) or budget.get('ok') is not True:errors.append('missing/failed calibrated budget '+name)
                    else:metric('linux/normalized-suite',budget.get('normalized_ms'))
            elif kind.startswith('browser-'):
                data=read(base/'guards/browser-job.json');selected=['r265'] if kind=='browser-r265' else [x for x in catalog if x!='r265']
                if not isinstance(data,list) or sorted(r.get('name','') for r in data)!=sorted(selected):errors.append('incomplete browser inventory '+name);continue
                for row in data:
                    if row.get('exitCode')!=0:errors.append('browser guard failed '+row.get('name',''))
                    metric('browser/'+row['name'],row.get('duration_ms'));offline=read(base/'guards'/str(row['name']+'-offline.json'))
                    if not isinstance(offline,dict) or offline.get('code')!=0:errors.append('missing/failed offline proof '+name+'/'+row['name'])
            elif kind=='windows-cold':
                cold=read(base/'cold/summary.json')
                if not isinstance(cold,dict) or cold.get('iterations')!=30 or cold.get('failures')!=0 or cold.get('retries')!=0 or len(cold.get('rows',[]))!=30:errors.append('incomplete cold sample inventory '+name)
                elif not all(row.get('passed') and row.get('timeline_logged') for row in cold['rows']):errors.append('cold samples missing pass/timeline '+name)
                else:
                    for row in cold['rows']:metric('windows/cold',row['elapsed_seconds']*1000)
    if set(seen_jobs)!={(kind,index) for kind,index,_ in plan}:errors.append('unexpected sample jobs')
    shas={s.get('sha') for s in samples}
    if len(shas)!=1 or None in shas or '' in shas:errors.append('samples do not share one SHA')
    signatures=[]
    for signature,rows in failures.items():
        by_kind=[]
        for kind,label in sorted({(row['kind'],row['label']) for row in rows}):
            count=sum(row['kind']==kind and row['label']==label for row in rows)
            denominator=sum(row['kind']==kind and row['label']==label for row in samples)
            by_kind.append(dict(kind=kind,label=label,count=count,denominator=denominator,fraction=count/denominator))
        signatures.append(dict(signature=signature,count=len(rows),denominator=len(samples),fraction=len(rows)/len(samples) if samples else 0,byKind=by_kind,jobLinks=sorted({row['jobLink'] for row in rows if row.get('jobLink')}),examples=rows))
    upgrade_retries,upgrade_errors=upgrade_retry_evidence(upgrade_folder,next(iter(shas)) if len(shas)==1 else None)
    errors.extend(upgrade_errors)
    return dict(legacyInstallerRetries=upgrade_retries,success=bool(samples) and not errors and not signatures,sha=next(iter(shas)) if len(shas)==1 else None,runLink='https://github.com/'+repo+'/actions/runs/'+str(run),samples=len(samples),coverage={str(key):len(value[1]) for key,value in seen_jobs.items()},hexdataStress=hexdata_stress,errors=errors,failures=signatures,metrics={name:dict(count=len(values),min_ms=min(values),median_ms=statistics.median(values),max_ms=max(values)) for name,values in metrics.items()},sampleRecords=samples)
if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--input',type=Path,required=True);p.add_argument('--jobs',type=Path,required=True);p.add_argument('--output',type=Path,required=True);p.add_argument('--repo',required=True);p.add_argument('--run',required=True);p.add_argument('--upgrade-evidence',type=Path,required=True);a=p.parse_args()
    summary=summarize(a.input,json.loads(a.jobs.read_text()),a.repo,a.run,upgrade_folder=a.upgrade_evidence)
    a.output.parent.mkdir(parents=True,exist_ok=True);a.output.write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps({k:summary[k] for k in ['success','samples','errors']},indent=2));raise SystemExit(0 if summary['success'] else 1)
