import os
"""Canceled diagnostic producers must finish before the probe-count assertion."""
import argparse,hashlib,json,subprocess,tempfile,time
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--out',type=Path,required=True);a=p.parse_args();a.out.mkdir(parents=True,exist_ok=True)
file=ROOT/'backend/r276_update_probe_fixture_test.go';source=file.read_text();before=hashlib.sha256(file.read_bytes()).hexdigest();rows=[]
base=['go','test','-race','./backend','-count=1','-run','^Test(R196ProbeSelectsFastMirror|R276UpdateProbeDiagnosticsWaitForCanceledSource)$']
def run(name,command,negative=False):
 start=time.monotonic();r=subprocess.run(command,cwd=ROOT,env={**os.environ,"R287_MUTATION_PATH":"1" if negative else "0"},capture_output=True,text=True);log=r.stdout+r.stderr;(a.out/(name+'.log')).write_text(log)
 ok=(r.returncode!=0 and 'TestR276UpdateProbeDiagnosticsWaitForCanceledSource' in log and 'probe diagnostic fixture has 2 events; want all 3' in log and '--- FAIL:' in log and not any(x in log for x in ['build failed','syntax error:','undefined:','WARNING: DATA RACE'])) if negative else r.returncode==0
 rows.append(dict(name=name,exitCode=r.returncode,assertionVerified=ok,seconds=time.monotonic()-start));(a.out/'result.json').write_text(json.dumps(rows,indent=2)+'\n');print(json.dumps(rows[-1]),flush=True);assert ok,name
run('baseline',base)
with tempfile.TemporaryDirectory(prefix='r276-probe-diagnostics-') as folder:
 folder=Path(folder);needle='\tt.Helper()';assert source.count(needle)==1
 target=folder/'unpinned.go';target.write_text(source.replace(needle,needle+'\n\treturn'));overlay=folder/'overlay.json';overlay.write_text(json.dumps({'Replace':{str(file):str(target)}}))
 command=base[:];command.insert(2,'-overlay='+str(overlay));run('canceled-diagnostics-not-joined',command,True)
assert hashlib.sha256(file.read_bytes()).hexdigest()==before
run('restored',base)
