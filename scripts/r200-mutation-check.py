#!/usr/bin/env python3
"""R200 negative controls. Overlay production sources; never alter checkout."""
import json
import pathlib
import subprocess
import tempfile

root = pathlib.Path(__file__).resolve().parent.parent
report = root / 'docs/history/reports/r200'
report.mkdir(parents=True, exist_ok=True)
subset = (root / 'backend/champselect_subset.go').read_text()
execution = (root / 'backend/champselect_execution.go').read_text()
bench = (root / 'backend/champselect.go').read_text()

def replace_once(source, before, after):
    assert before in source, before
    return source.replace(before, after, 1)

cases = [
 ('card-full-pickable', 'TestR200OfferedSubsetOnly', {
  'backend/champselect_subset.go': replace_once(subset, 'if !session.AllowSubsetChampionPicks {', 'if true {'),
 }),
 ('failed-subset-fallback', 'TestR200SubsetUnavailableNeverFallsBack', {
  'backend/champselect_subset.go': replace_once(subset,
   'return nil, "unavailable", errors.New("subset unavailable")',
   'var full []int64; err:=client.RequestJSON(ctx,http.MethodGet,api+"/pickable-champion-ids",nil,&full);return full,api+"/pickable-champion-ids",err'),
 }),
 ('attempt-limit-no-advance', 'TestR200PickFailureAdvancesAndStopsAtSix', {
  'backend/champselect_execution.go': replace_once(execution,
   'r.champSelect.pickFailed[id][d.ChampionID] = true',
   'r.champSelect.pickFailed[id][d.ChampionID] = false'),
 }),
 ('bench-no-postflight-recovery', 'TestR200BenchPostflight/not-applied', {
  'backend/champselect.go': replace_once(bench,
   'if swap.SettledAt.IsZero() {',
   'if swap.SettledAt.IsZero() || swap.Decision.Action==champSelectActionBench {'),
 }),
]
results=[]
with tempfile.TemporaryDirectory(prefix='r200-overlay-') as temp:
    directory=pathlib.Path(temp)
    for name, test, files in cases:
        mappings={}
        for rel, content in files.items():
            path=directory/(name+'-'+pathlib.Path(rel).name)
            path.write_text(content)
            mappings[str(root/rel)]=str(path)
        overlay=directory/(name+'.json')
        overlay.write_text(json.dumps({'Replace':mappings}))
        run=subprocess.run(['go','test','-overlay',str(overlay),'./backend','-run','^'+test+'$','-count=1','-timeout=30s'],cwd=root,capture_output=True,text=True)
        output=run.stdout+run.stderr
        (report/(name+'.log')).write_text(output)
        failed=run.returncode!=0 and '--- FAIL: '+test.split('/')[0] in output and '[build failed]' not in output
        results.append({'mutation':name,'test':test,'exit_code':run.returncode,'assertion_failed':failed})
        print(name, 'EXPECTED FAIL' if failed else 'INVALID',flush=True)
(report/'mutations.json').write_text(json.dumps(results,indent=2)+'\n')
assert all(row['assertion_failed'] for row in results), results
