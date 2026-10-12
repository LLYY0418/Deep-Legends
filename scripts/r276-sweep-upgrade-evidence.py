"""Attach same-SHA acceptance CI legacy retry evidence to a sweep (no installer reruns)."""
import argparse,json,subprocess
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--runs',required=True);p.add_argument('--sha',required=True);p.add_argument('--repo',required=True);p.add_argument('--out',type=Path,required=True);a=p.parse_args()
ids=a.runs.split(',');assert len(ids)==2 and len(set(ids))==2 and all(i.isdigit() for i in ids),'exactly two acceptance CI run IDs required'
a.out.mkdir(parents=True,exist_ok=True)
for run in ids:
    folder=a.out/run;folder.mkdir()
    meta=json.loads(subprocess.check_output(['gh','api','repos/'+a.repo+'/actions/runs/'+run]))
    assert meta['head_sha']==a.sha and meta['conclusion']=='success' and meta['run_attempt']==1,'acceptance run must be same-SHA attempt-one success'
    (folder/'acceptance-run.json').write_text(json.dumps(meta,indent=2)+'\n')
    subprocess.run(['gh','run','download',run,'--repo',a.repo,'--name','r206-real-upgrade-evidence','--dir',str(folder/'upgrade')],check=True)
