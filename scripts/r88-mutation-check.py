#!/usr/bin/env python3
"""Run isolated R88 rollbacks. Production files are never rewritten."""
import json, os, pathlib, shutil, subprocess, tempfile
ROOT = pathlib.Path(__file__).resolve().parents[1]
REPORT = ROOT / 'docs/r88/mutations.json'

def replace(old, new):
    def edit(s):
        assert old in s, old
        return s.replace(old, new, 1)
    return edit

def old_live_report(s):
    baseline = '  function recordLiveRefresh(reason, source, phaseChanged = false) {\n    void fetch("/api/diagnostics/client", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ event: "live_refresh_client", reason, source, hidden: document.hidden, section: state.section, phaseChanged, liveRefreshQueued: Boolean(state.liveRefreshQueued) }) }).catch(() => {});\n  }'
    def span(text):
        a = text.index('  function recordLiveRefresh(')
        return a, text.index('\n  }', a) + 4
    a,b=span(s);c,d=span(baseline)
    return s[:a]+baseline[c:d]+s[b:]

CASES = [
 ('P3-live-post', 'web/gameplay.js', old_live_report, 'R88 thirteen ChampSelect events'),
 ('P3-status', 'web/app.js', replace('state.statusFailures >= 3', 'state.statusFailures >= 1'), 'R88 real status timeout'),
 ('P3-cache', 'gameplay_refresh.go', replace('response := c.response', 'response := c.response\n c.at = time.Time{}'), 'TestR88OrdinaryLiveLoadsCacheAndReuseWithoutExtendingTTL'),
 ('P3-identities', 'gameplay_summoner_cache.go', replace('reference = normalizeGameplayReference(reference)', 'return loadGameplaySummonerUncached(client, reference)\n reference = normalizeGameplayReference(reference)'), 'TestR88SummonerCacheCoalescesExpiresAndIsolatesClients'),
 ('P2-queue-gate', 'champselect_execution.go', replace('raw[0] == -1 && active', 'raw[0] == -1 && session.QueueID == 3110 && active'), 'TestR88ArenaBanWildcardActuallyPatches'),
 ('P2-hover-prefix', 'champselect.go', replace('strings.HasPrefix(banSource, "wildcard-")', 'strings.HasPrefix(banSource, "custom-")'), 'TestR88ArenaBanWildcardActuallyPatches'),
 ('P2-guard-direct', 'r88_queue_guard_test.go', replace('return constant.ToInt(value).Kind() == constant.Int', 'return constant.ToInt(value).Kind() == constant.Int && value.ExactString() != "3110"'), 'TestR88QueueGuardMutations'),
 ('P2-guard-alias', 'r88_queue_guard_test.go', replace('e.Obj != nil && aliases[e.Obj]', 'false && e.Obj != nil && aliases[e.Obj]'), 'TestR88QueueGuardMutations'),
 ('P2-guard-switch', 'r88_queue_guard_test.go', replace('if isQueue(n.Tag) {', 'if false && isQueue(n.Tag) {'), 'TestR88QueueGuardMutations'),
 ('P4-OP', 'yourgg_arena_rankings.go', replace('"OP": 0, ', ''), 'TestR88ArenaRankingsOPAndPartialSchema'),
 ('P4-whole-table', 'yourgg_arena_rankings.go', replace('if field != "" {', 'if field != "" { return championRankingResponse{}, invalid;'), 'TestR88ArenaRankingsOPAndPartialSchema'),
 ('A5-live-fields', 'features.go', replace('if request.Event == "live_refresh_client" {', 'if request.Event == "live_refresh_client" { event["duration_ms"] = 0'), 'TestR88DiagnosticContractsAndBuildMarker'),
 ('build-marker', 'storage.go', replace('record["build_fingerprint"] = buildFingerprint', ''), 'TestR88DiagnosticContractsAndBuildMarker'),
 ('P2-raw-head', 'champselect.go', replace('banProbe["raw_head"] = append([]int64{}, bannableIDs[:min(3, len(bannableIDs))]...)', 'banProbe["raw_head"] = []int64{}'), 'TestR88ArenaBanWildcardActuallyPatches'),
 ('P1-tooltip', 'web/gameplay.js', replace('data-tooltip="${escapeHTML(fullName)}"', 'data-tooltip="${escapeHTML(visibleName)}"'), 'R88 arena names omit tags'),
 ('P1-protected-layout', 'web/gameplay.css', replace('justify-self: start; padding-left: 18px;', 'justify-self: end; padding-left: 18px;'), 'R88 protected active'),
 ('A4-crash-hook', 'desktop/main.cjs', replace('app.on("render-process-gone",', 'app.on("ignored-render-process-gone",'), 'R88 crash hooks'),
 ('A6-log-window', 'desktop/desktop-log.cjs', replace('if (at < now - 7*24*60*60*1000 || at > now)', 'if (false)'), 'R88 seven-day desktop export'),
 ('A4-rotation', 'main.go', replace('"log_rotation": len(rotation) > 0 && rotation[0]', '"log_rotation": false'), 'TestR88DiagnosticContractsAndBuildMarker'),
]
results=[]
with tempfile.TemporaryDirectory(prefix='r88-mutants-') as temp:
    temp=pathlib.Path(temp)
    for name,relative,edit,pattern in CASES:
        folder=temp/name;folder.mkdir()
        source=ROOT/relative
        changed=edit(source.read_text());assert changed!=source.read_text()
        env=dict(os.environ, GOCACHE=str(ROOT/'.gocache'), GOPATH=str(ROOT/'.gopath'))
        if relative.startswith('web/'):
            for f in ['app.js','gameplay.js','runtime.js','champions.js','gameplay.css']:shutil.copyfile(ROOT/'web'/f,folder/f)
            (folder/source.name).write_text(changed)
            env['R88_WEB_ROOT']=str(folder)
            command=['node','--test','--test-name-pattern',pattern,'web/r88.test.cjs']
        elif relative.startswith('desktop/'):
            for f in ['main.cjs','desktop-log.cjs']:shutil.copyfile(ROOT/'desktop'/f,folder/f)
            (folder/source.name).write_text(changed)
            env['R88_DESKTOP_ROOT']=str(folder)
            command=['node','--test','--test-name-pattern',pattern,'desktop/r88-diagnostics.test.cjs']
        else:
            dest=folder/source.name;dest.write_text(changed)
            overlay=folder/'overlay.json';overlay.write_text(json.dumps({'Replace':{str(source):str(dest)}}))
            command=['go','test','-vet=off','-overlay',str(overlay),'-run','^'+pattern+'$','-count=1','.']
        run=subprocess.run(command,cwd=ROOT,env=env,capture_output=True,text=True,timeout=120)
        output=run.stdout+run.stderr
        killed=run.returncode!=0 and ('--- FAIL: '+pattern in output if relative.endswith('.go') else 'AssertionError' in output)
        results.append({'mutation':name,'test':pattern,'exit':run.returncode,'killed':killed})
        print(name, 'KILLED' if killed else 'FAILED TO VERIFY', flush=True)
        if not killed:print(output[-3000:],flush=True)
REPORT.parent.mkdir(parents=True,exist_ok=True);REPORT.write_text(json.dumps(results,indent=2)+'\n')
raise SystemExit(0 if all(r['killed'] for r in results) else 1)
