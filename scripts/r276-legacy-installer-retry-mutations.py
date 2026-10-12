"""Only the approved legacy signature may retry; execute actual PowerShell function."""
import argparse,hashlib,json,os,subprocess,tempfile,time
from pathlib import Path
root=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--out',type=Path,required=True);a=p.parse_args();a.out.mkdir(parents=True,exist_ok=True)
source=root/'scripts/r206-real-upgrade-windows.ps1';original=source.read_text();digest=hashlib.sha256(source.read_bytes()).hexdigest();rows=[]
def run(name,file,negative=False):
    start=time.monotonic();command=[os.environ.get('PWSH_BIN','pwsh'),'-NoProfile','-File','scripts/r276-legacy-installer-retry.test.ps1','-RunSource',str(file)]
    result=subprocess.run(command,cwd=root,capture_output=True,text=True);log=result.stdout+result.stderr;(a.out/(name+'.log')).write_text(log)
    ok=result.returncode!=0 and 'ASSERTION FAILED:' in log if negative else result.returncode==0
    rows.append(dict(name=name,exitCode=result.returncode,assertionVerified=ok,duration_seconds=time.monotonic()-start,command=command));(a.out/'result.json').write_text(json.dumps(rows,indent=2)+'\n');print(json.dumps(rows[-1]),flush=True);assert ok,name
run('baseline',source)
with tempfile.TemporaryDirectory(prefix='r276-legacy-retry-mutants-') as temp:
    for name,needle,replacement in [
      ('signature-match-removed','$eligibleLegacy -and $matchesPointerSignature -and $retry -lt','$eligibleLegacy -and $retry -lt'),
      ('candidate-retries',"$PublishedLegacy -and $Version -ne $candidateVersion -and $Version -in @('0.12.65','0.12.68','0.12.76','0.12.80','0.12.81')","$PublishedLegacy -and $Version -in @('0.12.65','0.12.68','0.12.76','0.12.80','0.12.81','0.12.83')"),
      ('retry-limit-two','$maxLegacyRetries = 1','$maxLegacyRetries = 2')]:
        assert original.count(needle)==1,name
        file=Path(temp)/(name+'.ps1');file.write_text(original.replace(needle,replacement));run(name,file,True)
assert hashlib.sha256(source.read_bytes()).hexdigest()==digest
run('restored',source)
