#!/usr/bin/env python3
"""Refresh the bundled description-only CommunityDragon snapshot at release time.
Keep the raw variables/calculations: Go normalizes bundled and online identically.
Usage: python3 scripts/update-augment-descriptions.py [downloaded arena/zh_cn.json]
"""
import json, pathlib, subprocess, sys, tempfile
root = pathlib.Path(__file__).resolve().parent.parent
url = 'https://raw.communitydragon.org/latest/cdragon/arena/zh_cn.json'
if len(sys.argv) > 1:
    payload = pathlib.Path(sys.argv[1]).read_bytes()
else:
    with tempfile.TemporaryDirectory(prefix='augment-snapshot-') as folder:
        dest = pathlib.Path(folder) / 'arena.json'
        subprocess.run(['curl', '-fLsS', '--max-time', '30', '-A', 'Mozilla/5.0', url, '-o', str(dest)], check=True)
        payload = dest.read_bytes()
data = json.loads(payload)
rows = data['augments']
assert len(rows) >= 200 and all(r['id'] > 0 and r.get('name') and (r.get('desc') or r.get('tooltip')) for r in rows)
target = root / 'backend/data/augment_descriptions_20261017.json'
target.write_text(json.dumps(data, ensure_ascii=False, separators=(',', ':')) + '\n')
print(json.dumps({'source': url, 'rows': len(rows), 'target': str(target)}))
