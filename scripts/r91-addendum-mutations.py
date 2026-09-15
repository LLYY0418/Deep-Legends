#!/usr/bin/env python3
"""Mutate isolated source copies; require behavioral test failures, never build failures."""
import json, os, pathlib, subprocess, tempfile
ROOT = pathlib.Path(__file__).resolve().parent.parent
OUT = pathlib.Path(os.environ.get('R91_MUTATION_OUTPUT', str(ROOT / 'docs/r91-addendum/mutations')))
OUT.mkdir(parents=True, exist_ok=True)
cases = [
 ('quota-short-return', 'web/gameplay.js', '        return false;\n      }\n      if (error.name !== "RequestCancelled")', '      }\n      if (error.name !== "RequestCancelled")', 'js', '429 stays silent'),
 ('quota-classification', 'web/gameplay.js', 'status === 429 ? "rate-limited" : "http"', '"http"', 'js', 'header-only'),
 ('duplicate-five-first', 'web/gameplay.js', 'const requestCount = state.settings.matchCount;', 'const requestCount = 5;', 'js', 'exactly one count-20'),
 ('history-skeleton-flicker', 'web/gameplay.js', 'state.liveLoading === true && !player.recentGames?.length && !["ready", "empty", "unavailable", "failed"].includes(player.historyState)', 'state.liveLoading === true', 'js', 'existing history'),
 ('roster-dom-replaced', 'web/gameplay.js', 'nodes.liveContent._recommendationMarkup === markup &&', 'false &&', 'js', 'preserves roster DOM'),
 ('rank-default', 'web/suite.js', 'tier: lol.rankedLeagueTier || "UNRANKED"', 'tier: lol.rankedLeagueTier || "DIAMOND"', 'js', 'facade defaults'),
 ('loading-probe-interval', 'web/gameplay.js', 'loadingProbe ? 5_000 :', 'loadingProbe ? 20_000 :', 'js', 'Loading and early'),
 ('match-persistence', 'riot_api.go', 'p.persistRiotMatch(key, &match)', '// persistence removed', 'go', 'TestR91AddendumCacheThreeStates'),
 ('memory-cache-counter', 'riot_api.go', 'tracker.matchesFromMemory++', '// counter removed', 'go', 'TestR91AddendumCacheThreeStates'),
 ('loading-grouping', 'gameplay.go', 'if arenaMode && (phase == "GameStart" || phase == "InProgress" || phase == "Reconnect")', 'if arenaMode && (phase == "InProgress" || phase == "Reconnect")', 'go', 'TestR91AddendumGameStartGroupingAndCarryover'),
 ('loading-shape-sample', 'gameplay.go', 'event["game_id"], event["forced_sample"] = session.GameData.GameID, true\n\t\t\t\ta.appendDiagnosticEvent(event)', 'event["game_id"], event["forced_sample"] = session.GameData.GameID, true', 'go', 'TestR91AddendumForcedShapeAndUnattemptedPhase'),
 ('phase-not-attempted', 'arena_live_grouping.go', 'a.recordArenaOrderRejected(&previous, "phase", "phase-not-attempted")', '_ = previous', 'go', 'TestR91AddendumForcedShapeAndUnattemptedPhase'),
 ('playerlist-rejection-event', 'arena_live_grouping.go', 'a.recordArenaOrderRejected(response, "live-client", "playerlist-unavailable")', '// event removed', 'go', 'TestR91AddendumGameStartGroupingAndCarryover'),
 ('roster-rejection-event', 'arena_live_grouping.go', 'a.recordArenaOrderRejected(response, "session-order", reason)', '_ = reason', 'go', 'TestR91AddendumGameStartGroupingAndCarryover'),
 ('loading-carryover', 'arena_live_grouping.go', 'groups, ok = a.carriedArenaSessionGroups(client, current, response)', 'groups, ok = arenaSessionOrderGroups(response.QueueID, raw)', 'go', 'TestR91AddendumGameStartGroupingAndCarryover'),
 ('hover-cleared-takeover', 'champselect_takeover.go', 'if last.Decision.ForceHover {\n\t\treturn false\n\t}', 'if last.Decision.ForceHover {\n\t\treturn true\n\t}', 'go', 'TestR91AddendumWildcardHoverClearKeepsArmedLock'),
 ('hover-clear-lock-rejected', 'champselect.go', 'last.Confirmed && action.ChampionID != 0 && action.ChampionID != decision.ChampionID', 'last.Confirmed && action.ChampionID != decision.ChampionID', 'go', 'TestR91AddendumWildcardHoverClearKeepsArmedLock'),
 ('watch-save-phase', 'watch_rules.go', 'runner.handlePhase(client, phase)', '_ = phase', 'go', 'TestR91AddendumSaveReevaluatesCurrentPhase'),
 ('retryable-cache-ttl', 'gameplay_refresh.go', 'phase == "ChampSelect" || phase == "GameStart" || c.response.ArenaGroupingRetryable', 'phase == "ChampSelect" || phase == "GameStart"', 'go', 'TestR91AddendumRetryableRosterDoesNotBlockEarlyProbe'),
 ('arena-rank-restored', 'web/gameplay.js', 'const contextCopy = arenaMode ? "" :', 'const contextCopy = arenaMode ? rankCopy :', 'js', 'existing history'),
 ('arena-first-row-offset', 'web/gameplay.css', '.match-main { display: grid; align-self: start;', '.match-main { display: grid; align-self: center;', 'browser', 'card-first-row'),
 ('arena-damage-unit', 'web/gameplay.js', '${plainInteger(subject.damage)}</b></span>', '${number(subject.damage / 10000)}万</b></span>', 'browser', 'damage-integer'),
 ('arena-warning-restored', 'web/suite.js', '${champSelectPoolHelpHTML(definition, runtime)}<fieldset', '${champSelectPoolHelpHTML(definition, runtime)}${definition.groupId === "arena" ? \'<div class="suite-note is-warning">斗魂禁用环节需真机确认</div>\' : ""}<fieldset', 'browser', 'arena-warning'),

 ('hover-clear-global-exemption-ban', 'champselect_takeover.go', 'if last.Decision.ForceHover {', 'if true {', 'go', 'TestR91Addendum2NonWildcardBanClearYields'),
 ('hover-clear-global-exemption-pick', 'champselect_takeover.go', 'if last.Decision.ForceHover {', 'if true {', 'go', 'TestTakeoverClearingNonWildcardHoverYieldsButUnavailableHeroCanRecover'),
 ('hover-clear-unavailable-exemptions', 'champselect_takeover.go', 'return !champion.TeammatePicked && !champion.SelectionStatus.IsBanned', 'return !champion.TeammatePicked || !champion.SelectionStatus.IsBanned', 'go', 'TestTakeoverClearingNonWildcardHoverYieldsButUnavailableHeroCanRecover'),
 ('hover-clear-diagnostic-scope', 'champselect_takeover.go', '&& last.Decision.ForceHover && !last.HoverCleared', '&& !last.HoverCleared', 'go', 'TestR91Addendum2NonWildcardBanClearYields'),
]
results=[]
if os.environ.get('R91_MUTATION_NAMES'):
 cases=[c for c in cases if c[0] in os.environ['R91_MUTATION_NAMES'].split(',')]
with tempfile.TemporaryDirectory(prefix='r91-mutations-') as temp:
 for name, file, before, after, kind, pattern in cases:
  source=(ROOT/file).read_text()
  if source.count(before)!=1:
   raise RuntimeError(f'{name}: anchor count {source.count(before)}')
  mutant=pathlib.Path(temp)/(name+pathlib.Path(file).suffix)
  mutant.write_text(source.replace(before,after,1))
  env=dict(os.environ,GOCACHE='/tmp/deep-legends-go-cache')
  if kind=='js':
   env['R91_ADDENDUM_SUITE_SOURCE' if file.endswith('suite.js') else 'R91_ADDENDUM_GAMEPLAY_SOURCE']=str(mutant)
   cmd=['node','--test','--test-name-pattern='+pattern,'web/r91-addendum.test.cjs']
  elif kind=='browser':
   env[{'web/gameplay.css':'R91_ADDENDUM_CSS_SOURCE','web/gameplay.js':'R91_ADDENDUM_GAMEPLAY_SOURCE','web/suite.js':'R91_ADDENDUM_SUITE_SOURCE'}[file]]=str(mutant)
   env['R91_BROWSER_OUTPUT']=str(pathlib.Path(temp)/name)
   cmd=['node','desktop/r91-addendum-browser.cjs']
  else:
   overlay=pathlib.Path(temp)/'overlay.json';overlay.write_text(json.dumps({'Replace':{str(ROOT/file):str(mutant)}}))
   cmd=['go','test','-overlay',str(overlay),'-run','^'+pattern+'$','-count=1','-timeout=30s','.']
  proc=subprocess.run(cmd,cwd=ROOT,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=90)
  (OUT/(name+'.txt')).write_text(proc.stdout)
  killed=proc.returncode!=0 and ('AssertionError' in proc.stdout if kind in ['js','browser'] else '--- FAIL: '+pattern in proc.stdout) and 'build failed' not in proc.stdout and 'test timed out' not in proc.stdout
  results.append({'name':name,'source':file,'test':pattern,'exitCode':proc.returncode,'killed':killed})
  (OUT/'results.json').write_text(json.dumps(results,indent=2)+'\n')
  print(f'{name}: '+('KILLED' if killed else 'INVALID OR SURVIVED'),flush=True)
  if not killed: raise SystemExit(1)
print(f'{len(results)}/{len(results)} behavioral mutations killed',flush=True)
