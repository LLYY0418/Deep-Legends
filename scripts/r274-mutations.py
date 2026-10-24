import os
"""R274 slow-I/O gates exercise production cache code, even on fast disks."""
import argparse,json,subprocess,tempfile,time,re
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
import runpy
_proof=runpy.run_path(str(ROOT/'scripts/mutation-source-proof.py'));_snapshot=_proof['snapshot'](ROOT)
p=argparse.ArgumentParser();p.add_argument('--out',type=Path,required=True);args=p.parse_args();args.out=args.out.resolve();args.out.mkdir(parents=True,exist_ok=True)
pattern='^Test(R274.*|R100SuccessfulDetailSurvivesCallerCancellation|R272DetailDiskCrossCredential)$'
rows=[]
def run(name,overlay=None,expected=()):
    selected='^('+ '|'.join(re.escape(test.split('/')[0]) for test in expected)+')$' if expected else pattern
    command=['go','test']+(['-overlay='+str(overlay)] if overlay else [])+['./backend','-run',selected,'-count=1','-v']
    started=time.monotonic();result=subprocess.run(command,cwd=ROOT,env={**os.environ,"R287_MUTATION_PATH":"1" if expected else "0"},capture_output=True,text=True,timeout=90)
    log=result.stdout+result.stderr;(args.out/(name+'.log')).write_text(log)
    if expected:
        verified=result.returncode!=0 and all('--- FAIL: '+test in log for test in expected) and not any(x in log for x in ['build failed','undefined:','syntax error:','bind: operation not permitted'])
    else:
        verified=result.returncode==0 and all('--- PASS: '+test in log for test in (['TestR274ARAMKitReadOutsideProviderLock','TestR274ConcurrentDetailsReturnBeforeDisk','TestR274MemoryHitDuringBlockedWrite','TestR274ClearFencesBlockedAndQueuedWrites','TestR274SuccessfulCanceledCallerStillPersists','TestR274MemoryHitDoesNotWaitForTouch','TestR274IdentityReturnsBeforeDiskWrite','TestR274MatchTierReturnsBeforeDiskWrite','TestR274InvalidationHidesDiskWhileWriterBlocked','TestR274DiskQueueBoundedAndCoalesced','TestR274ShutdownDropsPendingKeepsExistingFiles','TestR274PromotedLeaderDoesNotPersist','TestR274DiskWriteDiagnosticsRetained','TestR274PrestigeReadAndWriteDoNotHoldStoreLock','TestR274PrestigeTouchDoesNotBlockForeground','TestR274ProgressWatchdogAllowsSlowDrain','TestR274ProgressWatchdogRejectsStalledDrain','TestR100SuccessfulDetailSurvivesCallerCancellation','TestR272DetailDiskCrossCredential'] if name=='baseline' else ['TestR274ProgressWatchdogAllowsSlowDrain','TestR274ProgressWatchdogRejectsStalledDrain']))
    rows.append({'name':name,'negative':bool(expected),'exitCode':result.returncode,'expectedFailures':list(expected),'assertionVerified':verified,'seconds':round(time.monotonic()-started,3)})
    (args.out/'result.json').write_text(json.dumps(rows,indent=2)+'\n');print(json.dumps(rows[-1]),flush=True);assert verified,name

run('baseline')
with tempfile.TemporaryDirectory(prefix='r274-mutants-') as tmp:
    tmp=Path(tmp)
    def mutate(name,relative,before,after,expected):
        source=(ROOT/relative).read_text();assert source.count(before)==1,(name,source.count(before))
        target=tmp/(name+'.go');target.write_text(source.replace(before,after,1))
        overlay=tmp/(name+'.json');overlay.write_text(json.dumps({'Replace':{str(ROOT/relative):str(target)}}))
        run(name,overlay,expected)
    mutate('disk-write-back-under-cacheMu','backend/riot_api.go','\t\tp.storeMatchLocked(matchID, &match)','\t\tif status != "disk" { p.persistRiotMatch(key, &match) }\n\t\tp.storeMatchLocked(matchID, &match)',('TestR274ConcurrentDetailsReturnBeforeDisk','TestR274MemoryHitDuringBlockedWrite','TestR274ClearFencesBlockedAndQueuedWrites'))
    mutate('touch-back-under-cacheMu','backend/riot_api.go','\t\tticket := p.matchDiskTicket("riot-match-v4|" + matchID)','\t\tp.touchRiotMatchDisk("riot-match-v4|" + matchID)\n\t\tticket := p.matchDiskTicket("riot-match-v4|" + matchID)',('TestR274MemoryHitDoesNotWaitForTouch',))
    mutate('clear-generation-write-check-lost','backend/riot_match_cache.go','p.persistRiotMatchChecked(ticket.key, match, valid)','p.persistRiotMatchChecked(ticket.key, match, nil)',('TestR274ClearFencesBlockedAndQueuedWrites',))
    mutate('identity-foreground-writes-synchronously','backend/champion_cache.go','\t\t\tc.enqueueDiskWrite(entry)','\t\t\t_ = c.writeDisk(entry)',('TestR274IdentityReturnsBeforeDiskWrite',))
    mutate('invalidated-disk-visible-before-delete','backend/champion_cache.go','if c.dir == "" || c.diskQueue.unreadable(key) {','if c.dir == "" {',('TestR274InvalidationHidesDiskWhileWriterBlocked',))
    mutate('paid-cancellation-cache-lost','backend/riot_api.go','if flight.generation == p.matchGeneration && (flight.priority == nil || !flight.priority.promoted) {','if ctx.Err() == nil && flight.generation == p.matchGeneration && (flight.priority == nil || !flight.priority.promoted) {',('TestR100SuccessfulDetailSurvivesCallerCancellation',))
    mutate('prestige-read-back-under-store-lock','backend/prestige_cache.go','\tif cache.beforeDiskIO != nil {\n\t\tcache.beforeDiskIO("prestige-read")','\ts.mu.Lock(); defer s.mu.Unlock()\n\tif cache.beforeDiskIO != nil {\n\t\tcache.beforeDiskIO("prestige-read")',('TestR274PrestigeReadAndWriteDoNotHoldStoreLock/prestige-read',))
    mutate('watchdog-fixed-five-second-total','backend/r274_watchdog_test.go','resetOnProgress := true\n\ttimer := clock.NewTimer(r274NoProgressLimit)','resetOnProgress := false\n\ttimer := clock.NewTimer(5 * time.Second)',('TestR274ProgressWatchdogAllowsSlowDrain',))
    mutate('watchdog-never-fails','backend/r274_watchdog_test.go','return zero, fmt.Errorf("disk queue stalled: no task completed for %s; completed=%d initial_completed=%d renewals=%d pending=%d running=%d", r274NoProgressLimit, latest.completed, initialCompleted, renewals, latest.pending, latest.running)','_ = initialCompleted; _ = renewals; continue // mutant: never reject a stalled queue',('TestR274ProgressWatchdogRejectsStalledDrain',))
_proof['unchanged'](ROOT,_snapshot)
pattern='^TestR274ProgressWatchdog'
run('restored-target',expected=())

_proof['unchanged'](ROOT,_snapshot)
