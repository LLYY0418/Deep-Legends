"""Predetermined sweep samples, never retries; every failure remains fatal."""
import argparse, json, os, re, subprocess, time
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--kind',required=True);p.add_argument('--index',type=int,default=0);p.add_argument('--iterations',type=int,required=True);p.add_argument('--out',type=Path,required=True);a=p.parse_args();a.out.mkdir(parents=True,exist_ok=True)
rows=[]
def run(label,command,env,folder,seed=None,cpu=None):
    start=time.monotonic();log=folder/(label+'.log')
    with log.open('w') as handle:
        r=subprocess.run(command,cwd=ROOT,env={**os.environ,**env},stdout=handle,stderr=subprocess.STDOUT)
    text=log.read_text(errors='replace');failures=sorted(set(re.findall(r'--- FAIL: (\S+)|^FAIL ([^:\n]+)',text,re.M)))
    signatures=sorted({left or right for left,right in failures})
    if 'WARNING: DATA RACE' in text:signatures.append('WARNING: DATA RACE')
    if r.returncode and not signatures:
        signatures=sorted(set(re.findall(r'^(?:AssertionError[^\n]*|Error: [^\n]*|File exceeded [^\n]*|FAIL [^\n]*|::error::[^\n]*)',text,re.M))) or ['exit='+str(r.returncode)+': '+label]
    row=dict(kind=a.kind,label=label,index=a.index,iteration=iteration,seed=seed,cpu=cpu,exitCode=r.returncode,duration_ms=(time.monotonic()-start)*1000,signatures=signatures,log=str(log.relative_to(a.out)),job=os.environ.get('R276_JOB_NAME'),run=os.environ.get('GITHUB_RUN_ID'),sha=os.environ.get('GITHUB_SHA'))
    rows.append(row);(a.out/'samples.json').write_text(json.dumps(rows,indent=2)+'\n');print(json.dumps(row),flush=True)
    return r.returncode
for iteration in range(a.iterations):
    folder=a.out/str(iteration);folder.mkdir(exist_ok=True)
    # Disjoint seeds across jobs/runs/samples. The literal seed is preserved in
    # both the Go output and sample record, so failure scheduling is replayable.
    seed=int(os.environ.get('GITHUB_RUN_ID','276'))*10000+a.index*100+iteration
    cpu=[1,2,4][(a.index+iteration)%3]
    if a.kind in ['linux-race','windows-go']:
        args=['go','test','-race'] if a.kind=='linux-race' else ['go','test']
        args+=['-count=1','-shuffle='+str(seed),'-cpu='+str(cpu),'./...']
        run('full-go',['node','scripts/r276-offline.cjs','--',*args],{'R276_OFFLINE_OUTPUT':str(folder/'offline.json')},folder,seed,cpu)
    elif a.kind in ['linux-hexdata','windows-hexdata']:
        run('hexdata',['node','scripts/r276-offline.cjs','--','node','scripts/r276-hexdata-stress.cjs',str(folder/'hexdata')],{'CGO_ENABLED':'1','R276_OFFLINE_OUTPUT':str(folder/'offline.json')},folder)
    elif a.kind in ['browser-main','browser-r265']:
        run('browser',['node','scripts/r276-browser-job.cjs',a.kind.removeprefix('browser-'),str(folder/'guards')],{},folder)
    elif a.kind in ['linux-renderers','windows-renderers']:
        scope='all' if a.kind=='linux-renderers' else 'desktop';timings=folder/'renderer-timings.json'
        if scope=='all':run('calibration-before',['node','scripts/runner-calibration.cjs',str(folder/'before.json')],{},folder)
        run('renderers',['node','scripts/test-renderers.cjs',scope],{'R222_NODE_TIMING_OUTPUT':str(timings)},folder)
        if scope=='all':
            run('calibration-after',['node','scripts/runner-calibration.cjs',str(folder/'after.json')],{},folder)
            run('budget',['node','scripts/renderer-budget-gate.cjs',str(timings),str(folder/'before.json'),str(folder/'after.json'),str(folder/'budget.json')],{},folder)
    elif a.kind=='windows-cold':
        binary=a.out/'r252-r104.test.exe'
        compiled=True
        if not binary.exists():compiled=run('compile-cold',['go','test','-c','-o',str(binary),'./backend'],{},folder)==0
        if compiled and binary.exists():run('cold',['python','scripts/r252-r104-soak.py','--binary',str(binary),'--output',str(folder/'cold'),'--iterations','30'],{},folder)
    else:raise ValueError(a.kind)
assert rows,'no sweep samples ran'
raise SystemExit(1 if any(row['exitCode'] for row in rows) else 0)
