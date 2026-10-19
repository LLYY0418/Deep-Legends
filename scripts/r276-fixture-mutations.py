"""Deterministic R276 fixture mutations use overlays and never edit checkout."""
import argparse, hashlib, json, os, subprocess, tempfile, time
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--out',type=Path,required=True);a=p.parse_args();a.out.mkdir(parents=True,exist_ok=True)
file=ROOT/'backend/r276_fixture_test.go';source=file.read_text();before=hashlib.sha256(file.read_bytes()).hexdigest();rows=[]
def run(name,command,signature=None):
    # Compilation is outside the fixture's behavior budget. Go's test timeout
    # and each fixture's no-progress watchdog still detect stalled test work.
    start=time.monotonic();r=subprocess.run(command,cwd=ROOT,env={**os.environ,"R287_MUTATION_PATH":"1" if signature is not None else "0"},capture_output=True,text=True)
    log=r.stdout+r.stderr;(a.out/(name+'.log')).write_text(log)
    ok=r.returncode==0 if signature is None else r.returncode!=0 and signature in log and '--- FAIL:' in log and not any(x in log for x in ['build failed','syntax error:','undefined:'])
    rows.append(dict(name=name,exitCode=r.returncode,assertionVerified=ok,seconds=time.monotonic()-start));(a.out/'result.json').write_text(json.dumps(rows,indent=2)+'\n');print(json.dumps(rows[-1]),flush=True);assert ok,name
baseline=['go','test','-race','./backend','-count=1','-run','^Test(R102SeedAnchorKeysAreUniquePerAccount|R276ParallelFixtureCounter|R276RefreshCompletionBarrierGated)$']
run('baseline',baseline)
with tempfile.TemporaryDirectory(prefix='r276-fixture-mutations-') as tmp:
    tmp=Path(tmp)
    for name,mutant,pattern,signature in [
        ('non-atomic-counter',source.replace('calls     atomic.Int64','calls     int64').replace('c.calls.Add(1)','c.calls++').replace('c.calls.Load()','c.calls').replace('\n\t"sync/atomic"',''),'^TestR276ParallelFixtureCounter$','WARNING: DATA RACE'),
        ('no-completion-barrier',source.replace('if err := counter.wait(ctx, 4, settled); err != nil {','if err := error(nil); err != nil {').replace('ctx, cancel := context.WithCancel(context.Background())','_, cancel := context.WithCancel(context.Background())'),'^TestR276RefreshCompletionBarrierGated$','completion barrier returned while both refresh gates were closed')]:
        assert mutant!=source;target=tmp/(name+'.go');target.write_text(mutant);overlay=tmp/(name+'.json');overlay.write_text(json.dumps({'Replace':{str(file):str(target)}}))
        run(name,['go','test','-race','-overlay='+str(overlay),'./backend','-count=1','-run',pattern],signature)
assert hashlib.sha256(file.read_bytes()).hexdigest()==before
run('restored',baseline)

with tempfile.TemporaryDirectory(prefix='r276-audit-mutations-') as tmp:
    tmp=Path(tmp)
    cases=[
        ('relay-summary-isolation-lost','backend/quality_test.go','\tr276IsolateRelaySummary(t)\n','','./backend','^TestR276DiagnosticCountIgnoresEarlierFixtureSummary$','diagnostic log stats'),
        ('roster-age-frozen-at-eight','backend/r276_isolation_test.go','return age >= rounded(from) && age <= rounded(until)','_ = rounded; return age == 8','./backend','^TestR276RosterAgeAccountsForElapsedCapture$','real elapsed capture age rejected'),
        ('capture-audit-disabled','scripts/testcaptureguard/guard.go','if !a.captured(e, f) {','if true {','./scripts/testcaptureguard','^TestCapturedWrites$','--- FAIL: TestCapturedWrites/counter'),
        ('stale-capture-exemption-accepted','scripts/testcaptureguard/exemptions.go','if entry.SourceSHA256 != hash {','if false && entry.SourceSHA256 != hash {','./scripts/testcaptureguard','^TestReviewedExceptionIsSourceBound$','stale exception accepted')]
    for name,relative,beforeText,afterText,package,pattern,signature in cases:
        file=ROOT/relative;source=file.read_text();assert source.count(beforeText)==1,(name,source.count(beforeText))
        target=tmp/(name+'.go');target.write_text(source.replace(beforeText,afterText,1));overlay=tmp/(name+'.json');overlay.write_text(json.dumps({'Replace':{str(file):str(target)}}))
        run(name,['go','test','-race','-overlay='+str(overlay),package,'-count=1','-run',pattern],signature)
run('isolation-restored',['go','test','-race','./backend','-count=1','-run','^TestR276(DiagnosticCount.*|RosterAge.*)$'])
run('audit-restored',['go','test','-race','./scripts/testcaptureguard','-count=1'])
