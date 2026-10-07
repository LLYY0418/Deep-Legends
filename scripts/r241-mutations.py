#!/usr/bin/env python3
import json,subprocess,hashlib,time
from pathlib import Path
root=Path(__file__).resolve().parents[1];out=root/'docs/history/reports/r241';out.mkdir(exist_ok=True)
mutations=[
 ('p2-mode-filter','backend/web/gameplay.js',lambda s:s.replace('const modeSet=Number(match.mapId)===11 ? "rift" : Number(match.mapId)===12 ? "aram" : Number(match.mapId)===30 ? "arena" : "other";','const modeSet="rift";'),['node','--test','--test-name-pattern=map matrix','backend/web/r241.test.cjs']),
 ('p3-wrap','backend/web/gameplay.css',lambda s:s.replace('grid-column: 1 / -1; flex-wrap: nowrap;','grid-column: 1 / -1; flex-wrap: wrap;'),['node','scripts/r241-browser.cjs','--guard-only']),
 ('p4-no-freshness','backend/season_head_refresh.go',lambda s:s.replace('if known {','if false && known {'),['go','test','./backend','-run','^TestR241HeadFreshness','-count=1']),
 ('p6-empty-tail','backend/web/gameplay.css',lambda s:s.replace('minmax(96px,1fr) minmax(188px,1.4fr)','minmax(96px,1fr) minmax(188px,1.4fr) minmax(0,1fr)'),['node','scripts/r241-browser.cjs','--guard-only']),
]
rows=[]
for name,file,mutate,cmd in mutations:
 p=root/file;original=p.read_bytes();changed=mutate(original.decode()).encode();assert changed!=original,name
 started=time.monotonic()
 try:
  p.write_bytes(changed)
  result=subprocess.run(cmd,cwd=root,capture_output=True,timeout=100)
  log=result.stdout+result.stderr;(out/(name+'.log')).write_bytes(log)
  killed=result.returncode!=0 and (b'AssertionError' in log or b'r241_test.go:' in log) and b'build failed' not in log and b'timeout !' not in log
  rows.append({'name':name,'file':file,'command':cmd,'exit_code':result.returncode,'killed':killed,'seconds':round(time.monotonic()-started,3)})
  print(name,'KILLED' if killed else 'SURVIVED',flush=True)
 finally:
  p.write_bytes(original);assert hashlib.sha256(p.read_bytes()).digest()==hashlib.sha256(original).digest()
(out/'mutations.json').write_text(json.dumps(rows,indent=2)+'\n')
assert all(r['killed'] for r in rows),rows
