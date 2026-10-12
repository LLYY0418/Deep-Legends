"""The same-origin recovery fixture must fail without its pinned context."""
import argparse,hashlib,json,subprocess,tempfile,time
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--out',type=Path,required=True);a=p.parse_args();a.out.mkdir(parents=True,exist_ok=True)
file=ROOT/'backend/r276_scope_route_timing_proof_test.go';source=file.read_text();before=hashlib.sha256(file.read_bytes()).hexdigest();rows=[]
base=['go','test','-race','./backend','-count=1','-run','^Test(R269DecryptRecoveryKeepsReturnedCredentialScope|R276RecoveryScopeFixtureControlsCooldown)$']
def run(name,command,negative=False):
 start=time.monotonic();r=subprocess.run(command,cwd=ROOT,capture_output=True,text=True);log=r.stdout+r.stderr;(a.out/(name+'.log')).write_text(log)
 ok=(r.returncode!=0 and 'TestR276RecoveryScopeFixtureControlsCooldown/pinned-one-recovery-context' in log and '1 2 0 0' in log and '--- FAIL:' in log and not any(x in log for x in ['build failed','syntax error:','undefined:','WARNING: DATA RACE'])) if negative else r.returncode==0
 rows.append(dict(name=name,exitCode=r.returncode,assertionVerified=ok,seconds=time.monotonic()-start));(a.out/'result.json').write_text(json.dumps(rows,indent=2)+'\n');print(json.dumps(rows[-1]),flush=True);assert ok,name
run('baseline',base)
with tempfile.TemporaryDirectory(prefix='r276-recovery-scope-') as folder:
 folder=Path(folder);needle='return riotPinnedIdentityContext(ctx)';assert source.count(needle)==1
 target=folder/'unpinned.go';target.write_text(source.replace(needle,'return ctx'));overlay=folder/'overlay.json';overlay.write_text(json.dumps({'Replace':{str(file):str(target)}}))
 command=base[:];command.insert(2,'-overlay='+str(overlay));run('origin-not-pinned',command,True)
assert hashlib.sha256(file.read_bytes()).hexdigest()==before
run('restored',base)
