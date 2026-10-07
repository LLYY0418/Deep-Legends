import datetime
import hashlib
import json
import pathlib
import subprocess
import sys
import time

ROOT = pathlib.Path(__file__).resolve().parents[4]
OUT = ROOT / 'docs/history/reports/r242'
OUT.mkdir(parents=True, exist_ok=True)
TZ = datetime.timezone(datetime.timedelta(hours=8))

def artifacts():
    result = {}
    for release in ('r240', 'r239', 'r236'):
        directory = ROOT / f'dist/{release.upper()}-staging-public'
        assert directory.is_dir(), directory
        result[release] = [dict(path=str(p.relative_to(directory)),
            sha256=hashlib.sha256(p.read_bytes()).hexdigest(), size=p.stat().st_size,
            mtime_ns=p.stat().st_mtime_ns, mode=p.stat().st_mode)
            for p in sorted(directory.rglob('*')) if p.is_file()]
    return result

if sys.argv[1] == 'protect':
    stage = sys.argv[2]
    current = artifacts()
    (OUT / f'artifacts-{stage}.json').write_text(json.dumps(current, indent=2) + '\n')
    if stage == 'after':
        before = json.loads((OUT / 'artifacts-before.json').read_text())
        summary = {k: dict(files=len(v), unchanged=v == before[k]) for k, v in current.items()}
        (OUT / 'artifact-preservation.json').write_text(json.dumps(summary, indent=2) + '\n')
        print(json.dumps(summary))
        assert current == before, 'Existing staging artifacts changed'
    else:
        print(json.dumps({k: len(v) for k, v in current.items()}))
    sys.exit(0)

name, cwd, *command = sys.argv[1:]
started = datetime.datetime.now(TZ)
begin = time.monotonic()
with (OUT / f'{name}.log').open('w') as log:
    result = subprocess.run(command, cwd=ROOT / cwd, stdout=log, stderr=subprocess.STDOUT)
lines = (OUT / f'{name}.log').read_text().splitlines()
summary = dict(command=command, cwd=cwd, started=started.isoformat(),
    finished=datetime.datetime.now(TZ).isoformat(), elapsed_seconds=round(time.monotonic() - begin, 3),
    exit_code=result.returncode, last_5_lines=lines[-5:])
if '-json' in command:
    events = []
    for line in lines:
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            pass
    summary.update(top_level_pass=sum(e.get('Action') == 'pass' and 'Test' in e and '/' not in e['Test'] for e in events),
        skip=sum(e.get('Action') == 'skip' for e in events), fail=sum(e.get('Action') == 'fail' for e in events))
(OUT / f'{name}-summary.json').write_text(json.dumps(summary, ensure_ascii=False, indent=2) + '\n')
print(json.dumps(summary, ensure_ascii=False))
sys.exit(result.returncode or int(bool(summary.get('skip') or summary.get('fail'))))
