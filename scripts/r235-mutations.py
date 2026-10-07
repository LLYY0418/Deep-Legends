#!/usr/bin/env python3
"""R235: mutate real production source, require assertion failure, always restore bytes."""
import hashlib,json,pathlib,subprocess,time
root=pathlib.Path(__file__).resolve().parents[1]
out=root/'docs/history/reports/r235'
out.mkdir(parents=True,exist_ok=True)

def shared_budget(s):
    marker='\t\tfor _, path := range []string{"/health", "/r/kr/lol/status/v4/platform-data"} {'
    s=s.replace(marker,'\t\tsharedContext, sharedCancel := context.WithTimeout(httptrace.WithClientTrace(context.Background(), trace), 8*time.Second)\n\t\tdefer sharedCancel()\n'+marker,1)
    return s.replace('ctx, cancel := context.WithTimeout(httptrace.WithClientTrace(context.Background(), trace), 8*time.Second)','ctx, cancel := sharedContext, func() {}',1)

cases=[
 ('p1-clear-migration','backend/season_stats.go',lambda s:s.replace('cache.SchemaVersion = seasonStatsCacheSchemaVersion','cache.GameIDs = nil; cache.Stats = nil; cache.QueueStats = nil\n\tcache.SchemaVersion = seasonStatsCacheSchemaVersion',1),['go','test','./backend','-run','^TestR235MigrationKeepsHistoricalGames$','-count=1']),
 ('p11-fixed-four','backend/arena_result.go',lambda s:s.replace('placement > (count+1)/2','placement > 4',1),['go','test','./backend','-run','^TestR235ArenaResultBoundariesAndOptionalFields$','-count=1']),
 ('p2-serial-streams','backend/season_stats.go',lambda s:s.replace('if scan.headOnly {\n\t\ta.seasonScanHeadConcurrent','if scan.headOnly && false {\n\t\ta.seasonScanHeadConcurrent',1),['go','test','./backend','-run','^TestR235HeadStreamsConcurrentAndFirstPagePush$','-count=1']),
 ('p5-shared-probe-budget','backend/riot_relay.go',shared_budget,['go','test','./backend','-run','^TestR235RelayProbeIndependentBudgets$','-count=1']),
 ('p8-fixed-15s','backend/web/app.js',lambda s:s.replace('}, 120000);','}, 15000);',1),['node','--test','--test-name-pattern=bounded fallback|30-second startup','backend/web/r219.test.cjs']),
 ('p9-slice-four','backend/web/gameplay.js',lambda s:s.replace('tags.map(tag=>`<span data-match-tag>','tags.slice(0,4).map(tag=>`<span data-match-tag>',1),['node','--test','--test-name-pattern=data labels','backend/web/r235.test.cjs']),
]
receipts=[]
for name,relative,mutate,cmd in cases:
    p=root/relative; original=p.read_bytes(); changed=mutate(original.decode()).encode(); assert changed!=original,name+' did not mutate'
    start=time.monotonic()
    try:
        p.write_bytes(changed)
        result=subprocess.run(cmd,cwd=root,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=100)
        output=result.stdout.decode(errors='replace');(out/(name+'.log')).write_text(output)
        killed=result.returncode!=0 and ('--- FAIL:' in output or '✖' in output) and 'build failed' not in output
        receipts.append({'name':name,'file':relative,'command':cmd,'returncode':result.returncode,'killed':killed,'seconds':round(time.monotonic()-start,3),'restored_sha256':hashlib.sha256(original).hexdigest()})
        print(json.dumps(receipts[-1]),flush=True)
        assert killed,name+' survived or failed to compile'
    finally:
        p.write_bytes(original)
        assert p.read_bytes()==original
        (out/'mutations.json').write_text(json.dumps(receipts,indent=2)+'\n')
