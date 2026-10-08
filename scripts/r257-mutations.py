"""R257 Go assertion mutations use overlays and never edit production files."""
import argparse, datetime, hashlib, json, pathlib, re, subprocess, tempfile, time
parser=argparse.ArgumentParser();parser.add_argument('--output',required=True);args=parser.parse_args()
root=pathlib.Path(__file__).resolve().parents[1];out=pathlib.Path(args.output);out.mkdir(parents=True,exist_ok=False)
paths=['backend/season_stats.go','backend/champion_table.go','backend/season_head.go','backend/season_head_refresh.go'];before={p:hashlib.sha256((root/p).read_bytes()).hexdigest() for p in paths};rows=[]
mutations=[
 ('remove-SSE-projection','backend/season_head.go','progress.applyUpstreamBoundary(cache)','// projection removed'),
 ('remove-refresh-projection','backend/season_head_refresh.go','progress.applyUpstreamBoundary(cache)','// projection removed'),
 ('remove-cap','backend/season_stats.go','p.UpstreamCapped = cache.CappedByUpstream','p.UpstreamCapped = false'),
 ('remove-ranked-date','backend/season_stats.go','p.RankedOldestAt = cache.Streams["ranked"].OldestCreatedAt','p.RankedOldestAt = 0'),
 ('ranked-wrong-stream','backend/season_stats.go','p.RankedOldestAt = cache.Streams["ranked"].OldestCreatedAt','p.RankedOldestAt = cache.Streams["mayhem"].OldestCreatedAt'),
 ('mayhem-wrong-stream','backend/season_stats.go','p.MayhemOldestAt = cache.Streams["mayhem"].OldestCreatedAt','p.MayhemOldestAt = cache.Streams["ranked"].OldestCreatedAt'),
 ('ignore-stream-cap','backend/season_stats.go','cache.CappedByUpstream && cache.Streams["ranked"].CappedByUpstream','cache.CappedByUpstream'),
 ('remove-snapshot-projection','backend/season_stats.go','progress.applyUpstreamBoundary(cache)','// projection removed'),
 ('remove-table-projection','backend/champion_table.go','progress.applyUpstreamBoundary(cache)','// projection removed'),
]
for name,target,old,new in mutations:
 source=(root/target).read_text();assert old in source,name;mutant=source.replace(old,new,1)
 with tempfile.TemporaryDirectory(prefix='r257-mutant-') as directory:
  temp=pathlib.Path(directory);replacement=temp/pathlib.Path(target).name;replacement.write_text(mutant)
  overlay=temp/'overlay.json';overlay.write_text(json.dumps({'Replace':{str(root/target):str(replacement)}}))
  command=['go','test','-overlay',str(overlay),'-count=1','-run','^TestR257','./backend']
  tick=time.monotonic();start=datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).isoformat()
  with (out/(name+'.log')).open('w') as log:result=subprocess.run(command,cwd=root,stdout=log,stderr=subprocess.STDOUT)
  end=datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8))).isoformat();log=(out/(name+'.log')).read_text(errors='replace')
  killed=result.returncode!=0 and '--- FAIL: TestR257' in log and not re.search(r'build failed|setup failed|undefined:',log)
  row=dict(name=name,command=command,start=start,end=end,seconds=round(time.monotonic()-tick,3),exit_code=result.returncode,assertion_killed=killed,last_five_lines=log.splitlines()[-5:]);rows.append(row);print(json.dumps(row),flush=True)
assert before=={p:hashlib.sha256((root/p).read_bytes()).hexdigest() for p in paths}
(out/'results.json').write_text(json.dumps(dict(unchanged_source=True,rows=rows,all_assertion_killed=all(row['assertion_killed'] for row in rows)),indent=2)+'\n')
raise SystemExit(0 if all(row['assertion_killed'] for row in rows) else 1)
