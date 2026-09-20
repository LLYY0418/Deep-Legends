#!/usr/bin/env python3
"""Reproducible R95 mutants; never edit tracked production source in place."""
import json, os, pathlib, subprocess, tempfile, time
ROOT=pathlib.Path(__file__).resolve().parents[2]
OUT=ROOT/'docs/r95-validation/mutations';OUT.mkdir(exist_ok=True)
CASES=[]
def case(name,file,old,new,kind='js',pattern='R95',expected='kill'):
 CASES.append((name,file,old,new,kind,pattern,expected))
case('P0-M1-remove-positive-mark','arena_live_grouping.go','response.Players[i].MySquad = true','response.Players[i].MySquad = false','go','TestR95SquadSurvives')
case('P0-M2-clear-on-rejection','arena_live_grouping.go','if len(grouping.ByIdentity) == 0 {','if len(grouping.ByIdentity) == 0 {\n for i := range response.Players { response.Players[i].MySquad = false }','go','TestR95SquadSurvives')
case('P0-M3-remove-SGP','arena_live_grouping.go','a.checkArenaGroupTruthInput(client, serverID, arenaGroupTruthFromRiot(info, "sgp"))','return','go','TestR95MissingKnownSquadMemberAndTruth')
case('P0-M4-constant-correct','arena_truth_diagnostics.go','"my_squad_correct": correct','"my_squad_correct": correct || true','go','TestR95MissingKnownSquadMemberAndTruth')
case('P0-M5-silent-none','arena_live_grouping.go','defer a.checkArenaGroupTruthInput(client, record.serverID, &arenaGroupTruthInput{Source: "none", GameID: record.gameID, QueueID: record.queueID, GameMode: "CHERRY"})','','go','TestR95TruthNoneAndRiotSource')
case('P1-M1-spread-partial','web/gameplay.js','? { ...previous, player: { ...previous.player, ...partial.player }, matches,','? { ...previous, ...partial, player: { ...previous.player, ...partial.player }, matches,')
case('P1-M2-omitempty','gameplay.go','`json:"masteries"`','`json:"masteries,omitempty"`','go','TestR95OverviewEmptyArraysAreExplicit')
case('P2-M1-all-visible','web/gameplay.js','if (!(force === true || source === "manual")) return;','if (!(true)) return;')
case('P2-M2-none-visible','web/gameplay.js','if (!(force === true || source === "manual")) return;','if (!(false)) return;')
case('P2-M3-zero-delay','web/gameplay.js','}, 240);','}, 0);')
case('P2-M4-remove-reserved-height','web/gameplay.css','[data-live-status] { min-height: 40px; }','','browser')
case('P2-M5-nonempty-idle','web/gameplay.js','if (!["InProgress", "Reconnect"].includes(data?.phase)) return "";','if (!["InProgress", "Reconnect"].includes(data?.phase)) return "x";','browser',expected='survive')
case('P3-M1-old-delay','champselect.go','case config.Strategy == "lock-now":\n\t\tdelay = 0','case config.Strategy == "lock-now":\n\t\tdelay = champSelectDelay(config.DelayMS, remaining)','go','TestR95LockStrategiesUseOnlyVisibleWait')
case('P3-M2-all-time-inputs','web/suite.js','const lockWait = strategy === "show-then-lock";','const lockWait = true;')
case('P3-M3-discard-ban-lock-wait','champselect.go','side.LockDelayMS = &lockDelay','side.LockDelayMS = &lockDelay\n if kind == "ban" { side.LockDelayMS = nil }','go','TestR95LockStrategiesUseOnlyVisibleWait')
case('P3-M4-happy-warn','champselect.go','r.champSelectChampionLog("ok", fmt.Sprintf("已发送','r.champSelectChampionLog("warn", fmt.Sprintf("已发送','go','TestR95LockStrategiesUseOnlyVisibleWait')
case('P3-M5-type-ban-into-pick','web/suite.js','kind === "lock" ? side : kind','kind === "lock" ? "pick" : kind')
case('P4-M1-fuzzy-name','pro_identity.go','badge = index.byKey["name:"+strings.ToLower(strings.TrimSpace(reference.GameName)+"#"+strings.TrimSpace(reference.TagLine))]','for key, value := range index.byKey { if strings.HasPrefix(key, "name:") && strings.Contains(key, strings.ToLower(reference.GameName)) { badge = value; break } }','go','TestR95ProExactIdentityPrivacyAndNoFetch')
case('P5-M1-remove-pick-title','web/suite.js','"champselect-pick": "自动选用",','')
case('P5-M2-English-fallback','web/suite.js','|| "自动规则";','|| action;')
case('P5-M3-exclude-champselect-count','web/suite.js','state.watchFired += 1;','if (!action.startsWith("champselect-")) state.watchFired += 1;')
results=[]
with tempfile.TemporaryDirectory(prefix='r95-mutants-') as tmp:
 for name,file,old,new,kind,pattern,expected in CASES:
  if os.environ.get('R95_MUTATION_FILTER') and os.environ['R95_MUTATION_FILTER'] not in name: continue
  source=(ROOT/file).read_text();assert old in source,(name,'anchor missing')
  altered=source.replace(old,new,1);mutant=pathlib.Path(tmp)/pathlib.Path(file).name;mutant.write_text(altered)
  env=os.environ.copy();env.update(GOCACHE='/tmp/deep-legends-go-cache',GOTMPDIR='/tmp/deep-legends-go-tmp')
  if kind=='go':
   overlay=pathlib.Path(tmp)/'overlay.json';overlay.write_text(json.dumps({'Replace':{str(ROOT/file):str(mutant)}}));cmd=['go','test','-overlay',str(overlay),'-count=1','-run',pattern,'.']
  else:
   key={'web/gameplay.js':'R95_GAMEPLAY_SOURCE','web/suite.js':'R95_SUITE_SOURCE','web/gameplay.css':'R95_CSS_SOURCE'}[file];env[key]=str(mutant)
   if kind=='browser':env['R95_BROWSER_OUTPUT']=str(OUT/name);cmd=['node','desktop/r95-browser.cjs']
   else:cmd=['node','--test','--test-name-pattern',pattern,'web/r95.test.cjs']
  start=time.monotonic()
  try:
   p=subprocess.run(cmd,cwd=ROOT,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=120,text=True)
   text=p.stdout;invalid=any(s in text for s in ['[build failed]','SyntaxError','Chrome startup timeout','CDP timeout','scheduler stuck','ReferenceError:'])
   result='invalid' if invalid else 'survive' if p.returncode==0 else 'kill' if ('AssertionError' in text or '--- FAIL:' in text or 'ERR_ASSERTION' in text) else 'invalid'
  except subprocess.TimeoutExpired as e:text=(e.stdout or b'').decode() if isinstance(e.stdout,bytes) else (e.stdout or '');result='timeout';p=None
  (OUT/(name+'.txt')).write_text('$ '+' '.join(cmd)+'\n'+text)
  row=dict(name=name,result=result,expected=expected,seconds=round(time.monotonic()-start,2),exit_code=p.returncode if p else None);results.append(row);print(json.dumps(row),flush=True)
  (OUT/'results.json').write_text(json.dumps(results,indent=2)+'\n')
if any(x['result']!=x['expected'] for x in results):raise SystemExit(1)
