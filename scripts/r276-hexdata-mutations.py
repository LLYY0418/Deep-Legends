"""Restart barrier mutants execute assertions via overlays; no checkout edits."""
import argparse,hashlib,json,subprocess,tempfile,time
from pathlib import Path
root=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--out',type=Path,required=True);a=p.parse_args();a.out.mkdir(parents=True,exist_ok=True)
rows=[]
def run(name,overlay=None,pattern='^TestR276HexdataRestartWaitsForStateAndUsesDisk$'):
    command=['go','test','-race','./backend','-count=1','-run',pattern]
    if overlay:command.insert(2,'-overlay='+str(overlay))
    start=time.monotonic();r=subprocess.run(command,cwd=root,capture_output=True,text=True);log=r.stdout+r.stderr;(a.out/(name+'.log')).write_text(log)
    signature='restart barrier returned while state writer gate was closed' if name=='state-owner-omitted' else 'restart made 1 additional same-hero requests'
    ok=(r.returncode!=0 and signature in log and '--- FAIL:' in log) if overlay else r.returncode==0
    rows.append({'name':name,'exitCode':r.returncode,'assertionVerified':ok,'seconds':time.monotonic()-start});(a.out/'result.json').write_text(json.dumps(rows,indent=2)+'\n');print(json.dumps(rows[-1]),flush=True);assert ok,name
files=[root/'backend/r276_hexdata_restart_test.go',root/'backend/hexdata_test.go'];hashes={str(f):hashlib.sha256(f.read_bytes()).hexdigest() for f in files}
run('baseline')
with tempfile.TemporaryDirectory(prefix='r276-hexdata-mutant-') as folder:
    folder=Path(folder)
    for name,file,needle,replacement,pattern in [
        ('state-owner-omitted',files[0],'p.communityImageCache, p.hexdata.stateDisk}','p.communityImageCache}',None),
        ('main-test-barrier-omitted',files[1],'r276HexdataRestartDrain(provider, r276ReleaseStateClock{r274RealClock{}, stateGate})','error(nil)','^TestHexdataSameHeroTenConcurrentLoadsUseOneRequestAndRestartUsesNone$')]:
        source=file.read_text();assert source.count(needle)==1,(name,source.count(needle));target=folder/(name+'.go');target.write_text(source.replace(needle,replacement));overlay=folder/(name+'.json');overlay.write_text(json.dumps({'Replace':{str(file):str(target)}}));run(name,overlay,pattern or '^TestR276HexdataRestartWaitsForStateAndUsesDisk$')
assert all(hashlib.sha256(f.read_bytes()).hexdigest()==hashes[str(f)] for f in files)
run('restored',pattern='^Test(R276Hexdata.*|HexdataSameHeroTenConcurrentLoadsUseOneRequestAndRestartUsesNone)$')
