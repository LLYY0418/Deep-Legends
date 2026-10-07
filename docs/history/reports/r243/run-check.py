import datetime,json,pathlib,subprocess,sys,time
ROOT=pathlib.Path(__file__).resolve().parents[4];OUT=ROOT/'docs/history/reports/r243';TZ=datetime.timezone(datetime.timedelta(hours=8))
name,cwd,*command=sys.argv[1:];start=datetime.datetime.now(TZ);begin=time.monotonic()
with (OUT/f'{name}.log').open('w') as f:r=subprocess.run(command,cwd=ROOT/cwd,stdout=f,stderr=subprocess.STDOUT)
lines=(OUT/f'{name}.log').read_text().splitlines();summary=dict(command=command,cwd=cwd,started=start.isoformat(),finished=datetime.datetime.now(TZ).isoformat(),elapsed_seconds=round(time.monotonic()-begin,3),exit_code=r.returncode,last_5_lines=lines[-5:])
if '-json' in command:
 events=[]
 for line in lines:
  try:events.append(json.loads(line))
  except json.JSONDecodeError:pass
 summary.update(top_level_pass=sum(e.get('Action')=='pass' and 'Test' in e and '/' not in e['Test'] for e in events),skip=sum(e.get('Action')=='skip' for e in events),fail=sum(e.get('Action')=='fail' for e in events))
(OUT/f'{name}-summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n');print(json.dumps(summary,ensure_ascii=False));sys.exit(r.returncode or int(bool(summary.get('skip') or summary.get('fail'))))
